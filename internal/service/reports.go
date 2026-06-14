//go:build !lite

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf16"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// AgentReportResponse is the API response type for agent reports.
type AgentReportResponse struct {
	ID            string   `json:"id"`
	ShortID       string   `json:"short_id"`
	Title         string   `json:"title"`
	Body          string   `json:"body"`
	CreatedByType string   `json:"created_by_type"`
	CreatedByID   *string  `json:"created_by_id"`
	CreatedByName string   `json:"created_by_name"`
	Priority      string   `json:"priority"`
	Tags          []string `json:"tags"`
	Author        *string  `json:"author,omitempty"`
	ReadAt        *string  `json:"read_at"`
	CreatedAt     string   `json:"created_at"`

	// SessionID, when populated, links this report to the MCP session that
	// produced it. Available to Go consumers (the home Feed folds a report
	// into the matching agent_session card by session_id) but suppressed
	// from JSON output so the MCP `submit_report` response contract stays
	// stable — the link is server-side internal, not a public field.
	SessionID *string `json:"-"`
}

func agentReportFromRow(r db.AgentReport) AgentReportResponse {
	tags := r.Tags
	if tags == nil {
		tags = []string{}
	}
	out := AgentReportResponse{
		ID:            formatUUID(r.ID),
		ShortID:       r.ShortID,
		Title:         r.Title,
		Body:          r.Body,
		CreatedByType: r.CreatedByType,
		CreatedByID:   textPtr(r.CreatedByID),
		CreatedByName: r.CreatedByName,
		Priority:      r.Priority,
		Tags:          tags,
		Author:        textPtr(r.Author),
		ReadAt:        timestampStr(r.ReadAt),
		CreatedAt:     pgconv.TimestampStr(r.CreatedAt),
	}
	if r.SessionID.Valid {
		s := formatUUID(r.SessionID)
		out.SessionID = &s
	}
	return out
}

// ValidReportPriorities lists allowed priority values.
var ValidReportPriorities = map[string]bool{
	"info":     true,
	"warning":  true,
	"critical": true,
}

// CreateAgentReport creates a new agent report, optionally linked to
// an MCP session and an agent run. agentRunShortID resolves to the
// agent_runs.id FK on the new row; empty leaves it NULL (operator-
// submitted reports, MCP sessions outside the agent SDK).
func (s *Service) CreateAgentReport(ctx context.Context, title, body string, actor Actor, priority string, tags []string, author string, sessionID string, agentRunShortID string) (AgentReportResponse, error) {
	if title == "" {
		return AgentReportResponse{}, fmt.Errorf("%w: title is required", ErrInvalidParameter)
	}
	if body == "" {
		return AgentReportResponse{}, fmt.Errorf("%w: body is required", ErrInvalidParameter)
	}
	// Repair over-escaped Unicode in the agent-authored prose. Models
	// occasionally emit a literal backslash-u escape (the six ASCII
	// characters backslash, u, 2, 0, 1, 4) inside the submit_report tool
	// argument instead of the em-dash rune it names. The JSON layer hands us
	// those bytes verbatim, so without this they get stored and then rendered
	// as that raw escape across every surface that shows the report: the runs
	// page, the report table, the activity feed, and notification payloads.
	// Decode here, at the single ingestion chokepoint, so the stored value is
	// clean everywhere.
	title = decodeStrayUnicodeEscapes(title)
	body = decodeStrayUnicodeEscapes(body)
	if priority == "" {
		priority = "info"
	}
	if !ValidReportPriorities[priority] {
		return AgentReportResponse{}, fmt.Errorf("%w: priority must be info, warning, or critical", ErrInvalidParameter)
	}
	if tags == nil {
		tags = []string{}
	}
	if len(tags) > 10 {
		return AgentReportResponse{}, fmt.Errorf("%w: maximum 10 tags allowed", ErrInvalidParameter)
	}

	createdByName := actor.Name
	if author != "" {
		createdByName = author
	}

	sessUUID, _ := s.ResolveSessionUUID(ctx, sessionID)

	var runUUID pgtype.UUID
	if agentRunShortID != "" {
		if run, err := s.Queries.GetAgentRunByShortID(ctx, agentRunShortID); err == nil {
			runUUID = run.ID
		}
	}

	report, err := s.Queries.CreateAgentReport(ctx, db.CreateAgentReportParams{
		Title:         title,
		Body:          body,
		CreatedByType: actor.Type,
		CreatedByID:   pgconv.TextIfNotEmpty(actor.ID),
		CreatedByName: createdByName,
		Priority:      priority,
		Tags:          tags,
		Author:        pgconv.TextIfNotEmpty(author),
		SessionID:     sessUUID,
		WorkflowRunID:    runUUID,
	})
	if err != nil {
		return AgentReportResponse{}, fmt.Errorf("create agent report: %w", err)
	}

	resp := agentReportFromRow(report)

	// Fan the report out to the operator's configured notification sink.
	// Only workflow/agent-authored reports notify — an operator submitting
	// a report from the dashboard doesn't need to be pinged about their own
	// action. This is strictly best-effort: it runs async on a fresh,
	// time-bounded context (the request ctx may already be cancelled by the
	// time the webhook fires) and never blocks or fails report creation.
	// SendWorkflowNotification is itself a no-op when no webhook is set, so
	// the goroutine is cheap in the common (unconfigured) case.
	if actor.Type == "agent" {
		payload := reportNotificationPayload(resp, actor.Name)
		go func() {
			nctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := s.SendWorkflowNotification(nctx, payload); err != nil && s.Logger != nil {
				s.Logger.Warn("workflow report notification failed",
					"report_id", resp.ShortID,
					"error", err)
			}
		}()
	}

	return resp, nil
}

// AgentRunReportSummary is the compact report-reference shape rendered
// inside an AgentRunRow chip on the runs landing. We deliberately
// don't carry the body — opening the report is one click away — so
// the row stays light and the page-level query is small.
type AgentRunReportSummary struct {
	ShortID  string
	Title    string
	Priority string
}

// ListReportSummariesForRunIDs fetches the report-summary chip data
// for a batch of agent_runs.id UUIDs in one query. Returns a map
// keyed by run-id (canonical UUID string); runs with no reports are
// absent from the map. Reports inside each slice are ordered oldest →
// newest so the chip line reads chronologically when an agent
// produces multiple reports per run.
func (s *Service) ListReportSummariesForRunIDs(ctx context.Context, runIDs []string) (map[string][]AgentRunReportSummary, error) {
	out := make(map[string][]AgentRunReportSummary, len(runIDs))
	if len(runIDs) == 0 {
		return out, nil
	}
	uuids := make([]pgtype.UUID, 0, len(runIDs))
	for _, id := range runIDs {
		u, err := pgconv.ParseUUID(id)
		if err != nil {
			continue
		}
		uuids = append(uuids, u)
	}
	if len(uuids) == 0 {
		return out, nil
	}
	rows, err := s.Queries.ListReportSummariesForRunIDs(ctx, uuids)
	if err != nil {
		return nil, fmt.Errorf("list report summaries for runs: %w", err)
	}
	for _, r := range rows {
		if !r.WorkflowRunID.Valid {
			continue
		}
		runID := pgconv.FormatUUID(r.WorkflowRunID)
		out[runID] = append(out[runID], AgentRunReportSummary{
			ShortID:  r.ShortID,
			Title:    r.Title,
			Priority: r.Priority,
		})
	}
	return out, nil
}

// ListAgentReports returns the most recent reports.
func (s *Service) ListAgentReports(ctx context.Context, limit int) ([]AgentReportResponse, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.Queries.ListAgentReports(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("list agent reports: %w", err)
	}
	result := make([]AgentReportResponse, len(rows))
	for i, r := range rows {
		result[i] = agentReportFromRow(r)
	}
	return result, nil
}

// ListUnreadAgentReports returns unread reports.
func (s *Service) ListUnreadAgentReports(ctx context.Context, limit int) ([]AgentReportResponse, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	rows, err := s.Queries.ListUnreadAgentReports(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("list unread agent reports: %w", err)
	}
	result := make([]AgentReportResponse, len(rows))
	for i, r := range rows {
		result[i] = agentReportFromRow(r)
	}
	return result, nil
}

// CountUnreadAgentReports returns the count of unread reports.
func (s *Service) CountUnreadAgentReports(ctx context.Context) (int64, error) {
	return s.Queries.CountUnreadAgentReports(ctx)
}

// GetAgentReport returns a single report by ID or short ID. A malformed ID
// (neither a UUID nor a short ID) is ErrInvalidParameter (400); a well-formed
// reference to a nonexistent report is ErrNotFound (404).
func (s *Service) GetAgentReport(ctx context.Context, reportID string) (AgentReportResponse, error) {
	uid, err := s.resolveAgentReportID(ctx, reportID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return AgentReportResponse{}, ErrNotFound
		}
		return AgentReportResponse{}, fmt.Errorf("%w: invalid report ID", ErrInvalidParameter)
	}
	row, err := s.Queries.GetAgentReport(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AgentReportResponse{}, ErrNotFound
		}
		return AgentReportResponse{}, fmt.Errorf("get agent report: %w", err)
	}
	return agentReportFromRow(row), nil
}

// MarkAgentReportRead marks a single report as read. Returns ErrNotFound if no
// report with the given ID exists. Returns nil (idempotent) if the report
// exists but is already read.
func (s *Service) MarkAgentReportRead(ctx context.Context, reportID string) error {
	uid, err := s.resolveAgentReportID(ctx, reportID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("%w: invalid report ID", ErrInvalidParameter)
	}
	// Use Pool.Exec directly to inspect rows affected — sqlc's :exec discards
	// the CommandTag. The generated query has `AND read_at IS NULL`, so a
	// row-affected count of zero is ambiguous (missing vs already read). To
	// distinguish, fall back to a SELECT EXISTS check before declaring
	// not-found.
	tag, err := s.Pool.Exec(ctx,
		"UPDATE agent_reports SET read_at = NOW() WHERE id = $1 AND read_at IS NULL", uid)
	if err != nil {
		return fmt.Errorf("mark agent report read: %w", err)
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := s.Pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM agent_reports WHERE id = $1)", uid).Scan(&exists); err != nil {
			return fmt.Errorf("mark agent report read: %w", err)
		}
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}

// MarkAgentReportUnread clears read_at on a single report, returning it to the
// unread queue. Returns ErrNotFound if no report with the given ID exists.
func (s *Service) MarkAgentReportUnread(ctx context.Context, reportID string) error {
	uid, err := s.resolveAgentReportID(ctx, reportID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("%w: invalid report ID", ErrInvalidParameter)
	}
	tag, err := s.Pool.Exec(ctx,
		"UPDATE agent_reports SET read_at = NULL WHERE id = $1", uid)
	if err != nil {
		return fmt.Errorf("mark agent report unread: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkAllAgentReportsRead marks all unread reports as read.
func (s *Service) MarkAllAgentReportsRead(ctx context.Context) error {
	return s.Queries.MarkAllAgentReportsRead(ctx)
}

// DeleteAgentReport hard-deletes a single report by ID. Returns ErrNotFound
// if no report with the given ID exists.
func (s *Service) DeleteAgentReport(ctx context.Context, reportID string) error {
	uid, err := s.resolveAgentReportID(ctx, reportID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("%w: invalid report ID", ErrInvalidParameter)
	}
	tag, err := s.Pool.Exec(ctx, "DELETE FROM agent_reports WHERE id = $1", uid)
	if err != nil {
		return fmt.Errorf("delete agent report: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// decodeStrayUnicodeEscapes repairs a string in which a model over-escaped a
// Unicode character: it emitted the literal text of a backslash-u escape (a
// backslash, the letter u, and four hex digits) rather than the rune that
// escape denotes. We see this in agent-authored report titles, where an
// em-dash arrives as the six characters backslash u 2 0 1 4. Decode any such
// sequence, combining a UTF-16 surrogate pair when present, back into the rune
// it names. A string with no backslash-u marker takes a fast path unchanged.
//
// Only backslash-u escapes are touched. Other escapes (newline, tab, quote)
// are left alone: they are either intentional or harmless in markdown, and
// rewriting them risks mangling legitimate content. The reported breakage is
// purely Unicode escapes.
func decodeStrayUnicodeEscapes(s string) string {
	if !strings.Contains(s, `\u`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if r, size, ok := readUnicodeEscapeAt(s, i); ok {
			b.WriteRune(r)
			i += size
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// readUnicodeEscapeAt tries to decode a backslash-u escape beginning at byte
// offset i, combining a following low surrogate when the first code unit is a
// high surrogate. It returns the decoded rune, the number of source bytes
// consumed, and whether a valid escape was found. A malformed or partial
// escape — a non-hex tail, an unpaired surrogate — reports ok=false so the
// caller copies the bytes through untouched.
func readUnicodeEscapeAt(s string, i int) (rune, int, bool) {
	if i+6 > len(s) || !strings.HasPrefix(s[i:], `\u`) {
		return 0, 0, false
	}
	hi, ok := parseHex4(s[i+2 : i+6])
	if !ok {
		return 0, 0, false
	}
	switch {
	case hi >= 0xD800 && hi <= 0xDBFF:
		// High surrogate: only valid when paired with a trailing low surrogate.
		if i+12 <= len(s) && strings.HasPrefix(s[i+6:], `\u`) {
			if lo, ok := parseHex4(s[i+8 : i+12]); ok && lo >= 0xDC00 && lo <= 0xDFFF {
				return utf16.DecodeRune(rune(hi), rune(lo)), 12, true
			}
		}
		return 0, 0, false
	case hi >= 0xDC00 && hi <= 0xDFFF:
		// Lone low surrogate: invalid on its own.
		return 0, 0, false
	default:
		return rune(hi), 6, true
	}
}

// parseHex4 parses exactly four hex digits into their integer value.
func parseHex4(s string) (uint32, bool) {
	if len(s) != 4 {
		return 0, false
	}
	var v uint32
	for i := 0; i < 4; i++ {
		c := s[i]
		var d uint32
		switch {
		case c >= '0' && c <= '9':
			d = uint32(c - '0')
		case c >= 'a' && c <= 'f':
			d = uint32(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = uint32(c-'A') + 10
		default:
			return 0, false
		}
		v = v<<4 | d
	}
	return v, true
}

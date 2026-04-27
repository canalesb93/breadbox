package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/shortid"

	"github.com/jackc/pgx/v5/pgtype"
)

// MCPSessionResponse is the API/MCP response for a session.
type MCPSessionResponse struct {
	ID            string  `json:"id"`
	ShortID       string  `json:"short_id"`
	Purpose       string  `json:"purpose"`
	APIKeyName    string  `json:"api_key_name"`
	CreatedAt     string  `json:"created_at"`
	ToolCallCount int64   `json:"tool_call_count,omitempty"`
	LastCallAt    *string `json:"last_call_at,omitempty"`
	AgentName     string  `json:"agent_name,omitempty"`
	ReportID      *string `json:"report_id,omitempty"`
	ReportTitle   string  `json:"report_title,omitempty"`
}

// ToolCallLogResponse is the response for a single tool call log entry.
type ToolCallLogResponse struct {
	ID             string           `json:"id"`
	ToolName       string           `json:"tool_name"`
	Classification string           `json:"classification"`
	Reason         string           `json:"reason"`
	RequestJSON    *json.RawMessage `json:"request_json,omitempty"`
	ResponseJSON   *json.RawMessage `json:"response_json,omitempty"`
	IsError        bool             `json:"is_error"`
	ActorName      string           `json:"actor_name"`
	DurationMs     *int32           `json:"duration_ms,omitempty"`
	CreatedAt      string           `json:"created_at"`
	// Sequence is the 1-based ordinal of this call within the session.
	Sequence int `json:"sequence,omitempty"`
	// OffsetLabel is a compact human-readable delta from the session's first
	// recorded tool call (e.g. "+0.0s", "+2.1s", "+1m12s"). Empty when there
	// is no reference call yet.
	OffsetLabel string `json:"offset_label,omitempty"`
}

// MCPSessionDetailResponse is a session with its tool calls.
type MCPSessionDetailResponse struct {
	MCPSessionResponse
	ToolCalls  []ToolCallLogResponse `json:"tool_calls"`
	ErrorCount int                   `json:"error_count"`
	WriteCount int                   `json:"write_count"`
	ReadCount  int                   `json:"read_count"`
}

// ToolCallLogInput is the input for logging a tool call.
type ToolCallLogInput struct {
	SessionID      string
	ToolName       string
	Classification string
	Reason         string
	RequestJSON    []byte
	ResponseJSON   []byte
	IsError        bool
	Actor          Actor
	DurationMs     int
}

// CreateMCPSession creates a new MCP session.
func (s *Service) CreateMCPSession(ctx context.Context, actor Actor, purpose string) (MCPSessionResponse, error) {
	if purpose == "" {
		return MCPSessionResponse{}, fmt.Errorf("%w: purpose is required", ErrInvalidParameter)
	}
	row, err := s.Queries.CreateMCPSession(ctx, db.CreateMCPSessionParams{
		ApiKeyID:   actor.ID,
		ApiKeyName: actor.Name,
		Purpose:    purpose,
	})
	if err != nil {
		return MCPSessionResponse{}, fmt.Errorf("create mcp session: %w", err)
	}
	return mcpSessionFromRow(row), nil
}

// GetMCPSession retrieves a session by ID or short_id.
func (s *Service) GetMCPSession(ctx context.Context, idOrShort string) (MCPSessionResponse, error) {
	if idOrShort == "" {
		return MCPSessionResponse{}, fmt.Errorf("%w: session ID is required", ErrInvalidParameter)
	}
	if shortid.IsShortID(idOrShort) {
		row, err := s.Queries.GetMCPSessionByShortID(ctx, idOrShort)
		if err != nil {
			return MCPSessionResponse{}, ErrNotFound
		}
		return mcpSessionFromRow(row), nil
	}
	uid, err := parseUUID(idOrShort)
	if err != nil {
		return MCPSessionResponse{}, fmt.Errorf("%w: invalid session ID", ErrInvalidParameter)
	}
	row, err := s.Queries.GetMCPSessionByID(ctx, uid)
	if err != nil {
		return MCPSessionResponse{}, ErrNotFound
	}
	return mcpSessionFromRow(row), nil
}

// LogToolCall logs a tool call to the database. Errors are logged but not returned.
func (s *Service) LogToolCall(ctx context.Context, input ToolCallLogInput) {
	var sessionID pgtype.UUID
	if input.SessionID != "" {
		if shortid.IsShortID(input.SessionID) {
			row, err := s.Queries.GetMCPSessionByShortID(ctx, input.SessionID)
			if err == nil {
				sessionID = row.ID
			}
		} else if uid, err := parseUUID(input.SessionID); err == nil {
			sessionID = uid
		}
	}

	var durationMs pgtype.Int4
	if input.DurationMs > 0 {
		durationMs = pgconv.Int4(int32(input.DurationMs))
	}

	err := s.Queries.CreateToolCallLog(ctx, db.CreateToolCallLogParams{
		SessionID:      sessionID,
		ToolName:       input.ToolName,
		Classification: input.Classification,
		Reason:         input.Reason,
		RequestJson:    input.RequestJSON,
		ResponseJson:   input.ResponseJSON,
		IsError:        input.IsError,
		ActorType:      input.Actor.Type,
		ActorID:        input.Actor.ID,
		ActorName:      input.Actor.Name,
		DurationMs:     durationMs,
	})
	if err != nil {
		slog.Error("failed to log tool call", "tool", input.ToolName, "error", err)
	}
}

// ListMCPSessions returns paginated sessions with tool call counts.
func (s *Service) ListMCPSessions(ctx context.Context, page, pageSize int) ([]MCPSessionResponse, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 25
	}

	total, err := s.Queries.CountMCPSessions(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count mcp sessions: %w", err)
	}

	rows, err := s.Queries.ListMCPSessions(ctx, db.ListMCPSessionsParams{
		Limit:  int32(pageSize),
		Offset: int32((page - 1) * pageSize),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list mcp sessions: %w", err)
	}

	result := make([]MCPSessionResponse, len(rows))
	for i, r := range rows {
		result[i] = MCPSessionResponse{
			ID:            formatUUID(r.ID),
			ShortID:       r.ShortID,
			Purpose:       r.Purpose,
			APIKeyName:    r.ApiKeyName,
			CreatedAt:     pgconv.TimestampStr(r.CreatedAt),
			ToolCallCount: r.ToolCallCount,
			LastCallAt:    timestampStr(r.LastCallAt),
			AgentName:     r.AgentName,
			ReportTitle:   r.ReportTitle,
		}
		if r.ReportID.Valid {
			rid := formatUUID(r.ReportID)
			result[i].ReportID = &rid
		}
	}
	return result, total, nil
}

// GetMCPSessionDetail retrieves a session with all its tool calls.
func (s *Service) GetMCPSessionDetail(ctx context.Context, idOrShort string) (MCPSessionDetailResponse, error) {
	session, err := s.GetMCPSession(ctx, idOrShort)
	if err != nil {
		return MCPSessionDetailResponse{}, err
	}

	uid, _ := parseUUID(session.ID)
	rows, err := s.Queries.ListToolCallsBySession(ctx, uid)
	if err != nil {
		return MCPSessionDetailResponse{}, fmt.Errorf("list tool calls: %w", err)
	}

	calls := make([]ToolCallLogResponse, len(rows))
	var base time.Time
	if len(rows) > 0 {
		base = rows[0].CreatedAt.Time
	}
	for i, r := range rows {
		calls[i] = toolCallFromRow(r)
		calls[i].Sequence = i + 1
		calls[i].OffsetLabel = formatOffset(r.CreatedAt.Time.Sub(base))
	}

	session.ToolCallCount = int64(len(calls))
	errorCount, writeCount, readCount := summarizeToolCalls(calls)

	return MCPSessionDetailResponse{
		MCPSessionResponse: session,
		ToolCalls:          calls,
		ErrorCount:         errorCount,
		WriteCount:         writeCount,
		ReadCount:          readCount,
	}, nil
}

// formatOffset renders a duration relative to the session's first tool call
// as a compact label suitable for inline display next to tool-call rows.
// Negative values clamp to "+0.0s" so the first row always reads "+0.0s"
// even if clock skew makes d slightly negative.
func formatOffset(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := d.Seconds()
	switch {
	case total < 60:
		return fmt.Sprintf("+%.1fs", total)
	case total < 3600:
		m := int(total) / 60
		s := int(total) % 60
		return fmt.Sprintf("+%dm%02ds", m, s)
	default:
		h := int(total) / 3600
		m := (int(total) % 3600) / 60
		return fmt.Sprintf("+%dh%02dm", h, m)
	}
}

// summarizeToolCalls aggregates error / write / read counts across tool calls.
// Calls with classifications other than "write" or "read" are not counted in
// the split (and thus won't distort the header pill); errors are counted
// independently of classification so a failed write still surfaces.
func summarizeToolCalls(calls []ToolCallLogResponse) (errorCount, writeCount, readCount int) {
	for _, c := range calls {
		if c.IsError {
			errorCount++
		}
		switch c.Classification {
		case "write":
			writeCount++
		case "read":
			readCount++
		}
	}
	return errorCount, writeCount, readCount
}

// ResolveSessionUUID resolves a session ID string (UUID or short_id) to pgtype.UUID.
func (s *Service) ResolveSessionUUID(ctx context.Context, idOrShort string) (pgtype.UUID, error) {
	if idOrShort == "" {
		return pgtype.UUID{}, nil
	}
	if shortid.IsShortID(idOrShort) {
		row, err := s.Queries.GetMCPSessionByShortID(ctx, idOrShort)
		if err != nil {
			return pgtype.UUID{}, fmt.Errorf("%w: invalid session_id", ErrInvalidParameter)
		}
		return row.ID, nil
	}
	uid, err := parseUUID(idOrShort)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("%w: invalid session_id", ErrInvalidParameter)
	}
	return uid, nil
}

func mcpSessionFromRow(r db.McpSession) MCPSessionResponse {
	return MCPSessionResponse{
		ID:         formatUUID(r.ID),
		ShortID:    r.ShortID,
		Purpose:    r.Purpose,
		APIKeyName: r.ApiKeyName,
		CreatedAt:  pgconv.TimestampStr(r.CreatedAt),
	}
}

func toolCallFromRow(r db.McpToolCall) ToolCallLogResponse {
	resp := ToolCallLogResponse{
		ID:             formatUUID(r.ID),
		ToolName:       r.ToolName,
		Classification: r.Classification,
		Reason:         r.Reason,
		IsError:        r.IsError,
		ActorName:      r.ActorName,
		CreatedAt:      pgconv.TimestampStr(r.CreatedAt),
	}
	if len(r.RequestJson) > 0 {
		raw := json.RawMessage(r.RequestJson)
		resp.RequestJSON = &raw
	}
	if len(r.ResponseJson) > 0 {
		raw := json.RawMessage(r.ResponseJson)
		resp.ResponseJSON = &raw
	}
	if r.DurationMs.Valid {
		resp.DurationMs = &r.DurationMs.Int32
	}
	return resp
}

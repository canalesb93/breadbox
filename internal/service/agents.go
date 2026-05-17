//go:build !lite

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"breadbox/internal/agent"
	"breadbox/internal/appconfig"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// DefaultAgentModel is the model used when a definition omits one.
// claude-opus-4-7 is the latest Opus (matches the agent_definitions
// migration default).
const DefaultAgentModel = "claude-opus-4-7"

// DefaultAgentMaxTurns is the per-run turn cap when a definition omits one.
const DefaultAgentMaxTurns = 10

// DefaultAgentMaxBudgetUSD mirrors the agent_definitions migration default.
// The column is NOT NULL, so we always send a value to sqlc.
const DefaultAgentMaxBudgetUSD = 1.0

// validAgentSlug is the canonical kebab-case format: lowercase letters,
// digits, and dashes; 2-64 chars. Matches the slug pattern used elsewhere
// in the codebase (rules, tags) for consistency.
var validAgentSlug = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$`)

// AgentDefinitionResponse is the API shape for an agent_definition row.
type AgentDefinitionResponse struct {
	ID              string           `json:"id"`
	ShortID         string           `json:"short_id"`
	Name            string           `json:"name"`
	Slug            string           `json:"slug"`
	Prompt          string           `json:"prompt"`
	SystemPrompt    *string          `json:"system_prompt,omitempty"`
	ScheduleCron    *string          `json:"schedule_cron,omitempty"`
	ToolScope       string           `json:"tool_scope"`
	AllowedTools    []string         `json:"allowed_tools"`
	Model           string           `json:"model"`
	MaxTurns        int              `json:"max_turns"`
	MaxBudgetUSD    *float64         `json:"max_budget_usd,omitempty"`
	Enabled               bool             `json:"enabled"`
	QuietHoursStart       *string          `json:"quiet_hours_start,omitempty"`
	QuietHoursEnd         *string          `json:"quiet_hours_end,omitempty"`
	// TriggerOnSyncComplete fires this agent after every successful sync
	// (in addition to any cron schedule). Disabled by default.
	TriggerOnSyncComplete bool             `json:"trigger_on_sync_complete"`
	LastRun               *AgentRunSummary `json:"last_run,omitempty"`
	// CostStats30d is populated only by ListAgentDefinitions (the surface
	// where users want to compare spend at a glance). Single-row
	// GetAgentDefinition leaves it nil so the edit-page hot path doesn't
	// pay for an extra aggregation query.
	CostStats30d *AgentCostStats `json:"cost_stats_30d,omitempty"`
	// NextFireAt is the next scheduled fire time accounting for quiet
	// hours, populated by ListAgentDefinitions only (list-only like
	// CostStats30d — single-row Get leaves nil). RFC3339 string when set.
	NextFireAt *string `json:"next_fire_at,omitempty"`
	// RecentErrorStats is the count of errors among the last 5 non-skipped
	// runs. Used by the v2 SPA to surface a warning when 3+ recent runs
	// failed. List-only; nil when the agent has no run history.
	RecentErrorStats *AgentRecentErrorStats `json:"recent_error_stats,omitempty"`
	// LastPromptPrefix is the most recent non-null prompt_prefix this agent
	// was ever run with. Powers the "Use last prefix" affordance in the v2
	// SPA's Run now dialog. List-only; nil when no prefixed run exists.
	LastPromptPrefix *string `json:"last_prompt_prefix,omitempty"`
	// RecentCapStats is the count of cap-exhausted runs among the last 5
	// non-skipped runs. Used by the v2 SPA to surface a warning when 2+
	// recent runs hit a safety ceiling. List-only; nil when no run history.
	RecentCapStats *AgentRecentCapStats `json:"recent_cap_stats,omitempty"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

// AgentRecentErrorStats is the "is this agent broken right now?" signal —
// errors among the last 5 non-skipped runs.
type AgentRecentErrorStats struct {
	ErrorCount int `json:"error_count"`
	RunCount   int `json:"run_count"` // up to 5; less when there's less history
}

// AgentRecentCapStats is the "is this agent regularly hitting its safety
// ceilings?" signal — cap-exhausted (max_turns OR max_budget) runs among
// the last 5 non-skipped. Surfaced as a warning pill on the v2 SPA list
// when CapCount >= 2 (threshold tuned to avoid flagging one-off cap hits).
type AgentRecentCapStats struct {
	CapCount int `json:"cap_count"`
	RunCount int `json:"run_count"` // up to 5
}

// AgentCostStats is the per-agent cost rollup over the last 30 days.
// run_count excludes 'skipped' rows (no real spend incurred).
type AgentCostStats struct {
	RunCount     int     `json:"run_count"`
	TotalCostUSD float64 `json:"total_cost_usd"`
}

// AgentRunSummary is the inline last-run shape on list/detail responses.
type AgentRunSummary struct {
	ShortID      string   `json:"short_id"`
	Status       string   `json:"status"`
	Trigger      string   `json:"trigger"`
	StartedAt    string   `json:"started_at"`
	CompletedAt  *string  `json:"completed_at,omitempty"`
	DurationMs   *int     `json:"duration_ms,omitempty"`
	TotalCostUSD *float64 `json:"total_cost_usd,omitempty"`
}

// AgentRunResponse is the full shape for one agent run.
type AgentRunResponse struct {
	ID                  string   `json:"id"`
	ShortID             string   `json:"short_id"`
	AgentDefinitionID   *string  `json:"agent_definition_id,omitempty"`
	Trigger             string   `json:"trigger"`
	Status              string   `json:"status"`
	StartedAt           string   `json:"started_at"`
	CompletedAt         *string  `json:"completed_at,omitempty"`
	DurationMs          *int     `json:"duration_ms,omitempty"`
	TotalCostUSD        *float64 `json:"total_cost_usd,omitempty"`
	InputTokens         *int     `json:"input_tokens,omitempty"`
	OutputTokens        *int     `json:"output_tokens,omitempty"`
	CacheReadTokens     *int     `json:"cache_read_tokens,omitempty"`
	CacheCreationTokens *int     `json:"cache_creation_tokens,omitempty"`
	TurnCount           *int     `json:"turn_count,omitempty"`
	MaxTurnsUsed        *int     `json:"max_turns_used,omitempty"`
	NumToolCalls        *int     `json:"num_tool_calls,omitempty"`
	ErrorMessage        *string  `json:"error_message,omitempty"`
	TranscriptPath      *string  `json:"transcript_path,omitempty"`
	SessionID           *string  `json:"session_id,omitempty"`
	OperatorNote        *string  `json:"operator_note,omitempty"`
	PromptPrefix        *string  `json:"prompt_prefix,omitempty"`
	// HitCap names the safety ceiling this run bumped into when it
	// terminated, if any: "max_turns" | "max_budget" | nil. Lets the v2
	// SPA flag "ran into the ceiling" runs separately from clean successes.
	HitCap *string `json:"hit_cap,omitempty"`
}

// AgentRunListResult is the paginated envelope for run lists.
type AgentRunListResult struct {
	Runs    []AgentRunResponse `json:"runs"`
	Limit   int                `json:"limit"`
	Offset  int                `json:"offset"`
	HasMore bool               `json:"has_more"`
}

// CreateAgentDefinitionParams holds validated inputs for definition creation.
type CreateAgentDefinitionParams struct {
	Name                  string
	Slug                  string
	Prompt                string
	SystemPrompt          *string
	ScheduleCron          *string
	ToolScope             string
	AllowedTools          []string
	Model                 string
	MaxTurns              int
	MaxBudgetUSD          *float64
	Enabled               bool
	QuietHoursStart       *string // "HH:MM" 24-hour; nil disables window
	QuietHoursEnd         *string
	TriggerOnSyncComplete bool // fire after each successful sync completes
}

// UpdateAgentDefinitionParams uses pointer fields for PATCH semantics:
// nil = don't touch; non-nil = replace. Slug is mutable here; if you want
// it pinned, omit Slug from your PATCH body.
type UpdateAgentDefinitionParams struct {
	Name                  *string
	Slug                  *string
	Prompt                *string
	SystemPrompt          *string
	ScheduleCron          *string
	ToolScope             *string
	AllowedTools          *[]string
	Model                 *string
	MaxTurns              *int
	MaxBudgetUSD          *float64
	Enabled               *bool
	QuietHoursStart       *string
	QuietHoursEnd         *string
	TriggerOnSyncComplete *bool
}

// ListAgentDefinitionsForSyncWebhook returns enabled definitions with
// trigger_on_sync_complete=true. Used by the post-sync hook in the
// orchestrator to dispatch webhook-triggered runs.
func (s *Service) ListAgentDefinitionsForSyncWebhook(ctx context.Context) ([]AgentDefinitionResponse, error) {
	rows, err := s.Queries.ListAgentDefinitionsForSyncWebhook(ctx)
	if err != nil {
		return nil, fmt.Errorf("list sync-webhook agents: %w", err)
	}
	out := make([]AgentDefinitionResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, agentDefinitionFromRow(row, nil))
	}
	return out, nil
}

// ListAgentDefinitions returns all definitions ordered by created_at DESC,
// each with last_run inlined. N+1 is acceptable here — definition count
// is small (O(10s)).
func (s *Service) ListAgentDefinitions(ctx context.Context) ([]AgentDefinitionResponse, error) {
	rows, err := s.Queries.ListAgentDefinitions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list agent definitions: %w", err)
	}
	// Cost stats are a single aggregation query keyed by definition id —
	// fetch once outside the per-row loop.
	statsRows, err := s.Queries.GetAgentCostStats30d(ctx)
	if err != nil {
		// Soft-fail: a stats query hiccup shouldn't block the list page.
		// Log + render without cost columns.
		s.Logger.Warn("list agent definitions: cost stats query failed", "error", err)
		statsRows = nil
	}
	statsByID := make(map[string]AgentCostStats, len(statsRows))
	for _, r := range statsRows {
		cost, _ := pgconv.NumericToFloat(r.TotalCostUsd)
		statsByID[pgconv.FormatUUID(r.AgentDefinitionID)] = AgentCostStats{
			RunCount:     int(r.RunCount),
			TotalCostUSD: cost,
		}
	}

	// Recent error rollup — same soft-fail pattern.
	errStatsRows, err := s.Queries.GetAgentRecentErrorStats(ctx)
	if err != nil {
		s.Logger.Warn("list agent definitions: recent error stats query failed", "error", err)
		errStatsRows = nil
	}
	errStatsByID := make(map[string]AgentRecentErrorStats, len(errStatsRows))
	for _, r := range errStatsRows {
		errStatsByID[pgconv.FormatUUID(r.AgentDefinitionID)] = AgentRecentErrorStats{
			ErrorCount: int(r.ErrorCount),
			RunCount:   int(r.RunCount),
		}
	}

	// Recent cap rollup — count of cap-exhausted runs in the last 5
	// non-skipped. Mirrors the error-rollup pattern.
	capStatsRows, err := s.Queries.GetAgentRecentCapStats(ctx)
	if err != nil {
		s.Logger.Warn("list agent definitions: recent cap stats query failed", "error", err)
		capStatsRows = nil
	}
	capStatsByID := make(map[string]AgentRecentCapStats, len(capStatsRows))
	for _, r := range capStatsRows {
		capStatsByID[pgconv.FormatUUID(r.AgentDefinitionID)] = AgentRecentCapStats{
			CapCount: int(r.CapCount),
			RunCount: int(r.RunCount),
		}
	}

	// Last-prefix rollup — feeds the "Use last prefix" button in the v2 SPA's
	// Run now dialog. One row per definition (the most recent non-null
	// prompt_prefix); definitions that never had a prefixed run won't appear.
	prefixRows, err := s.Queries.GetAgentLastPromptPrefixes(ctx)
	if err != nil {
		s.Logger.Warn("list agent definitions: last prefix query failed", "error", err)
		prefixRows = nil
	}
	prefixByID := make(map[string]string, len(prefixRows))
	for _, r := range prefixRows {
		if r.PromptPrefix.Valid && r.PromptPrefix.String != "" {
			prefixByID[pgconv.FormatUUID(r.AgentDefinitionID)] = r.PromptPrefix.String
		}
	}

	out := make([]AgentDefinitionResponse, 0, len(rows))
	for _, row := range rows {
		last, err := s.lastRunSummary(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		resp := agentDefinitionFromRow(row, last)
		if stats, ok := statsByID[resp.ID]; ok {
			resp.CostStats30d = &stats
		}
		if errStats, ok := errStatsByID[resp.ID]; ok {
			resp.RecentErrorStats = &errStats
		}
		if capStats, ok := capStatsByID[resp.ID]; ok {
			resp.RecentCapStats = &capStats
		}
		if prefix, ok := prefixByID[resp.ID]; ok {
			p := prefix
			resp.LastPromptPrefix = &p
		}
		if resp.Enabled {
			if nextFire := ComputeNextFire(&resp, time.Now()); nextFire != nil {
				s := nextFire.Format(time.RFC3339)
				resp.NextFireAt = &s
			}
		}
		out = append(out, resp)
	}
	return out, nil
}

// GetAgentDefinition resolves by slug (most common), then short_id, then UUID.
func (s *Service) GetAgentDefinition(ctx context.Context, slugOrID string) (*AgentDefinitionResponse, error) {
	row, err := s.resolveAgentDefinition(ctx, slugOrID)
	if err != nil {
		return nil, err
	}
	last, err := s.lastRunSummary(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	resp := agentDefinitionFromRow(row, last)
	return &resp, nil
}

// CreateAgentDefinition validates, marshals, persists, returns the new row.
func (s *Service) CreateAgentDefinition(ctx context.Context, p CreateAgentDefinitionParams) (*AgentDefinitionResponse, error) {
	if err := validateAgentDefinitionFields(p.Name, p.Slug, p.Prompt, p.ToolScope, p.Model, p.MaxTurns, p.MaxBudgetUSD, p.ScheduleCron); err != nil {
		return nil, err
	}
	allowedJSON, err := agentAllowedToolsToBytes(p.AllowedTools)
	if err != nil {
		return nil, fmt.Errorf("marshal allowed_tools: %w", err)
	}
	model := p.Model
	if model == "" {
		model = DefaultAgentModel
	}
	maxTurns := p.MaxTurns
	if maxTurns <= 0 {
		maxTurns = DefaultAgentMaxTurns
	}
	toolScope := p.ToolScope
	if toolScope == "" {
		toolScope = "read_write"
	}
	budget := p.MaxBudgetUSD
	if budget == nil {
		// Mirror the migration's DEFAULT 1.0000 — column is NOT NULL.
		def := DefaultAgentMaxBudgetUSD
		budget = &def
	}

	if err := validateQuietHours(p.QuietHoursStart, p.QuietHoursEnd); err != nil {
		return nil, err
	}
	row, err := s.Queries.CreateAgentDefinition(ctx, db.CreateAgentDefinitionParams{
		Name:                  p.Name,
		Slug:                  p.Slug,
		Prompt:                p.Prompt,
		SystemPrompt:          pgconv.TextPtrIfNotEmpty(p.SystemPrompt),
		ScheduleCron:          pgconv.TextPtrIfNotEmpty(p.ScheduleCron),
		ToolScope:             toolScope,
		AllowedTools:          allowedJSON,
		Model:                 model,
		MaxTurns:              int32(maxTurns),
		MaxBudgetUsd:          agentNumericFromFloat(budget),
		Enabled:               p.Enabled,
		QuietHoursStart:       pgconv.TextPtrIfNotEmpty(p.QuietHoursStart),
		QuietHoursEnd:         pgconv.TextPtrIfNotEmpty(p.QuietHoursEnd),
		TriggerOnSyncComplete: p.TriggerOnSyncComplete,
	})
	if err != nil {
		return nil, fmt.Errorf("create agent definition: %w", err)
	}
	s.notifyDefinitionChanged()
	resp := agentDefinitionFromRow(row, nil)
	return &resp, nil
}

// UpdateAgentDefinition applies PATCH semantics: fetch, merge non-nil
// fields, validate, persist.
func (s *Service) UpdateAgentDefinition(ctx context.Context, slugOrID string, p UpdateAgentDefinitionParams) (*AgentDefinitionResponse, error) {
	existing, err := s.resolveAgentDefinition(ctx, slugOrID)
	if err != nil {
		return nil, err
	}

	merged := agentDefinitionFromRow(existing, nil)
	if p.Name != nil {
		merged.Name = *p.Name
	}
	if p.Slug != nil {
		merged.Slug = *p.Slug
	}
	if p.Prompt != nil {
		merged.Prompt = *p.Prompt
	}
	if p.SystemPrompt != nil {
		merged.SystemPrompt = p.SystemPrompt
	}
	if p.ScheduleCron != nil {
		merged.ScheduleCron = p.ScheduleCron
	}
	if p.ToolScope != nil {
		merged.ToolScope = *p.ToolScope
	}
	if p.AllowedTools != nil {
		merged.AllowedTools = *p.AllowedTools
	}
	if p.Model != nil {
		merged.Model = *p.Model
	}
	if p.MaxTurns != nil {
		merged.MaxTurns = *p.MaxTurns
	}
	if p.MaxBudgetUSD != nil {
		merged.MaxBudgetUSD = p.MaxBudgetUSD
	}
	if p.Enabled != nil {
		merged.Enabled = *p.Enabled
	}
	if p.QuietHoursStart != nil {
		merged.QuietHoursStart = emptyToNil(*p.QuietHoursStart)
	}
	if p.QuietHoursEnd != nil {
		merged.QuietHoursEnd = emptyToNil(*p.QuietHoursEnd)
	}
	if p.TriggerOnSyncComplete != nil {
		merged.TriggerOnSyncComplete = *p.TriggerOnSyncComplete
	}

	if err := validateAgentDefinitionFields(merged.Name, merged.Slug, merged.Prompt, merged.ToolScope, merged.Model, merged.MaxTurns, merged.MaxBudgetUSD, merged.ScheduleCron); err != nil {
		return nil, err
	}
	if err := validateQuietHours(merged.QuietHoursStart, merged.QuietHoursEnd); err != nil {
		return nil, err
	}
	allowedJSON, err := agentAllowedToolsToBytes(merged.AllowedTools)
	if err != nil {
		return nil, fmt.Errorf("marshal allowed_tools: %w", err)
	}
	budget := merged.MaxBudgetUSD
	if budget == nil {
		def := DefaultAgentMaxBudgetUSD
		budget = &def
	}

	row, err := s.Queries.UpdateAgentDefinition(ctx, db.UpdateAgentDefinitionParams{
		ID:                    existing.ID,
		Name:                  merged.Name,
		Slug:                  merged.Slug,
		Prompt:                merged.Prompt,
		SystemPrompt:          pgconv.TextPtrIfNotEmpty(merged.SystemPrompt),
		ScheduleCron:          pgconv.TextPtrIfNotEmpty(merged.ScheduleCron),
		ToolScope:             merged.ToolScope,
		AllowedTools:          allowedJSON,
		Model:                 merged.Model,
		MaxTurns:              int32(merged.MaxTurns),
		MaxBudgetUsd:          agentNumericFromFloat(budget),
		Enabled:               merged.Enabled,
		QuietHoursStart:       pgconv.TextPtrIfNotEmpty(merged.QuietHoursStart),
		QuietHoursEnd:         pgconv.TextPtrIfNotEmpty(merged.QuietHoursEnd),
		TriggerOnSyncComplete: merged.TriggerOnSyncComplete,
	})
	if err != nil {
		return nil, fmt.Errorf("update agent definition: %w", err)
	}
	s.notifyDefinitionChanged()
	resp := agentDefinitionFromRow(row, nil)
	return &resp, nil
}

// DeleteAgentDefinition removes a definition by slug/short_id/UUID.
// Historical runs are preserved (FK set null).
func (s *Service) DeleteAgentDefinition(ctx context.Context, slugOrID string) error {
	existing, err := s.resolveAgentDefinition(ctx, slugOrID)
	if err != nil {
		return err
	}
	n, err := s.Queries.DeleteAgentDefinition(ctx, existing.ID)
	if err != nil {
		return fmt.Errorf("delete agent definition: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	s.notifyDefinitionChanged()
	return nil
}

// SetAgentDefinitionEnabled flips the enabled flag.
func (s *Service) SetAgentDefinitionEnabled(ctx context.Context, slugOrID string, enabled bool) (*AgentDefinitionResponse, error) {
	existing, err := s.resolveAgentDefinition(ctx, slugOrID)
	if err != nil {
		return nil, err
	}
	row, err := s.Queries.SetAgentDefinitionEnabled(ctx, db.SetAgentDefinitionEnabledParams{
		ID:      existing.ID,
		Enabled: enabled,
	})
	if err != nil {
		return nil, fmt.Errorf("set agent definition enabled: %w", err)
	}
	s.notifyDefinitionChanged()
	resp := agentDefinitionFromRow(row, nil)
	return &resp, nil
}

// AgentRunListParams carries the optional filters for ListAgentRuns. Empty
// fields mean "don't filter on this dimension."
type AgentRunListParams struct {
	Limit   int
	Offset  int
	Status  string // "" | "success" | "error" | "in_progress" | "skipped" | "timeout"
	Trigger string // "" | "cron" | "manual" | "webhook"
	HitCap  string // "" | "max_turns" | "max_budget" | "any"
	// Start / End are inclusive bounds on started_at. RFC3339 or YYYY-MM-DD
	// values from the API layer get parsed at the handler boundary.
	Start *time.Time
	End   *time.Time
}

// ListAgentRuns returns offset-paginated runs for one definition with
// optional status / trigger / date-range filters. Hand-rolled SQL keeps
// the conditional WHERE clauses composable.
func (s *Service) ListAgentRuns(ctx context.Context, agentSlugOrID string, p AgentRunListParams) (*AgentRunListResult, error) {
	limit := p.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := p.Offset
	if offset < 0 {
		offset = 0
	}
	def, err := s.resolveAgentDefinition(ctx, agentSlugOrID)
	if err != nil {
		return nil, err
	}

	args := []any{def.ID}
	where := []string{"agent_definition_id = $1"}
	idx := 2
	if p.Status != "" {
		where = append(where, fmt.Sprintf("status = $%d", idx))
		args = append(args, p.Status)
		idx++
	}
	if p.Trigger != "" {
		where = append(where, fmt.Sprintf(`"trigger" = $%d`, idx))
		args = append(args, p.Trigger)
		idx++
	}
	switch p.HitCap {
	case "max_turns", "max_budget":
		where = append(where, fmt.Sprintf("hit_cap = $%d", idx))
		args = append(args, p.HitCap)
		idx++
	case "any":
		where = append(where, "hit_cap IS NOT NULL")
	case "":
		// no-op
	}
	if p.Start != nil {
		where = append(where, fmt.Sprintf("started_at >= $%d", idx))
		args = append(args, *p.Start)
		idx++
	}
	if p.End != nil {
		where = append(where, fmt.Sprintf("started_at <= $%d", idx))
		args = append(args, *p.End)
		idx++
	}

	// Peek for has_more by asking for limit+1.
	args = append(args, limit+1, offset)
	query := fmt.Sprintf(`
		SELECT id, short_id, agent_definition_id, "trigger", status, started_at, completed_at,
		       duration_ms, total_cost_usd, input_tokens, output_tokens, cache_read_tokens,
		       cache_creation_tokens, turn_count, max_turns_used, num_tool_calls,
		       error_message, transcript_path, session_id,
		       operator_note, prompt_prefix, hit_cap
		FROM agent_runs
		WHERE %s
		ORDER BY started_at DESC
		LIMIT $%d OFFSET $%d`,
		strings.Join(where, " AND "), idx, idx+1)

	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list agent runs: %w", err)
	}
	defer rows.Close()

	var out []AgentRunResponse
	for rows.Next() {
		var r db.AgentRun
		if scanErr := rows.Scan(
			&r.ID, &r.ShortID, &r.AgentDefinitionID, &r.Trigger, &r.Status,
			&r.StartedAt, &r.CompletedAt, &r.DurationMs, &r.TotalCostUsd,
			&r.InputTokens, &r.OutputTokens, &r.CacheReadTokens,
			&r.CacheCreationTokens, &r.TurnCount, &r.MaxTurnsUsed,
			&r.NumToolCalls, &r.ErrorMessage, &r.TranscriptPath, &r.SessionID,
			&r.OperatorNote, &r.PromptPrefix, &r.HitCap,
		); scanErr != nil {
			return nil, fmt.Errorf("scan agent run: %w", scanErr)
		}
		out = append(out, agentRunFromRow(r))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agent runs: %w", err)
	}

	hasMore := len(out) > limit
	if hasMore {
		out = out[:limit]
	}
	if out == nil {
		out = []AgentRunResponse{}
	}
	return &AgentRunListResult{
		Runs:    out,
		Limit:   limit,
		Offset:  offset,
		HasMore: hasMore,
	}, nil
}

// AgentRunNoteMaxLen caps the operator note size both in the SPA textarea
// and on the server. Free-form text but bounded so we don't accidentally
// host arbitrarily-large blobs.
const AgentRunNoteMaxLen = 2000

// SetAgentRunNote updates the operator note on one run. Empty string
// clears the field. Returns the updated row.
func (s *Service) SetAgentRunNote(ctx context.Context, shortIDOrUUID, note string) (*AgentRunResponse, error) {
	if len(note) > AgentRunNoteMaxLen {
		return nil, fmt.Errorf("%w: operator note must be <= %d chars", ErrInvalidParameter, AgentRunNoteMaxLen)
	}
	existing, err := s.resolveAgentRun(ctx, shortIDOrUUID)
	if err != nil {
		return nil, err
	}
	row, err := s.Queries.SetAgentRunNote(ctx, db.SetAgentRunNoteParams{
		ID:           existing.ID,
		OperatorNote: pgconv.TextIfNotEmpty(note),
	})
	if err != nil {
		return nil, fmt.Errorf("set agent run note: %w", err)
	}
	resp := agentRunFromRow(row)
	return &resp, nil
}

// GetAgentRun resolves by short_id or UUID.
func (s *Service) GetAgentRun(ctx context.Context, shortIDOrUUID string) (*AgentRunResponse, error) {
	row, err := s.resolveAgentRun(ctx, shortIDOrUUID)
	if err != nil {
		return nil, err
	}
	resp := agentRunFromRow(row)
	return &resp, nil
}

// notifyDefinitionChanged invokes the (optional) registered callback so the
// agent scheduler can re-register cron entries after a CRUD mutation.
func (s *Service) notifyDefinitionChanged() {
	if s.OnDefinitionChanged != nil {
		s.OnDefinitionChanged()
	}
}

// --- Orchestrator-facing helpers (called by internal/service/agent_orchestrator.go) ---

// CreateAgentRunDB inserts an in_progress run row.
func (s *Service) CreateAgentRunDB(ctx context.Context, defID pgtype.UUID, trigger string) (db.AgentRun, error) {
	return s.Queries.CreateAgentRun(ctx, db.CreateAgentRunParams{
		AgentDefinitionID: defID,
		Trigger:           trigger,
	})
}

// MarkAgentRunErrorDB transitions a run row to status='error'.
func (s *Service) MarkAgentRunErrorDB(ctx context.Context, runID pgtype.UUID, errMsg, transcriptPath string) error {
	return s.Queries.MarkAgentRunError(ctx, db.MarkAgentRunErrorParams{
		ID:             runID,
		ErrorMessage:   pgtype.Text{String: errMsg, Valid: errMsg != ""},
		TranscriptPath: pgtype.Text{String: transcriptPath, Valid: transcriptPath != ""},
	})
}

// MarkAgentRunSkippedDB transitions a run row to status='skipped'.
func (s *Service) MarkAgentRunSkippedDB(ctx context.Context, runID pgtype.UUID, reason string) error {
	return s.Queries.MarkAgentRunSkipped(ctx, db.MarkAgentRunSkippedParams{
		ID:           runID,
		ErrorMessage: pgtype.Text{String: reason, Valid: reason != ""},
	})
}

// SetAgentRunPromptPrefixDB stores the operator-supplied per-run prompt prefix
// alongside an existing in_progress run row. Called right after CreateAgentRunDB
// so the audit trail captures the prefix even when AssembleJobSpec fails.
func (s *Service) SetAgentRunPromptPrefixDB(ctx context.Context, runID pgtype.UUID, prefix string) error {
	return s.Queries.SetAgentRunPromptPrefix(ctx, db.SetAgentRunPromptPrefixParams{
		ID:           runID,
		PromptPrefix: pgtype.Text{String: prefix, Valid: prefix != ""},
	})
}

// SetAgentRunHitCapDB records which safety ceiling terminated the run.
// Called by the orchestrator after CompleteAgentRunDB when the runner
// surfaces ErrMaxTurnsReached / ErrBudgetExceeded. cap must be one of
// "max_turns", "max_budget" — the DB CHECK rejects others.
func (s *Service) SetAgentRunHitCapDB(ctx context.Context, runID pgtype.UUID, cap string) (db.AgentRun, error) {
	return s.Queries.SetAgentRunHitCap(ctx, db.SetAgentRunHitCapParams{
		ID:     runID,
		HitCap: pgtype.Text{String: cap, Valid: cap != ""},
	})
}

// CompleteAgentRunDB persists a terminal RunResult onto the run row.
// Used by the orchestrator after Runner.Run returns.
func (s *Service) CompleteAgentRunDB(ctx context.Context, runID pgtype.UUID, result agent.RunResult) (db.AgentRun, error) {
	costPtr := result.TotalCostUSD
	return s.Queries.CompleteAgentRun(ctx, db.CompleteAgentRunParams{
		ID:                  runID,
		Status:              result.Status,
		DurationMs:          pgtype.Int4{Int32: int32(result.DurationMs), Valid: true},
		TotalCostUsd:        agentNumericFromFloat(&costPtr),
		InputTokens:         pgtype.Int4{Int32: int32(result.InputTokens), Valid: true},
		OutputTokens:        pgtype.Int4{Int32: int32(result.OutputTokens), Valid: true},
		CacheReadTokens:     pgtype.Int4{Int32: int32(result.CacheReadTokens), Valid: true},
		CacheCreationTokens: pgtype.Int4{Int32: int32(result.CacheCreationTokens), Valid: true},
		TurnCount:           pgtype.Int4{Int32: int32(result.TurnCount), Valid: true},
		MaxTurnsUsed:        pgtype.Int4{Int32: int32(result.TurnCount), Valid: true},
		NumToolCalls:        pgtype.Int4{Int32: int32(result.NumToolCalls), Valid: true},
		TranscriptPath:      pgtype.Text{String: result.TranscriptPath, Valid: result.TranscriptPath != ""},
		SessionID:           pgtype.Text{String: result.SessionID, Valid: result.SessionID != ""},
	})
}

// AgentRunFromRow is exported so the orchestrator (same package) can build
// API responses from raw db rows.
func AgentRunFromRow(row db.AgentRun) AgentRunResponse {
	return agentRunFromRow(row)
}

// MintRunAPIKey mints a scoped API key for one agent run.
// Returns the plaintext + the created record. The orchestrator (iter 3)
// is responsible for revocation via RevokeAPIKey on completion.
func (s *Service) MintRunAPIKey(ctx context.Context, def *AgentDefinitionResponse, runShortID string) (*CreateAPIKeyResult, error) {
	scope := "full_access"
	if def.ToolScope == "read_only" {
		scope = "read_only"
	}
	return s.CreateAPIKey(ctx, CreateAPIKeyParams{
		Name:      fmt.Sprintf("agent:%s:%s", def.Slug, runShortID),
		Scope:     scope,
		ActorType: "agent",
		ActorName: def.Slug,
	})
}

// AssembleJobSpec builds the agent.JobSpec the runner needs.
// Reads encrypted auth tokens from app_config and assembles MCP config.
// Does NOT start the run — the orchestrator (iter 3) calls Runner.Run.
func (s *Service) AssembleJobSpec(ctx context.Context, def *AgentDefinitionResponse, run *AgentRunResponse, apiKeyPlaintext string, encKey []byte) (*agent.JobSpec, error) {
	authMode := appconfig.String(ctx, s.Queries, appconfig.KeyAgentAuthMode, appconfig.AuthModeSubscription)

	var token string
	switch authMode {
	case appconfig.AuthModeSubscription:
		t, ok, err := appconfig.ReadEncrypted(ctx, s.Queries, appconfig.KeyAgentSubscriptionToken, encKey)
		if err != nil {
			return nil, fmt.Errorf("read subscription token: %w", err)
		}
		if !ok {
			return nil, agent.ErrAuthNotConfigured
		}
		token = t
	case appconfig.AuthModeAPIKey:
		t, ok, err := appconfig.ReadEncrypted(ctx, s.Queries, appconfig.KeyAgentAnthropicAPIKey, encKey)
		if err != nil {
			return nil, fmt.Errorf("read anthropic api key: %w", err)
		}
		if !ok {
			return nil, agent.ErrAuthNotConfigured
		}
		token = t
	default:
		return nil, fmt.Errorf("agent: unknown auth_mode %q", authMode)
	}

	transcriptDir := appconfig.String(ctx, s.Queries, appconfig.KeyAgentTranscriptDir, "")

	mcpServers := map[string]agent.MCPServerConfig{
		"breadbox": {
			Command: "breadbox",
			Args:    []string{"mcp"},
			Env: map[string]string{
				"BREADBOX_API_KEY": apiKeyPlaintext,
			},
		},
	}

	maxBudget := 1.0
	if def.MaxBudgetUSD != nil {
		maxBudget = *def.MaxBudgetUSD
	}

	spec := &agent.JobSpec{
		RunID:             run.ID,
		AgentDefinitionID: def.ID,
		Prompt:            def.Prompt,
		SystemPrompt:      derefString(def.SystemPrompt),
		Model:             def.Model,
		MaxTurns:          def.MaxTurns,
		MaxBudgetUsd:      maxBudget,
		ToolScope:         def.ToolScope,
		AllowedTools:      def.AllowedTools,
		MCPServers:        mcpServers,
		Auth: agent.AuthConfig{
			Mode:  authMode,
			Token: token,
		},
	}
	if run.SessionID != nil {
		spec.SessionID = *run.SessionID
	}
	if transcriptDir != "" {
		spec.TranscriptPath = transcriptDir + "/" + run.ShortID + ".ndjson"
	}
	return spec, nil
}

// --- internal helpers ---

func (s *Service) resolveAgentDefinition(ctx context.Context, slugOrID string) (db.AgentDefinition, error) {
	if slugOrID == "" {
		return db.AgentDefinition{}, ErrNotFound
	}
	// Try slug first.
	if row, err := s.Queries.GetAgentDefinitionBySlug(ctx, slugOrID); err == nil {
		return row, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return db.AgentDefinition{}, fmt.Errorf("get by slug: %w", err)
	}
	// Try short_id (8 chars).
	if len(slugOrID) == 8 {
		if row, err := s.Queries.GetAgentDefinitionByShortID(ctx, slugOrID); err == nil {
			return row, nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return db.AgentDefinition{}, fmt.Errorf("get by short_id: %w", err)
		}
	}
	// Try UUID.
	if u, err := pgconv.ParseUUID(slugOrID); err == nil {
		if row, err := s.Queries.GetAgentDefinition(ctx, u); err == nil {
			return row, nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return db.AgentDefinition{}, fmt.Errorf("get by uuid: %w", err)
		}
	}
	return db.AgentDefinition{}, ErrNotFound
}

func (s *Service) resolveAgentRun(ctx context.Context, shortIDOrUUID string) (db.AgentRun, error) {
	if shortIDOrUUID == "" {
		return db.AgentRun{}, ErrNotFound
	}
	if len(shortIDOrUUID) == 8 {
		if row, err := s.Queries.GetAgentRunByShortID(ctx, shortIDOrUUID); err == nil {
			return row, nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return db.AgentRun{}, fmt.Errorf("get run by short_id: %w", err)
		}
	}
	if u, err := pgconv.ParseUUID(shortIDOrUUID); err == nil {
		if row, err := s.Queries.GetAgentRun(ctx, u); err == nil {
			return row, nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return db.AgentRun{}, fmt.Errorf("get run by uuid: %w", err)
		}
	}
	return db.AgentRun{}, ErrNotFound
}

func (s *Service) lastRunSummary(ctx context.Context, defID pgtype.UUID) (*AgentRunSummary, error) {
	row, err := s.Queries.GetLatestAgentRun(ctx, defID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest agent run: %w", err)
	}
	sum := agentRunSummaryFromRow(row)
	return &sum, nil
}

func validateAgentDefinitionFields(name, slug, prompt, toolScope, model string, maxTurns int, maxBudget *float64, scheduleCron *string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidParameter)
	}
	if !validAgentSlug.MatchString(slug) {
		return fmt.Errorf("%w: slug must be kebab-case (lowercase letters, digits, dashes; 2-64 chars)", ErrInvalidParameter)
	}
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("%w: prompt is required", ErrInvalidParameter)
	}
	if toolScope != "" && toolScope != "read_only" && toolScope != "read_write" {
		return fmt.Errorf("%w: tool_scope must be read_only or read_write", ErrInvalidParameter)
	}
	if model != "" && strings.TrimSpace(model) == "" {
		return fmt.Errorf("%w: model cannot be blank", ErrInvalidParameter)
	}
	if maxTurns < 0 || maxTurns > 100 {
		return fmt.Errorf("%w: max_turns must be 1-100", ErrInvalidParameter)
	}
	if maxBudget != nil && (*maxBudget < 0 || *maxBudget > 1000) {
		return fmt.Errorf("%w: max_budget_usd must be 0-1000", ErrInvalidParameter)
	}
	if scheduleCron != nil && *scheduleCron != "" {
		// Light validation: 5 fields. robfig/cron does full parsing at registration.
		fields := strings.Fields(*scheduleCron)
		if len(fields) != 5 {
			return fmt.Errorf("%w: schedule_cron must be a 5-field cron expression", ErrInvalidParameter)
		}
	}
	return nil
}

func agentAllowedToolsToBytes(tools []string) ([]byte, error) {
	if len(tools) == 0 {
		return []byte("[]"), nil
	}
	return json.Marshal(tools)
}

func agentAllowedToolsFromBytes(b []byte) []string {
	if len(b) == 0 {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal(b, &out); err != nil {
		return []string{}
	}
	if out == nil {
		return []string{}
	}
	return out
}

func agentNumericFromFloat(f *float64) pgtype.Numeric {
	if f == nil {
		return pgtype.Numeric{}
	}
	var n pgtype.Numeric
	_ = n.Scan(strconv.FormatFloat(*f, 'f', 4, 64))
	return n
}

func agentFloatFromNumeric(n pgtype.Numeric) *float64 {
	v, ok := pgconv.NumericToFloat(n)
	if !ok {
		return nil
	}
	return &v
}

func agentIntFromInt4(n pgtype.Int4) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int32)
	return &v
}

func agentDefinitionFromRow(row db.AgentDefinition, lastRun *AgentRunSummary) AgentDefinitionResponse {
	return AgentDefinitionResponse{
		ID:              pgconv.FormatUUID(row.ID),
		ShortID:         row.ShortID,
		Name:            row.Name,
		Slug:            row.Slug,
		Prompt:          row.Prompt,
		SystemPrompt:    pgconv.TextPtr(row.SystemPrompt),
		ScheduleCron:    pgconv.TextPtr(row.ScheduleCron),
		ToolScope:       row.ToolScope,
		AllowedTools:    agentAllowedToolsFromBytes(row.AllowedTools),
		Model:           row.Model,
		MaxTurns:        int(row.MaxTurns),
		MaxBudgetUSD:    agentFloatFromNumeric(row.MaxBudgetUsd),
		Enabled:         row.Enabled,
		QuietHoursStart:       pgconv.TextPtr(row.QuietHoursStart),
		QuietHoursEnd:         pgconv.TextPtr(row.QuietHoursEnd),
		TriggerOnSyncComplete: row.TriggerOnSyncComplete,
		LastRun:               lastRun,
		CreatedAt:       pgconv.TimestampStr(row.CreatedAt),
		UpdatedAt:       pgconv.TimestampStr(row.UpdatedAt),
	}
}

func agentRunFromRow(row db.AgentRun) AgentRunResponse {
	var defID *string
	if row.AgentDefinitionID.Valid {
		s := pgconv.FormatUUID(row.AgentDefinitionID)
		defID = &s
	}
	return AgentRunResponse{
		ID:                  pgconv.FormatUUID(row.ID),
		ShortID:             row.ShortID,
		AgentDefinitionID:   defID,
		Trigger:             row.Trigger,
		Status:              row.Status,
		StartedAt:           pgconv.TimestampStr(row.StartedAt),
		CompletedAt:         pgconv.TimestampStrPtr(row.CompletedAt),
		DurationMs:          agentIntFromInt4(row.DurationMs),
		TotalCostUSD:        agentFloatFromNumeric(row.TotalCostUsd),
		InputTokens:         agentIntFromInt4(row.InputTokens),
		OutputTokens:        agentIntFromInt4(row.OutputTokens),
		CacheReadTokens:     agentIntFromInt4(row.CacheReadTokens),
		CacheCreationTokens: agentIntFromInt4(row.CacheCreationTokens),
		TurnCount:           agentIntFromInt4(row.TurnCount),
		MaxTurnsUsed:        agentIntFromInt4(row.MaxTurnsUsed),
		NumToolCalls:        agentIntFromInt4(row.NumToolCalls),
		ErrorMessage:        pgconv.TextPtr(row.ErrorMessage),
		TranscriptPath:      pgconv.TextPtr(row.TranscriptPath),
		SessionID:           pgconv.TextPtr(row.SessionID),
		OperatorNote:        pgconv.TextPtr(row.OperatorNote),
		PromptPrefix:        pgconv.TextPtr(row.PromptPrefix),
		HitCap:              pgconv.TextPtr(row.HitCap),
	}
}

func agentRunSummaryFromRow(row db.AgentRun) AgentRunSummary {
	return AgentRunSummary{
		ShortID:      row.ShortID,
		Status:       row.Status,
		Trigger:      row.Trigger,
		StartedAt:    pgconv.TimestampStr(row.StartedAt),
		CompletedAt:  pgconv.TimestampStrPtr(row.CompletedAt),
		DurationMs:   agentIntFromInt4(row.DurationMs),
		TotalCostUSD: agentFloatFromNumeric(row.TotalCostUsd),
	}
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func emptyToNil(s string) *string {
	if s == "" {
		return nil
	}
	out := s
	return &out
}

// validateQuietHours enforces both bounds are valid "HH:MM" 24-hour
// strings, or both nil. One-sided is rejected — a quiet window needs both edges.
func validateQuietHours(start, end *string) error {
	startSet := start != nil && *start != ""
	endSet := end != nil && *end != ""
	if !startSet && !endSet {
		return nil
	}
	if startSet != endSet {
		return fmt.Errorf("%w: quiet_hours_start and quiet_hours_end must both be set or both empty", ErrInvalidParameter)
	}
	if _, ok := parseHHMM(*start); !ok {
		return fmt.Errorf("%w: quiet_hours_start must be HH:MM (24-hour)", ErrInvalidParameter)
	}
	if _, ok := parseHHMM(*end); !ok {
		return fmt.Errorf("%w: quiet_hours_end must be HH:MM (24-hour)", ErrInvalidParameter)
	}
	return nil
}

// parseHHMM returns minutes-from-midnight and ok=true for a valid "HH:MM"
// 24-hour string.
func parseHHMM(s string) (int, bool) {
	if len(s) != 5 || s[2] != ':' {
		return 0, false
	}
	h, err := strconv.Atoi(s[:2])
	if err != nil || h < 0 || h > 23 {
		return 0, false
	}
	m, err := strconv.Atoi(s[3:])
	if err != nil || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// IsWithinQuietHours reports whether `now` falls inside the [start, end)
// window. False on unset/unparseable. Handles windows that wrap midnight
// (e.g. start=22:00, end=07:00 → quiet 10 PM through 7 AM next morning).
func IsWithinQuietHours(now time.Time, start, end *string) bool {
	if start == nil || end == nil || *start == "" || *end == "" {
		return false
	}
	startMin, ok := parseHHMM(*start)
	if !ok {
		return false
	}
	endMin, ok := parseHHMM(*end)
	if !ok {
		return false
	}
	if startMin == endMin {
		return false
	}
	cur := now.Hour()*60 + now.Minute()
	if startMin < endMin {
		return cur >= startMin && cur < endMin
	}
	return cur >= startMin || cur < endMin
}

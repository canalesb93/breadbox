//go:build !lite

package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"breadbox/internal/agent"
	"breadbox/internal/app"
	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// --- Request envelopes ---

type createAgentRequest struct {
	Name                  string   `json:"name"`
	Slug                  string   `json:"slug"`
	Prompt                string   `json:"prompt"`
	SystemPrompt          *string  `json:"system_prompt"`
	ScheduleCron          *string  `json:"schedule_cron"`
	ToolScope             string   `json:"tool_scope"`
	AllowedTools          []string `json:"allowed_tools"`
	Model                 string   `json:"model"`
	MaxTurns              int      `json:"max_turns"`
	MaxBudgetUSD          *float64 `json:"max_budget_usd"`
	Enabled               bool     `json:"enabled"`
	QuietHoursStart       *string  `json:"quiet_hours_start"`
	QuietHoursEnd         *string  `json:"quiet_hours_end"`
	TriggerOnSyncComplete bool     `json:"trigger_on_sync_complete"`
}

type updateAgentRequest struct {
	Name                  *string   `json:"name"`
	Slug                  *string   `json:"slug"`
	Prompt                *string   `json:"prompt"`
	SystemPrompt          *string   `json:"system_prompt"`
	ScheduleCron          *string   `json:"schedule_cron"`
	ToolScope             *string   `json:"tool_scope"`
	AllowedTools          *[]string `json:"allowed_tools"`
	Model                 *string   `json:"model"`
	MaxTurns              *int      `json:"max_turns"`
	MaxBudgetUSD          *float64  `json:"max_budget_usd"`
	Enabled               *bool     `json:"enabled"`
	QuietHoursStart       *string   `json:"quiet_hours_start"`
	QuietHoursEnd         *string   `json:"quiet_hours_end"`
	TriggerOnSyncComplete *bool     `json:"trigger_on_sync_complete"`
}

type updateAgentSettingsRequest struct {
	AuthMode           *string  `json:"auth_mode"`
	SubscriptionToken  *string  `json:"subscription_token"`
	AnthropicAPIKey    *string  `json:"anthropic_api_key"`
	MaxConcurrent      *int     `json:"max_concurrent"`
	GlobalMaxBudgetUSD *float64 `json:"global_max_budget_usd"`
	RuntimePath        *string  `json:"runtime_path"`
	TranscriptDir      *string  `json:"transcript_dir"`
}

// --- Handlers: definitions ---

// ListAgentDefinitionsHandler returns all agents with last_run inlined.
func ListAgentDefinitionsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := svc.ListAgentDefinitions(r.Context())
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list agents")
			return
		}
		writeData(w, out)
	}
}

// GetAgentDefinitionHandler resolves by slug/short_id/UUID.
func GetAgentDefinitionHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		def, err := svc.GetAgentDefinition(r.Context(), slug)
		if err != nil {
			writeAgentError(w, err, "agent not found")
			return
		}
		writeData(w, def)
	}
}

// CreateAgentDefinitionHandler creates a new agent.
func CreateAgentDefinitionHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createAgentRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		out, err := svc.CreateAgentDefinition(r.Context(), service.CreateAgentDefinitionParams{
			Name:                  req.Name,
			Slug:                  req.Slug,
			Prompt:                req.Prompt,
			SystemPrompt:          req.SystemPrompt,
			ScheduleCron:          req.ScheduleCron,
			ToolScope:             req.ToolScope,
			AllowedTools:          req.AllowedTools,
			Model:                 req.Model,
			MaxTurns:              req.MaxTurns,
			MaxBudgetUSD:          req.MaxBudgetUSD,
			Enabled:               req.Enabled,
			QuietHoursStart:       req.QuietHoursStart,
			QuietHoursEnd:         req.QuietHoursEnd,
			TriggerOnSyncComplete: req.TriggerOnSyncComplete,
		})
		if err != nil {
			if writeAgentDefinitionMutationError(w, err, "Failed to create agent") {
				return
			}
		}
		w.WriteHeader(http.StatusCreated)
		writeData(w, out)
	}
}

// UpdateAgentDefinitionHandler patches an existing agent.
func UpdateAgentDefinitionHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		var req updateAgentRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		out, err := svc.UpdateAgentDefinition(r.Context(), slug, service.UpdateAgentDefinitionParams{
			Name:                  req.Name,
			Slug:                  req.Slug,
			Prompt:                req.Prompt,
			SystemPrompt:          req.SystemPrompt,
			ScheduleCron:          req.ScheduleCron,
			ToolScope:             req.ToolScope,
			AllowedTools:          req.AllowedTools,
			Model:                 req.Model,
			MaxTurns:              req.MaxTurns,
			MaxBudgetUSD:          req.MaxBudgetUSD,
			Enabled:               req.Enabled,
			QuietHoursStart:       req.QuietHoursStart,
			QuietHoursEnd:         req.QuietHoursEnd,
			TriggerOnSyncComplete: req.TriggerOnSyncComplete,
		})
		if err != nil {
			// Try not-found / validation first, then mutation-shaped errors
			// (duplicate slug from a rename → 409 CONFLICT, otherwise 500).
			// Before iter-35 the dup-slug branch only existed on Create;
			// Update fell through to a generic 500.
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "agent not found")
				return
			}
			if writeAgentDefinitionMutationError(w, err, "Failed to update agent") {
				return
			}
		}
		writeData(w, out)
	}
}

// writeAgentDefinitionMutationError maps the shared shape of Create +
// Update errors: ErrInvalidParameter → 400 VALIDATION_ERROR, Postgres
// unique-constraint failure → 409 CONFLICT (the slug column is the only
// unique field), everything else → 500 with the supplied message.
// Returns true if a response was written (caller should `return`).
func writeAgentDefinitionMutationError(w http.ResponseWriter, err error, fallbackMsg string) bool {
	if errors.Is(err, service.ErrInvalidParameter) {
		mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return true
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "duplicate key") || strings.Contains(lower, "unique constraint") {
		mw.WriteError(w, http.StatusConflict, "CONFLICT", "An agent with this slug already exists")
		return true
	}
	mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", fallbackMsg)
	return true
}

// DeleteAgentDefinitionHandler deletes an agent (runs preserved).
func DeleteAgentDefinitionHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		if err := svc.DeleteAgentDefinition(r.Context(), slug); err != nil {
			writeAgentError(w, err, "agent not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// EnableAgentHandler flips enabled=true.
func EnableAgentHandler(svc *service.Service) http.HandlerFunc {
	return setAgentEnabled(svc, true)
}

// DisableAgentHandler flips enabled=false.
func DisableAgentHandler(svc *service.Service) http.HandlerFunc {
	return setAgentEnabled(svc, false)
}

func setAgentEnabled(svc *service.Service, enabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		out, err := svc.SetAgentDefinitionEnabled(r.Context(), slug, enabled)
		if err != nil {
			writeAgentError(w, err, "agent not found")
			return
		}
		writeData(w, out)
	}
}

// --- Handlers: runs ---

// ListAgentRunsHandler returns paginated runs for one agent. Supports
// optional status / trigger / date-range filters via query params.
func ListAgentRunsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		q := r.URL.Query()
		limit, err := parseIntParam(q, "limit", 50, 1, 200)
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}
		offset, err := parseIntParam(q, "offset", 0, 0, 1<<20)
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}
		params := service.AgentRunListParams{
			Limit:   limit,
			Offset:  offset,
			Status:  q.Get("status"),
			Trigger: q.Get("trigger"),
			HitCap:  q.Get("hit_cap"),
		}
		if s := q.Get("start"); s != "" {
			t, perr := parseDateOrRFC3339(s)
			if perr != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "start must be YYYY-MM-DD or RFC3339")
				return
			}
			params.Start = &t
		}
		if s := q.Get("end"); s != "" {
			t, perr := parseDateOrRFC3339(s)
			if perr != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "end must be YYYY-MM-DD or RFC3339")
				return
			}
			// YYYY-MM-DD inputs land at 00:00; bump to end-of-day so the
			// inclusive bound matches user expectation ("through Friday").
			if len(s) == 10 {
				t = t.Add(24*time.Hour - time.Second)
			}
			params.End = &t
		}
		if !isAllowedRunStatus(params.Status) {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"status must be one of: success, error, in_progress, skipped, timeout (empty = all)")
			return
		}
		if !isAllowedRunTrigger(params.Trigger) {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"trigger must be one of: cron, manual, webhook (empty = all)")
			return
		}
		if !isAllowedRunHitCap(params.HitCap) {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"hit_cap must be one of: max_turns, max_budget, any (empty = all)")
			return
		}
		out, err := svc.ListAgentRuns(r.Context(), slug, params)
		if err != nil {
			writeAgentError(w, err, "agent not found")
			return
		}
		writeData(w, out)
	}
}

func parseDateOrRFC3339(s string) (time.Time, error) {
	if len(s) == 10 { // YYYY-MM-DD
		return time.Parse("2006-01-02", s)
	}
	return time.Parse(time.RFC3339, s)
}

func isAllowedRunStatus(s string) bool {
	switch s {
	case "", "success", "error", "in_progress", "skipped", "timeout":
		return true
	}
	return false
}

func isAllowedRunHitCap(s string) bool {
	switch s {
	case "", "max_turns", "max_budget", "any":
		return true
	}
	return false
}

func isAllowedRunTrigger(s string) bool {
	switch s {
	case "", "cron", "manual", "webhook":
		return true
	}
	return false
}

// GetAgentRunHandler resolves by short_id or UUID.
func GetAgentRunHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "shortId")
		out, err := svc.GetAgentRun(r.Context(), id)
		if err != nil {
			writeAgentError(w, err, "run not found")
			return
		}
		writeData(w, out)
	}
}

// GetAgentRunTranscriptHandler streams the NDJSON transcript file.
func GetAgentRunTranscriptHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "shortId")
		run, err := svc.GetAgentRun(r.Context(), id)
		if err != nil {
			writeAgentError(w, err, "run not found")
			return
		}
		if run.TranscriptPath == nil || *run.TranscriptPath == "" {
			mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Transcript not available for this run")
			return
		}
		f, err := os.Open(*run.TranscriptPath)
		if err != nil {
			mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Transcript file missing on disk")
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		// Best-effort copy; partial reads are OK for a viewer.
		buf := make([]byte, 32*1024)
		for {
			n, rerr := f.Read(buf)
			if n > 0 {
				if _, werr := w.Write(buf[:n]); werr != nil {
					return
				}
			}
			if rerr != nil {
				return
			}
		}
	}
}

// --- Handlers: settings ---

// AgentSubsystemStatusHandler reports whether the agent subsystem is ready
// to fire — same checks as `breadbox doctor`, side-effect-free. The v2 SPA
// list page calls this to render onboarding hints before the user sees a
// wall of seeded starter agents they can't run.
func AgentSubsystemStatusHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeData(w, svc.GetAgentSubsystemStatus(r.Context()))
	}
}

// GetAgentSettingsHandler returns the agent.* config with masked tokens.
func GetAgentSettingsHandler(svc *service.Service, a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := svc.GetAgentSettings(r.Context(), a.Config.EncryptionKey)
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to read agent settings")
			return
		}
		writeData(w, out)
	}
}

// UpdateAgentSettingsHandler applies a PATCH-style update.
func UpdateAgentSettingsHandler(svc *service.Service, a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateAgentSettingsRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		out, err := svc.UpdateAgentSettings(r.Context(), service.UpdateAgentSettingsParams{
			AuthMode:           req.AuthMode,
			SubscriptionToken:  req.SubscriptionToken,
			AnthropicAPIKey:    req.AnthropicAPIKey,
			MaxConcurrent:      req.MaxConcurrent,
			GlobalMaxBudgetUSD: req.GlobalMaxBudgetUSD,
			RuntimePath:        req.RuntimePath,
			TranscriptDir:      req.TranscriptDir,
		}, a.Config.EncryptionKey)
		if err != nil {
			if errors.Is(err, service.ErrInvalidParameter) {
				mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update agent settings")
			return
		}
		writeData(w, out)
	}
}

// runAgentNowRequest is the optional JSON body for "run now". An empty
// body — common for the v2 SPA's bare-button click — leaves both fields
// unset, which the orchestrator treats as "use the saved def.Prompt
// verbatim."
type runAgentNowRequest struct {
	PromptPrefix string `json:"prompt_prefix,omitempty"`
	// PromptOverride replaces def.Prompt entirely for this fire. Powers
	// the iter-45 "Test this prompt" button on the agent edit page so an
	// operator can dry-fire an unsaved prompt without round-tripping
	// through Save. Mutually exclusive with PromptPrefix at apply time
	// (override wins) per Orchestrator.RunNowWith.
	PromptOverride string `json:"prompt,omitempty"`
}

// PromptPrefixMaxLen caps operator-supplied prefixes so a runaway paste
// can't bloat the prompt past the model's effective context. Matches the
// 2000-char ceiling used for operator notes — the two surfaces feel
// related from the operator's POV.
const PromptPrefixMaxLen = 2000

// PromptOverrideMaxLen caps the iter-45 full-prompt override. 20× the
// prefix cap because real agent prompts are long-form markdown (the
// seeded starters run 1000-2500 chars each). 40 KB is well under any
// model's effective context but blocks pathological pastes.
const PromptOverrideMaxLen = 40_000

// RunAgentNowHandler triggers an immediate synchronous run of the named agent.
// 503 CONCURRENCY_LOCKED when another run is in progress (no DB row written).
// 422 AUTH_NOT_CONFIGURED when no Anthropic credentials are set.
// 422 AGENT_BINARY_NOT_FOUND when the sidecar binary can't be located.
// 400 PROMPT_PREFIX_TOO_LONG when prompt_prefix exceeds PromptPrefixMaxLen.
// 200 with the agent_runs row otherwise (status may be 'error' if the run
// itself failed — the row contains error_message).
//
// Body is optional: { "prompt_prefix": "Focus on …" } prepends the supplied
// text to the agent's stored prompt for this fire only. An empty body or an
// empty prefix is equivalent to the v1 bare-button behavior.
func RunAgentNowHandler(svc *service.Service, orch *service.Orchestrator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if orch == nil {
			mw.WriteError(w, http.StatusServiceUnavailable, "AGENTS_DISABLED",
				"Agent orchestrator is not configured on this server")
			return
		}
		slug := chi.URLParam(r, "slug")
		def, err := svc.GetAgentDefinition(r.Context(), slug)
		if err != nil {
			writeAgentError(w, err, "agent not found")
			return
		}
		var req runAgentNowRequest
		if r.ContentLength > 0 {
			if !decodeJSON(w, r, &req) {
				return
			}
		}
		if len(req.PromptPrefix) > PromptPrefixMaxLen {
			mw.WriteError(w, http.StatusBadRequest, "PROMPT_PREFIX_TOO_LONG",
				fmt.Sprintf("prompt_prefix must be at most %d characters", PromptPrefixMaxLen))
			return
		}
		if len(req.PromptOverride) > PromptOverrideMaxLen {
			mw.WriteError(w, http.StatusBadRequest, "PROMPT_TOO_LONG",
				fmt.Sprintf("prompt must be at most %d characters", PromptOverrideMaxLen))
			return
		}
		runResp, runErr := orch.RunNowWith(r.Context(), def, service.RunOverrides{
			PromptPrefix:   req.PromptPrefix,
			PromptOverride: req.PromptOverride,
		})
		if errors.Is(runErr, agent.ErrConcurrencyLocked) {
			mw.WriteError(w, http.StatusServiceUnavailable,
				"CONCURRENCY_LOCKED",
				"Another agent run is in progress. Retry when it completes.")
			return
		}
		if errors.Is(runErr, agent.ErrAuthNotConfigured) {
			mw.WriteError(w, http.StatusUnprocessableEntity,
				"AUTH_NOT_CONFIGURED",
				"Agent authentication is not configured. Set subscription_token or anthropic_api_key in settings.")
			return
		}
		if errors.Is(runErr, agent.ErrBinaryNotFound) {
			mw.WriteError(w, http.StatusUnprocessableEntity,
				"AGENT_BINARY_NOT_FOUND",
				"breadbox-agent binary not found. Run `make agent-sidecar` or set agent.runtime_path.")
			return
		}
		// runResp may be non-nil even when runErr is set (mint succeeded but
		// the sidecar reported failure). Return the row in that case.
		if runResp != nil {
			w.WriteHeader(http.StatusCreated)
			writeData(w, runResp)
			return
		}
		if runErr != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Agent run failed")
			return
		}
		// Unreachable in practice: runResp nil and err nil.
		w.WriteHeader(http.StatusNoContent)
	}
}

type updateAgentRunNoteRequest struct {
	Note string `json:"note"`
}

// UpdateAgentRunNoteHandler sets/clears the operator note on one run.
// Body: { "note": "..." }; empty string clears the field. Capped at 2000
// chars server-side (matches the SPA textarea cap).
func UpdateAgentRunNoteHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "shortId")
		var req updateAgentRunNoteRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		out, err := svc.SetAgentRunNote(r.Context(), id, req.Note)
		if err != nil {
			writeAgentError(w, err, "run not found")
			return
		}
		writeData(w, out)
	}
}

// agentTestResponse mirrors agent.SmokeResult for the JSON wire format.
type agentTestResponse struct {
	AuthMode     string  `json:"auth_mode"`
	BinaryPath   string  `json:"binary_path,omitempty"`
	Model        string  `json:"model"`
	DurationMs   int64   `json:"duration_ms"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	Response     string  `json:"response,omitempty"`
}

// SmokeTestAgentHandler runs the same diagnostic the CLI's
// `breadbox agent test` does. Surfaces it through the UI so non-CLI
// self-hosters can validate their setup before scheduling real runs.
// Cost-bounded (~5¢ ceiling).
func SmokeTestAgentHandler(orch *service.Orchestrator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if orch == nil {
			mw.WriteError(w, http.StatusServiceUnavailable, "AGENTS_DISABLED",
				"Agent orchestrator is not configured on this server")
			return
		}
		result, err := orch.SmokeTest(r.Context())
		if err != nil {
			switch {
			case errors.Is(err, agent.ErrAuthNotConfigured):
				mw.WriteError(w, http.StatusUnprocessableEntity,
					"AUTH_NOT_CONFIGURED",
					"Set a subscription token or Anthropic API key in agent settings before running the smoke test.")
			case errors.Is(err, agent.ErrBinaryNotFound):
				mw.WriteError(w, http.StatusUnprocessableEntity,
					"AGENT_BINARY_NOT_FOUND",
					"breadbox-agent binary not found. Run `make agent-sidecar` or set agent.runtime_path.")
			default:
				mw.WriteError(w, http.StatusInternalServerError,
					"AGENT_TEST_FAILED", err.Error())
			}
			return
		}
		writeData(w, agentTestResponse{
			AuthMode:     result.AuthMode,
			BinaryPath:   result.BinaryPath,
			Model:        result.Model,
			DurationMs:   result.DurationMs,
			TotalCostUSD: result.TotalCostUSD,
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
			Response:     result.AssistantText,
		})
	}
}

// writeAgentError maps a service error to the JSON error envelope.
func writeAgentError(w http.ResponseWriter, err error, notFoundMsg string) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", notFoundMsg)
	case errors.Is(err, service.ErrInvalidParameter):
		mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Unexpected error")
	}
}

// RunAgentCleanupHandler triggers an on-demand agent cleanup pass (the
// same one the 3:15 AM cron does) and returns the resulting counts. Lets
// operators who just lowered retention see the effect without waiting
// for the next cron tick.
// 503 AGENTS_DISABLED when no scheduler is configured.
// 200 with { runs_deleted, transcripts_deleted, transcripts_scanned,
// retention_days, transcript_dir } otherwise.
func RunAgentCleanupHandler(sched *service.AgentScheduler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if sched == nil {
			mw.WriteError(w, http.StatusServiceUnavailable, "AGENTS_DISABLED",
				"Agent scheduler is not configured on this server")
			return
		}
		result := sched.RunCleanupNow(r.Context())
		writeData(w, result)
	}
}

// ListRecentErroredAgentRunsHandler returns the most recent errored runs
// across all agents in the last `hours` hours (default 24, max 168),
// capped at `limit` (default 5, max 50). Powers the v2 SPA "Run-failed
// banner" on /v2/agents — catches operators who don't drill into each
// agent's history daily. Read-only; no scope upgrade required.
func ListRecentErroredAgentRunsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		hours, err := parseIntParam(q, "hours", 24, 1, 168)
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}
		limit, err := parseIntParam(q, "limit", 5, 1, 50)
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}
		out, err := svc.ListRecentErroredAgentRuns(r.Context(), hours, limit)
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR",
				"Failed to list recent errored runs")
			return
		}
		writeData(w, out)
	}
}

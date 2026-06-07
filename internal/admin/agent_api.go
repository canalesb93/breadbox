//go:build !headless && !lite

package admin

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"breadbox/internal/agent"
	"breadbox/internal/app"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// EnableAgentAdminHandler handles POST /-/agents/{slug}/enable. Returns JSON
// for Alpine fetch — admin-list page uses this to flip the toggle without a
// full page reload.
func EnableAgentAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setAgentEnabledJSON(w, r, svc, true)
	}
}

// DisableAgentAdminHandler handles POST /-/agents/{slug}/disable.
func DisableAgentAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setAgentEnabledJSON(w, r, svc, false)
	}
}

func setAgentEnabledJSON(w http.ResponseWriter, r *http.Request, svc *service.Service, enabled bool) {
	slug := chi.URLParam(r, "slug")
	def, err := svc.SetAgentDefinitionEnabled(r.Context(), slug, enabled)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "AGENT_TOGGLE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, def)
}

// RunAgentNowAdminHandler handles POST /-/agents/{slug}/run. Forwards to the
// orchestrator via the service layer; mirrors the REST endpoint's semantics
// (503 CONCURRENCY_LOCKED when another run is in-flight).
//
// The actual run call is exposed on Service via its orchestrator hook —
// admin shouldn't reach past the service into agent.* directly. Until
// service grows a convenience wrapper, we fall back to looking up the
// definition and asking the orchestrator embedded on the service if it's
// available; otherwise we return 422 to keep the surface honest.
func RunAgentNowAdminHandler(a *app.App, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")

		// Pull the optional prompt overrides from the body. Form OR JSON
		// both work — admin forms send urlencoded, the list-page Alpine
		// fetch may send empty body.
		var promptPrefix, promptOverride string
		ctype := r.Header.Get("Content-Type")
		switch {
		case strings.HasPrefix(ctype, "application/json"):
			var body struct {
				PromptPrefix string `json:"prompt_prefix"`
				Prompt       string `json:"prompt"`
			}
			if !decodeJSON(w, r, &body) {
				return
			}
			promptPrefix = strings.TrimSpace(body.PromptPrefix)
			promptOverride = strings.TrimSpace(body.Prompt)
		default:
			_ = r.ParseForm()
			promptPrefix = strings.TrimSpace(r.FormValue("prompt_prefix"))
			promptOverride = strings.TrimSpace(r.FormValue("prompt"))
		}

		orchestrator := a.AgentOrchestrator
		if orchestrator == nil {
			writeError(w, http.StatusServiceUnavailable, "AGENTS_DISABLED", "Agent orchestrator not configured")
			return
		}

		def, err := svc.GetAgentDefinition(r.Context(), slug)
		if err != nil {
			writeError(w, http.StatusNotFound, "AGENT_NOT_FOUND", err.Error())
			return
		}

		// Async dispatch: returns the in_progress row immediately, hands the
		// sidecar invocation + completion + revoke to a goroutine that owns a
		// fresh 30-minute context. Critical because spending-report / bulk-review
		// runs routinely exceed the exe.dev proxy's ~8s HTTP timeout; the sync
		// variant left rows stuck in_progress when the request context cancelled
		// mid-run (sidecar's orphaned Node child still completed via MCP, but
		// the persist step failed with context canceled). The list-page UI
		// already calls window.location.reload() 800ms after the response, so
		// it doesn't need the completed row — only that the run started.
		run, err := orchestrator.RunNowAsyncWith(r.Context(), def, service.RunOverrides{
			PromptPrefix:   promptPrefix,
			PromptOverride: promptOverride,
		})
		if err != nil {
			if errors.Is(err, agent.ErrConcurrencyLocked) {
				writeError(w, http.StatusServiceUnavailable, "CONCURRENCY_LOCKED", "Another run is in progress")
				return
			}
			if errors.Is(err, service.ErrBudgetCeilingReached) {
				writeError(w, http.StatusTooManyRequests, "BUDGET_CEILING_REACHED", err.Error())
				return
			}
			if errors.Is(err, agent.ErrAuthNotConfigured) {
				writeError(w, http.StatusUnprocessableEntity, "AUTH_NOT_CONFIGURED", err.Error())
				return
			}
			if errors.Is(err, agent.ErrBinaryNotFound) {
				writeError(w, http.StatusUnprocessableEntity, "BINARY_NOT_FOUND", err.Error())
				return
			}
			writeError(w, http.StatusUnprocessableEntity, "RUN_FAILED", err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, run)
	}
}

// RunWorkflowPresetAdminHandler handles POST /-/workflow-presets/{slug}/run for
// on-demand (one-off) workflows. Unlike a recurring preset — which is
// explicitly "set up" once, then runs on its trigger — a one-off has no
// recurring trigger, so this single endpoint does the whole gesture: enforce
// the household consent gate, instantiate the manual-only workflow on first use
// (reusing it thereafter), then dispatch a run via the orchestrator. Mirrors
// RunAgentNowAdminHandler's async semantics and error envelope.
func RunWorkflowPresetAdminHandler(a *app.App, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")

		// First-use consent gate: instantiating a workflow authorizes AI spend
		// over the household's ledger. Until acknowledged, refuse with a code
		// the gallery catches to route the user through the setup drawer (which
		// carries the consent checkbox). Mirrors EnableWorkflowPresetAdminHandler.
		if !svc.WorkflowsConsentAcknowledged(r.Context()) {
			writeError(w, http.StatusConflict, "CONSENT_REQUIRED",
				"Acknowledge that workflows run Claude over your financial data before running.")
			return
		}

		orchestrator := a.AgentOrchestrator
		if orchestrator == nil {
			writeError(w, http.StatusServiceUnavailable, "AGENTS_DISABLED", "Agent orchestrator not configured")
			return
		}

		def, err := svc.EnsureOneOffWorkflow(r.Context(), slug)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrNotFound):
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Workflow preset not found")
			case errors.Is(err, service.ErrInvalidParameter):
				writeError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			default:
				writeError(w, http.StatusUnprocessableEntity, "WORKFLOW_ENABLE_FAILED", err.Error())
			}
			return
		}

		run, err := orchestrator.RunNowAsyncWith(r.Context(), def, service.RunOverrides{})
		if err != nil {
			if errors.Is(err, agent.ErrConcurrencyLocked) {
				writeError(w, http.StatusServiceUnavailable, "CONCURRENCY_LOCKED", "Another run is in progress")
				return
			}
			if errors.Is(err, service.ErrBudgetCeilingReached) {
				writeError(w, http.StatusTooManyRequests, "BUDGET_CEILING_REACHED", err.Error())
				return
			}
			if errors.Is(err, agent.ErrAuthNotConfigured) {
				writeError(w, http.StatusUnprocessableEntity, "AUTH_NOT_CONFIGURED", err.Error())
				return
			}
			if errors.Is(err, agent.ErrBinaryNotFound) {
				writeError(w, http.StatusUnprocessableEntity, "BINARY_NOT_FOUND", err.Error())
				return
			}
			writeError(w, http.StatusUnprocessableEntity, "RUN_FAILED", err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, run)
	}
}

// CancelWorkflowRunAdminHandler handles POST /-/workflows/runs/{shortId}/cancel.
// It aborts an in-progress run by asking the orchestrator to cancel that run's
// context — which SIGKILLs the sidecar process group (see internal/agent/
// sidecar.go). The run goroutine then persists a terminal 'cancelled' status,
// which the run-detail live poller picks up on its next refresh. Editor-level,
// mirroring the run/enable/disable surface (managing an existing run, not
// authorizing new recurring spend).
func CancelWorkflowRunAdminHandler(a *app.App, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		shortID := chi.URLParam(r, "shortId")
		orchestrator := a.AgentOrchestrator
		if orchestrator == nil {
			writeError(w, http.StatusServiceUnavailable, "AGENTS_DISABLED", "Agent orchestrator not configured")
			return
		}
		run, err := svc.GetAgentRun(r.Context(), shortID)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Run not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load run")
			return
		}
		if run.Status != "in_progress" {
			writeError(w, http.StatusConflict, "INVALID_STATE", "Run is not in progress")
			return
		}
		if !orchestrator.CancelRun(shortID) {
			// Not in this process's in-flight set — it just finished, or is owned
			// by another instance. Nothing to abort here.
			writeError(w, http.StatusConflict, "NOT_CANCELLABLE", "Run is no longer cancellable")
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "cancelling"})
	}
}

// WorkflowRunStatusAdminHandler handles GET /-/workflows/runs/{shortId}/status —
// a lightweight JSON status poll for the gallery's one-off Run button, which
// keeps its spinner up for the whole run (not just the async dispatch). Returns
// just short_id + status so the poll stays cheap (no transcript parsing, unlike
// the run-detail /live fragment).
func WorkflowRunStatusAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		shortID := chi.URLParam(r, "shortId")
		run, err := svc.GetAgentRun(r.Context(), shortID)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Run not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load run")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"short_id": run.ShortID,
			"status":   run.Status,
		})
	}
}

// UpdateAgentRunNoteAdminHandler handles POST /-/agents/runs/{shortID}/note —
// form-data, redirects back to the run detail page.
func UpdateAgentRunNoteAdminHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		shortID := chi.URLParam(r, "shortId")
		back := "/agents/runs/" + shortID

		if err := r.ParseForm(); err != nil {
			FlashRedirect(w, r, sm, "error", "Invalid form data.", back)
			return
		}
		note := r.FormValue("note")
		if len(note) > 2000 {
			FlashRedirect(w, r, sm, "error", "Note must be 2000 characters or fewer.", back)
			return
		}

		if _, err := svc.SetAgentRunNote(r.Context(), shortID, note); err != nil {
			FlashRedirect(w, r, sm, "error", "Failed to save note: "+err.Error(), back)
			return
		}
		SetFlash(r.Context(), sm, "success", "Note saved.")
		http.Redirect(w, r, back, http.StatusSeeOther)
	}
}

// UpdateAgentSDKSettingsAdminHandler handles POST /-/agents/settings —
// admin-only. Empty token fields preserve the existing value (per the
// form's "Leave blank to keep current value" affordance).
func UpdateAgentSDKSettingsAdminHandler(a *app.App, svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			FlashRedirect(w, r, sm, "error", "Invalid form data.", "/settings/workflows")
			return
		}

		params := service.UpdateAgentSettingsParams{}

		if v := strings.TrimSpace(r.FormValue("auth_mode")); v != "" {
			params.AuthMode = &v
		}
		// Empty token submit = leave existing value alone. Non-empty = update.
		if v := strings.TrimSpace(r.FormValue("subscription_token")); v != "" {
			params.SubscriptionToken = &v
		}
		if v := strings.TrimSpace(r.FormValue("anthropic_api_key")); v != "" {
			params.AnthropicAPIKey = &v
		}
		if v := strings.TrimSpace(r.FormValue("runtime_path")); v != "" {
			params.RuntimePath = &v
		} else if r.Form.Has("runtime_path") {
			empty := ""
			params.RuntimePath = &empty
		}
		if v := strings.TrimSpace(r.FormValue("transcript_dir")); v != "" {
			params.TranscriptDir = &v
		} else if r.Form.Has("transcript_dir") {
			empty := ""
			params.TranscriptDir = &empty
		}
		if v := strings.TrimSpace(r.FormValue("max_concurrent")); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 50 {
				FlashRedirect(w, r, sm, "error", "Max concurrent must be a whole number between 1 and 50.", "/settings/workflows")
				return
			}
			params.MaxConcurrent = &n
		}
		if v := strings.TrimSpace(r.FormValue("global_max_budget_usd")); v != "" {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil || f < 0 {
				FlashRedirect(w, r, sm, "error", "Global max budget must be a non-negative number.", "/settings/workflows")
				return
			}
			params.GlobalMaxBudgetUSD = &f
		} else if r.Form.Has("global_max_budget_usd") {
			zero := 0.0
			params.GlobalMaxBudgetUSD = &zero
		}
		if _, err := svc.UpdateAgentSettings(r.Context(), params, a.Config.EncryptionKey, a.Config.DataDir); err != nil {
			FlashRedirect(w, r, sm, "error", "Failed to save settings: "+err.Error(), "/settings/workflows")
			return
		}
		SetFlash(r.Context(), sm, "success", "Agent settings saved.")
		http.Redirect(w, r, "/settings/workflows", http.StatusSeeOther)
	}
}

// NotifyTestAdminHandler handles POST /-/agents/notify-test — admin-only.
// Fires a sample notification to the configured webhook so the operator can
// verify wiring. Returns JSON {ok} or {ok:false, error} (rendered inline via
// Alpine). 422 when unconfigured or the webhook errors.
func NotifyTestAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.SendTestNotification(r.Context()); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// SmokeTestAgentAdminHandler handles POST /-/agents/test — admin-only.
// Returns JSON. The settings page renders the result inline via Alpine.
func SmokeTestAgentAdminHandler(a *app.App, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orchestrator := a.AgentOrchestrator
		if orchestrator == nil {
			writeError(w, http.StatusServiceUnavailable, "AGENTS_DISABLED", "Agent orchestrator not configured")
			return
		}
		result, err := orchestrator.SmokeTest(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"ok":    false,
				"error": err.Error(),
			})
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// AgentCleanupAdminHandler handles POST /-/agents/cleanup — admin-only.
// Forces an immediate retention pass and returns counts as JSON.
func AgentCleanupAdminHandler(a *app.App, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scheduler := a.AgentScheduler
		if scheduler == nil {
			writeError(w, http.StatusServiceUnavailable, "AGENTS_DISABLED", "Agent scheduler not configured")
			return
		}
		result := scheduler.RunCleanupNow(r.Context())
		writeJSON(w, http.StatusOK, result)
	}
}

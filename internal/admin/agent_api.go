//go:build !headless && !lite

package admin

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"breadbox/internal/agent"
	"breadbox/internal/app"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// CreateAgentDefinitionAdminHandler handles POST /-/agents — admin form submit
// from /agents/new. Redirects to /agents on success, back to /agents/new with
// flash on validation failure.
func CreateAgentDefinitionAdminHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			FlashRedirect(w, r, sm, "error", "Invalid form data.", "/agents/new")
			return
		}

		params, err := parseAgentDefinitionForm(r)
		if err != nil {
			FlashRedirect(w, r, sm, "error", err.Error(), "/agents/new")
			return
		}

		def, err := svc.CreateAgentDefinition(r.Context(), params)
		if err != nil {
			FlashRedirect(w, r, sm, "error", "Failed to create agent: "+err.Error(), "/agents/new")
			return
		}

		SetFlash(r.Context(), sm, "success", fmt.Sprintf("Created agent %q.", def.Name))
		http.Redirect(w, r, "/agents/"+def.Slug+"/edit", http.StatusSeeOther)
	}
}

// UpdateAgentDefinitionAdminHandler handles POST /-/agents/{slug}/update —
// admin form submit from /agents/{slug}/edit. PATCH-style: only fields present
// in the form payload are updated. Enabled is a checkbox so we always send it.
func UpdateAgentDefinitionAdminHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		back := "/agents/" + slug + "/edit"

		if err := r.ParseForm(); err != nil {
			FlashRedirect(w, r, sm, "error", "Invalid form data.", back)
			return
		}

		params, err := parseAgentDefinitionUpdateForm(r)
		if err != nil {
			FlashRedirect(w, r, sm, "error", err.Error(), back)
			return
		}

		def, err := svc.UpdateAgentDefinition(r.Context(), slug, params)
		if err != nil {
			FlashRedirect(w, r, sm, "error", "Failed to update agent: "+err.Error(), back)
			return
		}

		SetFlash(r.Context(), sm, "success", fmt.Sprintf("Saved agent %q.", def.Name))
		http.Redirect(w, r, "/agents/"+def.Slug+"/edit", http.StatusSeeOther)
	}
}

// DeleteAgentDefinitionAdminHandler handles POST /-/agents/{slug}/delete —
// admin form submit. Returns to /agents.
func DeleteAgentDefinitionAdminHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")

		if err := svc.DeleteAgentDefinition(r.Context(), slug); err != nil {
			FlashRedirect(w, r, sm, "error", "Failed to delete agent: "+err.Error(), "/agents/"+slug+"/edit")
			return
		}

		SetFlash(r.Context(), sm, "success", "Deleted agent.")
		http.Redirect(w, r, "/agents", http.StatusSeeOther)
	}
}

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

// parseAgentDefinitionForm pulls all create-form fields into the service
// param struct. Returns a friendly error string for the first invalid field.
func parseAgentDefinitionForm(r *http.Request) (service.CreateAgentDefinitionParams, error) {
	p := service.CreateAgentDefinitionParams{}
	p.Name = strings.TrimSpace(r.FormValue("name"))
	p.Slug = strings.TrimSpace(r.FormValue("slug"))
	p.Prompt = r.FormValue("prompt")
	p.Model = strings.TrimSpace(r.FormValue("model"))
	p.ToolScope = strings.TrimSpace(r.FormValue("tool_scope"))
	p.Enabled = r.FormValue("enabled") == "on" || r.FormValue("enabled") == "true"
	p.TriggerOnSyncComplete = r.FormValue("trigger_on_sync_complete") == "on" || r.FormValue("trigger_on_sync_complete") == "true"

	if v := strings.TrimSpace(r.FormValue("system_prompt")); v != "" {
		p.SystemPrompt = &v
	}
	if v := strings.TrimSpace(r.FormValue("schedule_cron")); v != "" {
		p.ScheduleCron = &v
	}
	if v := strings.TrimSpace(r.FormValue("quiet_hours_start")); v != "" {
		p.QuietHoursStart = &v
	}
	if v := strings.TrimSpace(r.FormValue("quiet_hours_end")); v != "" {
		p.QuietHoursEnd = &v
	}
	if v := strings.TrimSpace(r.FormValue("allowed_tools")); v != "" {
		p.AllowedTools = splitAllowedTools(v)
	}
	if v := strings.TrimSpace(r.FormValue("max_turns")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			return p, fmt.Errorf("max_turns must be a whole number between 1 and 200")
		}
		p.MaxTurns = n
	}
	if v := strings.TrimSpace(r.FormValue("max_budget_usd")); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 0 {
			return p, fmt.Errorf("max_budget_usd must be a non-negative number")
		}
		p.MaxBudgetUSD = &f
	}

	if p.Name == "" {
		return p, fmt.Errorf("name is required")
	}
	if p.Slug == "" {
		return p, fmt.Errorf("slug is required")
	}
	if strings.TrimSpace(p.Prompt) == "" {
		return p, fmt.Errorf("prompt is required")
	}

	return p, nil
}

// parseAgentDefinitionUpdateForm builds an UpdateAgentDefinitionParams
// where every populated field is set. Empty fields are sent as nil to
// preserve the "don't touch" semantic of the PATCH endpoint.
func parseAgentDefinitionUpdateForm(r *http.Request) (service.UpdateAgentDefinitionParams, error) {
	p := service.UpdateAgentDefinitionParams{}

	if v := strings.TrimSpace(r.FormValue("name")); v != "" {
		p.Name = &v
	}
	if v := strings.TrimSpace(r.FormValue("slug")); v != "" {
		p.Slug = &v
	}
	if r.Form.Has("prompt") {
		v := r.FormValue("prompt")
		p.Prompt = &v
	}
	if r.Form.Has("system_prompt") {
		v := strings.TrimSpace(r.FormValue("system_prompt"))
		p.SystemPrompt = &v
	}
	if r.Form.Has("schedule_cron") {
		v := strings.TrimSpace(r.FormValue("schedule_cron"))
		p.ScheduleCron = &v
	}
	if r.Form.Has("quiet_hours_start") {
		v := strings.TrimSpace(r.FormValue("quiet_hours_start"))
		p.QuietHoursStart = &v
	}
	if r.Form.Has("quiet_hours_end") {
		v := strings.TrimSpace(r.FormValue("quiet_hours_end"))
		p.QuietHoursEnd = &v
	}
	if r.Form.Has("allowed_tools") {
		tools := splitAllowedTools(r.FormValue("allowed_tools"))
		p.AllowedTools = &tools
	}
	if v := strings.TrimSpace(r.FormValue("model")); v != "" {
		p.Model = &v
	}
	if v := strings.TrimSpace(r.FormValue("tool_scope")); v != "" {
		p.ToolScope = &v
	}
	if v := strings.TrimSpace(r.FormValue("max_turns")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			return p, fmt.Errorf("max_turns must be a whole number between 1 and 200")
		}
		p.MaxTurns = &n
	}
	if v := strings.TrimSpace(r.FormValue("max_budget_usd")); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 0 {
			return p, fmt.Errorf("max_budget_usd must be a non-negative number")
		}
		p.MaxBudgetUSD = &f
	} else if r.Form.Has("max_budget_usd") {
		zero := 0.0
		p.MaxBudgetUSD = &zero
	}

	enabled := r.FormValue("enabled") == "on" || r.FormValue("enabled") == "true"
	p.Enabled = &enabled
	triggerOnSync := r.FormValue("trigger_on_sync_complete") == "on" || r.FormValue("trigger_on_sync_complete") == "true"
	p.TriggerOnSyncComplete = &triggerOnSync

	return p, nil
}

func splitAllowedTools(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

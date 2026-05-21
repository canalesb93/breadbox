//go:build !headless && !lite

package webapp

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"breadbox/internal/appconfig"
	"breadbox/internal/service"
	"breadbox/internal/webapp/pages"
)

// agentSlugFormat mirrors the service-layer kebab-case slug rule
// (lowercase letters, digits, dashes; must start/end alphanumeric).
var agentSlugFormat = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$`)

// agentHHMMFormat mirrors the service quiet-hours format check.
var agentHHMMFormat = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// registerAgents wires the agents + runs routes onto the authenticated subrouter.
// Static routes (/agents/new, /agents/runs, /agents/settings) are registered
// before the dynamic /agents/{slug}/* routes so chi resolves them unambiguously.
func (h *Handler) registerAgents(r chi.Router) {
	r.Get("/agents", h.agentsList)
	r.Post("/agents", h.requireSameOrigin(h.createAgent))
	r.Get("/agents/new", h.newAgent)
	r.Get("/agents/runs", h.allAgentRuns)
	r.Get("/agents/settings", h.agentSettings)
	r.Post("/agents/settings", h.requireSameOrigin(h.updateAgentSettings))
	r.Get("/agents/{slug}/runs", h.agentRuns)
	h.registerAgentStream(r) // /agents/{slug}/runs/{shortId} + /stream (SSE)
	r.Get("/agents/{slug}/edit", h.editAgent)
	r.Post("/agents/{slug}", h.requireSameOrigin(h.updateAgent))
}

// agentsList renders all agent definitions as cards.
func (h *Handler) agentsList(w http.ResponseWriter, r *http.Request) {
	agents, err := h.app.Service.ListAgentDefinitions(r.Context())
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	render(w, r, http.StatusOK, pages.AgentsList(h.shellData(r, "Agents"), agents))
}

// agentRuns renders the run history for one agent definition.
func (h *Handler) agentRuns(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	agent, err := h.app.Service.GetAgentDefinition(r.Context(), slug)
	if errors.Is(err, service.ErrNotFound) {
		h.notFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	result, err := h.app.Service.ListAgentRuns(r.Context(), slug, service.AgentRunListParams{Limit: 100})
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	render(w, r, http.StatusOK, pages.AgentRuns(h.shellData(r, agent.Name+" runs"), agent, result.Runs))
}

// allAgentRuns renders the cross-agent run history.
func (h *Handler) allAgentRuns(w http.ResponseWriter, r *http.Request) {
	result, err := h.app.Service.ListAllAgentRuns(r.Context(), service.AllAgentRunListParams{Limit: 100})
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	render(w, r, http.StatusOK, pages.AllAgentRuns(h.shellData(r, "All runs"), result.Runs))
}

// ----------------------------------------------------------------------------
// Create / edit agent definition
// ----------------------------------------------------------------------------

// newAgent renders the empty create form with sensible defaults pre-filled.
func (h *Handler) newAgent(w http.ResponseWriter, r *http.Request) {
	render(w, r, http.StatusOK, pages.AgentForm(h.shellData(r, "New agent"), pages.AgentFormData{
		Mode:      "create",
		ActionURL: "/app/agents",
		CancelURL: "/app/agents",
		Values: pages.AgentFormValues{
			ToolScope: "read_write",
			Model:     service.DefaultAgentModel,
			Enabled:   true,
		},
		Errors: map[string]string{},
	}))
}

// createAgent validates and creates an agent definition. On validation failure
// it re-renders the form with field errors at HTTP 422; on success it 303s to
// the new agent's runs page.
func (h *Handler) createAgent(w http.ResponseWriter, r *http.Request) {
	vals := readAgentFormValues(r)
	fieldErrs := validateAgentForm(vals)
	if len(fieldErrs) > 0 {
		h.rerenderAgentForm(w, r, "create", "/app/agents", "/app/agents", vals, fieldErrs)
		return
	}

	params := service.CreateAgentDefinitionParams{
		Name:                  vals.Name,
		Slug:                  vals.Slug,
		Prompt:                vals.Prompt,
		SystemPrompt:          optPtr(vals.SystemPrompt),
		ScheduleCron:          optPtr(vals.ScheduleCron),
		ToolScope:             vals.ToolScope,
		Model:                 vals.Model,
		MaxTurns:              atoiOrZero(vals.MaxTurns),
		MaxBudgetUSD:          parseOptFloat(vals.MaxBudgetUSD),
		Enabled:               vals.Enabled,
		QuietHoursStart:       optPtr(vals.QuietHoursStart),
		QuietHoursEnd:         optPtr(vals.QuietHoursEnd),
		TriggerOnSyncComplete: vals.TriggerOnSyncComplete,
	}

	def, err := h.app.Service.CreateAgentDefinition(r.Context(), params)
	if err != nil {
		if isAgentSlugConflict(err) {
			h.rerenderAgentForm(w, r, "create", "/app/agents", "/app/agents", vals,
				map[string]string{"slug": "An agent with this slug already exists."})
			return
		}
		if errors.Is(err, service.ErrInvalidParameter) {
			h.rerenderAgentForm(w, r, "create", "/app/agents", "/app/agents", vals,
				map[string]string{"form": cleanServiceError(err)})
			return
		}
		h.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/app/agents/"+def.Slug+"/runs", http.StatusSeeOther)
}

// editAgent prefills the form for an existing agent. Slug is read-only on edit.
func (h *Handler) editAgent(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	def, err := h.app.Service.GetAgentDefinition(r.Context(), slug)
	if errors.Is(err, service.ErrNotFound) {
		h.notFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	render(w, r, http.StatusOK, pages.AgentForm(h.shellData(r, "Edit agent"), pages.AgentFormData{
		Mode:      "edit",
		ActionURL: "/app/agents/" + def.Slug,
		CancelURL: "/app/agents/" + def.Slug + "/runs",
		Values:    agentFormValuesFromDef(def),
		Errors:    map[string]string{},
	}))
}

// updateAgent validates and updates an agent definition. Re-renders with errors
// + 422 on failure; 303s to the runs page on success. Slug is immutable here.
func (h *Handler) updateAgent(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	def, err := h.app.Service.GetAgentDefinition(r.Context(), slug)
	if errors.Is(err, service.ErrNotFound) {
		h.notFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	vals := readAgentFormValues(r)
	vals.Slug = def.Slug // immutable on edit

	fieldErrs := validateAgentForm(vals)
	delete(fieldErrs, "slug") // slug is pinned; never a user error on edit
	if len(fieldErrs) > 0 {
		h.rerenderAgentForm(w, r, "edit", "/app/agents/"+def.Slug, "/app/agents/"+def.Slug+"/runs", vals, fieldErrs)
		return
	}

	maxTurns := atoiOrZero(vals.MaxTurns)
	scope := vals.ToolScope
	params := service.UpdateAgentDefinitionParams{
		Name:                  &vals.Name,
		Prompt:                &vals.Prompt,
		SystemPrompt:          ptrEmptyToClear(vals.SystemPrompt),
		ScheduleCron:          ptrEmptyToClear(vals.ScheduleCron),
		ToolScope:             &scope,
		Model:                 &vals.Model,
		MaxTurns:              &maxTurns,
		MaxBudgetUSD:          parseOptFloat(vals.MaxBudgetUSD),
		Enabled:               &vals.Enabled,
		QuietHoursStart:       ptrEmptyToClear(vals.QuietHoursStart),
		QuietHoursEnd:         ptrEmptyToClear(vals.QuietHoursEnd),
		TriggerOnSyncComplete: &vals.TriggerOnSyncComplete,
	}

	if _, err := h.app.Service.UpdateAgentDefinition(r.Context(), def.Slug, params); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			h.notFound(w, r)
			return
		}
		if errors.Is(err, service.ErrInvalidParameter) {
			h.rerenderAgentForm(w, r, "edit", "/app/agents/"+def.Slug, "/app/agents/"+def.Slug+"/runs", vals,
				map[string]string{"form": cleanServiceError(err)})
			return
		}
		h.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/app/agents/"+def.Slug+"/runs", http.StatusSeeOther)
}

// rerenderAgentForm re-renders the create/edit form with errors at HTTP 422.
func (h *Handler) rerenderAgentForm(w http.ResponseWriter, r *http.Request, mode, action, cancel string, vals pages.AgentFormValues, fieldErrs map[string]string) {
	title := "New agent"
	if mode == "edit" {
		title = "Edit agent"
	}
	render(w, r, http.StatusUnprocessableEntity, pages.AgentForm(h.shellData(r, title), pages.AgentFormData{
		Mode:      mode,
		ActionURL: action,
		CancelURL: cancel,
		Values:    vals,
		Errors:    fieldErrs,
	}))
}

// ----------------------------------------------------------------------------
// Agent settings (token storage)
// ----------------------------------------------------------------------------

// agentSettings renders the settings form. Token fields show only the masked
// preview from the service — plaintext never leaves the server.
func (h *Handler) agentSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.app.Service.GetAgentSettings(r.Context(), h.app.Config.EncryptionKey)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	render(w, r, http.StatusOK, pages.AgentSettings(h.shellData(r, "Agent settings"), pages.AgentSettingsFormData{
		ActionURL: "/app/agents/settings",
		CancelURL: "/app/agents",
		Values:    agentSettingsValuesFromResponse(settings),
		Errors:    map[string]string{},
	}))
}

// updateAgentSettings validates and persists agent settings. Empty token fields
// mean "keep existing" — we only send a non-nil token pointer when the operator
// typed a new value, so the service's empty-clears-it path is never reached for
// an untouched field.
func (h *Handler) updateAgentSettings(w http.ResponseWriter, r *http.Request) {
	authMode := strings.TrimSpace(r.FormValue("auth_mode"))
	subToken := strings.TrimSpace(r.FormValue("subscription_token"))
	apiKey := strings.TrimSpace(r.FormValue("anthropic_api_key"))
	maxConcurrentRaw := strings.TrimSpace(r.FormValue("max_concurrent"))
	budgetRaw := strings.TrimSpace(r.FormValue("global_max_budget_usd"))
	runtimePath := strings.TrimSpace(r.FormValue("runtime_path"))
	transcriptDir := strings.TrimSpace(r.FormValue("transcript_dir"))

	fieldErrs := map[string]string{}
	if authMode != appconfig.AuthModeSubscription && authMode != appconfig.AuthModeAPIKey {
		fieldErrs["auth_mode"] = "Choose subscription or API key."
	}
	var maxConcurrent *int
	if maxConcurrentRaw != "" {
		n, err := strconv.Atoi(maxConcurrentRaw)
		if err != nil || n < 1 || n > 50 {
			fieldErrs["max_concurrent"] = "Must be a whole number between 1 and 50."
		} else {
			maxConcurrent = &n
		}
	}
	var budget *float64
	if budgetRaw != "" {
		f, err := strconv.ParseFloat(budgetRaw, 64)
		if err != nil || f < 0 || f > 1000 {
			fieldErrs["global_max_budget_usd"] = "Must be a number between 0 and 1000."
		} else {
			budget = &f
		}
	}

	encKey := h.app.Config.EncryptionKey

	if len(fieldErrs) > 0 {
		h.rerenderAgentSettings(w, r, fieldErrs)
		return
	}

	params := service.UpdateAgentSettingsParams{
		AuthMode:           &authMode,
		MaxConcurrent:      maxConcurrent,
		GlobalMaxBudgetUSD: budget,
		RuntimePath:        &runtimePath,
		TranscriptDir:      &transcriptDir,
	}
	// Empty = keep existing: only set the token pointer when the operator typed one.
	if subToken != "" {
		params.SubscriptionToken = &subToken
	}
	if apiKey != "" {
		params.AnthropicAPIKey = &apiKey
	}

	if _, err := h.app.Service.UpdateAgentSettings(r.Context(), params, encKey); err != nil {
		if errors.Is(err, service.ErrInvalidParameter) {
			h.rerenderAgentSettings(w, r, map[string]string{"form": cleanServiceError(err)})
			return
		}
		h.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/app/agents/settings", http.StatusSeeOther)
}

// rerenderAgentSettings re-reads the current masked state and re-renders the
// form with errors at HTTP 422. It re-fetches so token previews stay accurate
// (we never echo submitted secrets).
func (h *Handler) rerenderAgentSettings(w http.ResponseWriter, r *http.Request, fieldErrs map[string]string) {
	settings, err := h.app.Service.GetAgentSettings(r.Context(), h.app.Config.EncryptionKey)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	vals := agentSettingsValuesFromResponse(settings)
	// Preserve the operator's non-secret edits on re-render.
	if v := strings.TrimSpace(r.FormValue("auth_mode")); v != "" {
		vals.AuthMode = v
	}
	vals.MaxConcurrent = strings.TrimSpace(r.FormValue("max_concurrent"))
	vals.GlobalMaxBudgetUSD = strings.TrimSpace(r.FormValue("global_max_budget_usd"))
	vals.RuntimePath = strings.TrimSpace(r.FormValue("runtime_path"))
	vals.TranscriptDir = strings.TrimSpace(r.FormValue("transcript_dir"))
	render(w, r, http.StatusUnprocessableEntity, pages.AgentSettings(h.shellData(r, "Agent settings"), pages.AgentSettingsFormData{
		ActionURL: "/app/agents/settings",
		CancelURL: "/app/agents",
		Values:    vals,
		Errors:    fieldErrs,
	}))
}

// ----------------------------------------------------------------------------
// Form helpers
// ----------------------------------------------------------------------------

// readAgentFormValues pulls the agent definition form fields off the request.
func readAgentFormValues(r *http.Request) pages.AgentFormValues {
	return pages.AgentFormValues{
		Name:                  strings.TrimSpace(r.FormValue("name")),
		Slug:                  strings.TrimSpace(r.FormValue("slug")),
		Prompt:                strings.TrimSpace(r.FormValue("prompt")),
		SystemPrompt:          strings.TrimSpace(r.FormValue("system_prompt")),
		ScheduleCron:          strings.TrimSpace(r.FormValue("schedule_cron")),
		ToolScope:             strings.TrimSpace(r.FormValue("tool_scope")),
		Model:                 strings.TrimSpace(r.FormValue("model")),
		MaxTurns:              strings.TrimSpace(r.FormValue("max_turns")),
		MaxBudgetUSD:          strings.TrimSpace(r.FormValue("max_budget_usd")),
		QuietHoursStart:       strings.TrimSpace(r.FormValue("quiet_hours_start")),
		QuietHoursEnd:         strings.TrimSpace(r.FormValue("quiet_hours_end")),
		Enabled:               r.FormValue("enabled") != "",
		TriggerOnSyncComplete: r.FormValue("trigger_on_sync_complete") != "",
	}
}

// agentFormValuesFromDef maps a definition response into editable form values.
func agentFormValuesFromDef(d *service.AgentDefinitionResponse) pages.AgentFormValues {
	v := pages.AgentFormValues{
		Name:                  d.Name,
		Slug:                  d.Slug,
		Prompt:                d.Prompt,
		SystemPrompt:          pages.Deref(d.SystemPrompt, ""),
		ScheduleCron:          pages.Deref(d.ScheduleCron, ""),
		ToolScope:             d.ToolScope,
		Model:                 d.Model,
		MaxTurns:              strconv.Itoa(d.MaxTurns),
		QuietHoursStart:       pages.Deref(d.QuietHoursStart, ""),
		QuietHoursEnd:         pages.Deref(d.QuietHoursEnd, ""),
		Enabled:               d.Enabled,
		TriggerOnSyncComplete: d.TriggerOnSyncComplete,
	}
	if d.MaxBudgetUSD != nil {
		v.MaxBudgetUSD = strconv.FormatFloat(*d.MaxBudgetUSD, 'f', -1, 64)
	}
	return v
}

// agentSettingsValuesFromResponse maps the masked settings response into form
// values. Token MASKS are display-only; the input fields stay blank.
func agentSettingsValuesFromResponse(s *service.AgentSettingsResponse) pages.AgentSettingsFormValues {
	v := pages.AgentSettingsFormValues{
		AuthMode:              s.AuthMode,
		SubscriptionTokenMask: pages.Deref(s.SubscriptionToken, ""),
		AnthropicAPIKeyMask:   pages.Deref(s.AnthropicAPIKey, ""),
		MaxConcurrent:         strconv.Itoa(s.MaxConcurrent),
		RuntimePath:           s.RuntimePath,
		TranscriptDir:         s.TranscriptDir,
	}
	if s.GlobalMaxBudgetUSD != nil {
		v.GlobalMaxBudgetUSD = strconv.FormatFloat(*s.GlobalMaxBudgetUSD, 'f', -1, 64)
	}
	return v
}

// validateAgentForm runs server-side validation mirroring the service rules so
// the form can surface per-field errors before hitting the service layer.
func validateAgentForm(v pages.AgentFormValues) map[string]string {
	errs := map[string]string{}
	if v.Name == "" {
		errs["name"] = "Name is required."
	}
	if !agentSlugFormat.MatchString(v.Slug) {
		errs["slug"] = "Use kebab-case: lowercase letters, digits, and dashes (2-64 chars)."
	}
	if v.Prompt == "" {
		errs["prompt"] = "Prompt is required."
	}
	if v.ToolScope != "read_only" && v.ToolScope != "read_write" {
		errs["tool_scope"] = "Choose read only or read & write."
	}
	if v.MaxTurns != "" {
		n, err := strconv.Atoi(v.MaxTurns)
		if err != nil || n < 0 || n > 100 {
			errs["max_turns"] = "Must be a whole number between 0 and 100 (0 uses the default)."
		}
	}
	if v.MaxBudgetUSD != "" {
		f, err := strconv.ParseFloat(v.MaxBudgetUSD, 64)
		if err != nil || f < 0 || f > 1000 {
			errs["max_budget_usd"] = "Must be a number between 0 and 1000."
		}
	}
	if v.ScheduleCron != "" && len(strings.Fields(v.ScheduleCron)) != 5 {
		errs["schedule_cron"] = "Cron must have exactly 5 space-separated fields."
	}
	// Quiet hours: both or neither, each HH:MM.
	startSet, endSet := v.QuietHoursStart != "", v.QuietHoursEnd != ""
	if startSet != endSet {
		errs["quiet_hours_start"] = "Set both start and end, or leave both blank."
	}
	if startSet && !agentHHMMFormat.MatchString(v.QuietHoursStart) {
		errs["quiet_hours_start"] = "Use HH:MM (24-hour)."
	}
	if endSet && !agentHHMMFormat.MatchString(v.QuietHoursEnd) {
		errs["quiet_hours_end"] = "Use HH:MM (24-hour)."
	}
	return errs
}

// isAgentSlugConflict detects the unique-constraint violation surfaced when a
// duplicate slug is created (the slug column is the only unique field).
func isAgentSlugConflict(err error) bool {
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "duplicate key") || strings.Contains(lower, "unique constraint")
}

// cleanServiceError strips the "invalid parameter: " sentinel prefix so the
// remaining human-readable message can render in the form banner.
func cleanServiceError(err error) string {
	msg := err.Error()
	if i := strings.Index(msg, ": "); i >= 0 && i+2 < len(msg) {
		rest := msg[i+2:]
		return strings.ToUpper(rest[:1]) + rest[1:]
	}
	return msg
}

// atoiOrZero parses an int form value, returning 0 (the service's "use default"
// sentinel for max_turns) when blank or invalid.
func atoiOrZero(s string) int {
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// parseOptFloat returns a pointer to the parsed float, nil when blank/invalid
// (the service then applies its own default).
func parseOptFloat(s string) *float64 {
	if s == "" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}

// ptrEmptyToClear always returns a non-nil pointer (PATCH "replace"). An empty
// string clears the column — used for optional fields (system prompt, schedule,
// quiet hours) where a blank form field genuinely means "remove this value".
func ptrEmptyToClear(s string) *string {
	return &s
}

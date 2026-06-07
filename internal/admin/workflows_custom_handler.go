//go:build !headless && !lite

package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"breadbox/internal/avatar"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// Custom workflows are hand-authored agent definitions (source_template IS
// NULL): the operator writes the entire prompt themselves, rather than
// instantiating a code-defined preset. They're created and edited from the
// shared custom-workflow drawer on /workflows, which speaks the same field
// vocabulary as the preset drawers (trigger_on_sync, schedule_cron, model,
// max_turns, max_budget_usd) plus the custom-only `prompt` and `tool_scope`.

// customWorkflowInput is the parsed drawer payload, shared by create + edit.
type customWorkflowInput struct {
	name          string
	prompt        string
	model         string
	toolScope     string
	triggerOnSync bool
	scheduleCron  string   // empty = manual (no automatic trigger)
	maxTurns      int      // 0 = unset (leave default / untouched)
	maxBudget     *float64 // nil = unset
	enabled       bool     // create only — edit manages run-state via the card toggle
	avatarSeed    string   // empty = slug-seeded default
}

// readCustomWorkflowInput pulls the drawer fields out of the form, validating
// the two required fields (name, prompt) and the numeric caps.
func readCustomWorkflowInput(r *http.Request) (customWorkflowInput, error) {
	in := customWorkflowInput{
		name:          strings.TrimSpace(r.FormValue("name")),
		prompt:        strings.TrimSpace(r.FormValue("prompt")),
		model:         strings.TrimSpace(r.FormValue("model")),
		toolScope:     strings.TrimSpace(r.FormValue("tool_scope")),
		triggerOnSync: r.FormValue("trigger_on_sync") == "true",
		scheduleCron:  strings.TrimSpace(r.FormValue("schedule_cron")),
		enabled:       r.FormValue("enabled") == "true" || r.FormValue("enabled") == "on",
		avatarSeed:    strings.TrimSpace(r.FormValue("avatar_seed")),
	}
	if in.name == "" {
		return in, fmt.Errorf("name is required")
	}
	if in.prompt == "" {
		return in, fmt.Errorf("prompt is required")
	}
	if in.avatarSeed != "" && !avatar.IsValidSeed(in.avatarSeed) {
		return in, fmt.Errorf("invalid avatar seed")
	}
	if v := strings.TrimSpace(r.FormValue("max_turns")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 1000 {
			return in, fmt.Errorf("max_turns must be a whole number between 0 and 1000")
		}
		in.maxTurns = n
	}
	if v := strings.TrimSpace(r.FormValue("max_budget_usd")); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 0 {
			return in, fmt.Errorf("max_budget_usd must be a non-negative number")
		}
		in.maxBudget = &f
	}
	if in.toolScope != "read_only" {
		in.toolScope = "read_write"
	}
	return in, nil
}

// CreateCustomWorkflowAdminHandler handles POST /-/custom-workflows. It derives
// a unique slug from the name, then creates a hand-authored agent definition
// (source_template = NULL). Admin-only — creating a workflow authorizes
// recurring AI spend over household data, mirroring the preset-enable guard.
// Returns JSON {slug} for the gallery's async submit (which reloads on success).
func CreateCustomWorkflowAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		in, err := readCustomWorkflowInput(r)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "INVALID_PARAMETER", err.Error())
			return
		}
		slug, err := uniqueCustomWorkflowSlug(r.Context(), svc, slugifyWorkflowName(in.name))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "WORKFLOW_CREATE_FAILED", err.Error())
			return
		}
		params := service.CreateAgentDefinitionParams{
			Name:         in.name,
			Slug:         slug,
			Prompt:       in.prompt,
			Model:        in.model,
			ToolScope:    in.toolScope,
			MaxTurns:     in.maxTurns,
			MaxBudgetUSD: in.maxBudget,
			Enabled:      in.enabled,
			// SourceTemplate stays nil — this is a hand-authored workflow.
		}
		if in.triggerOnSync {
			params.TriggerOnSyncComplete = true
		} else if in.scheduleCron != "" {
			c := in.scheduleCron
			params.ScheduleCron = &c
		}
		def, err := svc.CreateAgentDefinition(r.Context(), params)
		if err != nil {
			if errors.Is(err, service.ErrInvalidParameter) {
				writeError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
				return
			}
			writeError(w, http.StatusUnprocessableEntity, "WORKFLOW_CREATE_FAILED", err.Error())
			return
		}
		// CreateAgentDefinitionParams carries no avatar seed, so persist a
		// chosen one in a follow-up update (best-effort: a failure here just
		// leaves the workflow slug-seeded, which is a valid default).
		if in.avatarSeed != "" {
			seed := in.avatarSeed
			_, _ = svc.UpdateAgentDefinition(r.Context(), def.Slug, service.UpdateAgentDefinitionParams{AvatarSeed: &seed})
		}
		writeJSON(w, http.StatusOK, map[string]any{"slug": def.Slug})
	}
}

// UpdateCustomWorkflowAdminHandler handles POST /-/custom-workflows/{slug}. It
// rewrites a hand-authored workflow's prompt, trigger, model, tool scope, and
// caps. Run-state (enabled) is untouched — that's the card toggle's job.
// Admin-only, mirroring the create + reconfigure guards.
func UpdateCustomWorkflowAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		def, err := svc.GetAgentDefinition(r.Context(), slug)
		if err != nil {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Workflow not found")
			return
		}
		if def.SourceTemplate != nil {
			// A preset-backed workflow is edited via the reconfigure drawer, not
			// this endpoint — refuse so the two paths can't cross-write.
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Not a custom workflow")
			return
		}
		_ = r.ParseForm()
		in, err := readCustomWorkflowInput(r)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "INVALID_PARAMETER", err.Error())
			return
		}
		name, prompt, model, scope := in.name, in.prompt, in.model, in.toolScope
		trigger := in.triggerOnSync
		// schedule_cron is always sent (a pointer to "" clears it → manual or
		// post-sync). The service nils an empty cron at the DB boundary.
		cron := ""
		if !trigger {
			cron = in.scheduleCron
		}
		params := service.UpdateAgentDefinitionParams{
			Name:                  &name,
			Prompt:                &prompt,
			Model:                 &model,
			ToolScope:             &scope,
			TriggerOnSyncComplete: &trigger,
			ScheduleCron:          &cron,
			// Empty seed clears back to slug-seeded (service emptyToNil).
			AvatarSeed: &in.avatarSeed,
		}
		if in.maxTurns > 0 {
			mt := in.maxTurns
			params.MaxTurns = &mt
		}
		if in.maxBudget != nil {
			params.MaxBudgetUSD = in.maxBudget
		}
		updated, err := svc.UpdateAgentDefinition(r.Context(), slug, params)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrNotFound):
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Workflow not found")
			case errors.Is(err, service.ErrInvalidParameter):
				writeError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			default:
				writeError(w, http.StatusUnprocessableEntity, "WORKFLOW_UPDATE_FAILED", err.Error())
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"slug": updated.Slug})
	}
}

// CustomWorkflowConfigAdminHandler handles GET /-/custom-workflows/{slug}. It
// returns a custom workflow's live config so the drawer can open prefilled for
// an edit. 404s for an unknown slug or a preset-backed workflow.
func CustomWorkflowConfigAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		def, err := svc.GetAgentDefinition(r.Context(), slug)
		if err != nil {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Workflow not found")
			return
		}
		if def.SourceTemplate != nil {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Not a custom workflow")
			return
		}
		cron := ""
		if def.ScheduleCron != nil {
			cron = *def.ScheduleCron
		}
		var budget float64
		if def.MaxBudgetUSD != nil {
			budget = *def.MaxBudgetUSD
		}
		var avatarSeed string
		if def.AvatarSeed != nil {
			avatarSeed = *def.AvatarSeed
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"name":            def.Name,
			"prompt":          def.Prompt,
			"tool_scope":      def.ToolScope,
			"model":           def.Model,
			"trigger_on_sync": def.TriggerOnSyncComplete,
			"schedule_cron":   cron,
			"max_turns":       def.MaxTurns,
			"max_budget_usd":  budget,
			"avatar_seed":     avatarSeed,
			"enabled":         def.Enabled,
		})
	}
}

// slugifyWorkflowName turns a display name into a URL/route-safe base slug:
// lowercased, non-alphanumerics collapsed to single dashes, trimmed, and
// capped to leave room for a de-collision suffix. Matches the
// service.validAgentSlug shape (^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$).
func slugifyWorkflowName(name string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		default:
			if b.Len() > 0 && !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 56 {
		s = strings.Trim(s[:56], "-")
	}
	if s == "" {
		s = "workflow"
	}
	return s
}

// uniqueCustomWorkflowSlug returns base, or base-2 / base-3 / … if base is
// already taken by an existing definition. The slug column is unique, so this
// avoids a create failure on a duplicate name.
func uniqueCustomWorkflowSlug(ctx context.Context, svc *service.Service, base string) (string, error) {
	defs, err := svc.ListAgentDefinitions(ctx)
	if err != nil {
		return "", err
	}
	taken := make(map[string]bool, len(defs))
	for _, d := range defs {
		taken[d.Slug] = true
	}
	if !taken[base] {
		return base, nil
	}
	for i := 2; i < 1000; i++ {
		cand := fmt.Sprintf("%s-%d", base, i)
		if !taken[cand] {
			return cand, nil
		}
	}
	return "", fmt.Errorf("could not allocate a unique slug for %q", base)
}

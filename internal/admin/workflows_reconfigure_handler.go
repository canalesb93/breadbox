//go:build !headless && !lite

package admin

import (
	"errors"
	"net/http"
	"strings"

	"breadbox/internal/avatar"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// WorkflowConfigAdminHandler handles GET /-/workflows/{slug}/config. It
// returns the live configuration of an already-enabled workflow (schedule,
// additional-instructions tail, chosen options) so the gallery can re-open
// the configure drawer prefilled for a reconfigure. Read-only — no consent
// gate. 404s when the slug isn't an enabled, preset-instantiated workflow.
func WorkflowConfigAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		cfg, err := svc.GetWorkflowConfig(r.Context(), slug)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrNotFound), errors.Is(err, service.ErrInvalidState):
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Workflow not found")
			default:
				writeError(w, http.StatusInternalServerError, "WORKFLOW_CONFIG_FAILED", err.Error())
			}
			return
		}
		writeJSON(w, http.StatusOK, cfg)
	}
}

// DeleteWorkflowAdminHandler handles POST /-/workflows/{slug}/delete. It removes
// the instantiated workflow (its agent_definition), resetting the preset card
// back to its un-configured "Set up" state. Run history is preserved (the FK is
// SET NULL), and the scheduler hot-reloads off the definition-changed hook so
// the cron entry is dropped immediately. Admin-only — deleting de-authorizes
// the recurring AI spend the enable gesture authorized, mirroring that guard.
// Returns JSON {ok:true} for the gallery's async fetch (no full-page redirect).
func DeleteWorkflowAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		if err := svc.DeleteAgentDefinition(r.Context(), slug); err != nil {
			if errors.Is(err, service.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Workflow not found")
				return
			}
			writeError(w, http.StatusUnprocessableEntity, "WORKFLOW_DELETE_FAILED", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// ReconfigureWorkflowAdminHandler handles POST /-/workflows/{slug}/reconfigure.
// It re-composes the configurable layers of an already-enabled workflow
// (schedule, additional-instructions tail, chosen options) without touching
// its run-state toggle or base prompt. Admin-only (the router pins this above
// the surrounding editor group), mirroring the preset-enable guard — a
// reconfigure re-authorizes recurring AI spend behavior on shared data.
func ReconfigureWorkflowAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		_ = r.ParseForm()

		cfg, cfgControl := parseWorkflowFormConfig(r)
		params := service.UpdateWorkflowConfigParams{
			AdditionalInstructions: r.FormValue("additional_instructions"),
			TriggerOnSync:          cfg.TriggerOnSync,
			ScheduleCron:           cfg.ScheduleCron,
			Model:                  cfg.Model,
			MaxTurns:               cfg.MaxTurns,
			MaxBudgetUSD:           cfg.MaxBudgetUSD,
		}
		// Identity edits from the drawer header. Both are optional — an absent
		// field leaves the current value untouched. The service ignores a blank
		// name (it's required); the avatar seed is validated to a URL/cache-safe
		// charset here, and an empty value clears it back to slug-seeded.
		if _, ok := r.Form["name"]; ok {
			n := r.FormValue("name")
			params.Name = &n
		}
		if _, ok := r.Form["avatar_seed"]; ok {
			seed := strings.TrimSpace(r.FormValue("avatar_seed"))
			if seed != "" && !avatar.IsValidSeed(seed) {
				writeError(w, http.StatusBadRequest, "INVALID_PARAMETER", "Invalid avatar seed")
				return
			}
			params.AvatarSeed = &seed
		}
		// Any non-control form field is a preset-specialized option (e.g.
		// apply_mode); the service validates each against the preset's
		// declared options and falls back to the default for unknown keys.
		// Same control-key set as the enable handler, minus enable-only
		// fields (enabled/consent) that don't apply to a reconfigure.
		// Connector toggles: the drawer renders a checkbox per library
		// connector and a hidden marker so an all-unchecked submit still
		// replaces (clears) rather than leaving the set untouched.
		if _, ok := r.Form["connectors_present"]; ok {
			names := append([]string{}, r.Form["connector"]...)
			params.Connectors = &names
		}

		control := map[string]bool{
			"additional_instructions": true,
			"name":                    true,
			"avatar_seed":             true,
			"_csrf":                   true,
			"connector":               true,
			"connectors_present":      true,
		}
		for k := range cfgControl {
			control[k] = true
		}
		params.Options = map[string]string{}
		for key := range r.Form {
			if !control[key] {
				params.Options[key] = r.FormValue(key)
			}
		}

		wf, err := svc.UpdateWorkflowConfig(r.Context(), slug, params)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrNotFound), errors.Is(err, service.ErrInvalidState):
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Workflow not found")
			case errors.Is(err, service.ErrInvalidParameter):
				writeError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			default:
				writeError(w, http.StatusUnprocessableEntity, "WORKFLOW_RECONFIGURE_FAILED", err.Error())
			}
			return
		}
		writeJSON(w, http.StatusOK, wf)
	}
}

//go:build !headless && !lite

package admin

import (
	"errors"
	"net/http"

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

		params := service.UpdateWorkflowConfigParams{
			AdditionalInstructions: r.FormValue("additional_instructions"),
		}
		if cron := r.FormValue("schedule_cron"); cron != "" {
			params.ScheduleCron = &cron
		}
		// Any non-control form field is a preset-specialized option (e.g.
		// apply_mode); the service validates each against the preset's
		// declared options and falls back to the default for unknown keys.
		// Same control-key set as the enable handler, minus enable-only
		// fields (enabled/consent) that don't apply to a reconfigure.
		control := map[string]bool{
			"schedule_cron":           true,
			"additional_instructions": true,
			"_csrf":                   true,
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

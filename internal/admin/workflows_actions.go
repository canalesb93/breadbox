//go:build !headless && !lite

package admin

import (
	"errors"
	"net/http"

	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// EnableWorkflowPresetAdminHandler handles POST /-/workflow-presets/{slug}/enable.
// It instantiates a workflow from the preset (enabled to run). Idempotent at the
// gallery level: an already-enabled preset returns 200 so the page just refreshes
// to show the run toggle. The instantiated workflow's run state is then toggled
// via the existing /-/agents/{slug}/enable|disable endpoints.
func EnableWorkflowPresetAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		_ = r.ParseForm()
		// Configure-drawer fields. enabled defaults to true (the drawer's
		// "Enable" CTA), but the form can pass enabled=false to set up paused.
		params := service.EnableWorkflowFromPresetParams{
			Enabled:                r.FormValue("enabled") != "false",
			AdditionalInstructions: r.FormValue("additional_instructions"),
		}
		if cron := r.FormValue("schedule_cron"); cron != "" {
			params.ScheduleCron = &cron
		}
		wf, err := svc.EnableWorkflowFromPreset(r.Context(), slug, params)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrConflict):
				writeJSON(w, http.StatusOK, map[string]any{"slug": slug, "already_enabled": true})
			case errors.Is(err, service.ErrNotFound):
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Workflow preset not found")
			case errors.Is(err, service.ErrInvalidParameter):
				writeError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			default:
				writeError(w, http.StatusUnprocessableEntity, "WORKFLOW_ENABLE_FAILED", err.Error())
			}
			return
		}
		writeJSON(w, http.StatusOK, wf)
	}
}

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
// via the /-/workflows/{slug}/enable|disable endpoints.
func EnableWorkflowPresetAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		_ = r.ParseForm()

		// First-enable consent gate: until the household has acknowledged
		// that workflows run Claude over their ledger (at cost), require an
		// explicit consent=true on the enable. Server-side enforcement
		// backs the drawer's disabled-until-checked button.
		consented := svc.WorkflowsConsentAcknowledged(r.Context())
		if !consented && r.FormValue("consent") != "true" {
			writeError(w, http.StatusBadRequest, "CONSENT_REQUIRED",
				"Acknowledge that workflows run Claude over your financial data before enabling.")
			return
		}

		// Configure-drawer fields. enabled defaults to true (the drawer's
		// "Enable" CTA), but the form can pass enabled=false to set up paused.
		cfg, cfgControl := parseWorkflowFormConfig(r)
		params := service.EnableWorkflowFromPresetParams{
			Enabled:                r.FormValue("enabled") != "false",
			AdditionalInstructions: r.FormValue("additional_instructions"),
			TriggerOnSync:          cfg.TriggerOnSync,
			ScheduleCron:           cfg.ScheduleCron,
			Model:                  cfg.Model,
			MaxTurns:               cfg.MaxTurns,
			MaxBudgetUSD:           cfg.MaxBudgetUSD,
		}
		// Any non-control form field is a preset-specialized option (e.g.
		// apply_mode); the service validates each against the preset's
		// declared options and ignores unknown keys.
		control := map[string]bool{
			"enabled": true, "additional_instructions": true, "consent": true, "_csrf": true,
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
		// Record the one-time consent on the first successful enable.
		if !consented {
			_ = svc.AcknowledgeWorkflowsConsent(r.Context())
		}
		writeJSON(w, http.StatusOK, wf)
	}
}

//go:build !lite

package api

import (
	"net/http"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// Workflow presets are the code-defined gallery templates. Listing is a read;
// enabling instantiates a workflow (agent_definition) and needs write scope.

// ListWorkflowPresetsHandler — GET /workflow-presets
func ListWorkflowPresetsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := svc.ListWorkflowPresets(r.Context())
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list workflow presets")
			return
		}
		writeData(w, out)
	}
}

// EnableWorkflowPresetHandler — POST /workflow-presets/{slug}/enable
// Optional body: {"enabled": true} to start the workflow immediately
// (default false — instantiated but paused for review).
func EnableWorkflowPresetHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		var input struct {
			Enabled bool `json:"enabled"`
		}
		if err := decodeJSONOptional(r, &input); err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body")
			return
		}
		wf, err := svc.EnableWorkflowFromPreset(r.Context(), slug, service.EnableWorkflowFromPresetParams{Enabled: input.Enabled})
		if err != nil {
			writeServiceError(w, err, "Workflow preset not found", "Failed to enable workflow preset")
			return
		}
		writeJSON(w, http.StatusCreated, wf)
	}
}

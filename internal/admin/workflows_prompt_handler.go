//go:build !headless && !lite

package admin

import (
	"errors"
	"net/http"

	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// WorkflowPromptPreviewAdminHandler handles GET /-/workflows/{slug}/prompt.
// It returns the fully composed base prompt for a preset as JSON
// {slug, title, prompt}, powering the gallery's "Preview prompt" modal. Read-
// only — it reads the code-defined preset registry, instantiates nothing.
func WorkflowPromptPreviewAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		preview, err := svc.ComposeWorkflowPrompt(r.Context(), slug)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrNotFound):
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Workflow preset not found")
			default:
				writeError(w, http.StatusUnprocessableEntity, "WORKFLOW_PROMPT_FAILED", err.Error())
			}
			return
		}
		writeJSON(w, http.StatusOK, preview)
	}
}

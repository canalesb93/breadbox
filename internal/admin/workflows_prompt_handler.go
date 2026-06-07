//go:build !headless && !lite

package admin

import (
	"errors"
	"net/http"

	"breadbox/internal/markdown"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// workflowPromptResponse is the JSON shape for GET /-/workflows/{slug}/prompt.
// It promotes the service preview fields (slug, title, prompt) and adds
// prompt_html — the prompt rendered server-side to a sanitized .bb-prose
// fragment. The gallery modal injects prompt_html for display and uses the
// raw prompt for the copy-to-clipboard affordance.
type workflowPromptResponse struct {
	*service.WorkflowPromptPreview
	PromptHTML string `json:"prompt_html"`
}

// WorkflowPromptPreviewAdminHandler handles GET /-/workflows/{slug}/prompt.
// It returns the fully composed base prompt for a preset as JSON
// {slug, title, prompt, prompt_html}, powering the gallery's "Preview prompt"
// modal. Read-only — it reads the code-defined preset registry, instantiates
// nothing.
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
		writeJSON(w, http.StatusOK, workflowPromptResponse{
			WorkflowPromptPreview: preview,
			PromptHTML:            string(markdown.Render(preview.Prompt)),
		})
	}
}

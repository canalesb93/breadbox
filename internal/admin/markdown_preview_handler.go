//go:build !headless && !lite

package admin

import (
	"encoding/json"
	"net/http"

	"breadbox/internal/markdown"
)

// markdownPreviewRequest is the JSON body for POST /-/markdown/preview:
// arbitrary Markdown source the admin is editing in the prompt modal.
type markdownPreviewRequest struct {
	Text string `json:"text"`
}

// markdownPreviewResponse returns the rendered, sanitized HTML fragment.
type markdownPreviewResponse struct {
	HTML string `json:"html"`
}

// MarkdownPreviewAdminHandler handles POST /-/markdown/preview. It renders an
// arbitrary Markdown string to a sanitized .bb-prose HTML fragment via the
// single server-side renderer (internal/markdown: goldmark + bluemonday), so
// the prompt modal can preview edited-but-unsaved text without shipping a
// client-side Markdown parser. Output is sanitized; there is no injection
// vector beyond what the authenticated admin already controls.
func MarkdownPreviewAdminHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req markdownPreviewRequest
		// Cap the body so a runaway editor can't OOM the renderer.
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_BODY", "Could not parse request body")
			return
		}
		writeJSON(w, http.StatusOK, markdownPreviewResponse{
			HTML: string(markdown.Render(req.Text)),
		})
	}
}

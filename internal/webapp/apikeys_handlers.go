//go:build !headless && !lite

package webapp

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"breadbox/internal/webapp/pages"
)

// registerAPIKeys wires the read-only API-keys route onto the authenticated subrouter.
func (h *Handler) registerAPIKeys(r chi.Router) {
	r.Get("/api-keys", h.apiKeysList)
}

// apiKeysList renders every API key, masked — plaintext is never re-rendered.
func (h *Handler) apiKeysList(w http.ResponseWriter, r *http.Request) {
	keys, err := h.app.Service.ListAPIKeys(r.Context())
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	render(w, r, http.StatusOK, pages.APIKeysList(h.shellData(r, "API keys"), keys))
}

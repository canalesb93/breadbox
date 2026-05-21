//go:build !headless && !lite

package webapp

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"breadbox/internal/service"
	"breadbox/internal/webapp/pages"
)

// registerConnections wires the connection read surfaces onto the authenticated
// /app subrouter. Every page is a real document — no client router.
func (h *Handler) registerConnections(r chi.Router) {
	r.Get("/connections", h.connectionsList)
	r.Get("/connections/{id}", h.connectionDetail)
}

// connectionsList renders every bank connection as a card grid.
func (h *Handler) connectionsList(w http.ResponseWriter, r *http.Request) {
	conns, err := h.app.Service.ListConnections(r.Context(), nil)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	render(w, r, http.StatusOK, pages.ConnectionsList(h.shellData(r, "Connections"), conns))
}

// connectionDetail renders one connection's header, detail grid, and accounts.
func (h *Handler) connectionDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	detail, err := h.app.Service.GetConnection(r.Context(), id)
	if errors.Is(err, service.ErrNotFound) {
		h.notFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	title := "Connection"
	if detail.InstitutionName != nil && *detail.InstitutionName != "" {
		title = *detail.InstitutionName
	}
	render(w, r, http.StatusOK, pages.ConnectionDetail(h.shellData(r, title), detail))
}

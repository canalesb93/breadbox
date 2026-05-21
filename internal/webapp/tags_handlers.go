//go:build !headless && !lite

package webapp

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"breadbox/internal/service"
	"breadbox/internal/webapp/pages"
)

// tagsList renders every tag as a grid of linked chips.
func (h *Handler) tagsList(w http.ResponseWriter, r *http.Request) {
	tags, err := h.app.Service.ListTags(r.Context())
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	render(w, r, http.StatusOK, pages.TagsList(h.shellData(r, "Tags"), tags))
}

// tagDetail renders one tag (by slug, short ID, or UUID) with its usage count.
func (h *Handler) tagDetail(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	tag, err := h.app.Service.GetTag(r.Context(), slug)
	if errors.Is(err, service.ErrNotFound) {
		h.notFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	usage, err := h.app.Service.CountTransactionsTag(r.Context(), tag.Slug)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	render(w, r, http.StatusOK, pages.TagDetail(h.shellData(r, tag.DisplayName), tag, usage))
}

// registerTags wires the tag read routes onto an authenticated subrouter.
func (h *Handler) registerTags(r chi.Router) {
	r.Get("/tags", h.tagsList)
	r.Get("/tags/{slug}", h.tagDetail)
}

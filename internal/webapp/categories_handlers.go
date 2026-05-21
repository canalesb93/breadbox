//go:build !headless && !lite

package webapp

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"breadbox/internal/webapp/pages"
)

// categoriesList renders the category hierarchy (top-level categories with their children).
func (h *Handler) categoriesList(w http.ResponseWriter, r *http.Request) {
	cats, err := h.app.Service.ListCategoryTree(r.Context())
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	render(w, r, http.StatusOK, pages.CategoriesList(h.shellData(r, "Categories"), cats))
}

// categoryDetail renders one category by slug, short ID, or UUID. It reads from the
// flat list (rather than GetCategory) so the response carries the parent's slug and
// display name for the parent link.
func (h *Handler) categoryDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cats, err := h.app.Service.ListCategories(r.Context())
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	for i := range cats {
		c := cats[i]
		if c.Slug == id || c.ShortID == id || c.ID == id {
			render(w, r, http.StatusOK, pages.CategoryDetail(h.shellData(r, c.DisplayName), &c))
			return
		}
	}
	h.notFound(w, r)
}

// registerCategories wires the category read routes onto an authenticated subrouter.
func (h *Handler) registerCategories(r chi.Router) {
	r.Get("/categories", h.categoriesList)
	r.Get("/categories/{id}", h.categoryDetail)
}

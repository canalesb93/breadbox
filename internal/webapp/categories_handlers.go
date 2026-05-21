//go:build !headless && !lite

package webapp

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"

	"breadbox/internal/service"
	"breadbox/internal/webapp/components"
	"breadbox/internal/webapp/pages"
)

// slugFormat mirrors the service-layer slug rule (lowercase alphanumeric + underscore).
var slugFormat = regexp.MustCompile(`^[a-z0-9_]+$`)

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

// newCategory renders the empty create form. Parent options are the existing top-level
// categories so a new sub-category can be nested.
func (h *Handler) newCategory(w http.ResponseWriter, r *http.Request) {
	opts, err := h.parentOptions(r)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	render(w, r, http.StatusOK, pages.CategoryForm(h.shellData(r, "New category"), pages.CategoryFormData{
		Mode:          "create",
		ActionURL:     "/app/categories",
		CancelURL:     "/app/categories",
		ParentOptions: opts,
		Errors:        map[string]string{},
	}))
}

// createCategory validates and creates a category. On validation failure it re-renders
// the same form with field errors and HTTP 422; on success it 303s to the new detail page.
func (h *Handler) createCategory(w http.ResponseWriter, r *http.Request) {
	vals := pages.CategoryFormValues{
		DisplayName: strings.TrimSpace(r.FormValue("display_name")),
		Slug:        strings.TrimSpace(r.FormValue("slug")),
		ParentID:    r.FormValue("parent_id"),
		Color:       strings.TrimSpace(r.FormValue("color")),
		Icon:        strings.TrimSpace(r.FormValue("icon")),
	}

	fieldErrs := validateCategory(vals.DisplayName, vals.Slug)
	if len(fieldErrs) > 0 {
		h.rerenderCategoryForm(w, r, "create", "/app/categories", "/app/categories", vals, fieldErrs)
		return
	}

	params := service.CreateCategoryParams{
		DisplayName: vals.DisplayName,
		Slug:        vals.Slug,
		Color:       optPtr(vals.Color),
		Icon:        optPtr(vals.Icon),
	}
	if vals.ParentID != "" {
		params.ParentID = &vals.ParentID
	}

	cat, err := h.app.Service.CreateCategory(r.Context(), params)
	if err != nil {
		if errors.Is(err, service.ErrSlugConflict) {
			h.rerenderCategoryForm(w, r, "create", "/app/categories", "/app/categories", vals,
				map[string]string{"slug": "A category with this slug already exists."})
			return
		}
		h.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/app/categories/"+cat.ShortID, http.StatusSeeOther)
}

// editCategory prefills the form for an existing category. Slug and parent are read-only
// on edit because UpdateCategory does not change them.
func (h *Handler) editCategory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cat, err := h.app.Service.GetCategory(r.Context(), id)
	if errors.Is(err, service.ErrCategoryNotFound) {
		h.notFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	render(w, r, http.StatusOK, pages.CategoryForm(h.shellData(r, "Edit category"), pages.CategoryFormData{
		Mode:      "edit",
		ActionURL: "/app/categories/" + cat.ShortID,
		CancelURL: "/app/categories/" + cat.ShortID,
		IsSystem:  cat.IsSystem,
		Values: pages.CategoryFormValues{
			DisplayName: cat.DisplayName,
			Slug:        cat.Slug,
			Color:       pages.Deref(cat.Color, ""),
			Icon:        pages.Deref(cat.Icon, ""),
			Hidden:      cat.Hidden,
		},
		Errors: map[string]string{},
	}))
}

// updateCategory validates and updates a category. Re-renders with errors + 422 on
// validation failure; 303s to the detail page on success.
func (h *Handler) updateCategory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cat, err := h.app.Service.GetCategory(r.Context(), id)
	if errors.Is(err, service.ErrCategoryNotFound) {
		h.notFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	vals := pages.CategoryFormValues{
		DisplayName: strings.TrimSpace(r.FormValue("display_name")),
		Slug:        cat.Slug, // immutable on edit; keep for display
		Color:       strings.TrimSpace(r.FormValue("color")),
		Icon:        strings.TrimSpace(r.FormValue("icon")),
		Hidden:      r.FormValue("hidden") != "",
	}

	if vals.DisplayName == "" {
		h.rerenderCategoryFormEdit(w, r, cat, vals, map[string]string{"display_name": "Display name is required."})
		return
	}

	_, err = h.app.Service.UpdateCategory(r.Context(), id, service.UpdateCategoryParams{
		DisplayName: vals.DisplayName,
		Color:       optPtr(vals.Color),
		Icon:        optPtr(vals.Icon),
		SortOrder:   cat.SortOrder,
		Hidden:      vals.Hidden,
	})
	if errors.Is(err, service.ErrCategoryNotFound) {
		h.notFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/app/categories/"+cat.ShortID, http.StatusSeeOther)
}

// rerenderCategoryForm re-renders the create form with errors at HTTP 422.
func (h *Handler) rerenderCategoryForm(w http.ResponseWriter, r *http.Request, mode, action, cancel string, vals pages.CategoryFormValues, fieldErrs map[string]string) {
	opts, err := h.parentOptions(r)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	render(w, r, http.StatusUnprocessableEntity, pages.CategoryForm(h.shellData(r, "New category"), pages.CategoryFormData{
		Mode:          mode,
		ActionURL:     action,
		CancelURL:     cancel,
		Values:        vals,
		Errors:        fieldErrs,
		ParentOptions: opts,
	}))
}

// rerenderCategoryFormEdit re-renders the edit form with errors at HTTP 422.
func (h *Handler) rerenderCategoryFormEdit(w http.ResponseWriter, r *http.Request, cat *service.CategoryResponse, vals pages.CategoryFormValues, fieldErrs map[string]string) {
	render(w, r, http.StatusUnprocessableEntity, pages.CategoryForm(h.shellData(r, "Edit category"), pages.CategoryFormData{
		Mode:      "edit",
		ActionURL: "/app/categories/" + cat.ShortID,
		CancelURL: "/app/categories/" + cat.ShortID,
		IsSystem:  cat.IsSystem,
		Values:    vals,
		Errors:    fieldErrs,
	}))
}

// parentOptions builds the SelectField options for top-level categories (a sub-category
// can only nest one level deep). The currently-selected parent is preserved by value.
func (h *Handler) parentOptions(r *http.Request) ([]components.Option, error) {
	tree, err := h.app.Service.ListCategoryTree(r.Context())
	if err != nil {
		return nil, err
	}
	opts := make([]components.Option, 0, len(tree))
	for _, c := range tree {
		opts = append(opts, components.Option{Value: c.ShortID, Label: c.DisplayName})
	}
	return opts, nil
}

// validateCategory runs the server-side rules: display name required; slug, when
// provided, must match the canonical slug format (it is auto-generated when blank).
func validateCategory(displayName, slug string) map[string]string {
	errs := map[string]string{}
	if displayName == "" {
		errs["display_name"] = "Display name is required."
	}
	if slug != "" && !slugFormat.MatchString(slug) {
		errs["slug"] = "Use lowercase letters, numbers, and underscores only."
	}
	return errs
}

// optPtr returns nil for an empty string, else a pointer to the trimmed value.
func optPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// registerCategories wires the category read + write routes onto an authenticated subrouter.
// "/categories/new" is registered before "/categories/{id}" so it isn't captured as an id.
func (h *Handler) registerCategories(r chi.Router) {
	r.Get("/categories", h.categoriesList)
	r.Post("/categories", h.requireSameOrigin(h.createCategory))
	r.Get("/categories/new", h.newCategory)
	r.Get("/categories/{id}", h.categoryDetail)
	r.Get("/categories/{id}/edit", h.editCategory)
	r.Post("/categories/{id}", h.requireSameOrigin(h.updateCategory))
}

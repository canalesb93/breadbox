//go:build !headless && !lite

package webapp

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"

	"breadbox/internal/service"
	"breadbox/internal/webapp/pages"
)

// tagSlugFormat mirrors the service-layer tag slug rule: lowercase alphanumerics with
// optional hyphens/colons between (single-char [a-z0-9] also allowed).
var tagSlugFormat = regexp.MustCompile(`^[a-z0-9]([a-z0-9\-:]*[a-z0-9])?$`)

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

// newTag renders the empty create form.
func (h *Handler) newTag(w http.ResponseWriter, r *http.Request) {
	render(w, r, http.StatusOK, pages.TagForm(h.shellData(r, "New tag"), pages.TagFormData{
		Mode:      "create",
		ActionURL: "/app/tags",
		CancelURL: "/app/tags",
		Errors:    map[string]string{},
	}))
}

// createTag validates and creates a tag. On validation failure it re-renders the same
// form with field errors and HTTP 422; on success it 303s to the new detail page.
func (h *Handler) createTag(w http.ResponseWriter, r *http.Request) {
	vals := pages.TagFormValues{
		DisplayName: strings.TrimSpace(r.FormValue("display_name")),
		Slug:        strings.TrimSpace(r.FormValue("slug")),
		Color:       strings.TrimSpace(r.FormValue("color")),
		Icon:        strings.TrimSpace(r.FormValue("icon")),
		Description: strings.TrimSpace(r.FormValue("description")),
	}

	fieldErrs := validateTag(vals.DisplayName, vals.Slug)
	if len(fieldErrs) > 0 {
		h.rerenderTagFormCreate(w, r, vals, fieldErrs)
		return
	}

	tag, err := h.app.Service.CreateTag(r.Context(), service.CreateTagParams{
		Slug:        vals.Slug,
		DisplayName: vals.DisplayName,
		Description: vals.Description,
		Color:       optPtr(vals.Color),
		Icon:        optPtr(vals.Icon),
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidParameter) {
			h.rerenderTagFormCreate(w, r, vals, map[string]string{"slug": "Use lowercase letters, numbers, hyphens, and colons only."})
			return
		}
		// Most likely a duplicate slug (unique constraint) — surface on the slug field.
		h.rerenderTagFormCreate(w, r, vals, map[string]string{"slug": "A tag with this slug already exists."})
		return
	}
	http.Redirect(w, r, "/app/tags/"+tag.Slug, http.StatusSeeOther)
}

// editTag prefills the form for an existing tag. Slug is read-only on edit because
// UpdateTag does not change it.
func (h *Handler) editTag(w http.ResponseWriter, r *http.Request) {
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
	render(w, r, http.StatusOK, pages.TagForm(h.shellData(r, "Edit tag"), tagEditFormData(tag, tagValuesFromResponse(tag), map[string]string{})))
}

// updateTag validates and updates a tag. Re-renders with errors + 422 on validation
// failure; 303s to the detail page on success.
func (h *Handler) updateTag(w http.ResponseWriter, r *http.Request) {
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

	vals := pages.TagFormValues{
		DisplayName: strings.TrimSpace(r.FormValue("display_name")),
		Slug:        tag.Slug, // immutable on edit; keep for display
		Color:       strings.TrimSpace(r.FormValue("color")),
		Icon:        strings.TrimSpace(r.FormValue("icon")),
		Description: strings.TrimSpace(r.FormValue("description")),
		Lifecycle:   strings.TrimSpace(r.FormValue("lifecycle")),
	}

	if vals.DisplayName == "" {
		render(w, r, http.StatusUnprocessableEntity, pages.TagForm(h.shellData(r, "Edit tag"),
			tagEditFormData(tag, vals, map[string]string{"display_name": "Display name is required."})))
		return
	}

	params := service.UpdateTagParams{
		DisplayName: &vals.DisplayName,
		Description: &vals.Description,
		Color:       &vals.Color,
		Icon:        &vals.Icon,
	}
	if vals.Lifecycle != "" {
		params.Lifecycle = &vals.Lifecycle
	}

	if _, err := h.app.Service.UpdateTag(r.Context(), slug, params); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			h.notFound(w, r)
			return
		}
		if errors.Is(err, service.ErrInvalidParameter) {
			render(w, r, http.StatusUnprocessableEntity, pages.TagForm(h.shellData(r, "Edit tag"),
				tagEditFormData(tag, vals, map[string]string{"display_name": "Display name is required."})))
			return
		}
		h.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/app/tags/"+tag.Slug, http.StatusSeeOther)
}

// rerenderTagFormCreate re-renders the create form with errors at HTTP 422.
func (h *Handler) rerenderTagFormCreate(w http.ResponseWriter, r *http.Request, vals pages.TagFormValues, fieldErrs map[string]string) {
	render(w, r, http.StatusUnprocessableEntity, pages.TagForm(h.shellData(r, "New tag"), pages.TagFormData{
		Mode:      "create",
		ActionURL: "/app/tags",
		CancelURL: "/app/tags",
		Values:    vals,
		Errors:    fieldErrs,
	}))
}

// tagEditFormData builds the edit-mode form data for a tag.
func tagEditFormData(tag *service.TagResponse, vals pages.TagFormValues, errs map[string]string) pages.TagFormData {
	return pages.TagFormData{
		Mode:      "edit",
		ActionURL: "/app/tags/" + tag.Slug,
		CancelURL: "/app/tags/" + tag.Slug,
		Values:    vals,
		Errors:    errs,
	}
}

// tagValuesFromResponse maps a tag record into editable form values.
func tagValuesFromResponse(tag *service.TagResponse) pages.TagFormValues {
	return pages.TagFormValues{
		DisplayName: tag.DisplayName,
		Slug:        tag.Slug,
		Color:       pages.Deref(tag.Color, ""),
		Icon:        pages.Deref(tag.Icon, ""),
		Description: tag.Description,
		Lifecycle:   tag.Lifecycle,
	}
}

// validateTag runs the server-side rules: display name required; slug required and must
// match the canonical tag slug format.
func validateTag(displayName, slug string) map[string]string {
	errs := map[string]string{}
	if displayName == "" {
		errs["display_name"] = "Display name is required."
	}
	if slug == "" {
		errs["slug"] = "Slug is required."
	} else if !tagSlugFormat.MatchString(slug) {
		errs["slug"] = "Use lowercase letters, numbers, hyphens, and colons only."
	}
	return errs
}

// registerTags wires the tag read + write routes onto an authenticated subrouter.
// "/tags/new" is registered before "/tags/{slug}" so it isn't captured as a slug.
func (h *Handler) registerTags(r chi.Router) {
	r.Get("/tags", h.tagsList)
	r.Post("/tags", h.requireSameOrigin(h.createTag))
	r.Get("/tags/new", h.newTag)
	r.Get("/tags/{slug}", h.tagDetail)
	r.Get("/tags/{slug}/edit", h.editTag)
	r.Post("/tags/{slug}", h.requireSameOrigin(h.updateTag))
}

package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"breadbox/internal/app"
	"breadbox/internal/service"
	"breadbox/internal/templates/components"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// TagsPageHandler serves GET /tags.
func TagsPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		tags, err := svc.ListTags(ctx)
		if err != nil {
			tr.RenderError(w, r)
			return
		}

		// Fetch usage counts for each tag.
		rows := make([]pages.TagRow, 0, len(tags))
		for _, t := range tags {
			count, err := svc.CountTransactionsTag(ctx, t.Slug)
			if err != nil {
				count = 0
			}
			rows = append(rows, pages.TagRow{
				ID:               t.ID,
				Slug:             t.Slug,
				DisplayName:      t.DisplayName,
				Description:      t.Description,
				Color:            t.Color,
				Icon:             t.Icon,
				TransactionCount: count,
			})
		}

		data := BaseTemplateData(r, sm, "tags", "Tags")
		tr.RenderWithTempl(w, r, data, pages.Tags(pages.TagsProps{Tags: rows}))
	}
}

// TagNewPageHandler serves GET /tags/new — renders the empty create form.
func TagNewPageHandler(sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := BaseTemplateData(r, sm, "tags", "Add Tag")
		tr.RenderWithTempl(w, r, data, pages.TagForm(pages.TagFormProps{
			IsEdit: false,
			Breadcrumbs: []components.Breadcrumb{
				{Label: "Tags", Href: "/tags"},
				{Label: "Add Tag"},
			},
		}))
	}
}

// TagEditPageHandler serves GET /tags/{id}/edit — renders the form populated from DB.
func TagEditPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		tag, err := svc.GetTag(r.Context(), id)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				tr.RenderNotFound(w, r)
				return
			}
			tr.RenderError(w, r)
			return
		}

		data := BaseTemplateData(r, sm, "tags", "Edit "+tag.DisplayName)
		tr.RenderWithTempl(w, r, data, pages.TagForm(pages.TagFormProps{
			IsEdit: true,
			Tag:    tag,
			Breadcrumbs: []components.Breadcrumb{
				{Label: "Tags", Href: "/tags"},
				{Label: tag.DisplayName},
			},
		}))
	}
}

// CreateTagAdminHandler handles POST /-/tags.
func CreateTagAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Slug        string  `json:"slug"`
			DisplayName string  `json:"display_name"`
			Description string  `json:"description"`
			Color       *string `json:"color"`
			Icon        *string `json:"icon"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		req.Slug = strings.TrimSpace(req.Slug)
		req.DisplayName = strings.TrimSpace(req.DisplayName)
		if req.Slug == "" || req.DisplayName == "" {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "slug and display_name are required")
			return
		}

		result, err := svc.CreateTag(r.Context(), service.CreateTagParams{
			Slug:        req.Slug,
			DisplayName: req.DisplayName,
			Description: req.Description,
			Color:       req.Color,
			Icon:        req.Icon,
		})
		if err != nil {
			handleTagError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, result)
	}
}

// UpdateTagAdminHandler handles PUT /-/tags/{id}.
func UpdateTagAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req struct {
			DisplayName *string `json:"display_name"`
			Description *string `json:"description"`
			Color       *string `json:"color"`
			Icon        *string `json:"icon"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		result, err := svc.UpdateTag(r.Context(), id, service.UpdateTagParams{
			DisplayName: req.DisplayName,
			Description: req.Description,
			Color:       req.Color,
			Icon:        req.Icon,
		})
		if err != nil {
			handleTagError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// DeleteTagAdminHandler handles DELETE /-/tags/{id}.
func DeleteTagAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.DeleteTag(r.Context(), id); err != nil {
			handleTagError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
	}
}

// AddTransactionTagAdminHandler handles POST /-/transactions/{id}/tags.
// Body: { slug: string, note?: string }.
func AddTransactionTagAdminHandler(a *app.App, sm *scs.SessionManager, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txnID := chi.URLParam(r, "id")
		actor := ActorFromSession(sm, r)

		var body struct {
			Slug string `json:"slug"`
			Note string `json:"note,omitempty"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		body.Slug = strings.TrimSpace(body.Slug)
		if body.Slug == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "slug is required"})
			return
		}
		added, alreadyPresent, err := svc.AddTransactionTag(r.Context(), txnID, body.Slug, actor, body.Note)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrNotFound):
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "transaction not found"})
			case errors.Is(err, service.ErrInvalidParameter):
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			default:
				a.Logger.Error("add transaction tag", "error", err)
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":              true,
			"added":           added,
			"already_present": alreadyPresent,
			"slug":            body.Slug,
		})
	}
}

// RemoveTransactionTagAdminHandler handles DELETE /-/transactions/{id}/tags/{slug}.
// Query/body: optional note recorded on the tag_removed annotation. Accepts
// either form-encoded or JSON.
func RemoveTransactionTagAdminHandler(a *app.App, sm *scs.SessionManager, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txnID := chi.URLParam(r, "id")
		slug := chi.URLParam(r, "slug")
		actor := ActorFromSession(sm, r)

		note := strings.TrimSpace(r.URL.Query().Get("note"))
		if note == "" {
			var body struct {
				Note string `json:"note"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			note = strings.TrimSpace(body.Note)
		}

		removed, alreadyAbsent, err := svc.RemoveTransactionTag(r.Context(), txnID, slug, actor, note)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrNotFound):
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "transaction not found"})
			case errors.Is(err, service.ErrInvalidParameter):
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			default:
				a.Logger.Error("remove transaction tag", "error", err)
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":             true,
			"removed":        removed,
			"already_absent": alreadyAbsent,
			"slug":           slug,
		})
	}
}

func handleTagError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Tag not found")
	case errors.Is(err, service.ErrInvalidParameter):
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
}


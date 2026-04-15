package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"breadbox/internal/app"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// TagRow is a tag entry enriched with usage counts for the admin list page.
type TagRow struct {
	service.TagResponse
	TransactionCount int64
}

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
		rows := make([]TagRow, 0, len(tags))
		for _, t := range tags {
			count, err := svc.CountTransactionsTag(ctx, t.Slug)
			if err != nil {
				count = 0
			}
			rows = append(rows, TagRow{TagResponse: t, TransactionCount: count})
		}

		data := BaseTemplateData(r, sm, "tags", "Tags")
		data["Tags"] = rows
		tr.Render(w, r, "tags.html", data)
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
			Lifecycle   string  `json:"lifecycle"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"code": "INVALID_REQUEST", "message": "Invalid request body"}})
			return
		}
		req.Slug = strings.TrimSpace(req.Slug)
		req.DisplayName = strings.TrimSpace(req.DisplayName)
		if req.Slug == "" || req.DisplayName == "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": map[string]any{"code": "VALIDATION_ERROR", "message": "slug and display_name are required"}})
			return
		}

		result, err := svc.CreateTag(r.Context(), service.CreateTagParams{
			Slug:        req.Slug,
			DisplayName: req.DisplayName,
			Description: req.Description,
			Color:       req.Color,
			Icon:        req.Icon,
			Lifecycle:   req.Lifecycle,
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
			Lifecycle   *string `json:"lifecycle"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"code": "INVALID_REQUEST", "message": "Invalid request body"}})
			return
		}
		result, err := svc.UpdateTag(r.Context(), id, service.UpdateTagParams{
			DisplayName: req.DisplayName,
			Description: req.Description,
			Color:       req.Color,
			Icon:        req.Icon,
			Lifecycle:   req.Lifecycle,
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
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
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
// Query/body: note (REQUIRED for ephemeral tags). Accepts either form-encoded or JSON.
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
		writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]any{"code": "NOT_FOUND", "message": "Tag not found"}})
	case errors.Is(err, service.ErrInvalidParameter):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": map[string]any{"code": "VALIDATION_ERROR", "message": err.Error()}})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"code": "INTERNAL_ERROR", "message": err.Error()}})
	}
}


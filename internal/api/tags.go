package api

import (
	"errors"
	"net/http"
	"strings"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ListTagsHandler returns all registered tags.
// GET /api/v1/tags — mirrors the list_tags MCP tool.
func ListTagsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tags, err := svc.ListTags(r.Context())
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list tags")
			return
		}
		writeData(w, map[string]any{"tags": tags})
	}
}

// createTagRequest is the JSON body shape for POST /api/v1/tags. Mirrors
// service.CreateTagParams plus an optional Color/Icon (both pointers so
// callers can omit to keep them NULL).
type createTagRequest struct {
	Slug        string  `json:"slug"`
	DisplayName string  `json:"display_name"`
	Description string  `json:"description"`
	Color       *string `json:"color,omitempty"`
	Icon        *string `json:"icon,omitempty"`
}

// CreateTagHandler creates a new tag record.
// POST /api/v1/tags — mirrors the create_tag MCP tool.
func CreateTagHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input createTagRequest
		if !decodeJSON(w, r, &input) {
			return
		}

		tag, err := svc.CreateTag(r.Context(), service.CreateTagParams{
			Slug:        input.Slug,
			DisplayName: input.DisplayName,
			Description: input.Description,
			Color:       input.Color,
			Icon:        input.Icon,
		})
		if err != nil {
			// Postgres returns 23505 (unique_violation) when the slug already
			// exists. The service surfaces it as a wrapped DB error rather
			// than a sentinel, so we map at the boundary.
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				mw.WriteError(w, http.StatusConflict, "SLUG_CONFLICT", "A tag with this slug already exists")
				return
			}
			writeServiceError(w, err, "Tag not found", "Failed to create tag")
			return
		}

		writeJSON(w, http.StatusCreated, tag)
	}
}

// GetTagHandler returns a single tag by UUID, short_id, or slug.
// GET /api/v1/tags/{slug}.
func GetTagHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idOrSlug := chi.URLParam(r, "slug")
		tag, err := svc.GetTag(r.Context(), idOrSlug)
		if err != nil {
			writeServiceError(w, err, "Tag not found", "Failed to get tag")
			return
		}
		writeData(w, tag)
	}
}

// updateTagRequest is the JSON body shape for PATCH /api/v1/tags/{slug}.
// Every field is optional; omitted fields stay unchanged.
type updateTagRequest struct {
	DisplayName *string `json:"display_name,omitempty"`
	Description *string `json:"description,omitempty"`
	Color       *string `json:"color,omitempty"`
	Icon        *string `json:"icon,omitempty"`
	Lifecycle   *string `json:"lifecycle,omitempty"`
}

// UpdateTagHandler partially updates mutable tag fields. Slug is immutable.
// PATCH /api/v1/tags/{slug} — mirrors the update_tag MCP tool.
func UpdateTagHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idOrSlug := chi.URLParam(r, "slug")

		var input updateTagRequest
		if !decodeJSON(w, r, &input) {
			return
		}

		tag, err := svc.UpdateTag(r.Context(), idOrSlug, service.UpdateTagParams{
			DisplayName: input.DisplayName,
			Description: input.Description,
			Color:       input.Color,
			Icon:        input.Icon,
			Lifecycle:   input.Lifecycle,
		})
		if err != nil {
			writeServiceError(w, err, "Tag not found", "Failed to update tag")
			return
		}
		writeData(w, tag)
	}
}

// DeleteTagHandler removes a tag. Cascades to transaction_tags rows;
// annotations referencing the tag retain tag_id=NULL.
// DELETE /api/v1/tags/{slug} — mirrors the delete_tag MCP tool.
func DeleteTagHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idOrSlug := chi.URLParam(r, "slug")
		if err := svc.DeleteTag(r.Context(), idOrSlug); err != nil {
			writeServiceError(w, err, "Tag not found", "Failed to delete tag")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// AddTransactionTagHandler attaches a tag to a transaction. Auto-creates the
// tag if the slug is not yet registered. Mirrors the admin-side handler.
// POST /api/v1/transactions/{id}/tags — body: {"slug":"..."}.
func AddTransactionTagHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txnID := chi.URLParam(r, "id")

		var body struct {
			Slug string `json:"slug"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		body.Slug = strings.TrimSpace(body.Slug)
		if body.Slug == "" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "slug is required")
			return
		}

		actor := service.ActorFromContext(r.Context())
		added, alreadyPresent, err := svc.AddTransactionTag(r.Context(), txnID, body.Slug, actor)
		if err != nil {
			writeServiceError(w, err, "Transaction not found", "Failed to add transaction tag")
			return
		}
		writeData(w, map[string]any{
			"added":           added,
			"already_present": alreadyPresent,
			"slug":            body.Slug,
			"transaction_id":  txnID,
		})
	}
}

// RemoveTransactionTagHandler removes a tag from a transaction.
// DELETE /api/v1/transactions/{id}/tags/{slug}.
func RemoveTransactionTagHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txnID := chi.URLParam(r, "id")
		slug := chi.URLParam(r, "slug")

		actor := service.ActorFromContext(r.Context())
		removed, alreadyAbsent, err := svc.RemoveTransactionTag(r.Context(), txnID, slug, actor)
		if err != nil {
			writeServiceError(w, err, "Transaction not found", "Failed to remove transaction tag")
			return
		}
		writeData(w, map[string]any{
			"removed":        removed,
			"already_absent": alreadyAbsent,
			"slug":           slug,
			"transaction_id": txnID,
		})
	}
}

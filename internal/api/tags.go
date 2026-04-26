package api

import (
	"net/http"
	"strings"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
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

package api

import (
	"errors"
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
// POST /api/v1/transactions/{id}/tags — body: {"slug":"...", "note":"..."}.
func AddTransactionTagHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txnID := chi.URLParam(r, "id")

		var body struct {
			Slug string `json:"slug"`
			Note string `json:"note,omitempty"`
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
		added, alreadyPresent, err := svc.AddTransactionTag(r.Context(), txnID, body.Slug, actor, body.Note)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrNotFound):
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Transaction not found")
			case errors.Is(err, service.ErrInvalidParameter):
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			default:
				mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to add transaction tag")
			}
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
// DELETE /api/v1/transactions/{id}/tags/{slug} — optional ?note=... or JSON
// body {"note":"..."} recorded on the tag_removed annotation.
func RemoveTransactionTagHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txnID := chi.URLParam(r, "id")
		slug := chi.URLParam(r, "slug")

		note := strings.TrimSpace(r.URL.Query().Get("note"))
		if note == "" && r.ContentLength > 0 {
			var body struct {
				Note string `json:"note"`
			}
			if !decodeJSON(w, r, &body) {
				return
			}
			note = strings.TrimSpace(body.Note)
		}

		actor := service.ActorFromContext(r.Context())
		removed, alreadyAbsent, err := svc.RemoveTransactionTag(r.Context(), txnID, slug, actor, note)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrNotFound):
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Transaction not found")
			case errors.Is(err, service.ErrInvalidParameter):
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			default:
				mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to remove transaction tag")
			}
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

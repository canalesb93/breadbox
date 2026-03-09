package api

import (
	"errors"
	"net/http"
	"strconv"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// ListAuditLogHandler returns audit log entries with optional filters.
func ListAuditLogHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		limit := 50
		if v := q.Get("limit"); v != "" {
			parsed, err := strconv.Atoi(v)
			if err != nil || parsed < 1 || parsed > 200 {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "limit must be between 1 and 200")
				return
			}
			limit = parsed
		}

		// If both entity_type and entity_id are provided, use entity-scoped query.
		entityType := q.Get("entity_type")
		entityID := q.Get("entity_id")

		if entityType != "" && entityID != "" {
			result, err := svc.ListAuditLog(r.Context(), service.AuditLogListParams{
				EntityType: entityType,
				EntityID:   entityID,
				Limit:      limit,
				Cursor:     q.Get("cursor"),
			})
			if err != nil {
				if errors.Is(err, service.ErrInvalidCursor) {
					mw.WriteError(w, http.StatusBadRequest, "INVALID_CURSOR", "The provided cursor is not valid")
					return
				}
				mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to query audit log")
				return
			}
			writeData(w, result)
			return
		}

		// Global query.
		var entityTypePtr *string
		if entityType != "" {
			entityTypePtr = &entityType
		}
		var actorType *string
		if v := q.Get("actor_type"); v != "" {
			actorType = &v
		}

		result, err := svc.ListAuditLogGlobal(r.Context(), service.AuditLogGlobalParams{
			EntityType: entityTypePtr,
			ActorType:  actorType,
			Limit:      limit,
			Cursor:     q.Get("cursor"),
		})
		if err != nil {
			if errors.Is(err, service.ErrInvalidCursor) {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_CURSOR", "The provided cursor is not valid")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to query audit log")
			return
		}

		writeData(w, result)
	}
}

// ListTransactionAuditLogHandler returns audit log for a specific transaction.
func ListTransactionAuditLogHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txnID := chi.URLParam(r, "id")
		q := r.URL.Query()

		limit := 50
		if v := q.Get("limit"); v != "" {
			parsed, err := strconv.Atoi(v)
			if err != nil || parsed < 1 || parsed > 200 {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "limit must be between 1 and 200")
				return
			}
			limit = parsed
		}

		result, err := svc.ListAuditLog(r.Context(), service.AuditLogListParams{
			EntityType: "transaction",
			EntityID:   txnID,
			Limit:      limit,
			Cursor:     q.Get("cursor"),
		})
		if err != nil {
			if errors.Is(err, service.ErrInvalidCursor) {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_CURSOR", "The provided cursor is not valid")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to query audit log")
			return
		}

		writeData(w, result)
	}
}

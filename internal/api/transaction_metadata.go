//go:build !lite

package api

import (
	"net/http"

	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// Transaction metadata REST surface. Four deliberately-scoped operations on the
// free-form `metadata` JSONB store. Each touches ONLY the metadata column — none
// can write a first-class field (category, tags, amount). Reads come back on the
// transaction itself (GET /transactions and GET /transactions/{id}); these are
// the write endpoints. All require full_access scope (RequireWriteScope group).

// SetTransactionMetadataKeyHandler upserts one metadata key.
// PATCH /transactions/{id}/metadata/{key}  body: {"value": <any JSON>}
func SetTransactionMetadataKeyHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		key := chi.URLParam(r, "key")
		var input struct {
			Value any `json:"value"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		if err := svc.SetTransactionMetadata(r.Context(), id, key, input.Value); err != nil {
			writeServiceError(w, err, "Transaction not found", "Failed to set transaction metadata")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// RemoveTransactionMetadataKeyHandler deletes one metadata key.
// DELETE /transactions/{id}/metadata/{key}
func RemoveTransactionMetadataKeyHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		key := chi.URLParam(r, "key")
		if err := svc.RemoveTransactionMetadata(r.Context(), id, key); err != nil {
			writeServiceError(w, err, "Transaction not found", "Failed to remove transaction metadata")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ReplaceTransactionMetadataHandler atomically replaces the entire metadata object.
// PUT /transactions/{id}/metadata  body: {"metadata": {...}}
func ReplaceTransactionMetadataHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var input struct {
			Metadata map[string]any `json:"metadata"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		if err := svc.ReplaceTransactionMetadata(r.Context(), id, input.Metadata); err != nil {
			writeServiceError(w, err, "Transaction not found", "Failed to replace transaction metadata")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ClearTransactionMetadataHandler resets metadata to the empty object.
// DELETE /transactions/{id}/metadata
func ClearTransactionMetadataHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.ClearTransactionMetadata(r.Context(), id); err != nil {
			writeServiceError(w, err, "Transaction not found", "Failed to clear transaction metadata")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

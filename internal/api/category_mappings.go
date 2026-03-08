package api

import (
	"encoding/json"
	"errors"
	"net/http"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// ListMappingsHandler returns category mappings, optionally filtered by provider.
func ListMappingsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provider := r.URL.Query().Get("provider")
		var providerPtr *string
		if provider != "" {
			providerPtr = &provider
		}

		mappings, err := svc.ListMappings(r.Context(), providerPtr)
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list mappings")
			return
		}

		writeData(w, mappings)
	}
}

type bulkUpsertMappingsRequest struct {
	Mappings []service.BulkMappingEntry `json:"mappings"`
}

// BulkUpsertMappingsHandler creates or updates multiple category mappings at once.
func BulkUpsertMappingsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input bulkUpsertMappingsRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body")
			return
		}

		if len(input.Mappings) == 0 {
			mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "mappings array is required and must not be empty")
			return
		}

		upserted, err := svc.BulkUpsertMappings(r.Context(), input.Mappings)
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to upsert mappings")
			return
		}

		writeData(w, map[string]int{"upserted": upserted})
	}
}

// DeleteMappingHandler deletes a single category mapping by ID.
func DeleteMappingHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		err := svc.DeleteMapping(r.Context(), id)
		if err != nil {
			if errors.Is(err, service.ErrMappingNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Mapping not found")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete mapping")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

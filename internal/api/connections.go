package api

import (
	"net/http"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// ListConnectionsHandler returns all connections, optionally filtered by user_id.
func ListConnectionsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.URL.Query().Get("user_id")
		var userIDPtr *string
		if userID != "" {
			userIDPtr = &userID
		}

		connections, err := svc.ListConnections(r.Context(), userIDPtr)
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list connections")
			return
		}

		writeData(w, connections)
	}
}

// GetConnectionStatusHandler returns the status of a single connection by ID.
func GetConnectionStatusHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		status, err := svc.GetConnectionStatus(r.Context(), id)
		if err != nil {
			writeServiceError(w, err, "Connection not found", "Failed to get connection status")
			return
		}

		writeData(w, status)
	}
}

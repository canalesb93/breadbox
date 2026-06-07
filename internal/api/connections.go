//go:build !lite

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

// GetConnectionHandler serves GET /api/v1/connections/{id}.
func GetConnectionHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		conn, err := svc.GetConnection(r.Context(), id)
		if err != nil {
			writeServiceError(w, err, "Connection not found", "Failed to get connection")
			return
		}

		writeData(w, conn)
	}
}

// DeleteConnectionHandler serves DELETE /api/v1/connections/{id}.
// Soft-disconnects the connection (status='disconnected', credentials wiped).
// Returns 204 on success, 404 if the connection is missing or already
// disconnected.
func DeleteConnectionHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		actor := service.ActorFromContext(r.Context())

		if err := svc.DeleteConnection(r.Context(), id, actor); err != nil {
			writeServiceError(w, err, "Connection not found", "Failed to delete connection")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// SyncConnectionHandler serves POST /api/v1/connections/{id}/sync —
// the per-connection variant of POST /sync.
func SyncConnectionHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		if err := svc.TriggerSync(r.Context(), &id); err != nil {
			writeServiceError(w, err, "Connection not found", "Failed to trigger sync")
			return
		}

		writeJSON(w, http.StatusAccepted, map[string]string{"status": "sync_triggered"})
	}
}

// PauseConnectionHandler serves POST /api/v1/connections/{id}/paused —
// body: {"paused": true|false}.
func PauseConnectionHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var body struct {
			Paused bool `json:"paused"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}

		actor := service.ActorFromContext(r.Context())
		conn, err := svc.UpdateConnectionPaused(r.Context(), id, body.Paused, actor)
		if err != nil {
			writeServiceError(w, err, "Connection not found", "Failed to update connection")
			return
		}

		writeData(w, conn)
	}
}

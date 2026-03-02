package api

import (
	"encoding/json"
	"errors"
	"net/http"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"
)

// TriggerSyncHandler triggers a manual sync of all connections, or a single connection if connection_id is provided.
func TriggerSyncHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var connectionID *string

		if r.Body != nil && r.ContentLength != 0 {
			var body struct {
				ConnectionID *string `json:"connection_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body")
				return
			}
			connectionID = body.ConnectionID
		}

		if err := svc.TriggerSync(r.Context(), connectionID); err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Connection not found")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to trigger sync")
			return
		}

		writeJSON(w, http.StatusAccepted, map[string]string{"status": "sync_triggered"})
	}
}

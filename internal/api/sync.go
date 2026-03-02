package api

import (
	"net/http"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"
)

// TriggerSyncHandler triggers a manual sync of all connections.
func TriggerSyncHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.TriggerSync(r.Context()); err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to trigger sync")
			return
		}

		writeJSON(w, http.StatusAccepted, map[string]string{"status": "sync_triggered"})
	}
}

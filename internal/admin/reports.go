package admin

import (
	"encoding/json"
	"net/http"

	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// MarkReportReadAdminHandler handles POST /-/reports/{id}/read.
func MarkReportReadAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.MarkAgentReportRead(r.Context(), id); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}
}

// MarkAllReportsReadAdminHandler handles POST /-/reports/read-all.
func MarkAllReportsReadAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.MarkAllAgentReportsRead(r.Context()); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}
}

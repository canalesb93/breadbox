package api

import (
	"net/http"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// ListReportsHandler handles GET /api/v1/reports.
func ListReportsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reports, err := svc.ListAgentReports(r.Context(), 50)
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list reports")
			return
		}
		writeData(w, reports)
	}
}

// UnreadReportCountHandler handles GET /api/v1/reports/unread-count.
func UnreadReportCountHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		count, err := svc.CountUnreadAgentReports(r.Context())
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to count unread reports")
			return
		}
		writeData(w, map[string]int64{"unread_count": count})
	}
}

// CreateReportHandler handles POST /api/v1/reports.
func CreateReportHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Title    string   `json:"title"`
			Body     string   `json:"body"`
			Priority string   `json:"priority"`
			Tags     []string `json:"tags"`
			Author   string   `json:"author"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		actor := service.ActorFromContext(r.Context())
		report, err := svc.CreateAgentReport(r.Context(), req.Title, req.Body, actor, req.Priority, req.Tags, req.Author, "")
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeData(w, report)
	}
}

// MarkReportReadHandler handles PATCH /api/v1/reports/{id}/read.
func MarkReportReadHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.MarkAgentReportRead(r.Context(), id); err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}
		writeData(w, map[string]bool{"ok": true})
	}
}

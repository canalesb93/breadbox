//go:build !lite

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
			writeServiceError(w, err, "", "Failed to create report")
			return
		}
		writeJSON(w, http.StatusCreated, report)
	}
}

// MarkReportReadHandler handles PATCH /api/v1/reports/{id}/read.
func MarkReportReadHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.MarkAgentReportRead(r.Context(), id); err != nil {
			writeServiceError(w, err, "Report not found", "Failed to mark report as read")
			return
		}
		writeData(w, map[string]bool{"ok": true})
	}
}

// GetReportHandler handles GET /api/v1/reports/{id}.
func GetReportHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		report, err := svc.GetAgentReport(r.Context(), id)
		if err != nil {
			writeServiceError(w, err, "Report not found", "Failed to get report")
			return
		}
		writeData(w, report)
	}
}

// MarkReportUnreadHandler handles PATCH /api/v1/reports/{id}/unread.
func MarkReportUnreadHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.MarkAgentReportUnread(r.Context(), id); err != nil {
			writeServiceError(w, err, "Report not found", "Failed to mark report as unread")
			return
		}
		writeData(w, map[string]bool{"ok": true})
	}
}

// MarkAllReportsReadHandler handles POST /api/v1/reports/read-all.
func MarkAllReportsReadHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.MarkAllAgentReportsRead(r.Context()); err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to mark all reports as read")
			return
		}
		writeData(w, map[string]bool{"ok": true})
	}
}

// DeleteReportHandler handles DELETE /api/v1/reports/{id}.
func DeleteReportHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.DeleteAgentReport(r.Context(), id); err != nil {
			writeServiceError(w, err, "Report not found", "Failed to delete report")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

package admin

import (
	"encoding/json"
	"net/http"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// ReportsPageHandler handles GET /reports.
func ReportsPageHandler(a *app.App, svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		statusFilter := r.URL.Query().Get("status") // "", "unread", "read"

		// Fetch reports based on filter.
		var rawReports []service.AgentReportResponse
		var err error
		switch statusFilter {
		case "unread":
			rawReports, err = svc.ListUnreadAgentReports(ctx, 100)
		default:
			rawReports, err = svc.ListAgentReports(ctx, 100)
		}
		if err != nil {
			a.Logger.Error("list agent reports", "error", err)
			http.Error(w, "Internal server error", 500)
			return
		}

		// Filter "read" in Go since we don't have a dedicated query for it.
		if statusFilter == "read" {
			var filtered []service.AgentReportResponse
			for _, r := range rawReports {
				if r.ReadAt != nil {
					filtered = append(filtered, r)
				}
			}
			rawReports = filtered
		}

		// Build template-friendly structs.
		type ReportItem struct {
			ID            string
			Title         string
			Body          string
			Priority      string
			Tags          []string
			DisplayAuthor string
			CreatedAt     string
			IsRead        bool
		}
		var reports []ReportItem
		for _, r := range rawReports {
			t, _ := time.Parse(time.RFC3339, r.CreatedAt)
			displayAuthor := r.CreatedByName
			if r.Author != nil && *r.Author != "" {
				displayAuthor = *r.Author
			}
			reports = append(reports, ReportItem{
				ID:            r.ID,
				Title:         r.Title,
				Body:          r.Body,
				Priority:      r.Priority,
				Tags:          r.Tags,
				DisplayAuthor: displayAuthor,
				CreatedAt:     relativeTime(t),
				IsRead:        r.ReadAt != nil,
			})
		}

		data := BaseTemplateData(r, sm, "reports", "Agent Reports")
		data["Reports"] = reports
		data["FilterStatus"] = statusFilter
		data["TotalCount"] = len(reports)
		tr.Render(w, r, "reports.html", data)
	}
}

// ReportDetailHandler handles GET /reports/{id}.
func ReportDetailHandler(a *app.App, svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		reportID := chi.URLParam(r, "id")
		if reportID == "" {
			tr.RenderNotFound(w, r)
			return
		}

		report, err := svc.GetAgentReport(ctx, reportID)
		if err != nil {
			a.Logger.Error("get agent report", "id", reportID, "error", err)
			tr.RenderNotFound(w, r)
			return
		}

		t, _ := time.Parse(time.RFC3339, report.CreatedAt)
		displayAuthor := report.CreatedByName
		if report.Author != nil && *report.Author != "" {
			displayAuthor = *report.Author
		}

		type ReportDetail struct {
			ID            string
			Title         string
			Body          string
			Priority      string
			Tags          []string
			DisplayAuthor string
			CreatedAt     string
			CreatedAtRel  string
			IsRead        bool
		}

		detail := ReportDetail{
			ID:            report.ID,
			Title:         report.Title,
			Body:          report.Body,
			Priority:      report.Priority,
			Tags:          report.Tags,
			DisplayAuthor: displayAuthor,
			CreatedAt:     t.Format("Jan 2, 2006 at 3:04 PM"),
			CreatedAtRel:  relativeTime(t),
			IsRead:        report.ReadAt != nil,
		}

		// Auto-mark as read when viewing.
		if report.ReadAt == nil {
			_ = svc.MarkAgentReportRead(ctx, reportID)
			detail.IsRead = true
		}

		data := BaseTemplateData(r, sm, "reports", report.Title)
		data["Report"] = detail
		data["Breadcrumbs"] = []Breadcrumb{
			{Label: "Reports", Href: "/reports"},
			{Label: report.Title},
		}
		tr.Render(w, r, "report_detail.html", data)
	}
}

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

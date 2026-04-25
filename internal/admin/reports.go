package admin

import (
	"net/http"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/service"
	"breadbox/internal/templates/components"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// reportDisplayAuthor returns the preferred display name for a report,
// falling back to the creator's actor name when the agent didn't set a custom author.
func reportDisplayAuthor(createdByName string, author *string) string {
	if author != nil && *author != "" {
		return *author
	}
	return createdByName
}

// ReportsPageHandler handles GET /reports.
func ReportsPageHandler(a *app.App, svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		statusFilter := r.URL.Query().Get("status") // "", "unread", "read"

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

		// "read" has no dedicated query; filter in Go.
		if statusFilter == "read" {
			var filtered []service.AgentReportResponse
			for _, r := range rawReports {
				if r.ReadAt != nil {
					filtered = append(filtered, r)
				}
			}
			rawReports = filtered
		}

		var reports []pages.ReportsItem
		for _, r := range rawReports {
			t, _ := time.Parse(time.RFC3339, r.CreatedAt)
			reports = append(reports, pages.ReportsItem{
				ID:            r.ID,
				Title:         r.Title,
				Body:          r.Body,
				Priority:      r.Priority,
				Tags:          r.Tags,
				DisplayAuthor: reportDisplayAuthor(r.CreatedByName, r.Author),
				CreatedAt:     relativeTime(t),
				IsRead:        r.ReadAt != nil,
			})
		}

		data := BaseTemplateData(r, sm, "reports", "Agent Reports")
		props := pages.ReportsProps{
			Reports:      reports,
			FilterStatus: statusFilter,
			TotalCount:   len(reports),
		}
		tr.RenderWithTempl(w, r, data, pages.Reports(props))
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

		detail := pages.ReportDetailReport{
			ID:            report.ID,
			Title:         report.Title,
			Body:          report.Body,
			Priority:      report.Priority,
			Tags:          report.Tags,
			DisplayAuthor: reportDisplayAuthor(report.CreatedByName, report.Author),
			CreatedAt:     t.Format("Jan 2, 2006 at 3:04 PM"),
			CreatedAtRel:  relativeTime(t),
			IsRead:        report.ReadAt != nil,
		}

		data := BaseTemplateData(r, sm, "reports", report.Title)
		// Use a short, fixed label for the current-page crumb. Agent report
		// titles can be full sentences; interpolating them into the breadcrumb
		// overflows on mobile/tablet and visually competes with the header
		// card below, which already shows the full title.
		breadcrumbs := []components.Breadcrumb{
			{Label: "Reports", Href: "/reports"},
			{Label: "Report"},
		}
		renderReportDetail(w, r, tr, data, pages.ReportDetailProps{
			Report:      detail,
			Breadcrumbs: breadcrumbs,
		})
	}
}

// renderReportDetail mirrors renderSyncLogDetail / renderPromptBuilder:
// hands the typed ReportDetailProps to the templ component and uses
// RenderWithTempl to host it inside base.html.
func renderReportDetail(w http.ResponseWriter, r *http.Request, tr *TemplateRenderer, data map[string]any, props pages.ReportDetailProps) {
	tr.RenderWithTempl(w, r, data, pages.ReportDetail(props))
}

// MarkReportReadAdminHandler handles POST /-/reports/{id}/read.
func MarkReportReadAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.MarkAgentReportRead(r.Context(), id); err != nil {
			writeError(w, http.StatusBadRequest, "MARK_READ_FAILED", err.Error())
			return
		}
		writeOK(w)
	}
}

// MarkReportUnreadAdminHandler handles POST /-/reports/{id}/unread.
func MarkReportUnreadAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.MarkAgentReportUnread(r.Context(), id); err != nil {
			writeError(w, http.StatusBadRequest, "MARK_UNREAD_FAILED", err.Error())
			return
		}
		writeOK(w)
	}
}

// MarkAllReportsReadAdminHandler handles POST /-/reports/read-all.
func MarkAllReportsReadAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.MarkAllAgentReportsRead(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		writeOK(w)
	}
}

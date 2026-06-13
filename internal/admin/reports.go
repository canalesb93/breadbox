//go:build !headless && !lite

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

		// Fetch the full (unfiltered) set once so the tab counts stay
		// accurate regardless of the active filter, then filter in Go for
		// the displayed list.
		all, err := svc.ListAgentReports(ctx, 100)
		if err != nil {
			a.Logger.Error("list agent reports", "error", err)
			http.Error(w, "Internal server error", 500)
			return
		}

		var unreadCount int
		for _, rep := range all {
			if rep.ReadAt == nil {
				unreadCount++
			}
		}

		reports := make([]components.ReportRowProps, 0, len(all))
		for _, rep := range all {
			isRead := rep.ReadAt != nil
			switch statusFilter {
			case "unread":
				if isRead {
					continue
				}
			case "read":
				if !isRead {
					continue
				}
			}
			t, _ := time.Parse(time.RFC3339, rep.CreatedAt)
			authorID := ""
			if rep.CreatedByID != nil {
				authorID = *rep.CreatedByID
			}
			reports = append(reports, components.ReportRowProps{
				ID:       rep.ID,
				Title:    rep.Title,
				Priority: rep.Priority,
				Author:   reportDisplayAuthor(rep.CreatedByName, rep.Author),
				AuthorID: authorID,
				IsAgent:  rep.CreatedByType == "agent",
				Time:     relativeTime(t),
				IsRead:   isRead,
			})
		}

		data := BaseTemplateData(r, sm, "reports", "Agent Reports")
		props := pages.ReportsProps{
			Reports:      reports,
			FilterStatus: statusFilter,
			AllCount:     len(all),
			UnreadCount:  unreadCount,
			ReadCount:    len(all) - unreadCount,
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
		// Render the absolute created-at clock in the viewer's timezone
		// (bb_tz cookie) rather than the server's — see UserLocation.
		loc := UserLocation(r)

		detail := pages.ReportDetailReport{
			ID:            report.ID,
			Title:         report.Title,
			Body:          report.Body,
			Priority:      report.Priority,
			DisplayAuthor: reportDisplayAuthor(report.CreatedByName, report.Author),
			CreatedAt:     t.In(loc).Format("Jan 2, 2006 at 3:04 PM"),
			CreatedAtRel:  relativeTime(t),
			IsRead:        report.ReadAt != nil,
		}

		data := BaseTemplateData(r, sm, "reports", report.Title)
		// Use a short, fixed label for the current-page crumb. Agent report
		// titles can be full sentences; interpolating them into the breadcrumb
		// overflows on mobile/tablet and visually competes with the header
		// card below, which already shows the full title.
		data["Breadcrumbs"] = []components.Breadcrumb{
			{Label: "Reports", Href: "/reports"},
			{Label: "Report"},
		}
		tr.RenderWithTempl(w, r, data, pages.ReportDetail(pages.ReportDetailProps{
			Report: detail,
		}))
	}
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

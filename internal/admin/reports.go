//go:build !headless && !lite

package admin

import (
	"net/http"
	"regexp"
	"strings"
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

// Markdown-stripping regexes for the inbox-row preview. Compiled once at
// package scope. Order matters: fenced code and images come out first,
// then link URLs (keeping the visible text), then line-leading markers.
var (
	reportPreviewFence    = regexp.MustCompile("(?s)```.*?```")
	reportPreviewRuleLine = regexp.MustCompile(`(?m)^[ \t]*[:\-|=_*\s]{3,}[ \t]*$`)
	reportPreviewImg      = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	reportPreviewLink     = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	reportPreviewInline   = regexp.MustCompile("`([^`]*)`")
	reportPreviewHeading  = regexp.MustCompile(`(?m)^\s{0,3}#{1,6}\s+`)
	reportPreviewQuote    = regexp.MustCompile(`(?m)^\s{0,3}>\s?`)
	reportPreviewListItem = regexp.MustCompile(`(?m)^\s*([-*+]|\d+\.)\s+`)
	// Emphasis is unwrapped by paired delimiters (keeping the inner text),
	// not by a blanket [*_~] strip — so snake_case identifiers and bare
	// arithmetic (file_name, 5*4) survive into the preview unscathed.
	reportPreviewBold   = regexp.MustCompile(`(\*\*|__)(.+?)(\*\*|__)`)
	reportPreviewItalic = regexp.MustCompile(`(^|[\s(])[*_]([^*_\n]+?)[*_]($|[\s).,!?;:])`)
	reportPreviewStrike = regexp.MustCompile(`~~(.+?)~~`)
	reportPreviewSpace  = regexp.MustCompile(`\s+`)
)

// reportPreview strips a markdown body down to a single line of plain
// text for the inbox-row preview. Stripping markup (rather than
// substringing raw markdown) keeps syntax characters and mid-token cuts
// out of the snippet. The result is whitespace-collapsed and truncated
// to ~140 chars on a word boundary.
func reportPreview(body string) string {
	s := body
	s = reportPreviewFence.ReplaceAllString(s, " ")
	s = reportPreviewRuleLine.ReplaceAllString(s, " ")
	s = reportPreviewImg.ReplaceAllString(s, " ")
	s = reportPreviewLink.ReplaceAllString(s, "$1")
	s = reportPreviewInline.ReplaceAllString(s, "$1")
	s = reportPreviewHeading.ReplaceAllString(s, "")
	s = reportPreviewQuote.ReplaceAllString(s, "")
	s = reportPreviewListItem.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "|", " ")
	s = reportPreviewBold.ReplaceAllString(s, "$2")
	s = reportPreviewItalic.ReplaceAllString(s, "$1$2$3")
	s = reportPreviewStrike.ReplaceAllString(s, "$1")
	s = reportPreviewSpace.ReplaceAllString(s, " ")
	return truncateWords(strings.TrimSpace(s), 140)
}

// truncateWords clips s to at most max runes, backing up to the last
// space so the snippet never ends mid-word, and appends an ellipsis.
func truncateWords(s string, max int) string {
	if len([]rune(s)) <= max {
		return s
	}
	cut := string([]rune(s)[:max])
	if i := strings.LastIndex(cut, " "); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,.;:—-") + "…"
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
			reports = append(reports, components.ReportRowProps{
				ID:       rep.ID,
				Title:    rep.Title,
				Preview:  reportPreview(rep.Body),
				Priority: rep.Priority,
				Tags:     rep.Tags,
				Author:   reportDisplayAuthor(rep.CreatedByName, rep.Author),
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
			Tags:          report.Tags,
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

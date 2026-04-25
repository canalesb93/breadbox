package pages

import "fmt"

// ReportsProps mirrors the field set the old reports.html read off the
// layout data map. The handler (admin/reports.go) builds these and the
// templ renders the agent-reports inbox without going through funcMap
// helpers.
type ReportsProps struct {
	Reports      []ReportsItem
	FilterStatus string // "", "unread", "read"
	TotalCount   int
}

// ReportsItem is a view-model wrapper around service.AgentReportResponse
// with the read-state and display author flattened so the templ does not
// have to call any admin helpers.
type ReportsItem struct {
	ID            string
	Title         string
	Body          string
	Priority      string // "", "warning", "critical"
	Tags          []string
	DisplayAuthor string
	CreatedAt     string // pre-rendered relative time
	IsRead        bool
}

// reportsTabClass returns the active/inactive class string for one of the
// All / Unread / Read filter tabs at the top of the inbox. Mirrors the
// `{{if eq .FilterStatus "x"}}…{{else}}…{{end}}` ternary in the source
// HTML.
func reportsTabClass(active bool) string {
	base := "px-2.5 sm:px-4 py-2.5 text-sm font-medium border-b-2 -mb-px transition-colors no-underline"
	if active {
		return base + " text-base-content border-primary"
	}
	return base + " text-base-content/40 border-transparent hover:text-base-content/60"
}

// reportsCountLabel renders "N report" / "N reports" — pluralized inline
// so the templ side stays declarative. Mirrors the
// `{{.TotalCount}} report{{if ne .TotalCount 1}}s{{end}}` pattern.
func reportsCountLabel(n int) string {
	if n == 1 {
		return fmt.Sprintf("%d report", n)
	}
	return fmt.Sprintf("%d reports", n)
}

// reportsEmptyTitle renders the "No reports" / "No unread reports" /
// "No read reports" header for the empty-state card, depending on the
// active status filter. Mirrors the inline ternary in the source HTML.
func reportsEmptyTitle(filterStatus string) string {
	switch filterStatus {
	case "unread":
		return "No unread reports"
	case "read":
		return "No read reports"
	default:
		return "No reports"
	}
}

// reportsEmptyMessage renders the supporting line under the empty-state
// title. The "unread" filter gets a friendlier "all caught up" copy;
// every other filter falls back to the generic prompt.
func reportsEmptyMessage(filterStatus string) string {
	if filterStatus == "unread" {
		return "You're all caught up. New agent messages will appear here."
	}
	return "Reports submitted by AI agents will appear here as messages."
}

// reportsTagHidden hides every meta tag past the first one on mobile.
// Mirrors the `{{if gt $i 0}}hidden sm:inline{{end}}` guard in the
// source HTML.
func reportsTagHidden(i int) string {
	if i > 0 {
		return "hidden sm:inline"
	}
	return ""
}

// reportsTitleClass picks the message-bubble paragraph color based on
// the report's read state. Read messages fade to /70; unread stay full
// brightness so they pop in the inbox.
func reportsTitleClass(isRead bool) string {
	if isRead {
		return "text-base-content/70"
	}
	return "text-base-content"
}

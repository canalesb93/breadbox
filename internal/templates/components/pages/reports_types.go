//go:build !headless && !lite

package pages

import "breadbox/internal/templates/components"

// ReportsProps mirrors the field set the handler (admin/reports.go)
// builds for the agent-reports inbox. The row view-model lives in the
// components package (components.ReportRowProps) so the inbox row is
// shared with the /design sandbox and any future recent-reports widget.
type ReportsProps struct {
	Reports      []components.ReportRowProps
	FilterStatus string // "", "unread", "read"
	// Per-tab counts shown as TabBar badges. Derived from the same
	// most-recent-100 unfiltered fetch the list is built from, so every
	// tab carries a consistent total regardless of the active filter.
	// (Accurate for the realistic bounded inbox; a household with >100
	// reports would see counts capped at the fetch window.)
	AllCount    int
	UnreadCount int
	ReadCount   int
}

// reportsCountPtr adapts an int count to the *int TabBarItem.Count slot,
// collapsing a zero count to nil so empty tabs render no badge instead
// of a noisy "0".
func reportsCountPtr(n int) *int {
	if n == 0 {
		return nil
	}
	v := n
	return &v
}

// reportsEmptyTitle renders the "No reports" / "No unread reports" /
// "No read reports" header for the empty-state card, depending on the
// active status filter.
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

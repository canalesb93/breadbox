//go:build !headless && !lite

package pages

import (
	"fmt"
	"strconv"

	"breadbox/internal/templates/components"
)

// partitionReportsByRead splits the (recency-ordered) report rows into an
// unread slice and a read slice, preserving the input order within each.
// The All view renders unread-first so the inbox surfaces what still needs
// attention before what's already been seen — the IA win over a flat list.
func partitionReportsByRead(rows []components.ReportRowProps) (unread, read []components.ReportRowProps) {
	for _, r := range rows {
		if r.IsRead {
			read = append(read, r)
		} else {
			unread = append(unread, r)
		}
	}
	return unread, read
}

// reportsGroupCount renders a group's row count for the label line.
func reportsGroupCount(n int) string {
	return strconv.Itoa(n)
}

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
	// At-a-glance: the agent contributing the most unread reports and how
	// many, used to render the quiet "N unread · M from <Agent>" context
	// line above the filter tabs. TopUnreadAgent is "" when no single agent
	// stands out (or there are no unread reports).
	TopUnreadAgent      string
	TopUnreadAgentCount int
}

// reportsGlance renders the quiet at-a-glance line above the filter tabs —
// plain muted text (not a hero/digest card), e.g. "8 unread · 3 from
// Review Agent". Returns "" when there's nothing unread, so the line only
// appears when it carries information. The "· M from <Agent>" clause is
// appended only when one agent clearly leads the unread pile (>= 2).
func reportsGlance(p ReportsProps) string {
	if p.UnreadCount <= 0 {
		return ""
	}
	line := fmt.Sprintf("%d unread", p.UnreadCount)
	if p.TopUnreadAgentCount >= 2 && p.TopUnreadAgent != "" {
		line += fmt.Sprintf(" · %d from %s", p.TopUnreadAgentCount, p.TopUnreadAgent)
	}
	return line
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

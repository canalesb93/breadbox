//go:build !headless && !lite

package pages

import "breadbox/internal/templates/components"

// design_reports_table_helpers.go holds the fixture rows for the
// SectionReportsTable sandbox entry. Kept beside the design types so the
// templ stays focused on layout. The variants exercise every axis of the
// reports index: the three priority tones in the status column, read vs
// unread (dot + weight), and a long summary that clamps to two lines.

// designReportRows returns the full fixture matrix passed to
// components.ReportsList in the sandbox.
func designReportRows() []components.ReportRowProps {
	return []components.ReportRowProps{
		designReportRowUnreadCritical(),
		designReportRowUnreadWarning(),
		designReportRowUnreadPlain(),
		designReportRowReadPlain(),
		designReportRowLongSummary(),
		designReportRowReadCritical(),
	}
}

func designReportRowUnreadCritical() components.ReportRowProps {
	return components.ReportRowProps{
		ID:       "rep_crit01",
		Title:    "3 duplicate subscriptions found totaling $47/mo",
		Priority: "critical",
		Author:   "Subscription watchdog",
		Time:     "12m ago",
		IsRead:   false,
	}
}

func designReportRowUnreadWarning() components.ReportRowProps {
	return components.ReportRowProps{
		ID:       "rep_warn01",
		Title:    "Dining spend is 38% over your monthly target",
		Priority: "warning",
		Author:   "Budget coach",
		Time:     "1h ago",
		IsRead:   false,
	}
}

func designReportRowUnreadPlain() components.ReportRowProps {
	return components.ReportRowProps{
		ID:       "rep_plain1",
		Title:    "Weekly recap: 42 transactions categorized, all matched",
		Priority: "",
		Author:   "Daily recap",
		Time:     "3h ago",
		IsRead:   false,
	}
}

func designReportRowReadPlain() components.ReportRowProps {
	return components.ReportRowProps{
		ID:       "rep_read01",
		Title:    "Monthly statement reconciled — no discrepancies",
		Priority: "",
		Author:   "Reconciler",
		Time:     "2d ago",
		IsRead:   true,
	}
}

func designReportRowLongSummary() components.ReportRowProps {
	return components.ReportRowProps{
		ID:       "rep_long01",
		Title:    "Heads up: your travel-rewards card annual fee of $550 posts on the 14th, and based on this year's redemptions it may no longer pay for itself",
		Priority: "warning",
		Author:   "Rewards analyst",
		Time:     "5h ago",
		IsRead:   false,
	}
}

func designReportRowReadCritical() components.ReportRowProps {
	return components.ReportRowProps{
		ID:       "rep_rc01",
		Title:    "Unusual $1,240 charge from a new merchant",
		Priority: "critical",
		Author:   "Fraud sentry",
		Time:     "3d ago",
		IsRead:   true,
	}
}

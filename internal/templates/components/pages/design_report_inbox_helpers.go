//go:build !headless && !lite

package pages

import "breadbox/internal/templates/components"

// design_report_inbox_helpers.go holds the fixture rows for the
// SectionReportInbox sandbox entry. Kept beside the design types so the
// templ stays focused on layout. The variants exercise every axis of
// ReportInboxRow: the three priority tones, read vs unread, a long
// headline that clamps to two lines, and a tag overflow.

func designReportRowUnreadCritical() components.ReportRowProps {
	return components.ReportRowProps{
		ID:       "rep_crit01",
		Title:    "3 duplicate subscriptions found totaling $47/mo",
		Preview:  "Netflix, Disney+ and Spotify each appear twice across two connected cards — likely a double-charge after the card swap in March.",
		Priority: "critical",
		Tags:     []string{"subscriptions", "anomaly"},
		Author:   "Subscription watchdog",
		Time:     "12m ago",
		IsRead:   false,
	}
}

func designReportRowUnreadWarning() components.ReportRowProps {
	return components.ReportRowProps{
		ID:       "rep_warn01",
		Title:    "Dining spend is 38% over your monthly target",
		Preview:  "You've spent $612 of a $440 budget with 9 days left in the cycle. Three large restaurant charges drove most of the overage.",
		Priority: "warning",
		Tags:     []string{"budget"},
		Author:   "Budget coach",
		Time:     "1h ago",
		IsRead:   false,
	}
}

func designReportRowUnreadPlain() components.ReportRowProps {
	return components.ReportRowProps{
		ID:       "rep_plain1",
		Title:    "Weekly recap: 42 transactions categorized, all matched",
		Preview:  "Nothing needed your attention this week. Income landed on schedule and every charge mapped to an existing rule.",
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
		Preview:  "Closing balances on all four accounts matched the provider statements to the cent.",
		Priority: "",
		Author:   "Reconciler",
		Time:     "2d ago",
		IsRead:   true,
	}
}

func designReportRowLongHeadline() components.ReportRowProps {
	return components.ReportRowProps{
		ID:       "rep_long01",
		Title:    "Heads up: your travel-rewards card annual fee of $550 posts on the 14th, and based on this year's redemptions it may no longer pay for itself",
		Preview:  "You redeemed $310 in travel credits over the last 12 months against a $550 fee — consider downgrading before the renewal date.",
		Priority: "warning",
		Tags:     []string{"cards"},
		Author:   "Rewards analyst",
		Time:     "5h ago",
		IsRead:   false,
	}
}

func designReportRowMultiTag() components.ReportRowProps {
	return components.ReportRowProps{
		ID:       "rep_tags01",
		Title:    "Unusual $1,240 charge from a new merchant",
		Preview:  "A first-seen merchant 'NORTHWIND LLC' charged $1,240 to your checking account — flagged because it's 6× your typical transaction size.",
		Priority: "critical",
		Tags:     []string{"anomaly", "checking", "merchant", "review"},
		Author:   "Fraud sentry",
		Time:     "just now",
		IsRead:   false,
	}
}

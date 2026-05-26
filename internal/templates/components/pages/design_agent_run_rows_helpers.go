//go:build !headless && !lite

package pages

import (
	"time"

	"breadbox/internal/templates/components"
)

// design_agent_run_rows_helpers.go holds the fixture rows for the
// SectionAgentRunRows sandbox entry. Kept beside the design types so
// the templ stays focused on layout. Times use a shared anchor (`now`
// via designAgentRunRowsNow) so refreshes don't reshuffle the
// relative-time labels mid-sentence.

func designAgentRunRowsNow() time.Time { return time.Now() }

func designAgentRunRowsAgo(d time.Duration) time.Time {
	return designAgentRunRowsNow().Add(-d)
}

func designAgentRunRowsFinished(d time.Duration) *time.Time {
	t := designAgentRunRowsAgo(d)
	return &t
}

// designAgentRunRowSuccess returns a clean, successful 30s run with
// modest cost — the "normal day" row.
func designAgentRunRowSuccess() components.AgentRunRowProps {
	return components.AgentRunRowProps{
		ShortID:    "ag42xyz9",
		AgentSlug:  "daily-recap",
		AgentName:  "Daily recap",
		Status:     "success",
		Trigger:    "cron",
		StartedAt:  designAgentRunRowsAgo(3 * time.Minute),
		FinishedAt: designAgentRunRowsFinished(2*time.Minute + 30*time.Second),
		DurationMs: 32_400,
		CostUSD:    0.0234,
		TokensIn:   8_421,
		TokensOut:  1_203,
		Turns:      4,
		ShowAgent:  true,
	}
}

// designAgentRunRowSuccessWithReport returns a success row that
// produced one info-priority report — the most common chip variant.
func designAgentRunRowSuccessWithReport() components.AgentRunRowProps {
	r := designAgentRunRowSuccess()
	r.ShortID = "ag99rstu"
	r.AgentSlug = "weekly-review"
	r.AgentName = "Weekly review"
	r.StartedAt = designAgentRunRowsAgo(48 * time.Minute)
	r.FinishedAt = designAgentRunRowsFinished(46 * time.Minute)
	r.DurationMs = 124_500
	r.CostUSD = 0.1873
	r.Turns = 11
	r.TokensIn = 32_104
	r.TokensOut = 4_872
	r.Reports = []components.AgentRunReportRef{
		{ShortID: "rp01abcd", Title: "Reviewed 47 transactions this week — 3 recategorized, no suspicious activity.", Priority: "info"},
	}
	return r
}

// designAgentRunRowError returns a failed run with an inline error
// message; the row should foreground the failure.
func designAgentRunRowError() components.AgentRunRowProps {
	return components.AgentRunRowProps{
		ShortID:              "ag77klmn",
		AgentSlug:            "anomaly-hunt",
		AgentName:            "Anomaly hunt",
		Status:               "error",
		Trigger:              "manual",
		StartedAt:            designAgentRunRowsAgo(14 * time.Minute),
		FinishedAt:           designAgentRunRowsFinished(13*time.Minute + 50*time.Second),
		DurationMs:           9_800,
		Turns:                2,
		ErrorMessage:         "Anthropic API: 529 overloaded — please retry shortly.",
		ErrorMessageFriendly: "",
		ShowAgent:            true,
	}
}

// designAgentRunRowInProgress returns a running row with no
// finished-at and the in-progress status pill.
func designAgentRunRowInProgress() components.AgentRunRowProps {
	return components.AgentRunRowProps{
		ShortID:   "ag88pqrs",
		AgentSlug: "pending-reviews",
		AgentName: "Pending reviews",
		Status:    "in_progress",
		Trigger:   "webhook",
		StartedAt: designAgentRunRowsAgo(35 * time.Second),
		Turns:     1,
		ShowAgent: true,
	}
}

// designAgentRunRowSkipped returns a quiet-hours / concurrency-locked
// row — opacity drops on the link so the row reads "audit only".
func designAgentRunRowSkipped() components.AgentRunRowProps {
	return components.AgentRunRowProps{
		ShortID:   "ag11uvwx",
		AgentSlug: "daily-recap",
		AgentName: "Daily recap",
		Status:    "skipped",
		Trigger:   "cron",
		StartedAt: designAgentRunRowsAgo(2 * time.Hour),
		FinishedAt: designAgentRunRowsFinished(2*time.Hour - time.Second),
		ShowAgent: true,
		Note:      "Skipped — another run was in progress.",
	}
}

// designAgentRunRowTimeout returns a run that bumped into the
// max_turns / max_budget ceiling.
func designAgentRunRowTimeout() components.AgentRunRowProps {
	return components.AgentRunRowProps{
		ShortID:    "ag22yzab",
		AgentSlug:  "categorize-batch",
		AgentName:  "Categorize batch",
		Status:     "timeout",
		Trigger:    "cron",
		StartedAt:  designAgentRunRowsAgo(6 * time.Hour),
		FinishedAt: designAgentRunRowsFinished(5*time.Hour + 50*time.Minute),
		DurationMs: 597_000,
		Turns:      25,
		HitCap:     "max_turns",
		CostUSD:    0.4901,
		TokensIn:   142_882,
		TokensOut:  18_433,
		ShowAgent:  true,
	}
}

// designAgentRunRowNoAgent flips ShowAgent off on any sample row so
// the sandbox can demonstrate the per-agent scoped variant.
func designAgentRunRowNoAgent(r components.AgentRunRowProps) components.AgentRunRowProps {
	r.ShowAgent = false
	return r
}

// designAgentRunRowWithReports returns a sample row carrying exactly
// one report whose priority matches the argument — the chip palette
// shifts per priority.
func designAgentRunRowWithReports(priority string) components.AgentRunRowProps {
	r := designAgentRunRowSuccess()
	switch priority {
	case "warning":
		r.ShortID = "ag55warn"
		r.AgentName = "Budget watch"
		r.Reports = []components.AgentRunReportRef{
			{ShortID: "rp02warn", Title: "Dining spend at 92% of monthly budget with 8 days left.", Priority: "warning"},
		}
	case "critical":
		r.ShortID = "ag66crit"
		r.AgentName = "Fraud sweep"
		r.Reports = []components.AgentRunReportRef{
			{ShortID: "rp03crit", Title: "Unusual $1,840 charge at 04:12 — verify with the cardholder.", Priority: "critical"},
		}
	default:
		r.ShortID = "ag44info"
		r.AgentName = "Weekly review"
		r.Reports = []components.AgentRunReportRef{
			{ShortID: "rp04info", Title: "Recategorized 12 transactions; nothing else flagged.", Priority: "info"},
		}
	}
	return r
}

// designAgentRunRowWithMultipleReports returns a row that produced
// several reports — chips wrap to a second line when the row gets
// narrow.
func designAgentRunRowWithMultipleReports() components.AgentRunRowProps {
	r := designAgentRunRowSuccess()
	r.ShortID = "agAA1234"
	r.AgentName = "Quarterly close"
	r.DurationMs = 412_000
	r.CostUSD = 0.7234
	r.Turns = 18
	r.Reports = []components.AgentRunReportRef{
		{ShortID: "rp10info", Title: "Q1 review complete — 312 transactions categorized.", Priority: "info"},
		{ShortID: "rp11warn", Title: "3 categories need a budget revision before Q2.", Priority: "warning"},
		{ShortID: "rp12crit", Title: "Possible duplicate Plaid feed for the Chase Sapphire connection.", Priority: "critical"},
	}
	return r
}

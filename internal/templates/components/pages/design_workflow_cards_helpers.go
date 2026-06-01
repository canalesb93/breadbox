//go:build !headless && !lite

package pages

import "time"

// design_workflow_cards_helpers.go holds the fixture props for the
// SectionWorkflowPresetCard sandbox entry — the /workflows gallery tile
// across its set-up / run states. Times use a shared `now` anchor so a
// refresh doesn't reshuffle the relative-time labels mid-comparison.

func designWFNow() time.Time                { return time.Now() }
func designWFAgo(d time.Duration) time.Time { return designWFNow().Add(-d) }

// designWFCardNotSetUp — a scheduled preset that hasn't been instantiated:
// gray tile, trigger summary on the left, "Set up" on the right, no kebab.
func designWFCardNotSetUp() WorkflowPresetCardProps {
	return WorkflowPresetCardProps{
		Slug:             "subscription-auditor",
		Name:             "Subscription Auditor",
		Description:      "Finds recurring charges and subscriptions, flagging price hikes and likely-forgotten ones.",
		Icon:             "repeat",
		TriggerLabel:     "Monthly",
		ScheduleCron:     "0 8 1 * *",
		EstCostPerRunUSD: 0.08,
		Enabled:          false,
	}
}

// designWFCardNotSetUpSync — a post-sync preset (no schedule), so the
// trigger label reads "After each sync". Still gray / not set up.
func designWFCardNotSetUpSync() WorkflowPresetCardProps {
	return WorkflowPresetCardProps{
		Slug:             "duplicate-charge-detector",
		Name:             "Duplicate Charge Detector",
		Description:      "Flags likely double-bills and gateway-retry duplicates so you can dispute them, right after each sync.",
		Icon:             "copy",
		TriggerLabel:     "After each sync",
		TriggerOnSync:    true,
		EstCostPerRunUSD: 0.03,
		Enabled:          false,
	}
}

// designWFCardRunning — set up + enabled, last run succeeded a few minutes
// ago: green tile, success pill + relative time, Run now + a checked toggle,
// and the Reconfigure kebab.
func designWFCardRunning() WorkflowPresetCardProps {
	return WorkflowPresetCardProps{
		Slug:            "routine-reviewer",
		Name:            "Routine Reviewer",
		Description:     "Auto-categorizes newly-synced transactions and flags anything it's unsure about.",
		Icon:            "sparkles",
		TriggerLabel:    "After each sync",
		TriggerOnSync:   true,
		Enabled:         true,
		WorkflowSlug:    "routine-reviewer",
		WorkflowEnabled: true,
		LastRun: &WorkflowLastRunProps{
			ShortID:    "wf42ab7c",
			Status:     "success",
			FinishedAt: designWFAgo(7 * time.Minute),
		},
	}
}

// designWFCardPaused — set up but the run toggle is off (unchecked). Still
// green (it's configured); the last successful run was a few days ago.
func designWFCardPaused() WorkflowPresetCardProps {
	c := designWFCardRunning()
	c.Slug = "backlog-closer"
	c.Name = "Backlog Closer"
	c.Icon = "list-checks"
	c.Description = "A weekly deep-clean of aged uncategorized transactions — thorough, and promotes repeat patterns to rules."
	c.TriggerOnSync = false
	c.TriggerLabel = "Weekly"
	c.WorkflowSlug = "backlog-closer"
	c.WorkflowEnabled = false
	c.LastRun = &WorkflowLastRunProps{
		ShortID:    "wf88dd2e",
		Status:     "success",
		FinishedAt: designWFAgo(3 * 24 * time.Hour),
	}
	return c
}

// designWFCardError — set up, last run failed: green tile, error pill.
func designWFCardError() WorkflowPresetCardProps {
	c := designWFCardRunning()
	c.Slug = "large-charge-sentinel"
	c.Name = "Large Charge Sentinel"
	c.Icon = "trending-up"
	c.Description = "Flags unusually large individual charges relative to your normal spending, right after each sync."
	c.WorkflowSlug = "large-charge-sentinel"
	c.LastRun = &WorkflowLastRunProps{
		ShortID:    "wf11ee9f",
		Status:     "error",
		FinishedAt: designWFAgo(20 * time.Minute),
	}
	return c
}

// designWFCardNeverRun — set up but never run: green tile, muted
// "Not run yet" instead of a last-run pill.
func designWFCardNeverRun() WorkflowPresetCardProps {
	c := designWFCardRunning()
	c.Slug = "monthly-close"
	c.Name = "Monthly Close"
	c.Icon = "calendar-check"
	c.Description = "A month-end summary of where the money went — by category and top merchants."
	c.TriggerOnSync = false
	c.TriggerLabel = "Monthly"
	c.WorkflowSlug = "monthly-close"
	c.LastRun = nil
	return c
}

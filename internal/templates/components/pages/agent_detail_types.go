//go:build !headless && !lite

package pages

import (
	"breadbox/internal/templates/components"
)

// AgentDetailProps is the typed view-model for the admin /agents/{slug}
// page — the per-agent detail/landing page. The handler in
// internal/admin/agent_detail_page.go assembles all derived values
// (relative-time strings, formatted money, schedule label) so this
// templ never reaches into time / service helpers directly.
type AgentDetailProps struct {
	// Identity + summary
	Slug                  string
	Name                  string
	Description           string // first non-empty line of Prompt
	Model                 string
	Enabled               bool
	TriggerOnSyncComplete bool

	// Schedule
	ScheduleCron    string // raw cron expression; empty when only sync-fire
	NextRunRelative string // pre-formatted "in 2h" / "—" when unscheduled

	// Configuration metadata for the info card
	ToolScope       string
	MaxTurns        int
	MaxBudgetUSD    float64 // 0 when unset (no budget cap)
	HasMaxBudget    bool
	QuietHoursStart string
	QuietHoursEnd   string
	AllowedTools    []string

	// Lifetime stats
	Stats AgentDetailStatsProps

	// Last N runs — typically the 10 most recent across all triggers.
	Recent []components.AgentRunRowProps

	// HasMoreRuns toggles the "View all runs" link footer on the runs
	// card. True whenever the agent has at least one run; the global
	// /agents view with ?agent=<slug> shows the full history.
	HasMoreRuns bool

	// CSRFToken powers the run-now / toggle endpoints when added later.
	CSRFToken string
}

// AgentDetailStatsProps mirrors service.AgentLifetimeStats for the templ
// side. Pre-formatted strings live on the props directly so the templ
// stays free of conversion helpers.
type AgentDetailStatsProps struct {
	RunCount           int
	SkippedCount       int
	ErrorCount         int
	TotalCostUSD       float64
	AvgDurationSeconds float64
}

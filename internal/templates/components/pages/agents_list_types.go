//go:build !headless && !lite

package pages

import "time"

// AgentsListProps is the typed view-model for the admin /agents page —
// the agent-definition list.
//
// The handler in internal/admin/agents_list_page.go flattens
// service.AgentDefinitionResponse into AgentsListRowProps so this templ
// never reaches into service types directly. Pre-rendering relative-time
// strings on the handler side keeps the templ free of time helpers.
type AgentsListProps struct {
	Agents []AgentsListRowProps
	Status AgentSubsystemStatusProps

	// LastPromptPrefixes is map[agent_slug] → most recent operator
	// prefix; surfaces the "Use last prefix" affordance in the shared
	// Run-an-agent modal. Empty map renders the modal without the
	// "Last prefix" chips.
	LastPromptPrefixes map[string]string

	CSRFToken string
}

// AgentsListRowProps is one row in the agents table. All time and money
// derivations happen on the handler — the templ just renders.
type AgentsListRowProps struct {
	Slug        string
	Name        string
	Description string // short blurb (derived from prompt first line)
	Model       string

	Enabled               bool
	TriggerOnSyncComplete bool

	ScheduleCron   string // raw cron expression; empty when only sync-fire
	SchedulePretty string // human-readable label (e.g. "Every day at 9 AM")

	LastRun *AgentsListLastRunProps

	NextRunRelative string // pre-formatted "in 2h" / "—" when unscheduled

	// Cost30dUSD is the 30-day rolling spend rolled up from CostStats30d on
	// the service side. Zero when the agent has no recent runs.
	Cost30dUSD float64
}

// AgentsListLastRunProps is the inline "last run" cell for a row.
type AgentsListLastRunProps struct {
	ShortID    string
	Status     string // success | error | in_progress | skipped
	Trigger    string // cron | webhook | manual | initial
	FinishedAt time.Time
	DurationMs int64
	CostUSD    float64
}

// AgentSubsystemStatusProps is the readiness banner state. Mirrors
// service.AgentSubsystemStatus but lives in the pages package so the
// templ doesn't import the service tree.
type AgentSubsystemStatusProps struct {
	Ready          bool
	AuthConfigured bool
	BinaryPresent  bool
	BinaryPath     string
}

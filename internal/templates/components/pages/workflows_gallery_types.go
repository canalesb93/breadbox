//go:build !headless && !lite

package pages

import (
	"fmt"
	"strings"
	"time"
)

// workflowConfigDrawerData builds the Alpine x-data object literal for the
// setup drawer, seeded from the preset's defaults. Mirrors the field set the
// reconfigure drawer hydrates from GET /config so both drawers share the
// trigger / schedule / model / advanced sub-templs. Values are quoted strings
// (form-friendly); cron falls back to a daily default when the preset has none.
func workflowConfigDrawerData(p WorkflowPresetCardProps) string {
	trigger := "false" // custom schedule
	if p.TriggerOnSync {
		trigger = "true"
	}
	cron := strings.TrimSpace(p.ScheduleCron)
	if cron == "" {
		cron = "0 8 * * *"
	}
	turns := p.MaxTurns
	if turns <= 0 {
		turns = 10
	}
	return fmt.Sprintf(
		"{ triggerOnSync: '%s', cron: '%s', model: '%s', maxTurns: '%d', maxBudget: '%s', consent: false }",
		trigger, cron, p.Model, turns, "1",
	)
}

// serverUTCOffsetMin returns the server's current UTC offset in minutes (east
// of UTC, matching time.Zone's sign), as a decimal string for a data-* attr.
// The workflows gallery embeds it on its root element so the cron shortcut
// pills can translate a friendly hour in the VIEWER's timezone (e.g. 9 AM
// "your time") into the SERVER-local cron the scheduler actually fires — the
// inverse of the shift service.DescribeCronInTZ applies for the live preview,
// so a pill round-trips. Read at the current instant, so it tracks the active
// DST offset for the near-term schedule being edited.
func serverUTCOffsetMin() string {
	_, off := time.Now().Zone()
	return fmt.Sprintf("%d", off/60)
}

// workflowBoolJS renders a Go bool as a JS boolean literal, for inline
// Alpine @click expressions (e.g. the reconfigure gear passing run-state).
func workflowBoolJS(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// workflowOpenConfigJS builds the @click for a not-set-up card's Set-up
// controls: open the per-preset config drawer and, for a custom-schedule
// preset, seed the live cron preview so it's populated before the first
// interaction. Post-sync presets skip the preview (no schedule shown).
func workflowOpenConfigJS(p WorkflowPresetCardProps) string {
	open := "$store.drawers.open('wf-config-" + p.Slug + "')"
	if p.TriggerOnSync {
		return open
	}
	cron := strings.TrimSpace(p.ScheduleCron)
	if cron == "" {
		cron = "0 8 * * *"
	}
	return open + "; describeCron('" + cron + "')"
}

// presetTileClasses returns the classes for a preset card's leading
// icon tile. Gray (neutral) by default; a green accent once the preset
// has been set up as a workflow, so a glance down the grid reads which
// automations are live. The shape (size, rounding, centering) is shared
// across both states.
// The tile is `relative` so an enabled card can pin a small status dot
// (last-run error) to its corner.
func presetTileClasses(enabled bool) string {
	const base = "relative flex items-center justify-center w-10 h-10 rounded-xl shrink-0 "
	if enabled {
		return base + "bg-success/15 text-success"
	}
	return base + "bg-base-200 text-base-content/55"
}

// WorkflowsGalleryProps is the view-model for the /workflows preset gallery.
type WorkflowsGalleryProps struct {
	Categories []WorkflowCategoryProps
	// Status mirrors the agent runtime readiness (reused from agents_list_types).
	Status    AgentSubsystemStatusProps
	CSRFToken string
	// ConsentAcknowledged is true once the household has acknowledged that
	// workflows run Claude over their ledger. When false, each configure
	// drawer shows a required consent checkbox gating the Enable button.
	ConsentAcknowledged bool
	// Spend drives the optional top-of-gallery spend-ceiling banner.
	Spend WorkflowSpendBanner
	// IsAdmin gates the "Set up" action: instantiating a workflow from a
	// preset is admin-only. Non-admins see a disabled control + hint.
	IsAdmin bool
}

// WorkflowSpendBanner is the gallery's spend-ceiling state: shown when a
// ceiling is set and 30-day spend is at/over 80% of it. Over=true means
// runs are currently paused (spent >= ceiling); otherwise it's an
// "approaching" warning. Strings are preformatted ("$2.72", "85%").
type WorkflowSpendBanner struct {
	Show       bool
	Over       bool
	SpentStr   string
	CeilingStr string
	PctStr     string
}

// WorkflowCategoryProps groups presets under a section header.
type WorkflowCategoryProps struct {
	Name    string
	Icon    string // lucide section icon
	Presets []WorkflowPresetCardProps
}

// WorkflowPresetCardProps is one preset row in the gallery.
type WorkflowPresetCardProps struct {
	Slug             string
	Name             string
	Description      string
	Icon             string  // lucide icon for the preset
	TriggerLabel     string  // human-readable trigger summary ("After each sync", "Weekly")
	ToolScope        string  // "read_only" | "read_write" — drives a small "applies changes" hint
	ScheduleCron     string  // default cron for scheduled presets (empty for post-sync)
	TriggerOnSync    bool    // default trigger: true = post-sync; user-switchable in the drawer
	EstCostPerRunUSD float64 // rough per-run cost estimate for the projected-cost hint

	// Model / MaxTurns seed the setup drawer's model select + Advanced
	// section. Resolved to non-empty/non-zero defaults by the page handler.
	Model    string
	MaxTurns int

	// Options are the preset's specialized configuration selects, rendered
	// in the configure drawer (e.g. apply-mode for categorization presets).
	Options []WorkflowPresetOptionProps

	// Enablement state.
	Enabled         bool   // the preset has been instantiated as a workflow
	WorkflowSlug    string // slug of the instantiated workflow (when Enabled)
	WorkflowEnabled bool   // the instantiated workflow's run toggle (when Enabled)

	// LastRun is the most recent run of the instantiated workflow, surfaced
	// inline on enabled cards as a "Last run" status + relative time. nil when
	// the workflow has never run (or isn't enabled). The "Run now" button uses
	// WorkflowSlug; this only drives the status line.
	LastRun *WorkflowLastRunProps
}

// WorkflowLastRunProps is the inline last-run summary on an enabled card: a
// status pill plus a relative-time link to the run-detail page. FinishedAt is
// the run's completion time (falling back to start time for in-progress runs),
// rendered via workflowsRelativeTime.
type WorkflowLastRunProps struct {
	ShortID    string // run short_id — deep-links to /workflows/runs/{shortID} when set
	Status     string // run status enum: success | error | in_progress | skipped
	FinishedAt time.Time
}

// WorkflowPresetOptionProps is one specialized option (a single-select) in
// the configure drawer.
type WorkflowPresetOptionProps struct {
	Key     string
	Label   string
	Help    string
	Default string // default choice Value (pre-selected)
	Choices []WorkflowPresetChoiceProps
}

// WorkflowPresetChoiceProps is one option value (the prompt Directive lives
// server-side; the drawer only needs the value + label).
type WorkflowPresetChoiceProps struct {
	Value string
	Label string
}

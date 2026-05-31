//go:build !headless && !lite

package pages

import (
	"fmt"
	"strconv"
	"time"

	"breadbox/internal/templates/components"
)

// workflowCostStr formats a per-run cost estimate as a 2-decimal string
// for the drawer's projected-cost hint (and as a literal arg into the
// reactive projectedCost() JS call).
func workflowCostStr(c float64) string {
	return strconv.FormatFloat(c, 'f', 2, 64)
}

// presetTileClasses returns the classes for a preset card's leading
// icon tile. Gray (neutral) by default; a green accent once the preset
// has been set up as a workflow, so a glance down the grid reads which
// automations are live. The shape (size, rounding, centering) is shared
// across both states.
func presetTileClasses(enabled bool) string {
	const base = "flex items-center justify-center w-10 h-10 rounded-xl shrink-0 "
	if enabled {
		return base + "bg-success/15 text-success"
	}
	return base + "bg-base-200 text-base-content/55"
}

// presetMenuItems builds the row's overflow ("⋯") menu — the secondary
// actions moved out of the inline row to declutter it: Preview prompt
// always, plus Reconfigure for an enabled workflow (admin only). The
// OnClick strings invoke the workflowsGallery Alpine factory methods and
// render inside the gallery's x-data root, so the handlers resolve.
func presetMenuItems(preset WorkflowPresetCardProps, isAdmin bool) []components.OverflowMenuItem {
	items := []components.OverflowMenuItem{{
		Label:   "Preview prompt",
		Icon:    "file-text",
		OnClick: fmt.Sprintf("previewPrompt('%s', '%s')", preset.Slug, preset.Name),
	}}
	if preset.Enabled && isAdmin {
		items = append(items, components.OverflowMenuItem{
			Label:   "Reconfigure",
			Icon:    "sliders-horizontal",
			OnClick: fmt.Sprintf("openReconfigure('%s', '%s')", preset.WorkflowSlug, preset.Name),
		})
	}
	return items
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
	TriggerOnSync    bool    // true = post-sync event trigger (no schedule editing)
	EstCostPerRunUSD float64 // rough per-run cost estimate for the projected-cost hint

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

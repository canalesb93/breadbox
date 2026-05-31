//go:build !headless && !lite

package pages

import "strconv"

// workflowCostStr formats a per-run cost estimate as a 2-decimal string
// for the drawer's projected-cost hint (and as a literal arg into the
// reactive projectedCost() JS call).
func workflowCostStr(c float64) string {
	return strconv.FormatFloat(c, 'f', 2, 64)
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

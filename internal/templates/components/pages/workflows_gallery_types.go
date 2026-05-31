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

	// Enablement state.
	Enabled         bool   // the preset has been instantiated as a workflow
	WorkflowSlug    string // slug of the instantiated workflow (when Enabled)
	WorkflowEnabled bool   // the instantiated workflow's run toggle (when Enabled)
}

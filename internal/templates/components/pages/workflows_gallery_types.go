//go:build !headless && !lite

package pages

// WorkflowsGalleryProps is the view-model for the /workflows preset gallery.
type WorkflowsGalleryProps struct {
	Categories []WorkflowCategoryProps
	// Status mirrors the agent runtime readiness (reused from agents_list_types).
	Status    AgentSubsystemStatusProps
	CSRFToken string
}

// WorkflowCategoryProps groups presets under a section header.
type WorkflowCategoryProps struct {
	Name    string
	Icon    string // lucide section icon
	Presets []WorkflowPresetCardProps
}

// WorkflowPresetCardProps is one preset row in the gallery.
type WorkflowPresetCardProps struct {
	Slug         string
	Name         string
	Description  string
	Icon         string // lucide icon for the preset
	TriggerLabel string // human-readable trigger summary ("After each sync", "Weekly")
	ToolScope    string // "read_only" | "read_write" — drives a small "applies changes" hint
	ScheduleCron string // default cron for scheduled presets (empty for post-sync)
	TriggerOnSync bool  // true = post-sync event trigger (no schedule editing)

	// Enablement state.
	Enabled         bool   // the preset has been instantiated as a workflow
	WorkflowSlug    string // slug of the instantiated workflow (when Enabled)
	WorkflowEnabled bool   // the instantiated workflow's run toggle (when Enabled)
}

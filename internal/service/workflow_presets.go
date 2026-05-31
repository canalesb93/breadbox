//go:build !lite

package service

import (
	"context"
	"fmt"
	"strings"
)

// WorkflowPreset is a code-defined, ready-to-enable Workflow template. Presets
// are NOT seeded database rows — the gallery renders them from this registry,
// and enabling one INSTANTIATES an agent_definition (tagged with source_template
// = the preset Slug). Each preset's base prompt is composed from the reusable
// blocks under prompts/agents/ so prompt content lives in one place.
type WorkflowPreset struct {
	Slug        string // stable identifier; also the slug of the instantiated workflow
	Name        string // user-facing title
	Category    string // gallery grouping
	Icon        string // lucide icon name
	Description string // one-line gallery card copy

	// PromptBlocks are prompt-block IDs (filenames under prompts/agents/ sans
	// .md) composed in order into the workflow's base prompt.
	PromptBlocks []string

	// Default run configuration applied when the preset is enabled.
	ToolScope             string // "read_only" | "read_write"
	Model                 string // empty = DefaultAgentModel
	MaxTurns              int    // 0 = DefaultAgentMaxTurns
	ScheduleCron          string // empty = no cron
	TriggerOnSyncComplete bool   // fire after each successful sync
}

// workflowPresets is the starter catalog. Order is the gallery display order.
// Grouped by Category. More presets land in later iterations (the 13-preset
// backlog); these three span the read↔write trust spectrum and all three
// trigger models (post-sync, cron-weekly, cron-monthly).
var workflowPresets = []WorkflowPreset{
	{
		Slug:        "routine-reviewer",
		Name:        "Routine Reviewer",
		Category:    "Categorization & Review",
		Icon:        "sparkles",
		Description: "Auto-categorizes newly-synced transactions and flags anything it's unsure about.",
		PromptBlocks: []string{
			"strategy-routine-review",
			"review-depth-efficient",
			"category-system",
		},
		ToolScope:             "read_write",
		TriggerOnSyncComplete: true,
	},
	{
		Slug:        "weekly-money-digest",
		Name:        "Weekly Money Digest",
		Category:    "Insights & Reports",
		Icon:        "bar-chart-3",
		Description: "A Monday-morning summary of last week's spending by category and top merchants.",
		PromptBlocks: []string{
			"strategy-spending-report",
		},
		ToolScope:    "read_only",
		ScheduleCron: "0 7 * * 1", // Mondays at 7:00
	},
	{
		Slug:        "subscription-auditor",
		Name:        "Subscription Auditor",
		Category:    "Insights & Reports",
		Icon:        "repeat",
		Description: "Finds recurring charges and subscriptions, flagging price hikes and likely-forgotten ones.",
		PromptBlocks: []string{
			"strategy-anomaly-detection",
			"merchant-analysis",
		},
		ToolScope:    "read_write",
		ScheduleCron: "0 8 1 * *", // 1st of the month at 08:00
	},
}

// WorkflowPresetView is a preset plus its enablement state, for the gallery.
type WorkflowPresetView struct {
	WorkflowPreset
	// Enabled is true when this preset has been instantiated as a workflow.
	Enabled bool `json:"enabled"`
	// WorkflowSlug is the slug of the instantiated workflow (when Enabled).
	WorkflowSlug *string `json:"workflow_slug,omitempty"`
	// WorkflowEnabled is the instantiated workflow's own enabled toggle
	// (a workflow can be instantiated but paused). Nil when not instantiated.
	WorkflowEnabled *bool `json:"workflow_enabled,omitempty"`
}

// presetBySlug returns the preset with the given slug.
func presetBySlug(slug string) (WorkflowPreset, bool) {
	for _, p := range workflowPresets {
		if p.Slug == slug {
			return p, true
		}
	}
	return WorkflowPreset{}, false
}

// composePresetPrompt concatenates the named prompt blocks into a single base
// prompt. Returns an error if any block ID is unknown — this is the eager
// validation that surfaces a typo at test time, not at a user's run.
func composePresetPrompt(blockIDs []string) (string, error) {
	blocks, err := loadPromptBlocks()
	if err != nil {
		return "", fmt.Errorf("load prompt blocks: %w", err)
	}
	byID := make(map[string]string, len(blocks))
	for _, b := range blocks {
		byID[b.ID] = b.Content
	}
	parts := make([]string, 0, len(blockIDs))
	for _, id := range blockIDs {
		content, ok := byID[id]
		if !ok {
			return "", fmt.Errorf("workflow preset references unknown prompt block %q", id)
		}
		parts = append(parts, strings.TrimSpace(content))
	}
	return strings.Join(parts, "\n\n"), nil
}

// ListWorkflowPresets returns the catalog annotated with enablement state by
// matching each preset against existing agent_definitions via source_template.
func (s *Service) ListWorkflowPresets(ctx context.Context) ([]WorkflowPresetView, error) {
	defs, err := s.ListAgentDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	bySource := make(map[string]AgentDefinitionResponse, len(defs))
	for _, d := range defs {
		if d.SourceTemplate != nil {
			bySource[*d.SourceTemplate] = d
		}
	}
	out := make([]WorkflowPresetView, 0, len(workflowPresets))
	for _, p := range workflowPresets {
		view := WorkflowPresetView{WorkflowPreset: p}
		if d, ok := bySource[p.Slug]; ok {
			view.Enabled = true
			slug := d.Slug
			en := d.Enabled
			view.WorkflowSlug = &slug
			view.WorkflowEnabled = &en
		}
		out = append(out, view)
	}
	return out, nil
}

// EnableWorkflowFromPresetParams carries optional overrides applied when a
// preset is instantiated. Empty fields fall back to the preset's defaults.
type EnableWorkflowFromPresetParams struct {
	// Enabled controls whether the instantiated workflow runs immediately.
	// Defaults to false (instantiated but paused) so a household can review it.
	Enabled bool
}

// EnableWorkflowFromPreset instantiates a workflow from a preset: it composes
// the base prompt, applies the preset defaults, stamps source_template, and
// creates the agent_definition. Returns ErrConflict if the preset is already
// enabled (one instance per preset).
func (s *Service) EnableWorkflowFromPreset(ctx context.Context, slug string, params EnableWorkflowFromPresetParams) (*AgentDefinitionResponse, error) {
	preset, ok := presetBySlug(slug)
	if !ok {
		return nil, fmt.Errorf("%w: unknown workflow preset %q", ErrNotFound, slug)
	}

	// One instance per preset. Reject a second enable so the gallery toggles an
	// existing workflow rather than duplicating it.
	views, err := s.ListWorkflowPresets(ctx)
	if err != nil {
		return nil, err
	}
	for _, v := range views {
		if v.Slug == slug && v.Enabled {
			return nil, fmt.Errorf("%w: workflow preset %q is already enabled", ErrConflict, slug)
		}
	}

	prompt, err := composePresetPrompt(preset.PromptBlocks)
	if err != nil {
		return nil, err
	}

	create := CreateAgentDefinitionParams{
		Name:                  preset.Name,
		Slug:                  preset.Slug,
		Prompt:                prompt,
		ToolScope:             preset.ToolScope,
		Model:                 preset.Model,
		MaxTurns:              preset.MaxTurns,
		Enabled:               params.Enabled,
		TriggerOnSyncComplete: preset.TriggerOnSyncComplete,
		SourceTemplate:        &preset.Slug,
	}
	if preset.ScheduleCron != "" {
		cron := preset.ScheduleCron
		create.ScheduleCron = &cron
	}
	return s.CreateAgentDefinition(ctx, create)
}

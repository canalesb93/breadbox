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

	// EstCostPerRunUSD is a rough per-run Anthropic-cost estimate, surfaced
	// as a "projected cost" hint in the configure drawer so a self-hoster
	// paying their own bill sees the order of magnitude before enabling.
	// Deliberately approximate — actual cost is recorded per run.
	EstCostPerRunUSD float64

	// Options are this preset's specialized configuration choices, rendered
	// as selects in the configure drawer (Mintlify "per-preset options").
	// The chosen choice's Directive is appended to the composed prompt.
	Options []WorkflowPresetOption
}

// WorkflowPresetOption is one preset-specific configuration choice (a
// single-select). The chosen choice's Directive (if any) is appended to
// the workflow's prompt under a heading named after the option's Label.
type WorkflowPresetOption struct {
	Key     string // form field name, e.g. "apply_mode"
	Label   string // drawer label + prompt heading
	Help    string // optional caption under the select
	Default string // default choice Value
	Choices []WorkflowPresetChoice
}

// WorkflowPresetChoice is one option value. Directive is the prompt text
// appended when this choice is selected (empty = the default behavior,
// no extra prompt).
type WorkflowPresetChoice struct {
	Value     string
	Label     string
	Directive string
}

// applyModeOption is the shared "auto-apply vs flag-only" choice for the
// categorization presets. Auto is the default (no extra directive); flag-only
// overrides the base prompt to suppress all category writes.
var applyModeOption = WorkflowPresetOption{
	Key:     "apply_mode",
	Label:   "Apply mode",
	Help:    "How it handles categories it's confident about.",
	Default: "auto",
	Choices: []WorkflowPresetChoice{
		{Value: "auto", Label: "Auto-apply categories", Directive: ""},
		{
			Value:     "flag_only",
			Label:     "Flag only — don't categorize",
			Directive: "APPLY MODE — FLAG ONLY: Do NOT set, change, or clear any transaction category, and do NOT call update_transactions to write a category. Your job in this mode is review-and-flag only: flag transactions that need a human's attention (and leave a brief comment explaining why), but leave all categorization decisions to the user.",
		},
	},
}

// resolveOptionDirectives returns the prompt text to append for a preset's
// chosen options. Each option resolves to a choice — the submitted value if
// valid, otherwise the option's Default — and any non-empty Directive is
// appended under a heading named for the option's Label.
func resolveOptionDirectives(preset WorkflowPreset, chosen map[string]string) string {
	var b strings.Builder
	for _, opt := range preset.Options {
		val := strings.TrimSpace(chosen[opt.Key])
		valid := false
		for _, ch := range opt.Choices {
			if ch.Value == val {
				valid = true
				break
			}
		}
		if !valid {
			val = opt.Default
		}
		for _, ch := range opt.Choices {
			if ch.Value == val && strings.TrimSpace(ch.Directive) != "" {
				b.WriteString("\n\n## ")
				b.WriteString(opt.Label)
				b.WriteString("\n\n")
				b.WriteString(ch.Directive)
			}
		}
	}
	return b.String()
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
		EstCostPerRunUSD:      0.02, // short, efficient per-sync review
		Options:               []WorkflowPresetOption{applyModeOption},
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
		ToolScope:        "read_only",
		ScheduleCron:     "0 7 * * 1", // Mondays at 7:00
		EstCostPerRunUSD: 0.05,        // reads a week of activity for the digest
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
		ToolScope:        "read_write",
		ScheduleCron:     "0 8 1 * *", // 1st of the month at 08:00
		EstCostPerRunUSD: 0.04,        // monthly recurring-charge scan
	},
	{
		Slug:        "backlog-closer",
		Name:        "Backlog Closer",
		Category:    "Categorization & Review",
		Icon:        "list-checks",
		Description: "A weekly deep-clean of aged uncategorized transactions — thorough, and promotes repeat patterns to rules.",
		PromptBlocks: []string{
			"strategy-bulk-review",
			"review-depth-thorough",
			"category-system",
		},
		ToolScope:        "read_write",
		ScheduleCron:     "0 7 * * 1", // Mondays at 07:00 (canonical "Weekly" — drawer-selectable)
		EstCostPerRunUSD: 0.08,        // thorough pass over an accumulated backlog
		Options:          []WorkflowPresetOption{applyModeOption},
	},
	{
		Slug:        "monthly-close",
		Name:        "Monthly Close",
		Category:    "Insights & Reports",
		Icon:        "calendar-check",
		Description: "A month-end summary of where the money went — by category and top merchants.",
		PromptBlocks: []string{
			"strategy-spending-report",
			"merchant-analysis",
		},
		ToolScope:        "read_only",
		ScheduleCron:     "0 8 1 * *", // 1st of the month at 08:00 (canonical "Monthly" — drawer-selectable)
		EstCostPerRunUSD: 0.07,        // a full month of activity + merchant breakdown
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
	// ScheduleCron overrides the preset's default cron for scheduled presets
	// (ignored for post-sync presets). nil = use the preset default.
	ScheduleCron *string
	// AdditionalInstructions, when non-empty, is appended to the composed base
	// prompt every run — the household's per-workflow tuning, mirroring
	// Mintlify's "additional prompt over the base prompt".
	AdditionalInstructions string
	// Options carries the chosen value for each of the preset's specialized
	// options (keyed by WorkflowPresetOption.Key). Unknown/missing keys fall
	// back to the option's Default. Chosen choices' Directives are appended
	// to the composed prompt.
	Options map[string]string
}

// maxAdditionalInstructions caps the per-workflow instruction tuning.
const maxAdditionalInstructions = 4000

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
	// Apply the preset's specialized options: each chosen choice's directive
	// is appended to the base prompt (auto/default choices have no directive).
	prompt += resolveOptionDirectives(preset, params.Options)
	// Append the household's per-workflow tuning to the base prompt.
	instr := strings.TrimSpace(params.AdditionalInstructions)
	if len(instr) > maxAdditionalInstructions {
		return nil, fmt.Errorf("%w: additional instructions exceed %d chars", ErrInvalidParameter, maxAdditionalInstructions)
	}
	if instr != "" {
		prompt = prompt + "\n\n## Additional instructions\n\n" + instr
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
	// Scheduled presets accept a cron override; post-sync presets keep their
	// event trigger (no cron).
	if !preset.TriggerOnSyncComplete {
		cron := preset.ScheduleCron
		if params.ScheduleCron != nil && strings.TrimSpace(*params.ScheduleCron) != "" {
			cron = strings.TrimSpace(*params.ScheduleCron)
		}
		if cron != "" {
			create.ScheduleCron = &cron
		}
	}
	return s.CreateAgentDefinition(ctx, create)
}

//go:build !lite

package service

import (
	"context"
	"errors"
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
	ToolScope             string  // "read_only" | "read_write"
	Model                 string  // empty = DefaultAgentModel
	MaxTurns              int     // 0 = DefaultAgentMaxTurns
	MaxBudgetUSD          float64 // 0 = DefaultAgentMaxBudgetUSD
	ScheduleCron          string  // empty = no cron
	TriggerOnSyncComplete bool    // fire after each successful sync

	// OneOff marks an on-demand workflow: it has no recurring trigger (no cron,
	// no post-sync) and runs only when a human hits "Run now". The gallery
	// renders these with copy/run/settings icon buttons instead of a run
	// toggle, and instantiation forces a manual-only definition regardless of
	// any trigger params. A one-off MUST leave both ScheduleCron and
	// TriggerOnSyncComplete in their zero state.
	OneOff bool

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
		{Value: "auto", Label: "Auto-apply", Directive: ""},
		{
			Value:     "flag_only",
			Label:     "Flag only",
			Directive: "APPLY MODE — FLAG ONLY: Do NOT set, change, or clear any transaction category, and do NOT call update_transactions to write a category. Your job in this mode is review-and-flag only: flag transactions that need a human's attention (and leave a brief comment explaining why), but leave all categorization decisions to the user.",
		},
	},
}

// ruleApplyModeOption is the "what to do with the rules it finds" choice for
// the Rule Foundation one-off. Both choices create the vetted rules (so future
// syncs auto-categorize); they differ only on the retroactive backfill. The
// default also applies each rule to the matching history; create-only suppresses
// the apply_rules sweep and leaves already-synced transactions untouched.
var ruleApplyModeOption = WorkflowPresetOption{
	Key:     "rule_mode",
	Label:   "Rule handling",
	Help:    "Both create the rules — choose whether to also backfill existing transactions.",
	Default: "create_apply",
	Choices: []WorkflowPresetChoice{
		{Value: "create_apply", Label: "Create & backfill", Directive: ""},
		{
			Value:     "create_only",
			Label:     "Create only",
			Directive: "RULE HANDLING — CREATE ONLY: Create the vetted rules as usual (dry-run each first), but do NOT call apply_rules — skip the backfill. Rules take effect on the next sync; leave existing transactions untouched. In your report, give each rule's dry-run match count and note that nothing was backfilled.",
		},
	},
}

// lookbackWindowOption is the shared "how far back to scan" choice for the
// alerts/anomaly presets. The default (7 days) carries no directive — the base
// strategy blocks already say "default: last 7 days"; the 30/90-day choices
// append a directive that widens the window the run reasons over.
var lookbackWindowOption = WorkflowPresetOption{
	Key:     "lookback_window",
	Label:   "Lookback window",
	Help:    "How far back each run scans for anomalies.",
	Default: "7",
	Choices: []WorkflowPresetChoice{
		{Value: "7", Label: "7 days", Directive: ""},
		{
			Value:     "30",
			Label:     "30 days",
			Directive: "LOOKBACK WINDOW — 30 DAYS: Scan the last 30 days, not the default 7. Wherever a step says \"last 7 days\", read it as \"last 30 days\". Establish baselines from the period immediately preceding the window so a one-month scan still has something to compare against.",
		},
		{
			Value:     "90",
			Label:     "90 days",
			Directive: "LOOKBACK WINDOW — 90 DAYS: Scan the last 90 days, not the default 7. Wherever a step says \"last 7 days\", read it as \"last 90 days\". This is a wide, catch-up sweep — prioritize the highest-signal findings and don't re-flag items a prior run already surfaced.",
		},
	},
}

// reportVerbosityOption is the shared "how much detail to write" choice for the
// alerts/anomaly presets. Concise is the default (no directive); detailed
// appends a directive asking for fuller evidence and reasoning in the report.
var reportVerbosityOption = WorkflowPresetOption{
	Key:     "report_verbosity",
	Label:   "Report verbosity",
	Help:    "How much detail each report carries.",
	Default: "concise",
	Choices: []WorkflowPresetChoice{
		{Value: "concise", Label: "Concise", Directive: ""},
		{
			Value:     "detailed",
			Label:     "Detailed",
			Directive: "REPORT VERBOSITY — DETAILED: Write a thorough report. For every flagged item include the full evidence trail (amounts, dates, merchant, account, and the baseline you compared against) and a one-line rationale. Open with a short summary of what you scanned and the headline count, then the itemized findings. When nothing is flagged, still summarize what you checked and why the window is clean.",
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
// Grouped by Category. The set spans the read↔write trust spectrum and all
// three trigger models (post-sync, cron-weekly, cron-monthly).
var workflowPresets = []WorkflowPreset{
	// ── Setup & Bulk ────────────────────────────────────────────────────────
	// On-demand one-offs (OneOff: true): no recurring trigger, run only when a
	// human clicks "Run now". They lead the gallery because they're the
	// get-started / catch-up actions — establish the rule foundation, then blitz
	// the backlog — before settling into the recurring automations below.
	{
		Slug:        "rule-foundation",
		Name:        "Rule Foundation",
		Category:    "Setup & Bulk",
		Icon:        "wand-sparkles",
		Description: "A one-time pass over your recent history to draft and carefully apply auto-categorization rules — so new transactions categorize themselves going forward.",
		PromptBlocks: []string{
			"strategy-rule-foundation",
			"category-system",
		},
		ToolScope: "read_write", // creates + applies rules (dry-run first)
		Model:     "claude-sonnet-4-6",
		OneOff:    true,
		// Foundational one-off: a single deep pass over 1000+ transactions that
		// drafts and applies rules. Turns stay unlimited (budget-bound) so a large
		// history isn't cut off mid-pass; the generous budget is the real ceiling.
		MaxBudgetUSD:     3.00,
		EstCostPerRunUSD: 0.50, // analyzes 1000+ transactions on Sonnet + drafts rules
		Options:          []WorkflowPresetOption{ruleApplyModeOption},
	},
	{
		Slug:        "bulk-catchup",
		Name:        "Bulk Catch-Up",
		Category:    "Setup & Bulk",
		Icon:        "layers",
		Description: "Auto-categorizes a large backlog of needs-review transactions in one fast pass, clearing the ones it's sure about and flagging only the rest.",
		PromptBlocks: []string{
			"strategy-bulk-catchup",
			"review-depth-efficient",
			"category-system",
		},
		ToolScope: "read_write", // categorizes + clears needs-review on resolved items
		Model:     "claude-haiku-4-5",
		OneOff:    true,
		// Batch one-off: clears a large backlog in one pass. Haiku keeps the
		// per-turn cost low, and turns stay unlimited so a deep backlog isn't
		// abandoned mid-pass — the forgiving budget is what bounds the run.
		MaxBudgetUSD:     2.00,
		EstCostPerRunUSD: 0.20, // hundreds–thousands of transactions on Haiku
		Options:          []WorkflowPresetOption{applyModeOption},
	},

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
		ToolScope:    "read_write",
		ScheduleCron: "0 7 * * 1", // Mondays at 07:00 (canonical "Weekly" — drawer-selectable)
		// Weekly deep-clean over an aged backlog — heavier than the per-sync
		// reviewer, so it gets a moderately higher budget; turns stay unlimited.
		MaxBudgetUSD:     1.50,
		EstCostPerRunUSD: 0.08, // thorough pass over an accumulated backlog
		Options:          []WorkflowPresetOption{applyModeOption},
	},
	{
		Slug:        "monthly-close",
		Name:        "Monthly Close",
		Category:    "Insights & Reports",
		Icon:        "calendar-check",
		Description: "A month-end summary of where the money went — by category and trend.",
		PromptBlocks: []string{
			"strategy-spending-report",
		},
		ToolScope:        "read_only",
		ScheduleCron:     "0 8 1 * *", // 1st of the month at 08:00 (canonical "Monthly" — drawer-selectable)
		EstCostPerRunUSD: 0.07,        // a full month of activity
	},

	// ── Alerts & Anomalies ──────────────────────────────────────────────────
	// Watchdogs that surface things worth a human's eyeballs. The sentinel flags
	// via a `needs-review` tag + a report and never recategorizes; it runs after
	// each sync for fast feedback and exposes the shared Lookback window + Report
	// verbosity options so a household can tune scan breadth and report detail.
	{
		Slug:        "large-charge-sentinel",
		Name:        "Large Charge Sentinel",
		Category:    "Alerts & Anomalies",
		Icon:        "trending-up",
		Description: "Flags unusually large individual charges relative to your normal spending, right after each sync.",
		PromptBlocks: []string{
			"strategy-large-charge-sentinel",
		},
		ToolScope:             "read_write", // tags flagged charges for human review
		TriggerOnSyncComplete: true,
		EstCostPerRunUSD:      0.03, // scans recent debits + a baseline pass per sync
		Options:               []WorkflowPresetOption{lookbackWindowOption, reportVerbosityOption},
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
			// An instantiated workflow can be renamed from the reconfigure
			// drawer; surface the live name on the card instead of the preset's
			// static label so the gallery matches the activity feed.
			if strings.TrimSpace(d.Name) != "" {
				view.Name = d.Name
			}
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
	// TriggerOnSync, when non-nil, overrides the preset's default trigger:
	// true = after each sync; false = custom schedule (uses ScheduleCron).
	// nil = use the preset's default. The trigger is user-selectable at
	// setup, not fixed by the preset.
	TriggerOnSync *bool
	// ScheduleCron overrides the cron when on a custom schedule. nil = use
	// the preset default (falling back to a daily default if the preset has
	// none — e.g. a post-sync preset switched to a custom schedule).
	ScheduleCron *string
	// Model overrides the run model. nil/empty = use the preset's model.
	Model *string
	// MaxTurns / MaxBudgetUSD override the Advanced caps. nil = preset/default.
	MaxTurns     *int
	MaxBudgetUSD *float64
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

	prompt, err := composeWorkflowPrompt(preset, params.Options, params.AdditionalInstructions)
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
	// Model override (Advanced / model select). Blank falls back to the preset.
	if params.Model != nil {
		if m := strings.TrimSpace(*params.Model); m != "" {
			create.Model = m
		}
	}
	// Preset budget default (e.g. the forgiving caps on the foundational/batch
	// one-offs). A non-zero preset budget seeds the instance; the form override
	// below still wins when the operator sets one explicitly.
	if preset.MaxBudgetUSD > 0 {
		b := preset.MaxBudgetUSD
		create.MaxBudgetUSD = &b
	}
	if params.MaxTurns != nil && *params.MaxTurns > 0 {
		create.MaxTurns = *params.MaxTurns
	}
	if params.MaxBudgetUSD != nil {
		create.MaxBudgetUSD = params.MaxBudgetUSD
	}

	if preset.OneOff {
		// One-off / on-demand: never scheduled, never post-sync. It runs only
		// when explicitly triggered (Run now), so we force a manual-only
		// definition and ignore any trigger/schedule overrides the form sent.
		create.TriggerOnSyncComplete = false
		create.ScheduleCron = nil
	} else {
		// Trigger is user-selectable at setup (custom schedule vs after-each-sync),
		// not fixed by the preset. The two modes are mutually exclusive — only a
		// custom schedule gets a cron, so a post-sync run never also fires on cron.
		triggerOnSync := preset.TriggerOnSyncComplete
		if params.TriggerOnSync != nil {
			triggerOnSync = *params.TriggerOnSync
		}
		create.TriggerOnSyncComplete = triggerOnSync
		if !triggerOnSync {
			cron := strings.TrimSpace(preset.ScheduleCron)
			if params.ScheduleCron != nil && strings.TrimSpace(*params.ScheduleCron) != "" {
				cron = strings.TrimSpace(*params.ScheduleCron)
			}
			if cron == "" {
				// A post-sync preset switched to a custom schedule has no preset
				// cron — seed a sensible daily default the drawer can override.
				cron = "0 8 * * *"
			}
			create.ScheduleCron = &cron
		}
	}
	return s.CreateAgentDefinition(ctx, create)
}

// EnsureOneOffWorkflow returns the instantiated definition for a one-off
// preset, instantiating it (manual-only, enabled) on first call and reusing it
// thereafter. It's the backing step for the gallery's "Run now" button on
// on-demand workflows: a one-off has no recurring trigger, so it must be
// instantiated before the orchestrator can run it, but the household never
// goes through an explicit "enable" first. Returns ErrNotFound for an unknown
// slug and ErrInvalidParameter when the slug is a recurring (non-one-off)
// preset.
func (s *Service) EnsureOneOffWorkflow(ctx context.Context, slug string) (*AgentDefinitionResponse, error) {
	preset, ok := presetBySlug(slug)
	if !ok {
		return nil, fmt.Errorf("%w: unknown workflow preset %q", ErrNotFound, slug)
	}
	if !preset.OneOff {
		return nil, fmt.Errorf("%w: workflow preset %q is not an on-demand workflow", ErrInvalidParameter, slug)
	}
	// Already instantiated? Reuse it (the definition slug == the preset slug).
	if def, err := s.GetAgentDefinition(ctx, slug); err == nil {
		return def, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	// First run: instantiate with the preset defaults (manual-only, enabled).
	return s.EnableWorkflowFromPreset(ctx, slug, EnableWorkflowFromPresetParams{Enabled: true})
}

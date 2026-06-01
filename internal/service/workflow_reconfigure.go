//go:build !lite

package service

import (
	"context"
	"fmt"
	"strings"
)

// additionalInstructionsHeading is the prompt marker under which the
// household's per-workflow tuning is appended. It must stay byte-identical
// to the suffix written by EnableWorkflowFromPreset so a reconfigure can
// split the tail back off and re-compose deterministically.
const additionalInstructionsHeading = "\n\n## Additional instructions\n\n"

// WorkflowConfigOption mirrors a preset's specialized option for the
// reconfigure drawer: the option metadata (key/label/help/choices) plus
// the currently-selected value derived from the live workflow's prompt.
type WorkflowConfigOption struct {
	Key      string                       `json:"key"`
	Label    string                       `json:"label"`
	Help     string                       `json:"help,omitempty"`
	Selected string                       `json:"selected"`
	Choices  []WorkflowConfigOptionChoice `json:"choices"`
}

// WorkflowConfigOptionChoice is one selectable value for an option.
type WorkflowConfigOptionChoice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// WorkflowConfig is the current configuration of an already-enabled
// workflow, shaped for the reconfigure drawer. It reports the live
// schedule, the appended additional-instructions tail, and the chosen
// value for each of the preset's specialized options — everything the
// drawer needs to render prefilled.
type WorkflowConfig struct {
	// Slug is the workflow (and preset) slug.
	Slug string `json:"slug"`
	// Name is the workflow's display name.
	Name string `json:"name"`
	// TriggerOnSync is true when the workflow fires after each sync (vs a
	// custom cron). The drawer's trigger radio is seeded from this; it's
	// now user-switchable per workflow, not fixed by the preset.
	TriggerOnSync bool `json:"trigger_on_sync"`
	// ScheduleCron is the live cron when on a custom schedule (empty when
	// trigger_on_sync). The drawer's schedule field is seeded from this.
	ScheduleCron string `json:"schedule_cron"`
	// Model is the workflow's current model id (drives the model select).
	Model string `json:"model"`
	// MaxTurns / MaxBudgetUSD seed the Advanced section. 0 / 0 mean "use
	// the default" (the drawer shows the effective default in that case).
	MaxTurns     int     `json:"max_turns"`
	MaxBudgetUSD float64 `json:"max_budget_usd"`
	// AdditionalInstructions is the per-workflow tuning currently appended
	// to the composed base prompt (empty when none).
	AdditionalInstructions string `json:"additional_instructions"`
	// Options carries every preset option with its currently-selected value.
	Options []WorkflowConfigOption `json:"options"`
}

// UpdateWorkflowConfigParams carries the reconfigure-drawer fields for an
// already-enabled workflow. It mirrors the enable-time configurable surface:
// schedule, additional-instructions tail, and chosen options. The base
// prompt blocks are NOT editable here (they're the preset's identity); only
// the household-tunable layers on top are.
type UpdateWorkflowConfigParams struct {
	// TriggerOnSync, when non-nil, switches the workflow's trigger:
	// true = after each sync (cron cleared); false = custom schedule (uses
	// ScheduleCron). nil leaves the trigger untouched. The two modes are
	// mutually exclusive — the scheduler keys off a non-empty cron and the
	// sync hook off the flag, so leaving both set would double-fire; the
	// service enforces the exclusivity here.
	TriggerOnSync *bool
	// ScheduleCron is the cron when on a custom schedule. nil = leave
	// untouched; applied only when the (resulting) trigger is a schedule.
	ScheduleCron *string
	// Model overrides the run model. nil/empty = leave untouched.
	Model *string
	// MaxTurns / MaxBudgetUSD override the Advanced caps. nil = leave
	// untouched.
	MaxTurns     *int
	MaxBudgetUSD *float64
	// AdditionalInstructions replaces the appended per-workflow tuning. An
	// empty string clears it.
	AdditionalInstructions string
	// Options carries the chosen value for each of the preset's options
	// (keyed by WorkflowPresetOption.Key). Unknown/missing keys fall back to
	// the option's Default, matching enable-time semantics.
	Options map[string]string
}

// presetForEnabledWorkflow resolves an enabled workflow by slug and returns
// both the live definition and the preset it was instantiated from. It is
// the shared guard for Get/UpdateWorkflowConfig: a workflow that wasn't
// instantiated from a known preset (hand-authored, or a retired preset
// slug) can't be reconfigured through this surface.
func (s *Service) presetForEnabledWorkflow(ctx context.Context, slug string) (*AgentDefinitionResponse, WorkflowPreset, error) {
	def, err := s.GetAgentDefinition(ctx, slug)
	if err != nil {
		// resolveAgentDefinition already returns ErrNotFound for an unknown slug.
		return nil, WorkflowPreset{}, err
	}
	if def.SourceTemplate == nil {
		return nil, WorkflowPreset{}, fmt.Errorf("%w: %q is not a preset-instantiated workflow", ErrInvalidState, slug)
	}
	preset, ok := presetBySlug(*def.SourceTemplate)
	if !ok {
		return nil, WorkflowPreset{}, fmt.Errorf("%w: workflow %q references unknown preset %q", ErrNotFound, slug, *def.SourceTemplate)
	}
	return def, preset, nil
}

// splitAdditionalInstructions separates a composed workflow prompt into its
// body (base + option directives) and the appended additional-instructions
// tail. Returns ("", body) when no tail marker is present. The marker is the
// exact suffix EnableWorkflowFromPreset writes, so the round-trip is lossless.
func splitAdditionalInstructions(prompt string) (instructions, body string) {
	idx := strings.LastIndex(prompt, additionalInstructionsHeading)
	if idx < 0 {
		return "", prompt
	}
	body = prompt[:idx]
	instructions = strings.TrimSpace(prompt[idx+len(additionalInstructionsHeading):])
	return instructions, body
}

// deriveChosenOptions reverse-engineers which option choices are currently
// active in a workflow's prompt body. A choice is detected by the presence
// of its (non-empty) directive text; an option whose active choice has no
// directive (the common "default/auto" case) resolves to its Default. This
// reads the body rather than the full prompt so trailing additional
// instructions can never be mistaken for a directive.
func deriveChosenOptions(preset WorkflowPreset, body string) map[string]string {
	chosen := make(map[string]string, len(preset.Options))
	for _, opt := range preset.Options {
		selected := opt.Default
		for _, ch := range opt.Choices {
			dir := strings.TrimSpace(ch.Directive)
			if dir != "" && strings.Contains(body, dir) {
				selected = ch.Value
				break
			}
		}
		chosen[opt.Key] = selected
	}
	return chosen
}

// GetWorkflowConfig returns the live configuration of an already-enabled
// workflow for the reconfigure drawer: its schedule, the appended
// additional-instructions tail, and the chosen value of each preset option,
// all derived from the instantiated definition. Resolves by slug.
func (s *Service) GetWorkflowConfig(ctx context.Context, slug string) (*WorkflowConfig, error) {
	def, preset, err := s.presetForEnabledWorkflow(ctx, slug)
	if err != nil {
		return nil, err
	}

	instructions, body := splitAdditionalInstructions(def.Prompt)
	chosen := deriveChosenOptions(preset, body)

	cfg := &WorkflowConfig{
		Slug:                   def.Slug,
		Name:                   def.Name,
		TriggerOnSync:          def.TriggerOnSyncComplete,
		Model:                  def.Model,
		MaxTurns:               def.MaxTurns,
		AdditionalInstructions: instructions,
	}
	if def.ScheduleCron != nil {
		cfg.ScheduleCron = *def.ScheduleCron
	}
	if def.MaxBudgetUSD != nil {
		cfg.MaxBudgetUSD = *def.MaxBudgetUSD
	}
	for _, opt := range preset.Options {
		co := WorkflowConfigOption{
			Key:      opt.Key,
			Label:    opt.Label,
			Help:     opt.Help,
			Selected: chosen[opt.Key],
		}
		for _, ch := range opt.Choices {
			co.Choices = append(co.Choices, WorkflowConfigOptionChoice{Value: ch.Value, Label: ch.Label})
		}
		cfg.Options = append(cfg.Options, co)
	}
	return cfg, nil
}

// composeWorkflowPrompt builds the full prompt for a preset from its base
// blocks, the chosen options' directives, and the household's additional
// instructions — byte-identical to what EnableWorkflowFromPreset assembles.
// Centralizing the assembly here keeps enable and reconfigure in lockstep.
func composeWorkflowPrompt(preset WorkflowPreset, options map[string]string, additionalInstructions string) (string, error) {
	prompt, err := composePresetPrompt(preset.PromptBlocks)
	if err != nil {
		return "", err
	}
	prompt += resolveOptionDirectives(preset, options)
	instr := strings.TrimSpace(additionalInstructions)
	if len(instr) > maxAdditionalInstructions {
		return "", fmt.Errorf("%w: additional instructions exceed %d chars", ErrInvalidParameter, maxAdditionalInstructions)
	}
	if instr != "" {
		prompt = prompt + additionalInstructionsHeading + instr
	}
	return prompt, nil
}

// UpdateWorkflowConfig re-composes and persists the configurable layers of an
// already-enabled workflow: schedule_cron (scheduled presets only), the
// chosen options' directives, and the appended additional-instructions tail.
// The base prompt blocks (the preset's identity) and run-state toggle are
// untouched — toggling runs stays on the enable/disable endpoints. Resolves
// by slug; returns the updated definition.
func (s *Service) UpdateWorkflowConfig(ctx context.Context, slug string, params UpdateWorkflowConfigParams) (*AgentDefinitionResponse, error) {
	def, preset, err := s.presetForEnabledWorkflow(ctx, slug)
	if err != nil {
		return nil, err
	}

	prompt, err := composeWorkflowPrompt(preset, params.Options, params.AdditionalInstructions)
	if err != nil {
		return nil, err
	}

	update := UpdateAgentDefinitionParams{Prompt: &prompt}

	// Trigger + schedule. The trigger is now user-switchable per workflow
	// (not fixed by the preset). The two modes are mutually exclusive — see
	// the UpdateWorkflowConfigParams.TriggerOnSync doc.
	switch {
	case params.TriggerOnSync != nil && *params.TriggerOnSync:
		// After each sync: set the flag, clear the cron (empty ptr → NULL via
		// TextPtrIfNotEmpty) so the scheduler doesn't also register it.
		t := true
		empty := ""
		update.TriggerOnSyncComplete = &t
		update.ScheduleCron = &empty
	case params.TriggerOnSync != nil:
		// Custom schedule: clear the sync flag, set the cron.
		f := false
		update.TriggerOnSyncComplete = &f
		if params.ScheduleCron != nil {
			if cron := strings.TrimSpace(*params.ScheduleCron); cron != "" {
				update.ScheduleCron = &cron
			}
		}
	default:
		// Trigger not specified — legacy cron-only update path. A bare
		// schedule applies only to a workflow already on a custom schedule;
		// a post-sync workflow ignores it (switching a post-sync workflow to
		// a schedule requires the explicit TriggerOnSync=false switch above,
		// which also clears the sync flag) so the two trigger modes can never
		// both be live.
		if !def.TriggerOnSyncComplete && params.ScheduleCron != nil {
			if cron := strings.TrimSpace(*params.ScheduleCron); cron != "" {
				update.ScheduleCron = &cron
			}
		}
	}

	if params.Model != nil {
		if m := strings.TrimSpace(*params.Model); m != "" {
			update.Model = &m
		}
	}
	if params.MaxTurns != nil {
		update.MaxTurns = params.MaxTurns
	}
	if params.MaxBudgetUSD != nil {
		update.MaxBudgetUSD = params.MaxBudgetUSD
	}

	return s.UpdateAgentDefinition(ctx, slug, update)
}

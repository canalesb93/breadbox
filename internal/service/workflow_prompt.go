//go:build !lite

package service

import (
	"context"
	"fmt"
)

// WorkflowPromptPreview is the composed base prompt for a preset, surfaced by
// the admin "Preview internal prompt" affordance. It's the exact base prompt a
// run starts from BEFORE per-workflow additional instructions are appended —
// the composed prompt blocks plus each option's default-choice directive.
type WorkflowPromptPreview struct {
	Slug   string `json:"slug"`
	Title  string `json:"title"`
	Prompt string `json:"prompt"`
}

// ComposeWorkflowPrompt returns the fully composed base prompt for a preset:
// the concatenated prompt blocks plus the directives for each option's DEFAULT
// choice. It reuses composePresetPrompt + resolveOptionDirectives — the same
// composition EnableWorkflowFromPreset performs — so the preview matches what a
// freshly-enabled workflow would run with (sans the household's per-workflow
// additional instructions, which are tuned at enable time).
//
// Passing a nil chosen map to resolveOptionDirectives makes every option fall
// back to its Default, which is the correct "what you'd get out of the box"
// preview.
func (s *Service) ComposeWorkflowPrompt(ctx context.Context, slug string) (*WorkflowPromptPreview, error) {
	preset, ok := presetBySlug(slug)
	if !ok {
		return nil, fmt.Errorf("%w: unknown workflow preset %q", ErrNotFound, slug)
	}

	prompt, err := composePresetPrompt(preset.PromptBlocks)
	if err != nil {
		return nil, err
	}
	// Append each option's default-choice directive (nil map ⇒ all defaults),
	// mirroring EnableWorkflowFromPreset's composition order.
	prompt += resolveOptionDirectives(preset, nil)

	return &WorkflowPromptPreview{
		Slug:   preset.Slug,
		Title:  preset.Name,
		Prompt: prompt,
	}, nil
}

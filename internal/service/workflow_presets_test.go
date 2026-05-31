//go:build !lite

package service

import "testing"

// TestWorkflowPresetsCompose is the eager-validation guard: every registered
// preset must reference real prompt blocks and produce a non-empty base prompt.
// A typo in a preset's PromptBlocks fails here (in the unit shard), not at a
// household's run.
func TestWorkflowPresetsCompose(t *testing.T) {
	if len(workflowPresets) == 0 {
		t.Fatal("workflowPresets registry is empty")
	}
	seen := map[string]bool{}
	for _, p := range workflowPresets {
		if p.Slug == "" || p.Name == "" || p.Category == "" {
			t.Errorf("preset %+v missing slug/name/category", p)
		}
		if seen[p.Slug] {
			t.Errorf("duplicate preset slug %q", p.Slug)
		}
		seen[p.Slug] = true
		if p.ToolScope != "read_only" && p.ToolScope != "read_write" {
			t.Errorf("preset %q has invalid tool_scope %q", p.Slug, p.ToolScope)
		}
		if len(p.PromptBlocks) == 0 {
			t.Errorf("preset %q has no prompt blocks", p.Slug)
		}
		prompt, err := composePresetPrompt(p.PromptBlocks)
		if err != nil {
			t.Errorf("preset %q failed to compose prompt: %v", p.Slug, err)
			continue
		}
		if len(prompt) == 0 {
			t.Errorf("preset %q composed an empty prompt", p.Slug)
		}
	}
}

func TestComposePresetPrompt_UnknownBlock(t *testing.T) {
	if _, err := composePresetPrompt([]string{"definitely-not-a-real-block"}); err == nil {
		t.Fatal("composePresetPrompt with an unknown block returned nil error")
	}
}

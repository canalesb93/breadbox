//go:build !lite

package service

import (
	"strings"
	"testing"
)

// T1-presets-compose: unit tests for the workflowPresets registry. No DB
// required — all assertions operate on the in-process registry and the
// embedded prompts.FS.

// knownPresetCategories is the authoritative set of gallery category strings.
// Update this list whenever a new category is added to workflow_presets.go.
var knownPresetCategories = map[string]bool{
	"Setup & Bulk":            true,
	"Categorization & Review": true,
	"Enrichment":              true,
	"Insights & Reports":      true,
	"Alerts & Anomalies":      true,
}

// T1RegistryNonEmpty asserts that the preset registry contains at least one
// entry (a totally empty registry is a silent broken build).
func TestT1RegistryNonEmpty(t *testing.T) {
	if len(workflowPresets) == 0 {
		t.Fatal("T1: workflowPresets registry is empty")
	}
}

// TestT1SlugsUniqueNonEmpty asserts that every preset has a non-empty Slug and
// that no two presets share a Slug. Duplicate slugs would cause the second
// enable to silently shadow the first in EnableWorkflowFromPreset.
func TestT1SlugsUniqueNonEmpty(t *testing.T) {
	seen := make(map[string]int) // slug → first index
	for i, p := range workflowPresets {
		if p.Slug == "" {
			t.Errorf("T1: preset[%d] has empty Slug", i)
			continue
		}
		if first, ok := seen[p.Slug]; ok {
			t.Errorf("T1: duplicate Slug %q: first at index %d, also at index %d", p.Slug, first, i)
		} else {
			seen[p.Slug] = i
		}
	}
}

// TestT1NameNonEmpty asserts that every preset has a non-empty Name (the
// user-facing gallery card title).
func TestT1NameNonEmpty(t *testing.T) {
	for i, p := range workflowPresets {
		if strings.TrimSpace(p.Name) == "" {
			t.Errorf("T1: preset[%d] (slug=%q) has empty Name", i, p.Slug)
		}
	}
}

// TestT1CategoryValid asserts that every preset's Category is one of the known
// gallery grouping strings. An unexpected category won't appear in the
// gallery unless the frontend also adds a column for it.
func TestT1CategoryValid(t *testing.T) {
	for _, p := range workflowPresets {
		if !knownPresetCategories[p.Category] {
			t.Errorf("T1: preset %q has unknown Category %q (known: %v)", p.Slug, p.Category, knownPresetCategoryList())
		}
	}
}

// TestT1TriggerValid asserts that every recurring preset has at least one
// trigger defined (TriggerOnSyncComplete=true OR a non-empty ScheduleCron). A
// one-off (OneOff=true) is exempt — it deliberately has neither and fires only
// via Run now; conversely it must NOT carry a recurring trigger.
func TestT1TriggerValid(t *testing.T) {
	for _, p := range workflowPresets {
		if p.OneOff {
			if p.TriggerOnSyncComplete || strings.TrimSpace(p.ScheduleCron) != "" {
				t.Errorf("T1: one-off preset %q must have no recurring trigger (TriggerOnSyncComplete=%v, ScheduleCron=%q)", p.Slug, p.TriggerOnSyncComplete, p.ScheduleCron)
			}
			continue
		}
		hasTrigger := p.TriggerOnSyncComplete || strings.TrimSpace(p.ScheduleCron) != ""
		if !hasTrigger {
			t.Errorf("T1: preset %q has no trigger (TriggerOnSyncComplete=false and ScheduleCron=%q)", p.Slug, p.ScheduleCron)
		}
	}
}

// TestT1TriggerMutuallyExclusive asserts that no preset mixes TriggerOnSyncComplete
// and a ScheduleCron, since EnableWorkflowFromPreset ignores the schedule for
// post-sync presets — a filled ScheduleCron on a post-sync preset is dead code
// and signals a copy-paste error.
func TestT1TriggerMutuallyExclusive(t *testing.T) {
	for _, p := range workflowPresets {
		if p.TriggerOnSyncComplete && strings.TrimSpace(p.ScheduleCron) != "" {
			t.Errorf("T1: preset %q sets both TriggerOnSyncComplete and ScheduleCron=%q; EnableWorkflowFromPreset ignores the cron for post-sync presets", p.Slug, p.ScheduleCron)
		}
	}
}

// TestT1ToolScopeValid asserts that every preset's ToolScope is either "read_only"
// or "read_write" — the two values accepted by CreateAgentDefinition.
func TestT1ToolScopeValid(t *testing.T) {
	for _, p := range workflowPresets {
		switch p.ToolScope {
		case "read_only", "read_write":
			// valid
		default:
			t.Errorf("T1: preset %q has invalid ToolScope %q (want read_only or read_write)", p.Slug, p.ToolScope)
		}
	}
}

// TestT1PromptBlocksNonEmpty asserts that every preset declares at least one
// prompt block. A preset with no blocks would compose an empty prompt.
func TestT1PromptBlocksNonEmpty(t *testing.T) {
	for _, p := range workflowPresets {
		if len(p.PromptBlocks) == 0 {
			t.Errorf("T1: preset %q declares no PromptBlocks", p.Slug)
		}
	}
}

// TestT1PromptBlocksExist asserts that every block ID named in every preset's
// PromptBlocks list corresponds to a real file under prompts/agents/. An
// unknown block ID would cause composePresetPrompt to return an error at
// runtime (and EnableWorkflowFromPreset to fail for every household).
func TestT1PromptBlocksExist(t *testing.T) {
	blocks, err := loadPromptBlocks()
	if err != nil {
		t.Fatalf("T1: loadPromptBlocks: %v", err)
	}
	known := make(map[string]bool, len(blocks))
	for _, b := range blocks {
		known[b.ID] = true
	}

	for _, p := range workflowPresets {
		for _, id := range p.PromptBlocks {
			if !known[id] {
				t.Errorf("T1: preset %q references unknown prompt block %q", p.Slug, id)
			}
		}
	}
}

// TestT1ComposePromptNonEmpty asserts that every preset produces a
// non-empty composed prompt. This is the critical invariant: if composition
// succeeds but returns empty text the workflow runs with a blank system
// prompt, defeating the entire preset mechanism.
func TestT1ComposePromptNonEmpty(t *testing.T) {
	for _, p := range workflowPresets {
		got, err := composePresetPrompt(p.PromptBlocks)
		if err != nil {
			t.Errorf("T1: preset %q composePresetPrompt error: %v", p.Slug, err)
			continue
		}
		if strings.TrimSpace(got) == "" {
			t.Errorf("T1: preset %q composePresetPrompt returned empty prompt", p.Slug)
		}
	}
}

// TestT1ComposePromptContainsBlocks asserts that each block contributing to a
// preset's composed prompt has non-trivial content (at least 10 non-whitespace
// characters). A nearly-empty block file would silently degrade the prompt.
func TestT1ComposePromptContainsBlocks(t *testing.T) {
	blocks, err := loadPromptBlocks()
	if err != nil {
		t.Fatalf("T1: loadPromptBlocks: %v", err)
	}
	byID := make(map[string]string, len(blocks))
	for _, b := range blocks {
		byID[b.ID] = b.Content
	}

	for _, p := range workflowPresets {
		for _, id := range p.PromptBlocks {
			content, ok := byID[id]
			if !ok {
				// Already caught by TestT1PromptBlocksExist; skip to avoid redundant noise.
				continue
			}
			if len(strings.TrimSpace(content)) < 10 {
				t.Errorf("T1: preset %q block %q has suspiciously short content (%d chars)", p.Slug, id, len(strings.TrimSpace(content)))
			}
		}
	}
}

// TestT1EstCostPositive asserts that every preset has a positive
// EstCostPerRunUSD. The cost estimate is surfaced in the configure drawer so
// a self-hoster sees the order of magnitude before enabling; a zero or
// negative value means the hint is absent or misleading.
func TestT1EstCostPositive(t *testing.T) {
	for _, p := range workflowPresets {
		if p.EstCostPerRunUSD <= 0 {
			t.Errorf("T1: preset %q has non-positive EstCostPerRunUSD=%v", p.Slug, p.EstCostPerRunUSD)
		}
	}
}

// TestT1OptionsWellFormed asserts that every WorkflowPresetOption on every
// preset is internally consistent: Key and Label are non-empty, at least one
// Choice exists, Default matches a real choice Value, and every Choice has
// a non-empty Value and Label.
func TestT1OptionsWellFormed(t *testing.T) {
	for _, p := range workflowPresets {
		for oi, opt := range p.Options {
			if strings.TrimSpace(opt.Key) == "" {
				t.Errorf("T1: preset %q option[%d] has empty Key", p.Slug, oi)
			}
			if strings.TrimSpace(opt.Label) == "" {
				t.Errorf("T1: preset %q option[%d] (key=%q) has empty Label", p.Slug, oi, opt.Key)
			}
			if len(opt.Choices) == 0 {
				t.Errorf("T1: preset %q option[%d] (key=%q) has no Choices", p.Slug, oi, opt.Key)
				continue
			}
			// Default must be a known choice Value.
			defaultFound := false
			for ci, ch := range opt.Choices {
				if strings.TrimSpace(ch.Value) == "" {
					t.Errorf("T1: preset %q option[%d] choice[%d] has empty Value", p.Slug, oi, ci)
				}
				if strings.TrimSpace(ch.Label) == "" {
					t.Errorf("T1: preset %q option[%d] choice[%d] (value=%q) has empty Label", p.Slug, oi, ci, ch.Value)
				}
				if ch.Value == opt.Default {
					defaultFound = true
				}
			}
			if !defaultFound {
				t.Errorf("T1: preset %q option[%d] (key=%q) Default=%q does not match any choice Value", p.Slug, oi, opt.Key, opt.Default)
			}
		}
	}
}

// TestT1OptionKeyUniquenessPerPreset asserts that within a single preset,
// no two options share the same Key. Duplicate keys would cause the last
// one to silently shadow earlier ones in resolveOptionDirectives.
func TestT1OptionKeyUniquenessPerPreset(t *testing.T) {
	for _, p := range workflowPresets {
		seen := make(map[string]int)
		for oi, opt := range p.Options {
			if first, ok := seen[opt.Key]; ok {
				t.Errorf("T1: preset %q has duplicate option Key %q at indices %d and %d", p.Slug, opt.Key, first, oi)
			} else {
				seen[opt.Key] = oi
			}
		}
	}
}

// TestT1ResolveOptionDirectivesDefault asserts that resolveOptionDirectives
// does not panic and produces consistent output when called with a nil / empty
// chosen map (i.e. every option falls back to its Default).
func TestT1ResolveOptionDirectivesDefault(t *testing.T) {
	for _, p := range workflowPresets {
		// Should not panic regardless of Options length.
		got := resolveOptionDirectives(p, nil)

		// Also exercise with an explicit empty map.
		got2 := resolveOptionDirectives(p, map[string]string{})
		if got != got2 {
			t.Errorf("T1: preset %q resolveOptionDirectives(nil) != resolveOptionDirectives({})", p.Slug)
		}
	}
}

// TestT1ResolveOptionDirectivesUnknownFallsBack asserts that an unknown choice
// value triggers a fall-back to the option's Default rather than returning an
// error or an empty directive when the default has one.
func TestT1ResolveOptionDirectivesUnknownFallsBack(t *testing.T) {
	for _, p := range workflowPresets {
		chosen := make(map[string]string)
		for _, opt := range p.Options {
			chosen[opt.Key] = "definitely-not-a-real-choice-value"
		}
		// Must not panic. The result equals the output for the default selections.
		withUnknown := resolveOptionDirectives(p, chosen)
		withDefault := resolveOptionDirectives(p, nil)
		if withUnknown != withDefault {
			t.Errorf("T1: preset %q: resolveOptionDirectives with unknown value differs from default fallback", p.Slug)
		}
	}
}

// TestT1ComposePresetPromptWithFlagOnlyOption asserts that composing a preset
// prompt for a preset that has an apply_mode option and choosing "flag_only"
// appends the FLAG ONLY directive text. This validates the option/directive
// pipeline end-to-end without a DB.
func TestT1ComposePresetPromptWithFlagOnlyOption(t *testing.T) {
	// Find a preset that has the apply_mode option (routine-reviewer and
	// backlog-closer both do; we test the first one we encounter).
	var target *WorkflowPreset
	for i := range workflowPresets {
		for _, opt := range workflowPresets[i].Options {
			if opt.Key == "apply_mode" {
				target = &workflowPresets[i]
				break
			}
		}
		if target != nil {
			break
		}
	}
	if target == nil {
		t.Skip("T1: no preset with apply_mode option found; skipping directive test")
	}

	base, err := composePresetPrompt(target.PromptBlocks)
	if err != nil {
		t.Fatalf("T1: composePresetPrompt(%q): %v", target.Slug, err)
	}

	flagOnly := resolveOptionDirectives(*target, map[string]string{"apply_mode": "flag_only"})
	auto := resolveOptionDirectives(*target, map[string]string{"apply_mode": "auto"})

	// flag_only directive must be non-empty and must contain the canonical text.
	if !strings.Contains(flagOnly, "FLAG ONLY") {
		t.Errorf("T1: preset %q flag_only directive missing FLAG ONLY text; got: %q", target.Slug, flagOnly)
	}
	// auto directive must be empty (no extra text appended for the default choice).
	if auto != "" {
		t.Errorf("T1: preset %q auto directive should be empty, got: %q", target.Slug, auto)
	}

	// Combining base + flag_only must still be non-empty and > base alone.
	full := base + flagOnly
	if len(full) <= len(base) {
		t.Errorf("T1: preset %q full prompt with flag_only should be longer than base alone", target.Slug)
	}
}

// TestT1PresetBySlugRoundtrip asserts that every preset slug found in the
// registry is retrievable by presetBySlug, and that presetBySlug returns
// false for an invented slug.
func TestT1PresetBySlugRoundtrip(t *testing.T) {
	for _, p := range workflowPresets {
		got, ok := presetBySlug(p.Slug)
		if !ok {
			t.Errorf("T1: presetBySlug(%q) returned ok=false", p.Slug)
			continue
		}
		if got.Slug != p.Slug {
			t.Errorf("T1: presetBySlug(%q) returned slug=%q", p.Slug, got.Slug)
		}
	}

	if _, ok := presetBySlug("T1-invented-slug-that-does-not-exist"); ok {
		t.Error("T1: presetBySlug returned ok=true for a non-existent slug")
	}
}

// knownPresetCategoryList is a helper for error messages; returns a slice of
// the known category strings.
func knownPresetCategoryList() []string {
	out := make([]string, 0, len(knownPresetCategories))
	for k := range knownPresetCategories {
		out = append(out, k)
	}
	return out
}

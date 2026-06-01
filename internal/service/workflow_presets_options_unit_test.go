//go:build !lite

package service

import (
	"strings"
	"testing"
)

// T2 prefix — unique to this unit (T2-options-resolve) so it never collides
// with another parallel agent's test names.

// T2_makeOptionPreset builds a minimal WorkflowPreset with the provided
// options list, ready to pass to resolveOptionDirectives.
func T2_makeOptionPreset(opts []WorkflowPresetOption) WorkflowPreset {
	return WorkflowPreset{
		Slug:    "t2-test-preset",
		Name:    "T2 Test Preset",
		Options: opts,
	}
}

// T2_twoChoiceOption returns a WorkflowPresetOption with two choices:
//   - "alpha" → Directive "Alpha directive text"
//   - "beta"  → Directive "Beta directive text"
//
// Default is "alpha".
func T2_twoChoiceOption(key string) WorkflowPresetOption {
	return WorkflowPresetOption{
		Key:     key,
		Label:   "T2 Option Label",
		Default: "alpha",
		Choices: []WorkflowPresetChoice{
			{Value: "alpha", Label: "Alpha", Directive: "Alpha directive text"},
			{Value: "beta", Label: "Beta", Directive: "Beta directive text"},
		},
	}
}

// T2_emptyDirectiveOption returns a WorkflowPresetOption whose default choice
// has an empty Directive (like applyModeOption's "auto").
func T2_emptyDirectiveOption(key string) WorkflowPresetOption {
	return WorkflowPresetOption{
		Key:     key,
		Label:   "T2 Silent Option",
		Default: "silent",
		Choices: []WorkflowPresetChoice{
			{Value: "silent", Label: "Silent", Directive: ""},
			{Value: "loud", Label: "Loud", Directive: "Loud directive text"},
		},
	}
}

// TestT2ResolveOptionDirectives_ValidChoice verifies that when a valid choice
// value is supplied, the corresponding Directive is appended beneath a heading
// that names the option's Label.
func TestT2ResolveOptionDirectives_ValidChoice(t *testing.T) {
	opt := T2_twoChoiceOption("mode")
	preset := T2_makeOptionPreset([]WorkflowPresetOption{opt})

	result := resolveOptionDirectives(preset, map[string]string{"mode": "beta"})

	if !strings.Contains(result, "Beta directive text") {
		t.Errorf("expected Beta directive text in result, got: %q", result)
	}
	if strings.Contains(result, "Alpha directive text") {
		t.Errorf("Alpha directive text should NOT appear for 'beta' choice, got: %q", result)
	}
	// The heading must name the option's Label.
	if !strings.Contains(result, "## T2 Option Label") {
		t.Errorf("expected '## T2 Option Label' heading in result, got: %q", result)
	}
}

// TestT2ResolveOptionDirectives_DefaultFallback verifies that when an unknown
// value is submitted the function falls back to the option's Default value,
// and that Default's Directive is appended accordingly.
func TestT2ResolveOptionDirectives_DefaultFallback(t *testing.T) {
	opt := T2_twoChoiceOption("mode")
	// "alpha" is the Default; its Directive is "Alpha directive text".
	preset := T2_makeOptionPreset([]WorkflowPresetOption{opt})

	result := resolveOptionDirectives(preset, map[string]string{"mode": "unknown-value"})

	if !strings.Contains(result, "Alpha directive text") {
		t.Errorf("expected Alpha directive text (default fallback), got: %q", result)
	}
	if strings.Contains(result, "Beta directive text") {
		t.Errorf("Beta directive text should NOT appear when unknown key forces default, got: %q", result)
	}
}

// TestT2ResolveOptionDirectives_MissingKey verifies that when a chosen map
// omits the option's key entirely, the function falls back to the Default.
func TestT2ResolveOptionDirectives_MissingKey(t *testing.T) {
	opt := T2_twoChoiceOption("mode")
	preset := T2_makeOptionPreset([]WorkflowPresetOption{opt})

	// Pass an empty map — no key for "mode".
	result := resolveOptionDirectives(preset, map[string]string{})

	if !strings.Contains(result, "Alpha directive text") {
		t.Errorf("expected Alpha directive text (missing-key default), got: %q", result)
	}
}

// TestT2ResolveOptionDirectives_EmptySelection verifies that an empty chosen
// map on a preset with NO options returns an empty string (the base prompt
// is left untouched at the call site).
func TestT2ResolveOptionDirectives_EmptySelection(t *testing.T) {
	preset := T2_makeOptionPreset(nil)

	result := resolveOptionDirectives(preset, map[string]string{})

	if result != "" {
		t.Errorf("expected empty string for preset with no options, got: %q", result)
	}
}

// TestT2ResolveOptionDirectives_EmptyStringChosenValue verifies that an
// explicitly empty string value is treated as invalid and falls back to the
// Default (mirrors what resolveOptionDirectives does via TrimSpace + loop).
func TestT2ResolveOptionDirectives_EmptyStringChosenValue(t *testing.T) {
	opt := T2_twoChoiceOption("mode")
	preset := T2_makeOptionPreset([]WorkflowPresetOption{opt})

	result := resolveOptionDirectives(preset, map[string]string{"mode": ""})

	// "" is not a valid choice value -> falls back to "alpha".
	if !strings.Contains(result, "Alpha directive text") {
		t.Errorf("expected Alpha directive text (empty-string fallback to default), got: %q", result)
	}
}

// TestT2ResolveOptionDirectives_WhitespaceOnlyChosenValue verifies that
// whitespace-only chosen values are trimmed and treated as invalid, falling
// back to the Default.
func TestT2ResolveOptionDirectives_WhitespaceOnlyChosenValue(t *testing.T) {
	opt := T2_twoChoiceOption("mode")
	preset := T2_makeOptionPreset([]WorkflowPresetOption{opt})

	result := resolveOptionDirectives(preset, map[string]string{"mode": "   "})

	if !strings.Contains(result, "Alpha directive text") {
		t.Errorf("expected Alpha directive text (whitespace trimmed to invalid -> default), got: %q", result)
	}
}

// TestT2ResolveOptionDirectives_DefaultChoiceEmptyDirective verifies that
// when the Default's Directive is empty (like applyModeOption's "auto"), no
// heading or extra text is appended -- the result is an empty string.
func TestT2ResolveOptionDirectives_DefaultChoiceEmptyDirective(t *testing.T) {
	opt := T2_emptyDirectiveOption("verbosity")
	preset := T2_makeOptionPreset([]WorkflowPresetOption{opt})

	// "silent" is the default and has an empty directive.
	result := resolveOptionDirectives(preset, map[string]string{"verbosity": "silent"})

	if result != "" {
		t.Errorf("expected empty string for choice with empty Directive, got: %q", result)
	}
}

// TestT2ResolveOptionDirectives_NonDefaultChoiceWithDirective verifies that
// choosing a non-default value whose Directive is non-empty appends both the
// heading and the directive text.
func TestT2ResolveOptionDirectives_NonDefaultChoiceWithDirective(t *testing.T) {
	opt := T2_emptyDirectiveOption("verbosity")
	preset := T2_makeOptionPreset([]WorkflowPresetOption{opt})

	result := resolveOptionDirectives(preset, map[string]string{"verbosity": "loud"})

	if !strings.Contains(result, "Loud directive text") {
		t.Errorf("expected 'Loud directive text', got: %q", result)
	}
	if !strings.Contains(result, "## T2 Silent Option") {
		t.Errorf("expected '## T2 Silent Option' heading, got: %q", result)
	}
}

// TestT2ResolveOptionDirectives_MultipleOptions verifies that when a preset has
// multiple options, ALL non-empty directives are appended in order.
func TestT2ResolveOptionDirectives_MultipleOptions(t *testing.T) {
	optA := WorkflowPresetOption{
		Key:     "opt_a",
		Label:   "Option A",
		Default: "a1",
		Choices: []WorkflowPresetChoice{
			{Value: "a1", Label: "A1", Directive: "Directive from A"},
			{Value: "a2", Label: "A2", Directive: ""},
		},
	}
	optB := WorkflowPresetOption{
		Key:     "opt_b",
		Label:   "Option B",
		Default: "b1",
		Choices: []WorkflowPresetChoice{
			{Value: "b1", Label: "B1", Directive: ""},
			{Value: "b2", Label: "B2", Directive: "Directive from B"},
		},
	}
	preset := T2_makeOptionPreset([]WorkflowPresetOption{optA, optB})

	result := resolveOptionDirectives(preset, map[string]string{
		"opt_a": "a1", // has a directive
		"opt_b": "b2", // has a directive
	})

	if !strings.Contains(result, "Directive from A") {
		t.Errorf("expected 'Directive from A', got: %q", result)
	}
	if !strings.Contains(result, "Directive from B") {
		t.Errorf("expected 'Directive from B', got: %q", result)
	}
	// Order: opt_a is first, so "Directive from A" must appear before "Directive from B".
	idxA := strings.Index(result, "Directive from A")
	idxB := strings.Index(result, "Directive from B")
	if idxA >= idxB {
		t.Errorf("expected opt_a directive before opt_b directive; idxA=%d idxB=%d in %q", idxA, idxB, result)
	}
}

// TestT2ResolveOptionDirectives_MultipleOptionsPartialEmpty verifies that when
// one of multiple options resolves to a choice with an empty Directive, only
// the non-empty directive is appended.
func TestT2ResolveOptionDirectives_MultipleOptionsPartialEmpty(t *testing.T) {
	optA := WorkflowPresetOption{
		Key:     "opt_a",
		Label:   "Option A",
		Default: "a1",
		Choices: []WorkflowPresetChoice{
			{Value: "a1", Label: "A1", Directive: ""}, // empty directive
		},
	}
	optB := WorkflowPresetOption{
		Key:     "opt_b",
		Label:   "Option B",
		Default: "b1",
		Choices: []WorkflowPresetChoice{
			{Value: "b1", Label: "B1", Directive: "Only B has a directive"},
		},
	}
	preset := T2_makeOptionPreset([]WorkflowPresetOption{optA, optB})

	result := resolveOptionDirectives(preset, map[string]string{
		"opt_a": "a1",
		"opt_b": "b1",
	})

	if strings.Contains(result, "## Option A") {
		t.Errorf("Option A heading should not appear when its directive is empty, got: %q", result)
	}
	if !strings.Contains(result, "Only B has a directive") {
		t.Errorf("expected 'Only B has a directive', got: %q", result)
	}
	if !strings.Contains(result, "## Option B") {
		t.Errorf("expected '## Option B' heading, got: %q", result)
	}
}

// TestT2ResolveOptionDirectives_ApplyModeOptionAuto mirrors the real
// applyModeOption with "auto" selection -- must return empty string (no
// directive, just use the base prompt).
func TestT2ResolveOptionDirectives_ApplyModeOptionAuto(t *testing.T) {
	preset := T2_makeOptionPreset([]WorkflowPresetOption{applyModeOption})

	result := resolveOptionDirectives(preset, map[string]string{"apply_mode": "auto"})

	if result != "" {
		t.Errorf("auto apply_mode should produce no directive, got: %q", result)
	}
}

// TestT2ResolveOptionDirectives_ApplyModeOptionFlagOnly mirrors the real
// applyModeOption with "flag_only" -- must append the suppress-categorization
// directive under the "Apply mode" heading.
func TestT2ResolveOptionDirectives_ApplyModeOptionFlagOnly(t *testing.T) {
	preset := T2_makeOptionPreset([]WorkflowPresetOption{applyModeOption})

	result := resolveOptionDirectives(preset, map[string]string{"apply_mode": "flag_only"})

	if !strings.Contains(result, "FLAG ONLY") {
		t.Errorf("flag_only apply_mode should contain 'FLAG ONLY' directive, got: %q", result)
	}
	if !strings.Contains(result, "## Apply mode") {
		t.Errorf("expected '## Apply mode' heading, got: %q", result)
	}
}

// TestT2ResolveOptionDirectives_ApplyModeOptionMissingKeyDefaultsToAuto
// verifies the real applyModeOption when no chosen value is provided --
// "auto" is the default and has an empty directive, so result is empty.
func TestT2ResolveOptionDirectives_ApplyModeOptionMissingKeyDefaultsToAuto(t *testing.T) {
	preset := T2_makeOptionPreset([]WorkflowPresetOption{applyModeOption})

	result := resolveOptionDirectives(preset, nil)

	if result != "" {
		t.Errorf("missing key on applyModeOption should default to 'auto' (no directive), got: %q", result)
	}
}

// TestT2ResolveOptionDirectives_OutputFormat verifies the exact leading format
// of the appended text: "\n\n## <Label>\n\n<Directive>".
func TestT2ResolveOptionDirectives_OutputFormat(t *testing.T) {
	opt := WorkflowPresetOption{
		Key:     "fmt_opt",
		Label:   "Format Check",
		Default: "fv",
		Choices: []WorkflowPresetChoice{
			{Value: "fv", Label: "Format Value", Directive: "The directive body"},
		},
	}
	preset := T2_makeOptionPreset([]WorkflowPresetOption{opt})

	result := resolveOptionDirectives(preset, map[string]string{"fmt_opt": "fv"})

	want := "\n\n## Format Check\n\nThe directive body"
	if result != want {
		t.Errorf("output format mismatch\n  got:  %q\n  want: %q", result, want)
	}
}

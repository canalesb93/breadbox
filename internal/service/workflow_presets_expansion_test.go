//go:build !lite

package service

import (
	"strings"
	"testing"
)

// This file (unit-prefixed F4*) covers the F4-presets-and-options expansion:
// the "Alerts & Anomalies" category, the large-charge sentinel, and the two
// reusable options (Lookback window, Report verbosity). It does NOT touch
// the DB — composition and option resolution are pure functions over the
// embedded prompt-block FS.

// f4ValidCategories is the set of gallery categories the registry is allowed to
// use. Adding a category here is deliberate — the gallery's category-icon and
// ordering code must keep up (internal/admin/workflows_gallery_page.go).
var f4ValidCategories = map[string]bool{
	"Setup & Bulk":            true,
	"Categorization & Review": true,
	"Insights & Reports":      true,
	"Hygiene & Maintenance":   true,
	"Alerts & Anomalies":      true,
}

// TestF4AllPresetsComposeValid is the broad guard over the WHOLE registry
// (old presets + the new Alerts & Anomalies ones): every preset must have a
// unique slug, a known category, a valid tool scope, a positive cost estimate,
// a coherent trigger, and compose a non-empty base prompt from real blocks.
func TestF4AllPresetsComposeValid(t *testing.T) {
	if len(workflowPresets) == 0 {
		t.Fatal("workflowPresets registry is empty")
	}
	slugs := map[string]bool{}
	for _, p := range workflowPresets {
		if p.Slug == "" {
			t.Fatalf("preset %q (%s) has an empty slug", p.Name, p.Category)
		}
		if slugs[p.Slug] {
			t.Errorf("duplicate preset slug %q", p.Slug)
		}
		slugs[p.Slug] = true

		if p.Name == "" {
			t.Errorf("preset %q has an empty name", p.Slug)
		}
		if !f4ValidCategories[p.Category] {
			t.Errorf("preset %q has unknown category %q", p.Slug, p.Category)
		}
		if p.Icon == "" {
			t.Errorf("preset %q has no icon", p.Slug)
		}
		if p.ToolScope != "read_only" && p.ToolScope != "read_write" {
			t.Errorf("preset %q has invalid tool_scope %q", p.Slug, p.ToolScope)
		}
		if p.EstCostPerRunUSD <= 0 {
			t.Errorf("preset %q has non-positive EstCostPerRunUSD %v", p.Slug, p.EstCostPerRunUSD)
		}

		// Trigger coherence. A one-off is manual-only: it MUST carry no
		// recurring trigger (no post-sync, no cron). Every other preset must
		// have exactly one of post-sync OR a cron schedule.
		if p.OneOff {
			if p.TriggerOnSyncComplete || p.ScheduleCron != "" {
				t.Errorf("one-off preset %q must have no recurring trigger (post-sync=%v, cron=%q)", p.Slug, p.TriggerOnSyncComplete, p.ScheduleCron)
			}
		} else {
			if p.TriggerOnSyncComplete && p.ScheduleCron != "" {
				t.Errorf("preset %q is both post-sync and cron-scheduled (%q)", p.Slug, p.ScheduleCron)
			}
			if !p.TriggerOnSyncComplete && p.ScheduleCron == "" {
				t.Errorf("preset %q has no trigger (neither post-sync nor cron)", p.Slug)
			}
		}

		// Prompt composition from real blocks → non-empty.
		if len(p.PromptBlocks) == 0 {
			t.Errorf("preset %q has no prompt blocks", p.Slug)
			continue
		}
		prompt, err := composePresetPrompt(p.PromptBlocks)
		if err != nil {
			t.Errorf("preset %q failed to compose prompt: %v", p.Slug, err)
			continue
		}
		if strings.TrimSpace(prompt) == "" {
			t.Errorf("preset %q composed an empty prompt", p.Slug)
		}
	}
}

// TestF4AlertsCategoryPresets pins the Alerts & Anomalies preset(s) to their
// intended shape so a future edit that drifts the trust spectrum / trigger
// model fails loudly: large-charge sentinel (read_write, post-sync).
func TestF4AlertsCategoryPresets(t *testing.T) {
	want := map[string]struct {
		scope    string
		postSync bool
		cron     string
	}{
		"large-charge-sentinel": {scope: "read_write", postSync: true, cron: ""},
	}
	got := map[string]bool{}
	for _, p := range workflowPresets {
		w, ok := want[p.Slug]
		if !ok {
			continue
		}
		got[p.Slug] = true
		if p.Category != "Alerts & Anomalies" {
			t.Errorf("%q category = %q, want Alerts & Anomalies", p.Slug, p.Category)
		}
		if p.ToolScope != w.scope {
			t.Errorf("%q tool_scope = %q, want %q", p.Slug, p.ToolScope, w.scope)
		}
		if p.TriggerOnSyncComplete != w.postSync {
			t.Errorf("%q TriggerOnSyncComplete = %v, want %v", p.Slug, p.TriggerOnSyncComplete, w.postSync)
		}
		if p.ScheduleCron != w.cron {
			t.Errorf("%q ScheduleCron = %q, want %q", p.Slug, p.ScheduleCron, w.cron)
		}
		if p.EstCostPerRunUSD <= 0 {
			t.Errorf("%q EstCostPerRunUSD = %v, want > 0", p.Slug, p.EstCostPerRunUSD)
		}
	}
	for slug := range want {
		if !got[slug] {
			t.Errorf("expected preset %q in the registry, not found", slug)
		}
	}
}

// TestF4LookbackWindowOptionResolves proves the new Lookback window option
// resolves directives: 7 (default) → no directive, 30/90 → a window directive,
// and an unknown/blank value falls back to the default (no directive).
func TestF4LookbackWindowOptionResolves(t *testing.T) {
	preset := WorkflowPreset{Options: []WorkflowPresetOption{lookbackWindowOption}}

	// Default value → no directive appended.
	if d := resolveOptionDirectives(preset, map[string]string{"lookback_window": "7"}); strings.TrimSpace(d) != "" {
		t.Errorf("lookback_window=7 should append no directive, got %q", d)
	}
	// Empty / unknown → falls back to default → no directive.
	if d := resolveOptionDirectives(preset, map[string]string{"lookback_window": "bogus"}); strings.TrimSpace(d) != "" {
		t.Errorf("unknown lookback_window should fall back to default (no directive), got %q", d)
	}
	if d := resolveOptionDirectives(preset, nil); strings.TrimSpace(d) != "" {
		t.Errorf("missing lookback_window should fall back to default (no directive), got %q", d)
	}
	// 30 / 90 → a non-empty directive under the option's label heading.
	for _, v := range []string{"30", "90"} {
		d := resolveOptionDirectives(preset, map[string]string{"lookback_window": v})
		if !strings.Contains(d, "## Lookback window") {
			t.Errorf("lookback_window=%s missing the option heading, got %q", v, d)
		}
		if !strings.Contains(d, "LOOKBACK WINDOW") || !strings.Contains(d, v) {
			t.Errorf("lookback_window=%s missing the window directive, got %q", v, d)
		}
	}
}

// TestF4ReportVerbosityOptionResolves proves the Report verbosity option:
// concise (default) → no directive, detailed → a fuller-report directive.
func TestF4ReportVerbosityOptionResolves(t *testing.T) {
	preset := WorkflowPreset{Options: []WorkflowPresetOption{reportVerbosityOption}}

	if d := resolveOptionDirectives(preset, map[string]string{"report_verbosity": "concise"}); strings.TrimSpace(d) != "" {
		t.Errorf("report_verbosity=concise should append no directive, got %q", d)
	}
	if d := resolveOptionDirectives(preset, map[string]string{"report_verbosity": "detailed"}); !strings.Contains(d, "## Report verbosity") || !strings.Contains(d, "REPORT VERBOSITY — DETAILED") {
		t.Errorf("report_verbosity=detailed missing the detailed directive, got %q", d)
	}
}

// TestF4OptionsCombineOnAlertsPreset proves the two new options compose
// together on a real registry preset: a 90-day detailed configuration appends
// BOTH directives, in option order, after the base prompt.
func TestF4OptionsCombineOnAlertsPreset(t *testing.T) {
	preset, ok := presetBySlug("large-charge-sentinel")
	if !ok {
		t.Fatal("large-charge-sentinel preset missing from registry")
	}
	base, err := composePresetPrompt(preset.PromptBlocks)
	if err != nil {
		t.Fatalf("composePresetPrompt: %v", err)
	}
	dir := resolveOptionDirectives(preset, map[string]string{
		"lookback_window":  "90",
		"report_verbosity": "detailed",
	})
	full := base + dir

	if !strings.Contains(full, "LOOKBACK WINDOW — 90 DAYS") {
		t.Errorf("combined prompt missing the 90-day lookback directive")
	}
	if !strings.Contains(full, "REPORT VERBOSITY — DETAILED") {
		t.Errorf("combined prompt missing the detailed-report directive")
	}
	// Lookback window is option[0] → its heading must precede Report verbosity's.
	li := strings.Index(full, "## Lookback window")
	ri := strings.Index(full, "## Report verbosity")
	if li < 0 || ri < 0 || li > ri {
		t.Errorf("option directives out of order: lookback@%d report@%d", li, ri)
	}
	// The base prompt is preserved ahead of the appended directives.
	if !strings.HasPrefix(full, strings.TrimSpace(base)[:1]) || li <= 0 {
		t.Errorf("base prompt should precede the appended option directives")
	}
}

// TestF4NewPromptBlocksLoad proves the alerts strategy block is present in the
// embedded library and carries non-empty content (so a missing or empty file
// fails here, not at a household's run).
func TestF4NewPromptBlocksLoad(t *testing.T) {
	blocks, err := loadPromptBlocks()
	if err != nil {
		t.Fatalf("loadPromptBlocks: %v", err)
	}
	byID := map[string]PromptBlock{}
	for _, b := range blocks {
		byID[b.ID] = b
	}
	for _, id := range []string{"strategy-large-charge-sentinel"} {
		b, ok := byID[id]
		if !ok {
			t.Errorf("new prompt block %q not loaded from prompts/agents/", id)
			continue
		}
		if strings.TrimSpace(b.Content) == "" {
			t.Errorf("new prompt block %q has empty content", id)
		}
		if b.Group != GroupStrategy {
			t.Errorf("new prompt block %q group = %q, want strategy", id, b.Group)
		}
	}
}

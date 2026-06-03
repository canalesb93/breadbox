//go:build integration && !lite

package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"breadbox/internal/service"
)

// TestT14_ErrConflictOnSecondEnable verifies that enabling the same preset
// a second time returns ErrConflict regardless of the params supplied on
// the second call.
func TestT14_ErrConflictOnSecondEnable(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// First enable must succeed.
	first, err := svc.EnableWorkflowFromPreset(ctx, "weekly-money-digest", service.EnableWorkflowFromPresetParams{
		Enabled: false,
	})
	if err != nil {
		t.Fatalf("T14: first enable: %v", err)
	}
	if first.SourceTemplate == nil || *first.SourceTemplate != "weekly-money-digest" {
		t.Fatalf("T14: source_template = %v, want weekly-money-digest", first.SourceTemplate)
	}

	// Second enable with Enabled=true (different params) must still conflict.
	_, err = svc.EnableWorkflowFromPreset(ctx, "weekly-money-digest", service.EnableWorkflowFromPresetParams{
		Enabled: true,
	})
	if !errors.Is(err, service.ErrConflict) {
		t.Fatalf("T14: second enable err = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), "weekly-money-digest") {
		t.Errorf("T14: ErrConflict message should include the preset slug, got: %q", err.Error())
	}
}

// TestT14_ErrConflictDistinctPerPreset verifies that ErrConflict is scoped
// to the specific preset: enabling preset A twice is a conflict, but enabling
// preset B afterward succeeds.
func TestT14_ErrConflictDistinctPerPreset(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// Enable backlog-closer.
	_, err := svc.EnableWorkflowFromPreset(ctx, "backlog-closer", service.EnableWorkflowFromPresetParams{})
	if err != nil {
		t.Fatalf("T14: enable backlog-closer: %v", err)
	}

	// Second enable of backlog-closer -> conflict.
	_, err = svc.EnableWorkflowFromPreset(ctx, "backlog-closer", service.EnableWorkflowFromPresetParams{})
	if !errors.Is(err, service.ErrConflict) {
		t.Fatalf("T14: expected ErrConflict for backlog-closer second enable, got %v", err)
	}

	// monthly-close has not been enabled yet -- must succeed.
	mc, err := svc.EnableWorkflowFromPreset(ctx, "monthly-close", service.EnableWorkflowFromPresetParams{})
	if err != nil {
		t.Fatalf("T14: enable monthly-close after conflict should succeed: %v", err)
	}
	if mc.Slug != "monthly-close" {
		t.Fatalf("T14: monthly-close slug = %q, want monthly-close", mc.Slug)
	}
}

// TestT14_OneOffPresetInstantiatesManualOnly verifies that enabling a one-off
// preset produces a manual-only workflow: no post-sync flag and no cron, even
// when the caller passes trigger/schedule overrides. The one-off's model
// default is also carried onto the definition.
func TestT14_OneOffPresetInstantiatesManualOnly(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// Pass overrides that WOULD schedule a recurring preset — a one-off must
	// ignore them and stay manual-only.
	syncTrue := true
	cron := "0 8 * * *"
	wf, err := svc.EnableWorkflowFromPreset(ctx, "rule-foundation", service.EnableWorkflowFromPresetParams{
		Enabled:       true,
		TriggerOnSync: &syncTrue,
		ScheduleCron:  &cron,
	})
	if err != nil {
		t.Fatalf("T14: enable one-off rule-foundation: %v", err)
	}
	if wf.TriggerOnSyncComplete {
		t.Errorf("T14: one-off must not trigger on sync complete")
	}
	if wf.ScheduleCron != nil {
		t.Errorf("T14: one-off must have nil schedule_cron, got %q", *wf.ScheduleCron)
	}
	if wf.Model != "claude-sonnet-4-6" {
		t.Errorf("T14: rule-foundation model = %q, want claude-sonnet-4-6", wf.Model)
	}
	if wf.SourceTemplate == nil || *wf.SourceTemplate != "rule-foundation" {
		t.Errorf("T14: source_template = %v, want rule-foundation", wf.SourceTemplate)
	}
}

// TestT14_EnsureOneOffWorkflow covers the Run-now backing path: it instantiates
// a one-off on first call, reuses it on the second (no ErrConflict), rejects a
// recurring preset slug, and 404s an unknown slug.
func TestT14_EnsureOneOffWorkflow(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	first, err := svc.EnsureOneOffWorkflow(ctx, "bulk-catchup")
	if err != nil {
		t.Fatalf("T14: ensure bulk-catchup (first): %v", err)
	}
	if first.Slug != "bulk-catchup" {
		t.Fatalf("T14: slug = %q, want bulk-catchup", first.Slug)
	}
	if first.TriggerOnSyncComplete || first.ScheduleCron != nil {
		t.Errorf("T14: ensured one-off must be manual-only")
	}

	// Second call must reuse the same definition, not conflict.
	second, err := svc.EnsureOneOffWorkflow(ctx, "bulk-catchup")
	if err != nil {
		t.Fatalf("T14: ensure bulk-catchup (second) should reuse, got: %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("T14: second ensure returned a different definition (%s vs %s)", second.ID, first.ID)
	}

	// A recurring (non-one-off) preset is rejected.
	_, err = svc.EnsureOneOffWorkflow(ctx, "routine-reviewer")
	if !errors.Is(err, service.ErrInvalidParameter) {
		t.Errorf("T14: ensure non-one-off err = %v, want ErrInvalidParameter", err)
	}

	// An unknown slug 404s.
	_, err = svc.EnsureOneOffWorkflow(ctx, "does-not-exist")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("T14: ensure unknown err = %v, want ErrNotFound", err)
	}
}

// TestT14_OptionDirectivesAppendedToPrompt verifies that the chosen
// WorkflowPresetOption Directive is appended to the stored prompt under a
// heading labelled with the option Label, and that the "auto" default
// choice (empty directive) appends nothing.
func TestT14_OptionDirectivesAppendedToPrompt(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// routine-reviewer has applyModeOption (key "apply_mode").
	// Choosing "flag_only" appends a FLAG ONLY directive under "## Apply mode".
	flagOnly, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{
		Options: map[string]string{"apply_mode": "flag_only"},
	})
	if err != nil {
		t.Fatalf("T14: enable with flag_only: %v", err)
	}
	if !strings.Contains(flagOnly.Prompt, "## Apply mode") {
		t.Errorf("T14: flag_only prompt missing '## Apply mode' heading; len=%d", len(flagOnly.Prompt))
	}
	if !strings.Contains(flagOnly.Prompt, "FLAG ONLY") {
		t.Errorf("T14: flag_only prompt missing 'FLAG ONLY' directive; len=%d", len(flagOnly.Prompt))
	}
	if !strings.Contains(flagOnly.Prompt, "do NOT call update_transactions") {
		t.Errorf("T14: flag_only prompt missing key directive text; len=%d", len(flagOnly.Prompt))
	}

	// backlog-closer also has applyModeOption. Choosing "auto" (the default)
	// must NOT append any directive because the directive for "auto" is empty.
	autoMode, err := svc.EnableWorkflowFromPreset(ctx, "backlog-closer", service.EnableWorkflowFromPresetParams{
		Options: map[string]string{"apply_mode": "auto"},
	})
	if err != nil {
		t.Fatalf("T14: enable backlog-closer with auto: %v", err)
	}
	if strings.Contains(autoMode.Prompt, "FLAG ONLY") {
		t.Errorf("T14: auto prompt should not contain 'FLAG ONLY'")
	}
	// The heading itself is only present when a directive is appended.
	if strings.Contains(autoMode.Prompt, "## Apply mode") {
		t.Errorf("T14: auto prompt should not contain '## Apply mode' heading (no directive to wrap)")
	}
}

// TestT14_UnknownOptionKeyFallsBackToDefault verifies that supplying an
// unknown option key is silently ignored and the option falls back to its
// Default choice. The "auto" default for applyModeOption has an empty
// directive, so no FLAG ONLY text should appear.
func TestT14_UnknownOptionKeyFallsBackToDefault(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// routine-reviewer expects key "apply_mode". We send a typo key.
	wf, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{
		Options: map[string]string{"apply_modex": "flag_only"},
	})
	if err != nil {
		t.Fatalf("T14: enable with unknown option key: %v", err)
	}
	// Default for applyModeOption is "auto" -- empty directive -- so FLAG ONLY must be absent.
	if strings.Contains(wf.Prompt, "FLAG ONLY") {
		t.Errorf("T14: unknown key should fall back to default (auto); prompt should not contain FLAG ONLY")
	}
	if len(wf.Prompt) == 0 {
		t.Errorf("T14: prompt is empty after enabling with unknown option key")
	}
}

// TestT14_ScheduleOverrideAppliedForScheduledPreset verifies that a non-nil
// ScheduleCron override replaces the preset default for a cron-triggered preset.
func TestT14_ScheduleOverrideAppliedForScheduledPreset(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// weekly-money-digest defaults to "0 7 * * 1". Override to daily at 09:00.
	override := "0 9 * * *"
	wf, err := svc.EnableWorkflowFromPreset(ctx, "weekly-money-digest", service.EnableWorkflowFromPresetParams{
		ScheduleCron: &override,
	})
	if err != nil {
		t.Fatalf("T14: enable with schedule override: %v", err)
	}
	if wf.ScheduleCron == nil {
		t.Fatal("T14: schedule_cron is nil, expected the override")
	}
	if *wf.ScheduleCron != override {
		t.Errorf("T14: schedule_cron = %q, want %q", *wf.ScheduleCron, override)
	}
	// post-sync trigger must be false for a purely cron-based preset.
	if wf.TriggerOnSyncComplete {
		t.Errorf("T14: weekly-money-digest should not trigger on sync complete")
	}
}

// TestT14_NilScheduleOverrideKeepsPresetDefault verifies that when
// ScheduleCron is nil the preset default cron is used unchanged.
func TestT14_NilScheduleOverrideKeepsPresetDefault(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// monthly-close defaults to "0 8 1 * *".
	wf, err := svc.EnableWorkflowFromPreset(ctx, "monthly-close", service.EnableWorkflowFromPresetParams{
		ScheduleCron: nil, // no override
	})
	if err != nil {
		t.Fatalf("T14: enable monthly-close (no override): %v", err)
	}
	const presetDefault = "0 8 1 * *"
	if wf.ScheduleCron == nil {
		t.Fatal("T14: schedule_cron is nil, expected preset default")
	}
	if *wf.ScheduleCron != presetDefault {
		t.Errorf("T14: schedule_cron = %q, want preset default %q", *wf.ScheduleCron, presetDefault)
	}
}

// TestT14_EmptyStringScheduleOverrideKeepsPresetDefault verifies that an
// empty-string ScheduleCron is treated the same as nil -- the preset default wins.
func TestT14_EmptyStringScheduleOverrideKeepsPresetDefault(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	blank := ""
	wf, err := svc.EnableWorkflowFromPreset(ctx, "weekly-money-digest", service.EnableWorkflowFromPresetParams{
		ScheduleCron: &blank,
	})
	if err != nil {
		t.Fatalf("T14: enable with empty-string override: %v", err)
	}
	const presetDefault = "0 7 * * 1"
	if wf.ScheduleCron == nil {
		t.Fatal("T14: schedule_cron is nil, expected preset default")
	}
	if *wf.ScheduleCron != presetDefault {
		t.Errorf("T14: schedule_cron = %q, want preset default %q", *wf.ScheduleCron, presetDefault)
	}
}

// TestT14_PostSyncPresetIgnoresScheduleOverride verifies that a schedule
// override is silently ignored for a post-sync-triggered preset, which keeps
// no cron schedule regardless.
func TestT14_PostSyncPresetIgnoresScheduleOverride(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// routine-reviewer is post-sync (TriggerOnSyncComplete = true, no ScheduleCron).
	override := "0 6 * * *"
	wf, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{
		ScheduleCron: &override,
	})
	if err != nil {
		t.Fatalf("T14: enable post-sync preset with schedule override: %v", err)
	}
	if wf.ScheduleCron != nil {
		t.Errorf("T14: post-sync preset must have nil schedule_cron, got %q", *wf.ScheduleCron)
	}
	if !wf.TriggerOnSyncComplete {
		t.Errorf("T14: routine-reviewer must have TriggerOnSyncComplete=true")
	}
}

// TestT14_AdditionalInstructionsAppendedAfterOptionDirectives verifies that
// AdditionalInstructions appears after option directives in the composed
// prompt and is wrapped under its own heading.
func TestT14_AdditionalInstructionsAppendedAfterOptionDirectives(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	const extraInstr = "Always add a comment when you flag a transaction."
	wf, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{
		Options:                map[string]string{"apply_mode": "flag_only"},
		AdditionalInstructions: extraInstr,
	})
	if err != nil {
		t.Fatalf("T14: enable with directives + instructions: %v", err)
	}
	flagPos := strings.Index(wf.Prompt, "FLAG ONLY")
	instrPos := strings.Index(wf.Prompt, extraInstr)
	headingPos := strings.Index(wf.Prompt, "## Additional instructions")
	if flagPos < 0 {
		t.Fatal("T14: FLAG ONLY directive missing from prompt")
	}
	if headingPos < 0 {
		t.Fatal("T14: '## Additional instructions' heading missing from prompt")
	}
	if instrPos < 0 {
		t.Fatal("T14: additional instruction text missing from prompt")
	}
	// Additional instructions heading must come after the FLAG ONLY directive.
	if headingPos < flagPos {
		t.Errorf("T14: additional instructions heading (%d) appears before FLAG ONLY directive (%d)", headingPos, flagPos)
	}
}

// TestT14_AdditionalInstructionsTooLong verifies that instructions exceeding
// the limit are rejected with ErrInvalidParameter before any DB write occurs.
func TestT14_AdditionalInstructionsTooLong(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// 4001 chars exceeds the maxAdditionalInstructions (4000) limit.
	tooLong := strings.Repeat("x", 4001)
	_, err := svc.EnableWorkflowFromPreset(ctx, "monthly-close", service.EnableWorkflowFromPresetParams{
		AdditionalInstructions: tooLong,
	})
	if !errors.Is(err, service.ErrInvalidParameter) {
		t.Fatalf("T14: oversized instructions err = %v, want ErrInvalidParameter", err)
	}

	// The preset must still be enable-able after the failed attempt (no partial write occurred).
	wf, err := svc.EnableWorkflowFromPreset(ctx, "monthly-close", service.EnableWorkflowFromPresetParams{})
	if err != nil {
		t.Fatalf("T14: enable after rejected attempt should succeed: %v", err)
	}
	if wf.Slug != "monthly-close" {
		t.Errorf("T14: slug = %q, want monthly-close", wf.Slug)
	}
}

// TestT14_GalleryReflectsEnabledAfterEnable verifies that ListWorkflowPresets
// correctly marks a preset as enabled once EnableWorkflowFromPreset succeeds,
// and that WorkflowSlug and WorkflowEnabled fields are populated.
func TestT14_GalleryReflectsEnabledAfterEnable(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	wf, err := svc.EnableWorkflowFromPreset(ctx, "backlog-closer", service.EnableWorkflowFromPresetParams{
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("T14: enable backlog-closer: %v", err)
	}

	views, err := svc.ListWorkflowPresets(ctx)
	if err != nil {
		t.Fatalf("T14: ListWorkflowPresets: %v", err)
	}
	var found bool
	for _, v := range views {
		if v.Slug != "backlog-closer" {
			continue
		}
		found = true
		if !v.Enabled {
			t.Errorf("T14: gallery view for backlog-closer should be Enabled=true")
		}
		if v.WorkflowSlug == nil || *v.WorkflowSlug != wf.Slug {
			t.Errorf("T14: gallery WorkflowSlug = %v, want %q", v.WorkflowSlug, wf.Slug)
		}
		if v.WorkflowEnabled == nil || !*v.WorkflowEnabled {
			t.Errorf("T14: gallery WorkflowEnabled = %v, want true", v.WorkflowEnabled)
		}
	}
	if !found {
		t.Fatal("T14: backlog-closer not found in ListWorkflowPresets")
	}
}

// TestT14_SourceTemplateStampedOnInstantiation verifies that the created
// agent_definition carries source_template equal to the preset slug, and
// that a hand-authored definition (no template) is not mistakenly tagged.
func TestT14_SourceTemplateStampedOnInstantiation(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	preset, err := svc.EnableWorkflowFromPreset(ctx, "weekly-money-digest", service.EnableWorkflowFromPresetParams{})
	if err != nil {
		t.Fatalf("T14: enable preset: %v", err)
	}
	if preset.SourceTemplate == nil {
		t.Fatal("T14: SourceTemplate must be set on a preset-instantiated workflow")
	}
	if *preset.SourceTemplate != "weekly-money-digest" {
		t.Errorf("T14: SourceTemplate = %q, want weekly-money-digest", *preset.SourceTemplate)
	}

	// Hand-authored definition must have nil SourceTemplate.
	plain := mustCreateAgentDefinition(t, svc, "t14-plain-no-template", false)
	if plain.SourceTemplate != nil {
		t.Errorf("T14: hand-authored definition must have nil SourceTemplate, got %q", *plain.SourceTemplate)
	}
}

// TestEnableWorkflowFromPreset_CustomName verifies a name supplied at setup
// (the new setup-drawer Name field) is applied to the instantiated workflow
// instead of the preset's static name, while the slug stays the preset slug.
func TestEnableWorkflowFromPreset_CustomName(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	custom := "Monthly Rule Tune-Up"
	def, err := svc.EnableWorkflowFromPreset(ctx, "rule-foundation", service.EnableWorkflowFromPresetParams{
		Name: &custom,
	})
	if err != nil {
		t.Fatalf("enable with custom name: %v", err)
	}
	if def.Name != custom {
		t.Errorf("Name = %q, want %q", def.Name, custom)
	}
	if def.Slug != "rule-foundation" {
		t.Errorf("Slug = %q, want rule-foundation (slug must not change with a custom name)", def.Slug)
	}
}

// TestEnableWorkflowFromPreset_BlankNameFallsBackToPreset verifies that a
// present-but-blank name (e.g. the user cleared the field) falls back to the
// preset's name rather than instantiating an empty-named workflow.
func TestEnableWorkflowFromPreset_BlankNameFallsBackToPreset(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	blank := "   "
	def, err := svc.EnableWorkflowFromPreset(ctx, "rule-foundation", service.EnableWorkflowFromPresetParams{
		Name: &blank,
	})
	if err != nil {
		t.Fatalf("enable with blank name: %v", err)
	}
	if def.Name != "Rule Foundation" {
		t.Errorf("Name = %q, want preset fallback %q", def.Name, "Rule Foundation")
	}
}

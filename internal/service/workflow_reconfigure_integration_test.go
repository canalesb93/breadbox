//go:build integration && !lite

package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"breadbox/internal/service"
)

// TestF3GetWorkflowConfig round-trips enable → GetWorkflowConfig: the live
// config must reflect the schedule, additional instructions, and chosen
// options the workflow was instantiated with.
func TestF3GetWorkflowConfig(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	cron := "0 8 1 * *"
	if _, err := svc.EnableWorkflowFromPreset(ctx, "weekly-money-digest", service.EnableWorkflowFromPresetParams{
		Enabled:                false,
		ScheduleCron:           &cron,
		AdditionalInstructions: "Focus on dining-out spend.",
	}); err != nil {
		t.Fatalf("EnableWorkflowFromPreset: %v", err)
	}

	cfg, err := svc.GetWorkflowConfig(ctx, "weekly-money-digest")
	if err != nil {
		t.Fatalf("GetWorkflowConfig: %v", err)
	}
	if cfg.Slug != "weekly-money-digest" {
		t.Fatalf("slug = %q, want weekly-money-digest", cfg.Slug)
	}
	if cfg.TriggerOnSync {
		t.Fatalf("weekly-money-digest is a scheduled preset; TriggerOnSync should be false")
	}
	if cfg.ScheduleCron != cron {
		t.Fatalf("schedule_cron = %q, want %q", cfg.ScheduleCron, cron)
	}
	if cfg.AdditionalInstructions != "Focus on dining-out spend." {
		t.Fatalf("additional_instructions = %q, want the enabled value", cfg.AdditionalInstructions)
	}
}

// TestF3GetWorkflowConfig_DerivesOption confirms the chosen option (apply_mode)
// is recovered from the instantiated workflow's prompt.
func TestF3GetWorkflowConfig_DerivesOption(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	if _, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{
		Options: map[string]string{"apply_mode": "flag_only"},
	}); err != nil {
		t.Fatalf("EnableWorkflowFromPreset: %v", err)
	}

	cfg, err := svc.GetWorkflowConfig(ctx, "routine-reviewer")
	if err != nil {
		t.Fatalf("GetWorkflowConfig: %v", err)
	}
	if cfg.TriggerOnSync != true {
		t.Fatalf("routine-reviewer should be post-sync (TriggerOnSync=true)")
	}
	var found bool
	for _, opt := range cfg.Options {
		if opt.Key == "apply_mode" {
			found = true
			if opt.Selected != "flag_only" {
				t.Fatalf("apply_mode selected = %q, want flag_only", opt.Selected)
			}
		}
	}
	if !found {
		t.Fatal("apply_mode option not present in config")
	}
}

// TestF3UpdateWorkflowConfig re-composes an enabled workflow: a new schedule,
// new option, and new additional instructions all land in the live row, and a
// subsequent GetWorkflowConfig reflects them (a full edit round-trip).
func TestF3UpdateWorkflowConfig(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// Enable a categorization preset paused, auto apply mode, no instructions.
	if _, err := svc.EnableWorkflowFromPreset(ctx, "backlog-closer", service.EnableWorkflowFromPresetParams{
		Enabled: false,
	}); err != nil {
		t.Fatalf("EnableWorkflowFromPreset: %v", err)
	}

	newCron := "0 8 * * *"
	wf, err := svc.UpdateWorkflowConfig(ctx, "backlog-closer", service.UpdateWorkflowConfigParams{
		ScheduleCron:           &newCron,
		AdditionalInstructions: "Promote only 3+ occurrence patterns.",
		Options:                map[string]string{"apply_mode": "flag_only"},
	})
	if err != nil {
		t.Fatalf("UpdateWorkflowConfig: %v", err)
	}
	if wf.ScheduleCron == nil || *wf.ScheduleCron != newCron {
		t.Fatalf("schedule_cron = %v, want %q", wf.ScheduleCron, newCron)
	}
	if !strings.Contains(wf.Prompt, "FLAG ONLY") {
		t.Fatalf("re-composed prompt missing the flag-only directive")
	}
	if !strings.Contains(wf.Prompt, "Promote only 3+ occurrence patterns.") {
		t.Fatalf("re-composed prompt missing the new additional instructions")
	}
	// The run-state toggle is untouched by a reconfigure.
	if wf.Enabled {
		t.Fatalf("reconfigure should not flip the enabled toggle")
	}

	// GetWorkflowConfig reflects the new values.
	cfg, err := svc.GetWorkflowConfig(ctx, "backlog-closer")
	if err != nil {
		t.Fatalf("GetWorkflowConfig after update: %v", err)
	}
	if cfg.ScheduleCron != newCron {
		t.Fatalf("config schedule_cron = %q, want %q", cfg.ScheduleCron, newCron)
	}
	if cfg.AdditionalInstructions != "Promote only 3+ occurrence patterns." {
		t.Fatalf("config additional_instructions = %q, want the updated value", cfg.AdditionalInstructions)
	}
	for _, opt := range cfg.Options {
		if opt.Key == "apply_mode" && opt.Selected != "flag_only" {
			t.Fatalf("config apply_mode = %q, want flag_only", opt.Selected)
		}
	}

	// Clearing the additional instructions removes the tail.
	if _, err := svc.UpdateWorkflowConfig(ctx, "backlog-closer", service.UpdateWorkflowConfigParams{
		AdditionalInstructions: "",
		Options:                map[string]string{"apply_mode": "auto"},
	}); err != nil {
		t.Fatalf("UpdateWorkflowConfig (clear): %v", err)
	}
	cfg, err = svc.GetWorkflowConfig(ctx, "backlog-closer")
	if err != nil {
		t.Fatalf("GetWorkflowConfig after clear: %v", err)
	}
	if cfg.AdditionalInstructions != "" {
		t.Fatalf("additional_instructions = %q, want empty after clear", cfg.AdditionalInstructions)
	}
}

// TestF3UpdateWorkflowConfig_PostSyncIgnoresSchedule confirms a schedule
// override is ignored for a post-sync workflow (mirrors enable-time semantics).
func TestF3UpdateWorkflowConfig_PostSyncIgnoresSchedule(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	if _, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{}); err != nil {
		t.Fatalf("EnableWorkflowFromPreset: %v", err)
	}
	cron := "0 8 * * *"
	wf, err := svc.UpdateWorkflowConfig(ctx, "routine-reviewer", service.UpdateWorkflowConfigParams{
		ScheduleCron: &cron,
	})
	if err != nil {
		t.Fatalf("UpdateWorkflowConfig: %v", err)
	}
	if wf.ScheduleCron != nil {
		t.Fatalf("post-sync workflow got a schedule_cron = %v, want nil", *wf.ScheduleCron)
	}
	if !wf.TriggerOnSyncComplete {
		t.Fatalf("routine-reviewer should still trigger on sync")
	}
}

// TestF3WorkflowConfig_NotEnabled guards the not-an-enabled-preset paths:
// an unknown slug is ErrNotFound; a hand-authored agent (no source_template)
// is ErrInvalidState.
func TestF3WorkflowConfig_NotEnabled(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	if _, err := svc.GetWorkflowConfig(ctx, "no-such-workflow"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("GetWorkflowConfig(unknown) err = %v, want ErrNotFound", err)
	}

	// A hand-authored agent carries no source_template → not reconfigurable.
	plain := mustCreateAgentDefinition(t, svc, "f3-plain-agent", true)
	if _, err := svc.GetWorkflowConfig(ctx, plain.Slug); !errors.Is(err, service.ErrInvalidState) {
		t.Fatalf("GetWorkflowConfig(hand-authored) err = %v, want ErrInvalidState", err)
	}
	if _, err := svc.UpdateWorkflowConfig(ctx, plain.Slug, service.UpdateWorkflowConfigParams{}); !errors.Is(err, service.ErrInvalidState) {
		t.Fatalf("UpdateWorkflowConfig(hand-authored) err = %v, want ErrInvalidState", err)
	}
}

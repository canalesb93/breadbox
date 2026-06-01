//go:build integration && !lite

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/service"
)

func wcoBool(b bool) *bool       { return &b }
func wcoStr(s string) *string    { return &s }
func wcoInt(i int) *int          { return &i }
func wcoFloat(f float64) *float64 { return &f }

// TestWorkflowConfigOptions_EnableCustomTriggerModelCaps verifies the setup
// drawer's new fields (trigger choice, model, Advanced caps) round-trip
// through enable → GetWorkflowConfig.
func TestWorkflowConfigOptions_EnableCustomTriggerModelCaps(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	_, err := svc.EnableWorkflowFromPreset(ctx, "weekly-money-digest", service.EnableWorkflowFromPresetParams{
		Enabled:       false,
		TriggerOnSync: wcoBool(false), // custom schedule
		ScheduleCron:  wcoStr("0 19 * * 2,4"),
		Model:         wcoStr("claude-sonnet-4-6"),
		MaxTurns:      wcoInt(20),
		MaxBudgetUSD:  wcoFloat(5),
	})
	if err != nil {
		t.Fatalf("enable: %v", err)
	}

	cfg, err := svc.GetWorkflowConfig(ctx, "weekly-money-digest")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if cfg.TriggerOnSync {
		t.Error("expected custom trigger (trigger_on_sync=false)")
	}
	if cfg.ScheduleCron != "0 19 * * 2,4" {
		t.Errorf("schedule_cron = %q, want 0 19 * * 2,4", cfg.ScheduleCron)
	}
	if cfg.Model != "claude-sonnet-4-6" {
		t.Errorf("model = %q, want claude-sonnet-4-6", cfg.Model)
	}
	if cfg.MaxTurns != 20 {
		t.Errorf("max_turns = %d, want 20", cfg.MaxTurns)
	}
	if cfg.MaxBudgetUSD != 5 {
		t.Errorf("max_budget_usd = %v, want 5", cfg.MaxBudgetUSD)
	}
}

// TestWorkflowConfigOptions_TriggerSwitchClearsCron is the load-bearing
// correctness check: switching a workflow to after-each-sync must clear its
// cron (the scheduler keys off a non-empty cron independently of the sync
// flag, so leaving both set would double-fire). Switching back restores a
// custom schedule.
func TestWorkflowConfigOptions_TriggerSwitchClearsCron(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	if _, err := svc.EnableWorkflowFromPreset(ctx, "weekly-money-digest", service.EnableWorkflowFromPresetParams{
		TriggerOnSync: wcoBool(false),
		ScheduleCron:  wcoStr("0 8 * * 1"),
	}); err != nil {
		t.Fatalf("enable: %v", err)
	}

	// custom → after each sync: cron must clear.
	if _, err := svc.UpdateWorkflowConfig(ctx, "weekly-money-digest", service.UpdateWorkflowConfigParams{
		TriggerOnSync: wcoBool(true),
	}); err != nil {
		t.Fatalf("reconfigure to sync: %v", err)
	}
	cfg, err := svc.GetWorkflowConfig(ctx, "weekly-money-digest")
	if err != nil {
		t.Fatalf("config after sync: %v", err)
	}
	if !cfg.TriggerOnSync {
		t.Error("expected trigger_on_sync=true after switch to sync")
	}
	if cfg.ScheduleCron != "" {
		t.Errorf("expected cron cleared after switch to sync, got %q", cfg.ScheduleCron)
	}

	// after each sync → custom: cron must be set again.
	if _, err := svc.UpdateWorkflowConfig(ctx, "weekly-money-digest", service.UpdateWorkflowConfigParams{
		TriggerOnSync: wcoBool(false),
		ScheduleCron:  wcoStr("0 9 * * 5"),
	}); err != nil {
		t.Fatalf("reconfigure to custom: %v", err)
	}
	cfg, err = svc.GetWorkflowConfig(ctx, "weekly-money-digest")
	if err != nil {
		t.Fatalf("config after custom: %v", err)
	}
	if cfg.TriggerOnSync {
		t.Error("expected trigger_on_sync=false after switch back to custom")
	}
	if cfg.ScheduleCron != "0 9 * * 5" {
		t.Errorf("schedule_cron = %q, want 0 9 * * 5", cfg.ScheduleCron)
	}
}

// TestWorkflowConfigOptions_PostSyncPresetToCustomSeedsCron verifies a
// post-sync preset (no default cron) enabled as a custom schedule gets a
// seeded daily default rather than an empty cron.
func TestWorkflowConfigOptions_PostSyncPresetToCustomSeedsCron(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	if _, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{
		TriggerOnSync: wcoBool(false), // override the post-sync default
	}); err != nil {
		t.Fatalf("enable: %v", err)
	}
	cfg, err := svc.GetWorkflowConfig(ctx, "routine-reviewer")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if cfg.TriggerOnSync {
		t.Error("expected custom trigger after override")
	}
	if cfg.ScheduleCron == "" {
		t.Error("expected a seeded default cron for a post-sync preset switched to custom")
	}
}

//go:build integration && !lite

package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"breadbox/internal/service"
)

func TestEnableWorkflowFromPreset(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// All presets start "available" (not enabled).
	views, err := svc.ListWorkflowPresets(ctx)
	if err != nil {
		t.Fatalf("ListWorkflowPresets: %v", err)
	}
	if len(views) < 3 {
		t.Fatalf("expected >=3 presets, got %d", len(views))
	}
	for _, v := range views {
		if v.Enabled {
			t.Fatalf("preset %q unexpectedly enabled on a fresh DB", v.Slug)
		}
	}

	// Enable the flagship — it instantiates a workflow stamped with source_template.
	wf, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{Enabled: false})
	if err != nil {
		t.Fatalf("EnableWorkflowFromPreset: %v", err)
	}
	if wf.SourceTemplate == nil || *wf.SourceTemplate != "routine-reviewer" {
		t.Fatalf("source_template = %v, want routine-reviewer", wf.SourceTemplate)
	}
	if wf.Slug != "routine-reviewer" {
		t.Fatalf("slug = %q, want routine-reviewer", wf.Slug)
	}
	if wf.ToolScope != "read_write" {
		t.Fatalf("tool_scope = %q, want read_write", wf.ToolScope)
	}
	if !wf.TriggerOnSyncComplete {
		t.Fatalf("routine-reviewer should trigger on sync complete")
	}
	if wf.Enabled {
		t.Fatalf("workflow should be instantiated paused (Enabled=false)")
	}
	if len(wf.Prompt) == 0 {
		t.Fatalf("composed prompt is empty")
	}

	// The gallery now reflects it as enabled.
	views, err = svc.ListWorkflowPresets(ctx)
	if err != nil {
		t.Fatalf("ListWorkflowPresets: %v", err)
	}
	var found bool
	for _, v := range views {
		if v.Slug == "routine-reviewer" {
			found = true
			if !v.Enabled || v.WorkflowSlug == nil || *v.WorkflowSlug != "routine-reviewer" {
				t.Fatalf("routine-reviewer view not marked enabled: %+v", v)
			}
		}
	}
	if !found {
		t.Fatal("routine-reviewer not in preset views")
	}

	// One instance per preset — a second enable conflicts.
	if _, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{}); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("double-enable err = %v, want ErrConflict", err)
	}

	// Unknown preset -> not found.
	if _, err := svc.EnableWorkflowFromPreset(ctx, "no-such-preset", service.EnableWorkflowFromPresetParams{}); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("unknown preset err = %v, want ErrNotFound", err)
	}
}

func TestEnableWorkflowFromPreset_WithConfig(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	cron := "0 8 * * *"
	wf, err := svc.EnableWorkflowFromPreset(ctx, "weekly-money-digest", service.EnableWorkflowFromPresetParams{
		Enabled:                false,
		ScheduleCron:           &cron,
		AdditionalInstructions: "Focus on dining-out spend and call out anything unusual.",
	})
	if err != nil {
		t.Fatalf("EnableWorkflowFromPreset: %v", err)
	}
	// Schedule override applied.
	if wf.ScheduleCron == nil || *wf.ScheduleCron != cron {
		t.Fatalf("schedule_cron = %v, want %q", wf.ScheduleCron, cron)
	}
	// Additional instructions appended to the composed base prompt.
	if !strings.Contains(wf.Prompt, "Additional instructions") || !strings.Contains(wf.Prompt, "dining-out spend") {
		t.Fatalf("prompt missing appended instructions; got %d chars", len(wf.Prompt))
	}
}

func TestEnableWorkflowFromPreset_PostSyncIgnoresSchedule(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	cron := "0 8 * * *"
	// routine-reviewer is post-sync; a schedule override must be ignored.
	wf, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{ScheduleCron: &cron})
	if err != nil {
		t.Fatalf("EnableWorkflowFromPreset: %v", err)
	}
	if wf.ScheduleCron != nil {
		t.Fatalf("post-sync preset got a schedule_cron = %v, want nil", *wf.ScheduleCron)
	}
	if !wf.TriggerOnSyncComplete {
		t.Fatalf("routine-reviewer should trigger on sync")
	}
}

func TestListAllAgentRuns_WorkflowsOnly(t *testing.T) {
	svc, _, pool := newService(t)
	ctx := context.Background()

	// A preset-instantiated workflow (source_template set) ...
	wf, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{Enabled: false})
	if err != nil {
		t.Fatalf("EnableWorkflowFromPreset: %v", err)
	}
	// ... and a hand-authored agent (no source_template).
	plain := mustCreateAgentDefinition(t, svc, "plain-agent-runs", true)

	// One run apiece. short_id/id/started_at are DB-defaulted.
	for _, defID := range []string{wf.ID, plain.ID} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO agent_runs (agent_definition_id, "trigger", status) VALUES ($1, 'manual', 'success')`,
			defID); err != nil {
			t.Fatalf("insert run for %s: %v", defID, err)
		}
	}

	// Unfiltered: both runs.
	all, err := svc.ListAllAgentRuns(ctx, service.AllAgentRunListParams{Limit: 50})
	if err != nil {
		t.Fatalf("ListAllAgentRuns(all): %v", err)
	}
	if len(all.Runs) != 2 {
		t.Fatalf("unfiltered = %d runs, want 2", len(all.Runs))
	}

	// WorkflowsOnly: just the preset-backed run.
	only, err := svc.ListAllAgentRuns(ctx, service.AllAgentRunListParams{Limit: 50, WorkflowsOnly: true})
	if err != nil {
		t.Fatalf("ListAllAgentRuns(workflows): %v", err)
	}
	if len(only.Runs) != 1 {
		t.Fatalf("WorkflowsOnly = %d runs, want 1", len(only.Runs))
	}
	if only.Runs[0].AgentSlug != wf.Slug {
		t.Fatalf("WorkflowsOnly run slug = %q, want %q", only.Runs[0].AgentSlug, wf.Slug)
	}
}

func TestWorkflowsConsent(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	if svc.WorkflowsConsentAcknowledged(ctx) {
		t.Fatal("fresh household should not have acknowledged consent")
	}
	if err := svc.AcknowledgeWorkflowsConsent(ctx); err != nil {
		t.Fatalf("AcknowledgeWorkflowsConsent: %v", err)
	}
	if !svc.WorkflowsConsentAcknowledged(ctx) {
		t.Fatal("consent should be acknowledged after AcknowledgeWorkflowsConsent")
	}
}

func TestWorkflowPresetsHaveCostEstimates(t *testing.T) {
	svc, _, _ := newService(t)
	views, err := svc.ListWorkflowPresets(context.Background())
	if err != nil {
		t.Fatalf("ListWorkflowPresets: %v", err)
	}
	for _, v := range views {
		if v.EstCostPerRunUSD <= 0 {
			t.Errorf("preset %q has non-positive EstCostPerRunUSD %v", v.Slug, v.EstCostPerRunUSD)
		}
	}
}

func TestWorkflowRunStatusCounts(t *testing.T) {
	svc, _, pool := newService(t)
	ctx := context.Background()

	wf, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{Enabled: false})
	if err != nil {
		t.Fatalf("EnableWorkflowFromPreset: %v", err)
	}
	plain := mustCreateAgentDefinition(t, svc, "plain-counts", true)

	ins := func(defID, status string) {
		if _, e := pool.Exec(ctx,
			`INSERT INTO agent_runs (agent_definition_id,"trigger",status) VALUES ($1,'cron',$2)`,
			defID, status); e != nil {
			t.Fatalf("insert run: %v", e)
		}
	}
	ins(wf.ID, "success")
	ins(wf.ID, "success")
	ins(wf.ID, "error")
	ins(wf.ID, "skipped")
	ins(plain.ID, "success") // not a workflow → excluded

	counts, err := svc.WorkflowRunStatusCounts(ctx, "")
	if err != nil {
		t.Fatalf("WorkflowRunStatusCounts: %v", err)
	}
	if counts["success"] != 2 || counts["error"] != 1 || counts["skipped"] != 1 {
		t.Fatalf("counts = %v, want success:2 error:1 skipped:1 (plain excluded)", counts)
	}

	// Filtered to the one workflow — same tally.
	scoped, err := svc.WorkflowRunStatusCounts(ctx, wf.Slug)
	if err != nil {
		t.Fatalf("scoped counts: %v", err)
	}
	if scoped["success"] != 2 {
		t.Fatalf("scoped success = %d, want 2", scoped["success"])
	}
}

func TestEnableWorkflowFromPreset_ApplyModeOption(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// flag_only → the suppress-categorization directive lands in the prompt.
	flag, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{
		Options: map[string]string{"apply_mode": "flag_only"},
	})
	if err != nil {
		t.Fatalf("enable flag_only: %v", err)
	}
	if !strings.Contains(flag.Prompt, "FLAG ONLY") || !strings.Contains(flag.Prompt, "Apply mode") {
		t.Fatalf("flag_only prompt missing the apply-mode directive (%d chars)", len(flag.Prompt))
	}

	// Default (auto) on a different preset → no directive appended.
	auto, err := svc.EnableWorkflowFromPreset(ctx, "backlog-closer", service.EnableWorkflowFromPresetParams{})
	if err != nil {
		t.Fatalf("enable default: %v", err)
	}
	if strings.Contains(auto.Prompt, "FLAG ONLY") {
		t.Fatalf("auto (default) prompt should NOT contain the flag-only directive")
	}
}

//go:build integration && !lite

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/service"
)

// TestT8_WorkflowsOnly_PresetRunIncluded verifies that a run belonging to a
// preset-instantiated workflow (source_template IS NOT NULL) is returned when
// WorkflowsOnly=true.
func TestT8_WorkflowsOnly_PresetRunIncluded(t *testing.T) {
	svc, _, pool := newService(t)
	ctx := context.Background()

	wf, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{Enabled: false})
	if err != nil {
		t.Fatalf("EnableWorkflowFromPreset: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO workflow_runs (workflow_id, "trigger", status) VALUES ($1, 'manual', 'success')`,
		wf.ID); err != nil {
		t.Fatalf("insert preset run: %v", err)
	}

	result, err := svc.ListAllAgentRuns(ctx, service.AllAgentRunListParams{
		Limit:         50,
		WorkflowsOnly: true,
	})
	if err != nil {
		t.Fatalf("ListAllAgentRuns(WorkflowsOnly=true): %v", err)
	}
	if len(result.Runs) != 1 {
		t.Fatalf("WorkflowsOnly=true: got %d runs, want 1", len(result.Runs))
	}
	if result.Runs[0].AgentSlug != wf.Slug {
		t.Errorf("run agent_slug = %q, want %q", result.Runs[0].AgentSlug, wf.Slug)
	}
}

// TestT8_WorkflowsOnly_NonPresetRunExcluded verifies that a run belonging to
// a hand-authored agent (source_template IS NULL) is excluded when
// WorkflowsOnly=true.
func TestT8_WorkflowsOnly_NonPresetRunExcluded(t *testing.T) {
	svc, _, pool := newService(t)
	ctx := context.Background()

	plain := mustCreateAgentDefinition(t, svc, "t8-plain-agent", true)
	if _, err := pool.Exec(ctx,
		`INSERT INTO workflow_runs (workflow_id, "trigger", status) VALUES ($1, 'manual', 'success')`,
		plain.ID); err != nil {
		t.Fatalf("insert plain run: %v", err)
	}

	result, err := svc.ListAllAgentRuns(ctx, service.AllAgentRunListParams{
		Limit:         50,
		WorkflowsOnly: true,
	})
	if err != nil {
		t.Fatalf("ListAllAgentRuns(WorkflowsOnly=true): %v", err)
	}
	if len(result.Runs) != 0 {
		t.Fatalf("WorkflowsOnly=true with only a plain agent: got %d runs, want 0", len(result.Runs))
	}
}

// TestT8_WorkflowsOnly_UnfilteredIncludesBoth verifies that when WorkflowsOnly
// is false (the default), runs from both preset-backed workflows and
// hand-authored agents appear in the result set.
func TestT8_WorkflowsOnly_UnfilteredIncludesBoth(t *testing.T) {
	svc, _, pool := newService(t)
	ctx := context.Background()

	wf, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{Enabled: false})
	if err != nil {
		t.Fatalf("EnableWorkflowFromPreset: %v", err)
	}
	plain := mustCreateAgentDefinition(t, svc, "t8-plain-unfiltered", true)

	for _, defID := range []string{wf.ID, plain.ID} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO workflow_runs (workflow_id, "trigger", status) VALUES ($1, 'manual', 'success')`,
			defID); err != nil {
			t.Fatalf("insert run for def %s: %v", defID, err)
		}
	}

	result, err := svc.ListAllAgentRuns(ctx, service.AllAgentRunListParams{
		Limit:         50,
		WorkflowsOnly: false,
	})
	if err != nil {
		t.Fatalf("ListAllAgentRuns(WorkflowsOnly=false): %v", err)
	}
	if len(result.Runs) != 2 {
		t.Fatalf("unfiltered: got %d runs, want 2", len(result.Runs))
	}
}

// TestT8_WorkflowsOnly_OnlyPresetRunsReturnedAmongMixed verifies the
// WorkflowsOnly filter in a mixed environment: one preset workflow run and
// one plain-agent run coexist; WorkflowsOnly=true returns exactly the preset
// run with the correct slug.
func TestT8_WorkflowsOnly_OnlyPresetRunsReturnedAmongMixed(t *testing.T) {
	svc, _, pool := newService(t)
	ctx := context.Background()

	wf, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{Enabled: false})
	if err != nil {
		t.Fatalf("EnableWorkflowFromPreset: %v", err)
	}
	plain := mustCreateAgentDefinition(t, svc, "t8-mixed-plain", true)

	for _, defID := range []string{wf.ID, plain.ID} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO workflow_runs (workflow_id, "trigger", status) VALUES ($1, 'manual', 'success')`,
			defID); err != nil {
			t.Fatalf("insert run for def %s: %v", defID, err)
		}
	}

	only, err := svc.ListAllAgentRuns(ctx, service.AllAgentRunListParams{
		Limit:         50,
		WorkflowsOnly: true,
	})
	if err != nil {
		t.Fatalf("ListAllAgentRuns(WorkflowsOnly=true): %v", err)
	}
	if len(only.Runs) != 1 {
		t.Fatalf("WorkflowsOnly=true: got %d runs, want 1", len(only.Runs))
	}
	if only.Runs[0].AgentSlug != wf.Slug {
		t.Errorf("filtered run slug = %q, want preset slug %q", only.Runs[0].AgentSlug, wf.Slug)
	}
}

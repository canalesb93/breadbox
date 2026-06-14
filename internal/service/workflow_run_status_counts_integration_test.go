//go:build integration && !lite

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/service"
)

// TestT7WorkflowRunStatusCounts_EmptyDB verifies that an empty database returns
// an empty map (not nil, no error) when no runs exist.
func TestT7WorkflowRunStatusCounts_EmptyDB(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	counts, err := svc.WorkflowRunStatusCounts(ctx, "")
	if err != nil {
		t.Fatalf("WorkflowRunStatusCounts on empty DB: %v", err)
	}
	if counts == nil {
		t.Fatal("expected non-nil map, got nil")
	}
	if len(counts) != 0 {
		t.Fatalf("expected empty map on empty DB, got %v", counts)
	}
}

// TestT7WorkflowRunStatusCounts_CustomWorkflowIncluded confirms that runs
// belonging to a custom/hand-authored workflow (source_template IS NULL) ARE
// counted. /workflows/runs is the single runs surface now (/agents is
// retired), so every workflow's runs must appear — the legacy
// source_template IS NOT NULL gate wrongly hid custom workflows added via the
// custom-workflow drawer.
func TestT7WorkflowRunStatusCounts_CustomWorkflowIncluded(t *testing.T) {
	svc, _, pool := newService(t)
	ctx := context.Background()

	custom := mustCreateAgentDefinition(t, svc, "t7-custom-included", true)

	// Insert a run for the custom workflow (no preset backing).
	if _, err := pool.Exec(ctx,
		`INSERT INTO workflow_runs (workflow_id,"trigger",status) VALUES ($1,'manual','success')`,
		custom.ID); err != nil {
		t.Fatalf("insert custom run: %v", err)
	}

	counts, err := svc.WorkflowRunStatusCounts(ctx, "")
	if err != nil {
		t.Fatalf("WorkflowRunStatusCounts: %v", err)
	}
	if counts["success"] != 1 {
		t.Fatalf("expected custom workflow run counted (success=1), got %v", counts)
	}
}

// TestT7WorkflowRunStatusCounts_SinglePreset verifies per-status aggregation
// across several runs for one preset-sourced workflow. Statuses with zero runs
// must be absent from the returned map.
func TestT7WorkflowRunStatusCounts_SinglePreset(t *testing.T) {
	svc, _, pool := newService(t)
	ctx := context.Background()

	wf, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{Enabled: false})
	if err != nil {
		t.Fatalf("EnableWorkflowFromPreset: %v", err)
	}

	ins := func(status string) {
		t.Helper()
		if _, e := pool.Exec(ctx,
			`INSERT INTO workflow_runs (workflow_id,"trigger",status) VALUES ($1,'cron',$2)`,
			wf.ID, status); e != nil {
			t.Fatalf("insert run (status=%s): %v", status, e)
		}
	}

	// Seed: 3 success, 2 error, 1 skipped — timeout and in_progress absent.
	ins("success")
	ins("success")
	ins("success")
	ins("error")
	ins("error")
	ins("skipped")

	counts, err := svc.WorkflowRunStatusCounts(ctx, "")
	if err != nil {
		t.Fatalf("WorkflowRunStatusCounts: %v", err)
	}

	if counts["success"] != 3 {
		t.Errorf("success = %d, want 3", counts["success"])
	}
	if counts["error"] != 2 {
		t.Errorf("error = %d, want 2", counts["error"])
	}
	if counts["skipped"] != 1 {
		t.Errorf("skipped = %d, want 1", counts["skipped"])
	}

	// Statuses with zero runs must be absent — not returned as zero.
	if _, ok := counts["timeout"]; ok {
		t.Errorf("timeout should be absent (zero runs), got %d", counts["timeout"])
	}
	if _, ok := counts["in_progress"]; ok {
		t.Errorf("in_progress should be absent (zero runs), got %d", counts["in_progress"])
	}
}

// TestT7WorkflowRunStatusCounts_AllStatuses seeds one run of every valid
// status and checks that all five appear in the result.
func TestT7WorkflowRunStatusCounts_AllStatuses(t *testing.T) {
	svc, _, pool := newService(t)
	ctx := context.Background()

	wf, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{Enabled: false})
	if err != nil {
		t.Fatalf("EnableWorkflowFromPreset: %v", err)
	}

	for _, status := range []string{"in_progress", "success", "error", "timeout", "skipped"} {
		if _, e := pool.Exec(ctx,
			`INSERT INTO workflow_runs (workflow_id,"trigger",status) VALUES ($1,'cron',$2)`,
			wf.ID, status); e != nil {
			t.Fatalf("insert run (status=%s): %v", status, e)
		}
	}

	counts, err := svc.WorkflowRunStatusCounts(ctx, "")
	if err != nil {
		t.Fatalf("WorkflowRunStatusCounts: %v", err)
	}

	for _, want := range []string{"in_progress", "success", "error", "timeout", "skipped"} {
		if counts[want] != 1 {
			t.Errorf("status %q: got %d, want 1 (counts=%v)", want, counts[want], counts)
		}
	}
}

// TestT7WorkflowRunStatusCounts_ScopedBySlug verifies that filtering by slug
// returns only that workflow's runs, leaving sibling-workflow runs excluded.
func TestT7WorkflowRunStatusCounts_ScopedBySlug(t *testing.T) {
	svc, _, pool := newService(t)
	ctx := context.Background()

	wfA, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{Enabled: false})
	if err != nil {
		t.Fatalf("EnableWorkflowFromPreset routine-reviewer: %v", err)
	}
	wfB, err := svc.EnableWorkflowFromPreset(ctx, "weekly-money-digest", service.EnableWorkflowFromPresetParams{Enabled: false})
	if err != nil {
		t.Fatalf("EnableWorkflowFromPreset weekly-money-digest: %v", err)
	}

	ins := func(defID, status string) {
		t.Helper()
		if _, e := pool.Exec(ctx,
			`INSERT INTO workflow_runs (workflow_id,"trigger",status) VALUES ($1,'cron',$2)`,
			defID, status); e != nil {
			t.Fatalf("insert run: %v", e)
		}
	}

	// wfA: 2 success, 1 error.
	ins(wfA.ID, "success")
	ins(wfA.ID, "success")
	ins(wfA.ID, "error")
	// wfB: 1 success (must not bleed into wfA's scoped view).
	ins(wfB.ID, "success")

	// Unfiltered: wfA (2 success + 1 error) + wfB (1 success) = 3 success, 1 error.
	all, err := svc.WorkflowRunStatusCounts(ctx, "")
	if err != nil {
		t.Fatalf("unfiltered counts: %v", err)
	}
	if all["success"] != 3 {
		t.Errorf("unfiltered success = %d, want 3", all["success"])
	}
	if all["error"] != 1 {
		t.Errorf("unfiltered error = %d, want 1", all["error"])
	}

	// Scoped to wfA by slug: 2 success, 1 error — wfB's run excluded.
	scoped, err := svc.WorkflowRunStatusCounts(ctx, wfA.Slug)
	if err != nil {
		t.Fatalf("scoped counts (slug): %v", err)
	}
	if scoped["success"] != 2 {
		t.Errorf("scoped success = %d, want 2", scoped["success"])
	}
	if scoped["error"] != 1 {
		t.Errorf("scoped error = %d, want 1", scoped["error"])
	}
	if len(scoped) != 2 {
		t.Errorf("scoped map has %d entries, want 2 (success+error): %v", len(scoped), scoped)
	}
}

// TestT7WorkflowRunStatusCounts_ScopedByShortID verifies that the short_id
// lookup path works as an alternative to slug-based filtering.
func TestT7WorkflowRunStatusCounts_ScopedByShortID(t *testing.T) {
	svc, _, pool := newService(t)
	ctx := context.Background()

	wf, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{Enabled: false})
	if err != nil {
		t.Fatalf("EnableWorkflowFromPreset: %v", err)
	}

	if _, e := pool.Exec(ctx,
		`INSERT INTO workflow_runs (workflow_id,"trigger",status) VALUES ($1,'cron','success')`,
		wf.ID); e != nil {
		t.Fatalf("insert run: %v", e)
	}

	if wf.ShortID == "" {
		t.Fatal("workflow short_id is empty — fixture broken")
	}

	counts, err := svc.WorkflowRunStatusCounts(ctx, wf.ShortID)
	if err != nil {
		t.Fatalf("WorkflowRunStatusCounts by short_id: %v", err)
	}
	if counts["success"] != 1 {
		t.Errorf("success by short_id = %d, want 1 (counts=%v)", counts["success"], counts)
	}
}

// TestT7WorkflowRunStatusCounts_UnknownFilter verifies that an unrecognised
// filter string returns an empty map rather than an error (mirrors the Runs
// page behaviour of dropping a bad workflow filter gracefully).
func TestT7WorkflowRunStatusCounts_UnknownFilter(t *testing.T) {
	svc, _, pool := newService(t)
	ctx := context.Background()

	wf, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{Enabled: false})
	if err != nil {
		t.Fatalf("EnableWorkflowFromPreset: %v", err)
	}

	if _, e := pool.Exec(ctx,
		`INSERT INTO workflow_runs (workflow_id,"trigger",status) VALUES ($1,'cron','success')`,
		wf.ID); e != nil {
		t.Fatalf("insert run: %v", e)
	}

	counts, err := svc.WorkflowRunStatusCounts(ctx, "no-such-workflow-slug")
	if err != nil {
		t.Fatalf("expected no error for unknown filter, got: %v", err)
	}
	if len(counts) != 0 {
		t.Fatalf("expected empty map for unknown filter, got %v", counts)
	}
}

// TestT7WorkflowRunStatusCounts_MultiPreset verifies that the unscoped call
// aggregates runs across multiple distinct preset-sourced workflows.
func TestT7WorkflowRunStatusCounts_MultiPreset(t *testing.T) {
	svc, _, pool := newService(t)
	ctx := context.Background()

	wfA, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{Enabled: false})
	if err != nil {
		t.Fatalf("EnableWorkflowFromPreset routine-reviewer: %v", err)
	}
	wfB, err := svc.EnableWorkflowFromPreset(ctx, "weekly-money-digest", service.EnableWorkflowFromPresetParams{Enabled: false})
	if err != nil {
		t.Fatalf("EnableWorkflowFromPreset weekly-money-digest: %v", err)
	}
	wfC, err := svc.EnableWorkflowFromPreset(ctx, "backlog-closer", service.EnableWorkflowFromPresetParams{Enabled: false})
	if err != nil {
		t.Fatalf("EnableWorkflowFromPreset backlog-closer: %v", err)
	}

	ins := func(defID, status string) {
		t.Helper()
		if _, e := pool.Exec(ctx,
			`INSERT INTO workflow_runs (workflow_id,"trigger",status) VALUES ($1,'cron',$2)`,
			defID, status); e != nil {
			t.Fatalf("insert run: %v", e)
		}
	}

	ins(wfA.ID, "success")
	ins(wfA.ID, "error")
	ins(wfB.ID, "success")
	ins(wfB.ID, "skipped")
	ins(wfC.ID, "timeout")

	// Also insert a custom-workflow run (source_template NULL) to confirm it
	// is now INCLUDED — every workflow's runs surface on /workflows/runs.
	custom := mustCreateAgentDefinition(t, svc, "t7-multi-custom", true)
	ins(custom.ID, "success")

	counts, err := svc.WorkflowRunStatusCounts(ctx, "")
	if err != nil {
		t.Fatalf("WorkflowRunStatusCounts: %v", err)
	}

	// Preset-backed: 2 success, 1 error, 1 skipped, 1 timeout — plus the custom
	// workflow's 1 success = 3 success total.
	if counts["success"] != 3 {
		t.Errorf("success = %d, want 3 (2 preset + 1 custom)", counts["success"])
	}
	if counts["error"] != 1 {
		t.Errorf("error = %d, want 1", counts["error"])
	}
	if counts["skipped"] != 1 {
		t.Errorf("skipped = %d, want 1", counts["skipped"])
	}
	if counts["timeout"] != 1 {
		t.Errorf("timeout = %d, want 1", counts["timeout"])
	}
	if _, ok := counts["in_progress"]; ok {
		t.Errorf("in_progress should be absent, got %d", counts["in_progress"])
	}
	total := counts["success"] + counts["error"] + counts["skipped"] + counts["timeout"]
	if total != 6 {
		t.Errorf("total workflow runs = %d, want 6 (5 preset + 1 custom)", total)
	}
}

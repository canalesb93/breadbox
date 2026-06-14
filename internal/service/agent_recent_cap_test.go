//go:build integration && !lite

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5/pgtype"
)

// TestListAgentDefinitions_PopulatesRecentCapStats pins the iter-32
// aggregate rollup: the list response carries recent_cap_stats based on
// the last 5 non-skipped runs, with skipped rows excluded. Seeds a mix
// of clean, capped, and skipped runs and verifies the totals.
func TestListAgentDefinitions_PopulatesRecentCapStats(t *testing.T) {
	svc, q, _ := newService(t)
	ctx := context.Background()

	def := mustCreateAgentDefinition(t, svc, "svc-cap-stats", true)
	defUUID, _ := pgconv.ParseUUID(def.ID)

	// 4 non-skipped runs: 2 capped, 2 clean. Plus 1 skipped (should not
	// affect the count or window).
	for i := 0; i < 4; i++ {
		mustInsertCompletedRun(t, q, defUUID, "0.01")
	}
	mustInsertSkippedRun(t, q, defUUID)

	// Mark the two most-recent rows as capped.
	rows, err := q.ListAgentRuns(ctx, db.ListAgentRunsParams{
		WorkflowID: defUUID,
		Limit:             5,
		Offset:            0,
	})
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(rows) < 4 {
		t.Fatalf("expected >=4 rows, got %d", len(rows))
	}
	// Skip any skipped row when marking caps.
	capped := 0
	for _, r := range rows {
		if r.Status == "skipped" {
			continue
		}
		if capped >= 2 {
			break
		}
		if _, err := q.SetAgentRunHitCap(ctx, db.SetAgentRunHitCapParams{
			ID:     r.ID,
			HitCap: pgtype.Text{String: "max_turns", Valid: true},
		}); err != nil {
			t.Fatalf("set hit_cap: %v", err)
		}
		capped++
	}

	list, err := svc.ListAgentDefinitions(ctx)
	if err != nil {
		t.Fatalf("list defs: %v", err)
	}
	for i := range list {
		if list[i].Slug != def.Slug {
			continue
		}
		got := list[i].RecentCapStats
		if got == nil {
			t.Fatal("RecentCapStats nil, want non-nil")
		}
		if got.CapCount != 2 {
			t.Errorf("CapCount = %d, want 2", got.CapCount)
		}
		// 4 non-skipped runs; window is last 5 so all 4 are eligible.
		if got.RunCount != 4 {
			t.Errorf("RunCount = %d, want 4 (skipped excluded)", got.RunCount)
		}
		return
	}
	t.Fatal("created agent missing from list response")
}

// TestListAgentDefinitions_RecentCapStats_NoCapped_LeavesZero verifies
// agents with no cap-exhausted runs surface cap_count=0 (not nil) so the
// SPA threshold check works the same way regardless of history depth.
func TestListAgentDefinitions_RecentCapStats_NoCapped_LeavesZero(t *testing.T) {
	svc, q, _ := newService(t)
	ctx := context.Background()
	def := mustCreateAgentDefinition(t, svc, "svc-cap-none", true)
	defUUID, _ := pgconv.ParseUUID(def.ID)

	mustInsertCompletedRun(t, q, defUUID, "0.01")

	list, err := svc.ListAgentDefinitions(ctx)
	if err != nil {
		t.Fatalf("list defs: %v", err)
	}
	for i := range list {
		if list[i].Slug != def.Slug {
			continue
		}
		if list[i].RecentCapStats == nil {
			t.Fatal("expected RecentCapStats non-nil (with cap_count=0) once history exists")
		}
		if list[i].RecentCapStats.CapCount != 0 {
			t.Errorf("CapCount = %d, want 0", list[i].RecentCapStats.CapCount)
		}
		return
	}
	t.Fatal("created agent missing from list response")
}

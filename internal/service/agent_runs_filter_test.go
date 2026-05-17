//go:build integration && !lite

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"

	"github.com/jackc/pgx/v5/pgtype"
)

// TestListAgentRuns_ScansNewColumns is a regression test for an iter-22
// through iter-27 bug: ListAgentRuns' hand-rolled SELECT was missing
// operator_note, prompt_prefix, and hit_cap, so the v2 SPA run history
// never showed those pills even when the underlying row had them. The
// fix landed in iter-31 (this iteration); this test pins it.
func TestListAgentRuns_ScansNewColumns(t *testing.T) {
	svc, q, _ := newService(t)
	ctx := context.Background()
	def := mustCreateAgentDefinition(t, svc, "list-scan-fix", true)
	defUUID, _ := pgconv.ParseUUID(def.ID)

	// Seed one run with all three of the previously-missing fields set.
	mustInsertCompletedRun(t, q, defUUID, "0.05")
	// Backfill the new fields on the latest row.
	row, err := q.GetLatestAgentRun(ctx, defUUID)
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if err := q.SetAgentRunPromptPrefix(ctx, db.SetAgentRunPromptPrefixParams{
		ID:           row.ID,
		PromptPrefix: pgtype.Text{String: "iter-23 prefix payload", Valid: true},
	}); err != nil {
		t.Fatalf("set prefix: %v", err)
	}
	if _, err := q.SetAgentRunNote(ctx, db.SetAgentRunNoteParams{
		ID:           row.ID,
		OperatorNote: pgtype.Text{String: "iter-22 note payload", Valid: true},
	}); err != nil {
		t.Fatalf("set note: %v", err)
	}
	if _, err := q.SetAgentRunHitCap(ctx, db.SetAgentRunHitCapParams{
		ID:     row.ID,
		HitCap: pgtype.Text{String: "max_turns", Valid: true},
	}); err != nil {
		t.Fatalf("set hit_cap: %v", err)
	}

	list, err := svc.ListAgentRuns(ctx, def.Slug, service.AgentRunListParams{})
	if err != nil {
		t.Fatalf("ListAgentRuns: %v", err)
	}
	if len(list.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(list.Runs))
	}
	got := list.Runs[0]
	if got.PromptPrefix == nil || *got.PromptPrefix != "iter-23 prefix payload" {
		t.Errorf("PromptPrefix from list = %v, want \"iter-23 prefix payload\"", got.PromptPrefix)
	}
	if got.OperatorNote == nil || *got.OperatorNote != "iter-22 note payload" {
		t.Errorf("OperatorNote from list = %v, want \"iter-22 note payload\"", got.OperatorNote)
	}
	if got.HitCap == nil || *got.HitCap != "max_turns" {
		t.Errorf("HitCap from list = %v, want \"max_turns\"", got.HitCap)
	}
}

// TestListAgentRuns_HitCapFilter exercises the new hit_cap filter param.
// Seeds a mix of runs (clean, max_turns, max_budget) and verifies each
// filter mode (specific value, "any", and empty=all) returns the right
// subset.
func TestListAgentRuns_HitCapFilter(t *testing.T) {
	svc, q, _ := newService(t)
	ctx := context.Background()
	def := mustCreateAgentDefinition(t, svc, "list-hitcap", true)
	defUUID, _ := pgconv.ParseUUID(def.ID)

	// Three runs: clean, max_turns, max_budget. Order matters for the
	// scan ordering check (DESC by started_at).
	mustInsertCompletedRun(t, q, defUUID, "0.01") // clean
	cleanRow, _ := q.GetLatestAgentRun(ctx, defUUID)

	mustInsertCompletedRun(t, q, defUUID, "0.01")
	maxTurnsRow, _ := q.GetLatestAgentRun(ctx, defUUID)
	if _, err := q.SetAgentRunHitCap(ctx, db.SetAgentRunHitCapParams{
		ID:     maxTurnsRow.ID,
		HitCap: pgtype.Text{String: "max_turns", Valid: true},
	}); err != nil {
		t.Fatalf("set max_turns cap: %v", err)
	}

	mustInsertCompletedRun(t, q, defUUID, "0.01")
	budgetRow, _ := q.GetLatestAgentRun(ctx, defUUID)
	if _, err := q.SetAgentRunHitCap(ctx, db.SetAgentRunHitCapParams{
		ID:     budgetRow.ID,
		HitCap: pgtype.Text{String: "max_budget", Valid: true},
	}); err != nil {
		t.Fatalf("set max_budget cap: %v", err)
	}
	_ = cleanRow // (silence unused — useful for debugging assertions if they fail)

	cases := []struct {
		name      string
		filter    string
		wantCount int
	}{
		{"empty filter returns all 3", "", 3},
		{"any returns 2 capped runs", "any", 2},
		{"max_turns returns 1", "max_turns", 1},
		{"max_budget returns 1", "max_budget", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			list, err := svc.ListAgentRuns(ctx, def.Slug, service.AgentRunListParams{
				HitCap: tc.filter,
			})
			if err != nil {
				t.Fatalf("ListAgentRuns: %v", err)
			}
			if len(list.Runs) != tc.wantCount {
				t.Errorf("got %d runs, want %d (filter=%q)",
					len(list.Runs), tc.wantCount, tc.filter)
			}
		})
	}
}

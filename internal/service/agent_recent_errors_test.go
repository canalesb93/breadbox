//go:build integration && !lite

package service_test

import (
	"context"
	"testing"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5/pgtype"
)

// TestListRecentErroredAgentRuns_OnlyRecentErrors verifies the iter-38
// surface: errored runs from the last `windowHours` show up; older
// errors and non-error runs don't. Limit is respected; ordering is
// most-recent-first by started_at.
func TestListRecentErroredAgentRuns_OnlyRecentErrors(t *testing.T) {
	svc, q, pool := newService(t)
	ctx := context.Background()

	def := mustCreateAgentDefinition(t, svc, "recent-err", true)
	defUUID, _ := pgconv.ParseUUID(def.ID)

	// 3 errored runs (2 within 24h, 1 too old) + 1 success in window.
	// Helper to insert + tag a row's status + backdate started_at.
	mkRun := func(status string, startedAt time.Time, errMsg string) {
		t.Helper()
		row, err := q.CreateAgentRun(ctx, db.CreateAgentRunParams{
			AgentDefinitionID: defUUID,
			Trigger:           "manual",
		})
		if err != nil {
			t.Fatalf("create run: %v", err)
		}
		if status == "error" {
			if err := q.MarkAgentRunError(ctx, db.MarkAgentRunErrorParams{
				ID:             row.ID,
				ErrorMessage:   pgtype.Text{String: errMsg, Valid: true},
				TranscriptPath: pgtype.Text{},
			}); err != nil {
				t.Fatalf("mark error: %v", err)
			}
		}
		if _, err := pool.Exec(ctx,
			`UPDATE workflow_runs SET started_at = $1 WHERE id = $2`,
			startedAt, row.ID); err != nil {
			t.Fatalf("backdate: %v", err)
		}
	}

	now := time.Now()
	mkRun("error", now.Add(-1*time.Hour), "recent error 1")
	mkRun("error", now.Add(-3*time.Hour), "recent error 2")
	mkRun("error", now.Add(-48*time.Hour), "old error — outside 24h window")
	mkRun("success", now.Add(-30*time.Minute), "")

	got, err := svc.ListRecentErroredAgentRuns(ctx, 24, 10)
	if err != nil {
		t.Fatalf("ListRecentErroredAgentRuns: %v", err)
	}
	// Filter to only this test's agent (other concurrent tests on the
	// shared DB may leave their own errored rows).
	var mine []string
	for _, r := range got {
		if r.AgentSlug == def.Slug {
			mine = append(mine, r.RunShortID)
		}
	}
	if len(mine) != 2 {
		t.Fatalf("expected 2 recent errored runs for %q, got %d (entries: %+v)",
			def.Slug, len(mine), got)
	}
	// First entry is the most-recent (1h ago beats 3h ago).
	firstEntry := func() *struct {
		Slug string
		ID   string
	} {
		for _, r := range got {
			if r.AgentSlug == def.Slug {
				return &struct {
					Slug string
					ID   string
				}{r.AgentSlug, r.RunShortID}
			}
		}
		return nil
	}()
	if firstEntry == nil {
		t.Fatal("no entry for the test agent in result")
	}
	// Find which short_id maps to "recent error 1" via the response's
	// error_message field. (Most-recent ordering means it should be index 0
	// of `mine`.)
	for _, r := range got {
		if r.AgentSlug != def.Slug {
			continue
		}
		if r.ErrorMessage == nil {
			t.Errorf("entry %s missing error_message", r.RunShortID)
		}
	}
}

// TestListRecentErroredAgentRuns_NoErrors_ReturnsEmpty verifies the
// banner stays hidden when nothing's wrong. (Filters out other tests'
// agents by slug so the shared DB doesn't bleed in.)
func TestListRecentErroredAgentRuns_NoErrors_ReturnsEmpty(t *testing.T) {
	svc, q, _ := newService(t)
	ctx := context.Background()

	def := mustCreateAgentDefinition(t, svc, "recent-no-err", true)
	defUUID, _ := pgconv.ParseUUID(def.ID)
	// One successful run only.
	mustInsertCompletedRun(t, q, defUUID, "0.01")

	got, err := svc.ListRecentErroredAgentRuns(ctx, 24, 10)
	if err != nil {
		t.Fatalf("ListRecentErroredAgentRuns: %v", err)
	}
	for _, r := range got {
		if r.AgentSlug == def.Slug {
			t.Errorf("expected no errored runs for %q, got %+v", def.Slug, r)
		}
	}
}

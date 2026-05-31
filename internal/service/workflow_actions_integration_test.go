//go:build integration && !lite

package service_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
)

// f1MustInsertCompletedRun creates and completes a run for the given workflow
// definition. Kept local (F1 prefix) so it can't collide with sibling test
// helpers in the shared service_test package.
func f1MustInsertCompletedRun(t *testing.T, q *db.Queries, defID pgtype.UUID) {
	t.Helper()
	run, err := q.CreateAgentRun(context.Background(), db.CreateAgentRunParams{
		AgentDefinitionID: defID,
		Trigger:           "manual",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	var cost pgtype.Numeric
	if err := cost.Scan("0.0100"); err != nil {
		t.Fatalf("numeric scan: %v", err)
	}
	if _, err := q.CompleteAgentRun(context.Background(), db.CompleteAgentRunParams{
		ID:                  run.ID,
		Status:              "success",
		DurationMs:          pgtype.Int4{Int32: 120, Valid: true},
		TotalCostUsd:        cost,
		InputTokens:         pgtype.Int4{Int32: 10, Valid: true},
		OutputTokens:        pgtype.Int4{Int32: 5, Valid: true},
		CacheReadTokens:     pgtype.Int4{Int32: 0, Valid: true},
		CacheCreationTokens: pgtype.Int4{Int32: 0, Valid: true},
		TurnCount:           pgtype.Int4{Int32: 1, Valid: true},
		MaxTurnsUsed:        pgtype.Int4{Int32: 10, Valid: true},
		NumToolCalls:        pgtype.Int4{Int32: 0, Valid: true},
		TranscriptPath:      pgtype.Text{},
		SessionID:           pgtype.Text{},
	}); err != nil {
		t.Fatalf("complete run: %v", err)
	}
}

// TestF1GetEnabledWorkflowLastRuns verifies the gallery last-run projection:
// a never-run workflow is absent from the map (callers treat that as "never
// run"), and once a run exists it surfaces keyed by the workflow's slug with
// the run's status carried through.
func TestF1GetEnabledWorkflowLastRuns(t *testing.T) {
	svc, q, _ := newService(t)
	ctx := context.Background()

	// No workflows instantiated yet → empty map, no error.
	runs, err := svc.GetEnabledWorkflowLastRuns(ctx)
	if err != nil {
		t.Fatalf("GetEnabledWorkflowLastRuns (empty): %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected no last-runs on a fresh DB, got %d", len(runs))
	}

	// Instantiate a workflow from a preset; it has no run history yet.
	wf, err := svc.EnableWorkflowFromPreset(ctx, "routine-reviewer", service.EnableWorkflowFromPresetParams{Enabled: false})
	if err != nil {
		t.Fatalf("EnableWorkflowFromPreset: %v", err)
	}

	runs, err = svc.GetEnabledWorkflowLastRuns(ctx)
	if err != nil {
		t.Fatalf("GetEnabledWorkflowLastRuns (no runs): %v", err)
	}
	if _, ok := runs["routine-reviewer"]; ok {
		t.Fatalf("workflow with no run history should be absent from the map")
	}

	// Record a completed run and re-query.
	defID, err := pgconv.ParseUUID(wf.ID)
	if err != nil {
		t.Fatalf("parse workflow id: %v", err)
	}
	f1MustInsertCompletedRun(t, q, defID)

	runs, err = svc.GetEnabledWorkflowLastRuns(ctx)
	if err != nil {
		t.Fatalf("GetEnabledWorkflowLastRuns (with run): %v", err)
	}
	last, ok := runs["routine-reviewer"]
	if !ok || last == nil {
		t.Fatalf("expected a last-run for routine-reviewer, got %+v", runs)
	}
	if last.Status != "success" {
		t.Fatalf("last-run status = %q, want success", last.Status)
	}
	if last.ShortID == "" {
		t.Fatal("last-run is missing its short_id")
	}
}

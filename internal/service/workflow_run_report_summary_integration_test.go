//go:build integration && !lite

// Integration tests for ListReportSummariesForRunIDs.
// Prefix: T22
// Run with: make test-integration
package service_test

import (
	"context"
	"testing"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"

	"github.com/jackc/pgx/v5/pgtype"
)

// T22_mustCreateRunWithReports creates an agent definition + run, attaches
// numReports agent reports to that run (via the agentRunShortID path in
// CreateAgentReport), and returns the run UUID string and the list of created
// report summaries in insertion order.
func T22_mustCreateRunWithReports(
	t *testing.T,
	svc *service.Service,
	q *db.Queries,
	slug string,
	priorities []string,
) (runID string, reports []service.AgentRunReportSummary) {
	t.Helper()
	ctx := context.Background()

	// Create an agent definition so we can create a run.
	def, err := svc.CreateAgentDefinition(ctx, service.CreateAgentDefinitionParams{
		Name:   "T22 Agent " + slug,
		Slug:   slug,
		Prompt: "Test workflow for T22.",
	})
	if err != nil {
		t.Fatalf("T22: CreateAgentDefinition(%q): %v", slug, err)
	}
	defUUID, err := pgconv.ParseUUID(def.ID)
	if err != nil {
		t.Fatalf("T22: parse def UUID: %v", err)
	}

	// Insert a run row.
	run, err := q.CreateAgentRun(ctx, db.CreateAgentRunParams{
		WorkflowID: defUUID,
		Trigger:           "manual",
	})
	if err != nil {
		t.Fatalf("T22: CreateAgentRun for %q: %v", slug, err)
	}
	runID = pgconv.FormatUUID(run.ID)

	actor := service.Actor{Type: "agent", ID: def.ID, Name: "T22 Agent"}

	// Create each report with a small sleep to ensure created_at ordering is
	// deterministic — the ListReportSummariesForRunIDs query orders by
	// created_at ASC so oldest-first matters.
	for i, priority := range priorities {
		title := slug + "-report-" + priority
		report, cerr := svc.CreateAgentReport(ctx, title, "body", actor, priority, nil, "", "", run.ShortID)
		if cerr != nil {
			t.Fatalf("T22: CreateAgentReport[%d] for run %s: %v", i, slug, cerr)
		}
		reports = append(reports, service.AgentRunReportSummary{
			ShortID:  report.ShortID,
			Title:    report.Title,
			Priority: report.Priority,
		})
		// Tiny sleep so created_at is strictly increasing across rows.
		time.Sleep(5 * time.Millisecond)
	}

	return runID, reports
}

// T22_mapKeys is a small test-only helper that returns the keys of a
// map[string][]AgentRunReportSummary for diagnostic output.
func T22_mapKeys(m map[string][]service.AgentRunReportSummary) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// T22TestListReportSummariesForRunIDs_RunWithReports verifies that a run
// linked to multiple reports has all its summaries returned in oldest->newest
// order with correct short_id, title, and priority.
func T22TestListReportSummariesForRunIDs_RunWithReports(t *testing.T) {
	svc, q, _ := newService(t)
	ctx := context.Background()

	runID, wantReports := T22_mustCreateRunWithReports(
		t, svc, q,
		"t22-with-reports",
		[]string{"info", "warning", "critical"},
	)

	got, err := svc.ListReportSummariesForRunIDs(ctx, []string{runID})
	if err != nil {
		t.Fatalf("T22: ListReportSummariesForRunIDs: %v", err)
	}

	summaries, ok := got[runID]
	if !ok {
		t.Fatalf("T22: run %q absent from result map; map keys = %v", runID, T22_mapKeys(got))
	}
	if len(summaries) != len(wantReports) {
		t.Fatalf("T22: got %d summaries, want %d", len(summaries), len(wantReports))
	}

	// Verify oldest->newest order and field correctness.
	for i, want := range wantReports {
		s := summaries[i]
		if s.ShortID != want.ShortID {
			t.Errorf("T22: summaries[%d].ShortID = %q, want %q", i, s.ShortID, want.ShortID)
		}
		if s.Title != want.Title {
			t.Errorf("T22: summaries[%d].Title = %q, want %q", i, s.Title, want.Title)
		}
		if s.Priority != want.Priority {
			t.Errorf("T22: summaries[%d].Priority = %q, want %q", i, s.Priority, want.Priority)
		}
	}
}

// T22TestListReportSummariesForRunIDs_RunWithNoReports verifies that a run
// that has no associated reports is absent from the result map (not present
// with an empty slice).
func T22TestListReportSummariesForRunIDs_RunWithNoReports(t *testing.T) {
	svc, q, _ := newService(t)
	ctx := context.Background()

	// Create a run but attach zero reports.
	def, err := svc.CreateAgentDefinition(ctx, service.CreateAgentDefinitionParams{
		Name:   "T22 Agent t22-no-reports",
		Slug:   "t22-no-reports",
		Prompt: "Test workflow with no reports.",
	})
	if err != nil {
		t.Fatalf("T22: CreateAgentDefinition: %v", err)
	}
	defUUID, err := pgconv.ParseUUID(def.ID)
	if err != nil {
		t.Fatalf("T22: parse def UUID: %v", err)
	}
	run, err := q.CreateAgentRun(ctx, db.CreateAgentRunParams{
		WorkflowID: defUUID,
		Trigger:           "manual",
	})
	if err != nil {
		t.Fatalf("T22: CreateAgentRun: %v", err)
	}
	runID := pgconv.FormatUUID(run.ID)

	got, err := svc.ListReportSummariesForRunIDs(ctx, []string{runID})
	if err != nil {
		t.Fatalf("T22: ListReportSummariesForRunIDs: %v", err)
	}
	if _, present := got[runID]; present {
		t.Errorf("T22: run with no reports should be absent from map, but it is present with %d entry(ies)", len(got[runID]))
	}
}

// T22TestListReportSummariesForRunIDs_MixedBatch verifies that when multiple
// run IDs are batched together, only runs that have reports appear in the
// result and their summaries are independent of each other.
func T22TestListReportSummariesForRunIDs_MixedBatch(t *testing.T) {
	svc, q, _ := newService(t)
	ctx := context.Background()

	// Run A: 2 reports.
	runAID, wantA := T22_mustCreateRunWithReports(
		t, svc, q,
		"t22-mixed-a",
		[]string{"info", "critical"},
	)

	// Run B: no reports.
	defB, err := svc.CreateAgentDefinition(ctx, service.CreateAgentDefinitionParams{
		Name:   "T22 Agent t22-mixed-b",
		Slug:   "t22-mixed-b",
		Prompt: "Run B has no reports.",
	})
	if err != nil {
		t.Fatalf("T22: CreateAgentDefinition B: %v", err)
	}
	defBUUID, err := pgconv.ParseUUID(defB.ID)
	if err != nil {
		t.Fatalf("T22: parse defB UUID: %v", err)
	}
	runB, err := q.CreateAgentRun(ctx, db.CreateAgentRunParams{
		WorkflowID: defBUUID,
		Trigger:           "cron",
	})
	if err != nil {
		t.Fatalf("T22: CreateAgentRun B: %v", err)
	}
	runBID := pgconv.FormatUUID(runB.ID)

	got, err := svc.ListReportSummariesForRunIDs(ctx, []string{runAID, runBID})
	if err != nil {
		t.Fatalf("T22: ListReportSummariesForRunIDs: %v", err)
	}

	// Run A must be present with 2 summaries.
	sumA, ok := got[runAID]
	if !ok {
		t.Fatalf("T22: run A absent from result map")
	}
	if len(sumA) != len(wantA) {
		t.Fatalf("T22: run A: got %d summaries, want %d", len(sumA), len(wantA))
	}
	for i, want := range wantA {
		if sumA[i].ShortID != want.ShortID {
			t.Errorf("T22: run A summaries[%d].ShortID = %q, want %q", i, sumA[i].ShortID, want.ShortID)
		}
		if sumA[i].Priority != want.Priority {
			t.Errorf("T22: run A summaries[%d].Priority = %q, want %q", i, sumA[i].Priority, want.Priority)
		}
	}

	// Run B must be absent.
	if _, present := got[runBID]; present {
		t.Errorf("T22: run B (no reports) should be absent from map")
	}
}

// T22TestListReportSummariesForRunIDs_EmptyInput verifies that passing an
// empty run-ID slice returns an empty map without error.
func T22TestListReportSummariesForRunIDs_EmptyInput(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	got, err := svc.ListReportSummariesForRunIDs(ctx, []string{})
	if err != nil {
		t.Fatalf("T22: ListReportSummariesForRunIDs(empty): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("T22: expected empty map for empty input, got %d entries", len(got))
	}
}

// T22TestListReportSummariesForRunIDs_InvalidUUIDsSkipped verifies that
// malformed run IDs are silently skipped and no error is returned.
func T22TestListReportSummariesForRunIDs_InvalidUUIDsSkipped(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	got, err := svc.ListReportSummariesForRunIDs(ctx, []string{"not-a-uuid", "also-bad"})
	if err != nil {
		t.Fatalf("T22: ListReportSummariesForRunIDs(bad UUIDs): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("T22: expected empty map for all-invalid UUIDs, got %d entries", len(got))
	}
}

// T22TestListReportSummariesForRunIDs_UnknownRunID verifies that a valid UUID
// that doesn't correspond to any run returns an empty map (not an error).
func T22TestListReportSummariesForRunIDs_UnknownRunID(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	unknownID := "00000000-0000-0000-0000-000000000099"
	got, err := svc.ListReportSummariesForRunIDs(ctx, []string{unknownID})
	if err != nil {
		t.Fatalf("T22: ListReportSummariesForRunIDs(unknown): %v", err)
	}
	if _, present := got[unknownID]; present {
		t.Errorf("T22: unknown run ID should be absent from map")
	}
	if len(got) != 0 {
		t.Errorf("T22: expected empty map, got %d entries", len(got))
	}
}

// T22TestListReportSummariesForRunIDs_SingleReport verifies the single-report
// case: the map entry has exactly one summary.
func T22TestListReportSummariesForRunIDs_SingleReport(t *testing.T) {
	svc, q, _ := newService(t)
	ctx := context.Background()

	runID, wantReports := T22_mustCreateRunWithReports(
		t, svc, q,
		"t22-single-report",
		[]string{"warning"},
	)

	got, err := svc.ListReportSummariesForRunIDs(ctx, []string{runID})
	if err != nil {
		t.Fatalf("T22: ListReportSummariesForRunIDs: %v", err)
	}

	summaries, ok := got[runID]
	if !ok {
		t.Fatalf("T22: run absent from result map")
	}
	if len(summaries) != 1 {
		t.Fatalf("T22: got %d summaries, want 1", len(summaries))
	}
	if summaries[0].ShortID != wantReports[0].ShortID {
		t.Errorf("T22: ShortID = %q, want %q", summaries[0].ShortID, wantReports[0].ShortID)
	}
	if summaries[0].Title != wantReports[0].Title {
		t.Errorf("T22: Title = %q, want %q", summaries[0].Title, wantReports[0].Title)
	}
	if summaries[0].Priority != "warning" {
		t.Errorf("T22: Priority = %q, want %q", summaries[0].Priority, "warning")
	}
}

// TestT22_ListReportSummariesForRunIDs is the top-level test function that
// runs all T22 sub-tests. This is the entry point used by go test.
func TestT22_ListReportSummariesForRunIDs(t *testing.T) {
	t.Run("RunWithReports", T22TestListReportSummariesForRunIDs_RunWithReports)
	t.Run("RunWithNoReports", T22TestListReportSummariesForRunIDs_RunWithNoReports)
	t.Run("MixedBatch", T22TestListReportSummariesForRunIDs_MixedBatch)
	t.Run("EmptyInput", T22TestListReportSummariesForRunIDs_EmptyInput)
	t.Run("InvalidUUIDsSkipped", T22TestListReportSummariesForRunIDs_InvalidUUIDsSkipped)
	t.Run("UnknownRunID", T22TestListReportSummariesForRunIDs_UnknownRunID)
	t.Run("SingleReport", T22TestListReportSummariesForRunIDs_SingleReport)
}

// compile-time assertion: ensure pgtype is imported.
var _ pgtype.UUID

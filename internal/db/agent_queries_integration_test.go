//go:build integration && !lite

package db_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"breadbox/internal/db"
	"breadbox/internal/testutil"
)

// numericFromFloat builds a pgtype.Numeric matching NUMERIC(10,4) columns.
func numericFromFloat(t *testing.T, v float64) pgtype.Numeric {
	t.Helper()
	var n pgtype.Numeric
	if err := n.Scan(fmt.Sprintf("%.4f", v)); err != nil {
		t.Fatalf("numeric scan: %v", err)
	}
	return n
}

func mustCreateDefinition(t *testing.T, q *db.Queries, slug string, enabled bool) db.Workflow {
	t.Helper()
	ctx := context.Background()
	out, err := q.CreateAgentDefinition(ctx, db.CreateAgentDefinitionParams{
		Name:         "Test " + slug,
		Slug:         slug,
		Prompt:       "Review uncategorized transactions and categorize them.",
		SystemPrompt: pgtype.Text{},
		ScheduleCron: pgtype.Text{},
		ToolScope:    "read_write",
		AllowedTools: []byte(`[]`),
		Model:        "claude-opus-4-7",
		MaxTurns:     10,
		MaxBudgetUsd: numericFromFloat(t, 0.5),
		Enabled:      enabled,
	})
	if err != nil {
		t.Fatalf("CreateAgentDefinition: %v", err)
	}
	return out
}

func TestCreateAndGetAgentDefinition(t *testing.T) {
	_, q := testutil.ServicePool(t)
	ctx := context.Background()

	d := mustCreateDefinition(t, q, "qry-create-"+t.Name(), false)
	if d.ShortID == "" || len(d.ShortID) != 8 {
		t.Errorf("expected 8-char short_id, got %q", d.ShortID)
	}
	if d.Enabled {
		t.Errorf("default enabled should be false on creation")
	}

	got, err := q.GetAgentDefinition(ctx, d.ID)
	if err != nil {
		t.Fatalf("GetAgentDefinition: %v", err)
	}
	if got.Slug != d.Slug {
		t.Errorf("Slug mismatch: want %q got %q", d.Slug, got.Slug)
	}

	gotBySlug, err := q.GetAgentDefinitionBySlug(ctx, d.Slug)
	if err != nil {
		t.Fatalf("GetAgentDefinitionBySlug: %v", err)
	}
	if gotBySlug.ID.String() != d.ID.String() {
		t.Errorf("ID mismatch via slug: want %v got %v", d.ID, gotBySlug.ID)
	}

	gotByShort, err := q.GetAgentDefinitionByShortID(ctx, d.ShortID)
	if err != nil {
		t.Fatalf("GetAgentDefinitionByShortID: %v", err)
	}
	if gotByShort.ID.String() != d.ID.String() {
		t.Errorf("ID mismatch via short_id: want %v got %v", d.ID, gotByShort.ID)
	}
}

func TestListEnabledAgentDefinitions(t *testing.T) {
	_, q := testutil.ServicePool(t)
	ctx := context.Background()

	dOff := mustCreateDefinition(t, q, "qry-list-off-"+t.Name(), false)
	dOn := mustCreateDefinition(t, q, "qry-list-on-"+t.Name(), true)

	enabled, err := q.ListEnabledAgentDefinitions(ctx)
	if err != nil {
		t.Fatalf("ListEnabledAgentDefinitions: %v", err)
	}
	var sawOn, sawOff bool
	for _, d := range enabled {
		if d.ID.String() == dOn.ID.String() {
			sawOn = true
		}
		if d.ID.String() == dOff.ID.String() {
			sawOff = true
		}
	}
	if !sawOn {
		t.Errorf("enabled definition missing from ListEnabledAgentDefinitions")
	}
	if sawOff {
		t.Errorf("disabled definition leaked into ListEnabledAgentDefinitions")
	}
}

func TestSetAgentDefinitionEnabled(t *testing.T) {
	_, q := testutil.ServicePool(t)
	ctx := context.Background()

	d := mustCreateDefinition(t, q, "qry-toggle-"+t.Name(), false)
	updated, err := q.SetAgentDefinitionEnabled(ctx, db.SetAgentDefinitionEnabledParams{ID: d.ID, Enabled: true})
	if err != nil {
		t.Fatalf("SetAgentDefinitionEnabled: %v", err)
	}
	if !updated.Enabled {
		t.Errorf("expected enabled=true after toggle, got false")
	}
}

func TestDeleteAgentDefinition(t *testing.T) {
	_, q := testutil.ServicePool(t)
	ctx := context.Background()

	d := mustCreateDefinition(t, q, "qry-delete-"+t.Name(), false)
	rows, err := q.DeleteAgentDefinition(ctx, d.ID)
	if err != nil {
		t.Fatalf("DeleteAgentDefinition: %v", err)
	}
	if rows != 1 {
		t.Errorf("expected 1 row deleted, got %d", rows)
	}
	if _, err := q.GetAgentDefinition(ctx, d.ID); err == nil {
		t.Errorf("expected error fetching deleted definition")
	} else if err != pgx.ErrNoRows {
		t.Logf("got %v (any not-found-style error is fine)", err)
	}
}

func TestCreateAndCompleteAgentRun(t *testing.T) {
	_, q := testutil.ServicePool(t)
	ctx := context.Background()

	d := mustCreateDefinition(t, q, "qry-run-"+t.Name(), true)
	run, err := q.CreateAgentRun(ctx, db.CreateAgentRunParams{
		WorkflowID: d.ID,
		Trigger:           "manual",
	})
	if err != nil {
		t.Fatalf("CreateAgentRun: %v", err)
	}
	if run.Status != "in_progress" {
		t.Errorf("expected status=in_progress, got %q", run.Status)
	}
	if run.ShortID == "" || len(run.ShortID) != 8 {
		t.Errorf("expected 8-char short_id on run, got %q", run.ShortID)
	}

	completed, err := q.CompleteAgentRun(ctx, db.CompleteAgentRunParams{
		ID:                  run.ID,
		Status:              "success",
		DurationMs:          pgtype.Int4{Int32: 1234, Valid: true},
		TotalCostUsd:        numericFromFloat(t, 0.0123),
		InputTokens:         pgtype.Int4{Int32: 100, Valid: true},
		OutputTokens:        pgtype.Int4{Int32: 50, Valid: true},
		CacheReadTokens:     pgtype.Int4{Int32: 0, Valid: true},
		CacheCreationTokens: pgtype.Int4{Int32: 0, Valid: true},
		TurnCount:           pgtype.Int4{Int32: 2, Valid: true},
		MaxTurnsUsed:        pgtype.Int4{Int32: 10, Valid: true},
		NumToolCalls:        pgtype.Int4{Int32: 1, Valid: true},
		TranscriptPath:      pgtype.Text{String: "/tmp/transcript.ndjson", Valid: true},
		SessionID:           pgtype.Text{String: "sess-abc", Valid: true},
	})
	if err != nil {
		t.Fatalf("CompleteAgentRun: %v", err)
	}
	if completed.Status != "success" {
		t.Errorf("expected status=success after completion, got %q", completed.Status)
	}
	if !completed.CompletedAt.Valid {
		t.Errorf("expected completed_at to be set")
	}
}

func TestCountInProgressAgentRuns(t *testing.T) {
	_, q := testutil.ServicePool(t)
	ctx := context.Background()

	d := mustCreateDefinition(t, q, "qry-count-"+t.Name(), true)
	r1, err := q.CreateAgentRun(ctx, db.CreateAgentRunParams{WorkflowID: d.ID, Trigger: "manual"})
	if err != nil {
		t.Fatalf("CreateAgentRun r1: %v", err)
	}
	if _, err := q.CreateAgentRun(ctx, db.CreateAgentRunParams{WorkflowID: d.ID, Trigger: "cron"}); err != nil {
		t.Fatalf("CreateAgentRun r2: %v", err)
	}

	before, err := q.CountInProgressAgentRuns(ctx)
	if err != nil {
		t.Fatalf("CountInProgressAgentRuns before: %v", err)
	}
	if before < 2 {
		t.Errorf("expected at least 2 in_progress runs, got %d", before)
	}

	if _, err := q.CompleteAgentRun(ctx, db.CompleteAgentRunParams{
		ID: r1.ID, Status: "success",
		DurationMs:          pgtype.Int4{Int32: 100, Valid: true},
		TotalCostUsd:        numericFromFloat(t, 0.01),
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
		t.Fatalf("CompleteAgentRun: %v", err)
	}

	after, err := q.CountInProgressAgentRuns(ctx)
	if err != nil {
		t.Fatalf("CountInProgressAgentRuns after: %v", err)
	}
	if after != before-1 {
		t.Errorf("expected count to drop by 1; before=%d after=%d", before, after)
	}
}

func TestDeleteAgentRunsOlderThan(t *testing.T) {
	pool, q := testutil.ServicePool(t)
	ctx := context.Background()

	d := mustCreateDefinition(t, q, "qry-cleanup-"+t.Name(), true)
	// Insert a deliberately-old completed run via direct SQL.
	if _, err := pool.Exec(ctx, `
		INSERT INTO workflow_runs (workflow_id, "trigger", status, started_at, completed_at)
		VALUES ($1, 'manual', 'success', NOW() - INTERVAL '31 days', NOW() - INTERVAL '31 days')
	`, d.ID); err != nil {
		t.Fatalf("insert old run: %v", err)
	}

	res, err := q.DeleteAgentRunsOlderThan(ctx, pgtype.Timestamptz{Time: time.Now().AddDate(0, 0, -30), Valid: true})
	if err != nil {
		t.Fatalf("DeleteAgentRunsOlderThan: %v", err)
	}
	if res.RowsAffected() < 1 {
		t.Errorf("expected at least 1 row deleted, got %d", res.RowsAffected())
	}
}

// TestCleanupOrphanedAgentApiKeys covers the startup sweep that revokes
// per-run agent API keys whose orchestrator never got to the deferred
// revoke (e.g. SIGKILL'd mid-run). Reaper should revoke old un-revoked
// agent keys and leave fresh ones + non-agent keys alone.
func TestCleanupOrphanedAgentApiKeys(t *testing.T) {
	pool, q := testutil.ServicePool(t)
	ctx := context.Background()

	// 1) Stale agent key (created > 1 hour ago, never revoked) — should be reaped.
	staleID := mustInsertOldApiKey(t, pool, "agent", time.Now().Add(-2*time.Hour))

	// 2) Fresh agent key (just minted) — must NOT be reaped.
	freshID := mustInsertOldApiKey(t, pool, "agent", time.Now())

	// 3) Stale user key — must NOT be reaped (different actor_type).
	userID := mustInsertOldApiKey(t, pool, "user", time.Now().Add(-2*time.Hour))

	// 4) Already-revoked stale agent key — sweep is a no-op (idempotent).
	revokedID := mustInsertOldApiKey(t, pool, "agent", time.Now().Add(-2*time.Hour))
	if _, err := pool.Exec(ctx, `UPDATE api_keys SET revoked_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, revokedID); err != nil {
		t.Fatalf("pre-revoke fixture: %v", err)
	}

	res, err := q.CleanupOrphanedAgentApiKeys(ctx)
	if err != nil {
		t.Fatalf("CleanupOrphanedAgentApiKeys: %v", err)
	}
	if got := res.RowsAffected(); got != 1 {
		t.Errorf("RowsAffected = %d, want exactly 1 (only the stale agent key)", got)
	}

	// Verify each fixture ended in the expected state.
	if !isRevoked(t, pool, staleID) {
		t.Errorf("stale agent key %v should be revoked but isn't", staleID)
	}
	if isRevoked(t, pool, freshID) {
		t.Errorf("fresh agent key %v should NOT be revoked", freshID)
	}
	if isRevoked(t, pool, userID) {
		t.Errorf("stale user key %v should NOT be revoked (wrong actor_type)", userID)
	}
	// revokedID was already revoked — should still be revoked (idempotent).
	if !isRevoked(t, pool, revokedID) {
		t.Errorf("pre-revoked key %v unexpectedly un-revoked", revokedID)
	}
}

func mustInsertOldApiKey(t *testing.T, pool *pgxpool.Pool, actorType string, createdAt time.Time) pgtype.UUID {
	t.Helper()
	ctx := context.Background()
	var id pgtype.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO api_keys (name, key_hash, key_prefix, scope, actor_type, actor_name, created_at)
		VALUES ($1, $2, $3, 'read_only', $4, $5, $6)
		RETURNING id
	`, "test-"+t.Name()+"-"+actorType+"-"+createdAt.Format("150405.000"),
		"hash-"+t.Name()+"-"+actorType+"-"+createdAt.Format("150405.000"),
		"bb_"+actorType[:3],
		actorType,
		"test-actor",
		createdAt,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert api_key: %v", err)
	}
	return id
}

func isRevoked(t *testing.T, pool *pgxpool.Pool, id pgtype.UUID) bool {
	t.Helper()
	var revoked pgtype.Timestamptz
	if err := pool.QueryRow(context.Background(), `SELECT revoked_at FROM api_keys WHERE id = $1`, id).Scan(&revoked); err != nil {
		if err == pgx.ErrNoRows {
			return false
		}
		t.Fatalf("query revoked: %v", err)
	}
	return revoked.Valid
}

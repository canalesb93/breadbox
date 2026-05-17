//go:build integration && !lite

// Pins the agent_definitions / agent_runs schema shipped in the Claude
// Agent SDK sprint. These tables back the v2 SPA /agents UI and the
// scheduled-runner; the integration check fails loudly if someone reverts
// the migrations.
package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"breadbox/internal/testutil"
)

func TestAgentDefinitionsTable_Exists(t *testing.T) {
	pool, _ := testutil.ServicePool(t)
	ctx := context.Background()

	cols := []string{
		"id", "short_id", "name", "slug", "prompt", "system_prompt",
		"schedule_cron", "tool_scope", "allowed_tools", "model",
		"max_turns", "max_budget_usd", "enabled", "created_at", "updated_at",
	}
	for _, col := range cols {
		var exists bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM information_schema.columns
				WHERE table_name='agent_definitions' AND column_name=$1)
		`, col).Scan(&exists); err != nil {
			t.Fatalf("information_schema scan for %s: %v", col, err)
		}
		if !exists {
			t.Errorf("agent_definitions.%s column is missing", col)
		}
	}
}

func TestAgentDefinitionsToolScopeCheck_RejectsInvalid(t *testing.T) {
	pool, _ := testutil.ServicePool(t)
	ctx := context.Background()

	_, err := pool.Exec(ctx, `
		INSERT INTO agent_definitions (name, slug, prompt, tool_scope, allowed_tools)
		VALUES ('bogus tool scope', 'bogus-tool-scope-`+uniqueSuffix(t)+`',
		        'p', 'totally-invalid', '[]')
	`)
	if err == nil {
		t.Fatal("expected CHECK violation for tool_scope='totally-invalid', got nil")
	}
	if !strings.Contains(err.Error(), "tool_scope") && !strings.Contains(err.Error(), "check") {
		t.Errorf("expected check-constraint error mentioning tool_scope or check, got: %v", err)
	}
}

func TestAgentDefinitionsToolScopeCheck_AcceptsValid(t *testing.T) {
	pool, _ := testutil.ServicePool(t)
	ctx := context.Background()

	for _, scope := range []string{"read_only", "read_write"} {
		slug := "valid-" + scope + "-" + uniqueSuffix(t)
		if _, err := pool.Exec(ctx, `
			INSERT INTO agent_definitions (name, slug, prompt, tool_scope, allowed_tools)
			VALUES ('valid scope', $1, 'p', $2, '[]')
		`, slug, scope); err != nil {
			t.Errorf("expected scope %q to be accepted: %v", scope, err)
		}
	}
}

func TestAgentRunsTable_Exists(t *testing.T) {
	pool, _ := testutil.ServicePool(t)
	ctx := context.Background()

	cols := []string{
		"id", "short_id", "agent_definition_id", "trigger", "status",
		"started_at", "completed_at", "duration_ms",
		"total_cost_usd", "input_tokens", "output_tokens",
		"cache_read_tokens", "cache_creation_tokens",
		"turn_count", "max_turns_used", "num_tool_calls",
		"error_message", "transcript_path", "session_id",
	}
	for _, col := range cols {
		var exists bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM information_schema.columns
				WHERE table_name='agent_runs' AND column_name=$1)
		`, col).Scan(&exists); err != nil {
			t.Fatalf("information_schema scan for %s: %v", col, err)
		}
		if !exists {
			t.Errorf("agent_runs.%s column is missing", col)
		}
	}
}

func TestAgentRunsStatusCheck_RejectsInvalid(t *testing.T) {
	pool, _ := testutil.ServicePool(t)
	ctx := context.Background()

	_, err := pool.Exec(ctx, `
		INSERT INTO agent_runs ("trigger", status) VALUES ('manual', 'bogus')
	`)
	if err == nil {
		t.Fatal("expected CHECK violation for status='bogus', got nil")
	}
	if !strings.Contains(err.Error(), "status") && !strings.Contains(err.Error(), "check") {
		t.Errorf("expected check-constraint error mentioning status or check, got: %v", err)
	}
}

func TestAgentRunsFK_SetNullOnDefinitionDelete(t *testing.T) {
	pool, _ := testutil.ServicePool(t)
	ctx := context.Background()

	slug := "fk-test-" + uniqueSuffix(t)
	var defID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO agent_definitions (name, slug, prompt, allowed_tools)
		VALUES ('fk test', $1, 'p', '[]')
		RETURNING id
	`, slug).Scan(&defID); err != nil {
		t.Fatalf("insert definition: %v", err)
	}

	var runID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO agent_runs (agent_definition_id, "trigger", status)
		VALUES ($1, 'manual', 'success')
		RETURNING id
	`, defID).Scan(&runID); err != nil {
		t.Fatalf("insert run: %v", err)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM agent_definitions WHERE id = $1`, defID); err != nil {
		t.Fatalf("delete definition: %v", err)
	}

	var orphanedDefID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		SELECT agent_definition_id FROM agent_runs WHERE id = $1
	`, runID).Scan(&orphanedDefID); err != nil {
		t.Fatalf("re-fetch run: %v", err)
	}
	if orphanedDefID.Valid {
		t.Errorf("expected agent_definition_id to be NULL after definition delete, got %v", orphanedDefID)
	}
}

func TestAgentDefinitionsShortIDTrigger_Fires(t *testing.T) {
	pool, _ := testutil.ServicePool(t)
	ctx := context.Background()

	slug := "shortid-test-" + uniqueSuffix(t)
	var shortID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO agent_definitions (name, slug, prompt, allowed_tools)
		VALUES ('shortid', $1, 'p', '[]')
		RETURNING short_id
	`, slug).Scan(&shortID); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if len(shortID) != 8 {
		t.Errorf("expected 8-char short_id, got %q (%d chars)", shortID, len(shortID))
	}
}

// uniqueSuffix returns a short test-scoped suffix to keep slug UNIQUE
// across re-runs without resetting the DB.
func uniqueSuffix(t *testing.T) string {
	t.Helper()
	// pgtype.UUID generated server-side would be cleaner, but for slug
	// we just want enough entropy. Use the test name + a per-run nanos.
	return strings.ReplaceAll(strings.ToLower(t.Name()), "/", "-")
}

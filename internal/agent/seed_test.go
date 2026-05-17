//go:build integration && !lite

package agent_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"breadbox/internal/agent"
	"breadbox/internal/db"
	"breadbox/internal/testutil"
)

func makeMinimalParams(slug string) db.CreateAgentDefinitionParams {
	var bud pgtype.Numeric
	_ = bud.Scan("1.0000")
	return db.CreateAgentDefinitionParams{
		Name:         "User Agent",
		Slug:         slug,
		Prompt:       "p",
		SystemPrompt: pgtype.Text{},
		ScheduleCron: pgtype.Text{},
		ToolScope:    "read_write",
		AllowedTools: []byte("[]"),
		Model:        "claude-opus-4-7",
		MaxTurns:     10,
		MaxBudgetUsd: bud,
		Enabled:      false,
	}
}

func TestMain(m *testing.M) {
	testutil.RunWithDB(m)
}

func TestSeedDefaults_PopulatesEmptyTable(t *testing.T) {
	_, q := testutil.ServicePool(t)
	ctx := context.Background()

	// Truncate is implicit between tests via testutil; agent_definitions
	// starts empty.
	if err := agent.SeedDefaults(ctx, q, slog.Default()); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}

	defs, err := q.ListAgentDefinitions(ctx)
	if err != nil {
		t.Fatalf("ListAgentDefinitions: %v", err)
	}
	if len(defs) != len(agent.DefaultSeed) {
		t.Errorf("seeded %d, want %d", len(defs), len(agent.DefaultSeed))
	}

	// Every seeded definition should be disabled by default.
	for _, d := range defs {
		if d.Enabled {
			t.Errorf("seeded definition %q should be disabled, got enabled=true", d.Slug)
		}
		if d.ToolScope != "read_write" {
			t.Errorf("seeded definition %q ToolScope = %q, want read_write", d.Slug, d.ToolScope)
		}
		if len(d.Prompt) < 50 {
			t.Errorf("seeded definition %q has suspiciously short prompt (%d chars)", d.Slug, len(d.Prompt))
		}
	}
}

func TestSeedDefaults_IdempotentOnSecondRun(t *testing.T) {
	_, q := testutil.ServicePool(t)
	ctx := context.Background()

	if err := agent.SeedDefaults(ctx, q, slog.Default()); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	defsFirst, _ := q.ListAgentDefinitions(ctx)

	// Second invocation should be a no-op — the table is no longer empty.
	if err := agent.SeedDefaults(ctx, q, slog.Default()); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	defsSecond, _ := q.ListAgentDefinitions(ctx)

	if len(defsFirst) != len(defsSecond) {
		t.Errorf("second seed mutated the table: first=%d second=%d",
			len(defsFirst), len(defsSecond))
	}
}

func TestSeedDefaults_SkipsWhenAnyDefinitionExists(t *testing.T) {
	_, q := testutil.ServicePool(t)
	ctx := context.Background()

	// Insert one unrelated definition so the seed sees a non-empty table.
	// Use the existing mustCreate pattern — borrow CreateAgentDefinition
	// directly with minimum required fields.
	if _, err := q.CreateAgentDefinition(ctx, makeMinimalParams("user-made")); err != nil {
		t.Fatalf("seed test: insert user row: %v", err)
	}

	if err := agent.SeedDefaults(ctx, q, slog.Default()); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}

	defs, err := q.ListAgentDefinitions(ctx)
	if err != nil {
		t.Fatalf("ListAgentDefinitions: %v", err)
	}
	if len(defs) != 1 {
		t.Errorf("seed should have skipped (user row already present); got %d defs", len(defs))
	}
}

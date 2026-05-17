//go:build !lite

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"breadbox/internal/db"
	"breadbox/prompts"

	"github.com/jackc/pgx/v5/pgtype"
)

// DefaultSeed is the canonical set of starter agents — direct ports of the
// v1 admin "agent prompts" wizard library. Each entry references a markdown
// file embedded under `prompts/agents/`. All seeds ship disabled so the
// self-hoster opts in by enabling them in the v2 SPA.
//
// Adding a new entry here AND a matching `prompts/agents/strategy-<slug>.md`
// is the entire workflow — the seed runs at startup on fresh installs only.
var DefaultSeed = []SeedDefinition{
	{
		Slug:         "initial-setup",
		Name:         "Initial Setup",
		Description:  "Establish rules and categories after connecting your first accounts.",
		PromptFile:   "strategy-initial-setup",
		MaxTurns:     30,
		MaxBudgetUSD: 1.50,
	},
	{
		Slug:         "bulk-review",
		Name:         "Bulk Review",
		Description:  "Thorough pass over a large pending-review queue with rule creation.",
		PromptFile:   "strategy-bulk-review",
		MaxTurns:     30,
		MaxBudgetUSD: 1.50,
	},
	{
		Slug:         "quick-review",
		Name:         "Quick Review",
		Description:  "Fast batch-categorize over a backlog. Prioritizes speed over precision.",
		PromptFile:   "strategy-quick-review",
		MaxTurns:     15,
		MaxBudgetUSD: 0.50,
	},
	{
		Slug:         "routine-review",
		Name:         "Routine Review",
		Description:  "Daily or weekly pass over fresh transactions. Best paired with a cron schedule.",
		PromptFile:   "strategy-routine-review",
		MaxTurns:     15,
		MaxBudgetUSD: 0.50,
	},
	{
		Slug:         "spending-report",
		Name:         "Spending Report",
		Description:  "Weekly or monthly category-grouped summary with anomaly callouts.",
		PromptFile:   "strategy-spending-report",
		MaxTurns:     10,
		MaxBudgetUSD: 0.30,
	},
}

// SeedDefinition is one entry in DefaultSeed.
type SeedDefinition struct {
	Slug         string
	Name         string
	Description  string  // short blurb for surfacing in seed logs / UI hints
	PromptFile   string  // filename in prompts/agents/ without .md
	MaxTurns     int32
	MaxBudgetUSD float64 // dollars
}

// SeederQueries is the minimal sqlc surface needed by SeedDefaults.
type SeederQueries interface {
	ListAgentDefinitions(ctx context.Context) ([]db.AgentDefinition, error)
	CreateAgentDefinition(ctx context.Context, arg db.CreateAgentDefinitionParams) (db.AgentDefinition, error)
}

// SeedDefaults inserts DefaultSeed entries into agent_definitions ONLY on a
// fresh install — when the table is empty. On any subsequent startup it is
// a no-op (so user edits to seeded agents are preserved across restarts).
//
// All seeded definitions are inserted with enabled=false; the user opts in
// via the v2 SPA. Schedule is NULL (manual-only) so nothing fires until the
// user picks a cron expression. Tool scope is read_write — these agents'
// job is to apply rules and enrich, not just suggest.
func SeedDefaults(ctx context.Context, q SeederQueries, logger *slog.Logger) error {
	existing, err := q.ListAgentDefinitions(ctx)
	if err != nil {
		return fmt.Errorf("agent seed: list existing: %w", err)
	}
	if len(existing) > 0 {
		logger.Debug("agent seed: skipping — agent_definitions already populated",
			"existing_count", len(existing))
		return nil
	}

	inserted := 0
	for _, s := range DefaultSeed {
		prompt, err := prompts.Agent(s.PromptFile)
		if err != nil {
			// A missing prompt is a programming error (we ship the .md
			// files in the binary), but we keep going — better to seed
			// the agents we have than abort the whole startup.
			logger.Warn("agent seed: prompt file missing", "slug", s.Slug, "file", s.PromptFile, "error", err)
			continue
		}
		var bud pgtype.Numeric
		_ = bud.Scan(fmt.Sprintf("%.4f", s.MaxBudgetUSD))

		_, err = q.CreateAgentDefinition(ctx, db.CreateAgentDefinitionParams{
			Name:         s.Name,
			Slug:         s.Slug,
			Prompt:       strings.TrimSpace(string(prompt)),
			SystemPrompt: pgtype.Text{},
			ScheduleCron: pgtype.Text{},
			ToolScope:    "read_write",
			AllowedTools: []byte("[]"),
			Model:        "claude-opus-4-7",
			MaxTurns:     s.MaxTurns,
			MaxBudgetUsd: bud,
			Enabled:      false,
		})
		if err != nil {
			// Soft-fail: log and continue so a single bad row doesn't
			// block the whole seed (or block startup).
			logger.Warn("agent seed: insert failed", "slug", s.Slug, "error", err)
			continue
		}
		inserted++
	}
	if inserted > 0 {
		logger.Info("agent seed: inserted default agents",
			"count", inserted, "total_planned", len(DefaultSeed))
	}
	return nil
}

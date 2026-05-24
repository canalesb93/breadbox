//go:build integration && !lite

package service_test

import (
	"context"
	"log/slog"
	"testing"

	"breadbox/internal/agent"
	"breadbox/internal/service"
)

// TestOrchestratorRunNow_MaxTurnsUsed_RecordsCapNotTurnCount is the
// iter-33 regression test. Before iter-33, max_turns_used was set to
// result.TurnCount instead of def.MaxTurns, which meant the column
// was useless: every run looked like it hit the cap (e.g. "7/7" for
// a clean 7-turn run with a cap of 10). The fix passes def.MaxTurns
// through CompleteAgentRunDB. This test pins both:
//   - max_turns_used == def.MaxTurns (the cap snapshot)
//   - turn_count == result.TurnCount (the actual turns used)
// so the admin UI can render "actual / cap" correctly.
func TestOrchestratorRunNow_MaxTurnsUsed_RecordsCapNotTurnCount(t *testing.T) {
	svc, _, _ := newService(t)
	encKey := seedSubscriptionAuth(t, svc)

	// max_turns cap = 25 (not the seed default of 10, to make the
	// regression more obvious if it returns).
	def, err := svc.CreateAgentDefinition(context.Background(), service.CreateAgentDefinitionParams{
		Name:         "orch-cap-vs-turns",
		Slug:         "orch-cap-vs-turns",
		Prompt:       "max-turns vs turn-count regression test prompt — must exceed validation length so this padding is here.",
		ToolScope:    "read_only",
		AllowedTools: []string{},
		Model:        "claude-haiku-4-5",
		MaxTurns:     25,
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Runner reports 7 actual turns; the cap of 25 should land in
	// max_turns_used; turn_count should be 7.
	runner := agent.RunnerFunc(func(_ context.Context, _ agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		return agent.RunResult{
			Status:     agent.StatusSuccess,
			TurnCount:  7,
			DurationMs: 50,
		}, nil
	})

	orch := service.NewOrchestrator(svc, runner, 3, encKey, slog.Default())
	runResp, err := orch.RunNow(context.Background(), def, "")
	if err != nil {
		t.Fatalf("RunNow: %v", err)
	}

	if runResp.TurnCount == nil || *runResp.TurnCount != 7 {
		t.Errorf("TurnCount = %v, want 7 (actual turns)", runResp.TurnCount)
	}
	if runResp.MaxTurnsUsed == nil || *runResp.MaxTurnsUsed != 25 {
		t.Errorf("MaxTurnsUsed = %v, want 25 (the cap, not the actual count)",
			runResp.MaxTurnsUsed)
	}

	// Verify persistence — the regression would otherwise silently flip
	// back via the row read path.
	persisted, err := svc.GetAgentRun(context.Background(), runResp.ShortID)
	if err != nil {
		t.Fatalf("GetAgentRun: %v", err)
	}
	if persisted.TurnCount == nil || *persisted.TurnCount != 7 {
		t.Errorf("persisted TurnCount = %v, want 7", persisted.TurnCount)
	}
	if persisted.MaxTurnsUsed == nil || *persisted.MaxTurnsUsed != 25 {
		t.Errorf("persisted MaxTurnsUsed = %v, want 25", persisted.MaxTurnsUsed)
	}
}

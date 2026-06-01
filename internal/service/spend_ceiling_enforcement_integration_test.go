//go:build integration && !lite

package service_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"breadbox/internal/agent"
	"breadbox/internal/service"
)

// T10 prefix — unique across all integration-test agents in this package.

// T10_neverRunner returns a RunnerFunc that fails the test immediately if invoked.
// Used to assert the runner is never called when the ceiling gate fires.
func T10_neverRunner(t *testing.T) agent.Runner {
	t.Helper()
	return agent.RunnerFunc(func(_ context.Context, _ agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		t.Error("T10: runner must NOT be invoked when the household ceiling is reached")
		return agent.RunResult{Status: agent.StatusSuccess}, nil
	})
}

// T10_successRunner returns a RunnerFunc that succeeds instantly.
// Used to prove that runs proceed normally when no ceiling is set (or spend is under it).
func T10_successRunner() agent.Runner {
	return agent.RunnerFunc(func(_ context.Context, _ agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		return agent.RunResult{Status: agent.StatusSuccess, TurnCount: 1, DurationMs: 5}, nil
	})
}

// TestT10_SpendCeilingEnforcement_ManualRunReturnsError verifies that
// RunNow returns ErrBudgetCeilingReached (without creating a run row) when
// KeyAgentGlobalMaxBudgetUSD is set and the rolling 30-day spend has reached it.
func TestT10_SpendCeilingEnforcement_ManualRunReturnsError(t *testing.T) {
	svc, _, pool := newService(t)
	ctx := context.Background()
	encKey := seedSubscriptionAuth(t, svc)
	def := mustCreateAgentDefinition(t, svc, "t10-manual-ceiling", true)

	// Configure a $0.10 ceiling.
	ceiling := 0.10
	if _, err := svc.UpdateAgentSettings(ctx, service.UpdateAgentSettingsParams{
		GlobalMaxBudgetUSD: &ceiling,
	}, encKey, ""); err != nil {
		t.Fatalf("T10: set ceiling: %v", err)
	}

	// Seed two completed runs whose costs sum to $0.13 — over the ceiling.
	for _, cost := range []string{"0.07", "0.06"} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO workflow_runs (agent_definition_id, "trigger", status, total_cost_usd, started_at)
			 VALUES ($1, 'cron', 'success', $2, NOW())`,
			def.ID, cost,
		); err != nil {
			t.Fatalf("T10: insert run: %v", err)
		}
	}

	orch := service.NewOrchestrator(svc, T10_neverRunner(t), 1, encKey, slog.Default())

	resp, err := orch.RunNow(ctx, def, "")
	if !errors.Is(err, service.ErrBudgetCeilingReached) {
		t.Fatalf("T10: RunNow err = %v, want ErrBudgetCeilingReached", err)
	}
	// RunNow returns nil resp when the ceiling gate fires — no row is created.
	if resp != nil {
		t.Errorf("T10: RunNow should return nil resp when ceiling fires (no row created), got %+v", resp)
	}

	// The error message must carry dollar-amount context so operators know the figures.
	if !strings.Contains(err.Error(), "$") {
		t.Errorf("T10: ceiling error should include dollar amounts, got: %v", err)
	}
}

// TestT10_SpendCeilingEnforcement_CronPathLeavesSkippedRow verifies that
// RunOrSkip (the cron/webhook path) creates a 'skipped' row with the ceiling
// message as error_message when the 30-day spend is at or over the ceiling.
func TestT10_SpendCeilingEnforcement_CronPathLeavesSkippedRow(t *testing.T) {
	svc, _, pool := newService(t)
	ctx := context.Background()
	encKey := seedSubscriptionAuth(t, svc)
	def := mustCreateAgentDefinition(t, svc, "t10-cron-ceiling", true)

	// Configure a $0.05 ceiling.
	ceiling := 0.05
	if _, err := svc.UpdateAgentSettings(ctx, service.UpdateAgentSettingsParams{
		GlobalMaxBudgetUSD: &ceiling,
	}, encKey, ""); err != nil {
		t.Fatalf("T10: set ceiling: %v", err)
	}

	// Seed a run at exactly the ceiling amount. The gate condition is spent >= ceiling,
	// so $0.05 spent against a $0.05 ceiling must block the next run.
	if _, err := pool.Exec(ctx,
		`INSERT INTO workflow_runs (agent_definition_id, "trigger", status, total_cost_usd, started_at)
		 VALUES ($1, 'cron', 'success', $2, NOW())`,
		def.ID, "0.05",
	); err != nil {
		t.Fatalf("T10: insert run at ceiling: %v", err)
	}

	orch := service.NewOrchestrator(svc, T10_neverRunner(t), 1, encKey, slog.Default())

	resp, err := orch.RunOrSkip(ctx, def, "cron")
	if !errors.Is(err, service.ErrBudgetCeilingReached) {
		t.Fatalf("T10: RunOrSkip err = %v, want ErrBudgetCeilingReached", err)
	}
	// RunOrSkip must always create a row — even for ceiling fires.
	if resp == nil {
		t.Fatal("T10: RunOrSkip must return a skipped run row, got nil")
	}
	if resp.Status != "skipped" {
		t.Errorf("T10: resp.Status = %q, want skipped", resp.Status)
	}
	if resp.Trigger != "cron" {
		t.Errorf("T10: resp.Trigger = %q, want cron", resp.Trigger)
	}

	// The row must be durable in the DB.
	persisted, perr := svc.GetAgentRun(ctx, resp.ShortID)
	if perr != nil {
		t.Fatalf("T10: GetAgentRun: %v", perr)
	}
	if persisted.Status != "skipped" {
		t.Errorf("T10: persisted Status = %q, want skipped", persisted.Status)
	}
	// The ceiling reason must surface in error_message so operators know why.
	if persisted.ErrorMessage == nil {
		t.Error("T10: persisted ErrorMessage should contain the ceiling reason, got nil")
	} else if !strings.Contains(*persisted.ErrorMessage, "ceiling") &&
		!strings.Contains(*persisted.ErrorMessage, "spend") &&
		!strings.Contains(*persisted.ErrorMessage, "budget") {
		t.Errorf("T10: persisted ErrorMessage = %q, expected ceiling/spend/budget language", *persisted.ErrorMessage)
	}
}

// TestT10_SpendCeilingEnforcement_UnsetCeilingNoCap verifies that when
// KeyAgentGlobalMaxBudgetUSD is not set (the default), RunNow succeeds even
// with substantial 30-day spend — no artificial cap is applied.
func TestT10_SpendCeilingEnforcement_UnsetCeilingNoCap(t *testing.T) {
	svc, _, pool := newService(t)
	ctx := context.Background()
	encKey := seedSubscriptionAuth(t, svc)
	def := mustCreateAgentDefinition(t, svc, "t10-unset-ceiling", true)

	// Seed a large spend. If checkHouseholdCeiling correctly returns nil for
	// an unset key, this spend must not block the run.
	if _, err := pool.Exec(ctx,
		`INSERT INTO workflow_runs (agent_definition_id, "trigger", status, total_cost_usd, started_at)
		 VALUES ($1, 'cron', 'success', $2, NOW())`,
		def.ID, "999.99",
	); err != nil {
		t.Fatalf("T10: insert large-cost run: %v", err)
	}

	// Deliberately do NOT set GlobalMaxBudgetUSD — leave it unset.

	orch := service.NewOrchestrator(svc, T10_successRunner(), 1, encKey, slog.Default())

	resp, err := orch.RunNow(ctx, def, "")
	if err != nil {
		t.Fatalf("T10: RunNow with unset ceiling err = %v, want nil", err)
	}
	if resp == nil || resp.Status != agent.StatusSuccess {
		t.Errorf("T10: expected success run, got resp=%v", resp)
	}
}

// TestT10_SpendCeilingEnforcement_ZeroCeilingNoCap verifies that a ceiling
// explicitly set to 0.0 is treated as "no limit" — runs are not blocked.
// This is enforced by the `<= 0` guard in checkHouseholdCeiling.
func TestT10_SpendCeilingEnforcement_ZeroCeilingNoCap(t *testing.T) {
	svc, _, pool := newService(t)
	ctx := context.Background()
	encKey := seedSubscriptionAuth(t, svc)
	def := mustCreateAgentDefinition(t, svc, "t10-zero-ceiling", true)

	// Set the ceiling to 0.0 (explicit "no cap").
	zeroCeiling := 0.0
	if _, err := svc.UpdateAgentSettings(ctx, service.UpdateAgentSettingsParams{
		GlobalMaxBudgetUSD: &zeroCeiling,
	}, encKey, ""); err != nil {
		t.Fatalf("T10: set zero ceiling: %v", err)
	}

	// Seed spend that would block a naive zero-ceiling implementation.
	if _, err := pool.Exec(ctx,
		`INSERT INTO workflow_runs (agent_definition_id, "trigger", status, total_cost_usd, started_at)
		 VALUES ($1, 'cron', 'success', $2, NOW())`,
		def.ID, "50.00",
	); err != nil {
		t.Fatalf("T10: insert run: %v", err)
	}

	orch := service.NewOrchestrator(svc, T10_successRunner(), 1, encKey, slog.Default())

	resp, err := orch.RunNow(ctx, def, "")
	if err != nil {
		t.Fatalf("T10: RunNow with zero ceiling err = %v, want nil", err)
	}
	if resp == nil || resp.Status != agent.StatusSuccess {
		t.Errorf("T10: expected success run with zero ceiling, got %v", resp)
	}
}

// TestT10_SpendCeilingEnforcement_SpendUnderCeilingAllowsRun verifies that
// when 30-day spend is below the configured ceiling, RunNow completes normally.
func TestT10_SpendCeilingEnforcement_SpendUnderCeilingAllowsRun(t *testing.T) {
	svc, _, pool := newService(t)
	ctx := context.Background()
	encKey := seedSubscriptionAuth(t, svc)
	def := mustCreateAgentDefinition(t, svc, "t10-under-ceiling", true)

	// Set a $1.00 ceiling — high enough that $0.10 spend is safely under it.
	ceiling := 1.00
	if _, err := svc.UpdateAgentSettings(ctx, service.UpdateAgentSettingsParams{
		GlobalMaxBudgetUSD: &ceiling,
	}, encKey, ""); err != nil {
		t.Fatalf("T10: set ceiling: %v", err)
	}

	// Seed $0.10 worth of runs — well under the $1.00 ceiling.
	if _, err := pool.Exec(ctx,
		`INSERT INTO workflow_runs (agent_definition_id, "trigger", status, total_cost_usd, started_at)
		 VALUES ($1, 'cron', 'success', $2, NOW())`,
		def.ID, "0.10",
	); err != nil {
		t.Fatalf("T10: insert run: %v", err)
	}

	orch := service.NewOrchestrator(svc, T10_successRunner(), 1, encKey, slog.Default())

	resp, err := orch.RunNow(ctx, def, "")
	if err != nil {
		t.Fatalf("T10: RunNow under ceiling err = %v, want nil", err)
	}
	if resp == nil || resp.Status != agent.StatusSuccess {
		t.Errorf("T10: expected success run, got %v", resp)
	}
}

// TestT10_SpendCeilingEnforcement_SkippedRunsExcludedFromSpend verifies that
// skipped workflow_runs are NOT counted toward the 30-day spend ceiling.
// Skipped rows never consumed API budget; including them would inflate the
// spend figure and cause false ceiling fires. Mirrors HouseholdCostSince's
// exclusion of status='skipped' rows.
func TestT10_SpendCeilingEnforcement_SkippedRunsExcludedFromSpend(t *testing.T) {
	svc, _, pool := newService(t)
	ctx := context.Background()
	encKey := seedSubscriptionAuth(t, svc)
	def := mustCreateAgentDefinition(t, svc, "t10-skipped-excluded", true)

	// $0.10 ceiling.
	ceiling := 0.10
	if _, err := svc.UpdateAgentSettings(ctx, service.UpdateAgentSettingsParams{
		GlobalMaxBudgetUSD: &ceiling,
	}, encKey, ""); err != nil {
		t.Fatalf("T10: set ceiling: %v", err)
	}

	// Insert a skipped run with an inflated cost value. In practice skipped
	// rows carry no real cost, but this explicitly guards the exclusion logic.
	if _, err := pool.Exec(ctx,
		`INSERT INTO workflow_runs (agent_definition_id, "trigger", status, total_cost_usd, started_at)
		 VALUES ($1, 'cron', 'skipped', $2, NOW())`,
		def.ID, "5.00",
	); err != nil {
		t.Fatalf("T10: insert skipped run: %v", err)
	}

	// Also insert a small real-cost run well under the ceiling.
	if _, err := pool.Exec(ctx,
		`INSERT INTO workflow_runs (agent_definition_id, "trigger", status, total_cost_usd, started_at)
		 VALUES ($1, 'cron', 'success', $2, NOW())`,
		def.ID, "0.02",
	); err != nil {
		t.Fatalf("T10: insert success run: %v", err)
	}

	// Effective spend = $0.02 (skipped $5.00 excluded). Ceiling = $0.10 —
	// run must be allowed.
	orch := service.NewOrchestrator(svc, T10_successRunner(), 1, encKey, slog.Default())

	resp, err := orch.RunNow(ctx, def, "")
	if err != nil {
		t.Fatalf("T10: RunNow (skipped excluded) err = %v; skipped run must not count toward ceiling", err)
	}
	if resp == nil || resp.Status != agent.StatusSuccess {
		t.Errorf("T10: expected success run, got %v", resp)
	}
}

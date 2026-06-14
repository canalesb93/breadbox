//go:build integration && !lite

package service_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"breadbox/internal/agent"
	"breadbox/internal/service"
)

// TestT11_FireSyncCompleteAgents_DebounceWindow exercises the two-sided
// post-sync debounce: a definition that ran within PostSyncDebounceWindow is
// skipped row-lessly, while a definition whose last run is older than the
// window IS dispatched on the same sync-complete event.
//
// Seeds two eligible agents (enabled=true, trigger_on_sync_complete=true):
//   - "t11-within-window": last non-skipped run started at NOW()
//     (squarely inside the debounce window)
//   - "t11-outside-window": last non-skipped run started at
//     NOW() - 2*PostSyncDebounceWindow (clearly outside the window)
//
// After FireSyncCompleteAgents fires, exactly one runner invocation is
// expected -- the outside-window agent. The within-window agent is silently
// coalesced: the debounce is intentionally row-less (design-doc section 4 --
// a coalesced trigger is not a "missed" run worth a skipped row).
func TestT11_FireSyncCompleteAgents_DebounceWindow(t *testing.T) {
	svc, _, pool := newService(t)
	encKey := seedSubscriptionAuth(t, svc)
	ctx := context.Background()

	// Both agents are eligible: enabled=true, trigger_on_sync_complete=true.
	defInWindow := mustCreateWebhookAgent(t, svc, "t11-within-window", true, true)
	defOutWindow := mustCreateWebhookAgent(t, svc, "t11-outside-window", true, true)

	// Seed a recent non-skipped run for the within-window agent.
	// started_at = NOW() places it squarely inside PostSyncDebounceWindow.
	if _, err := pool.Exec(ctx,
		`INSERT INTO workflow_runs (workflow_id, "trigger", status, started_at)
		 VALUES ($1, 'webhook', 'success', NOW())`,
		defInWindow.ID,
	); err != nil {
		t.Fatalf("seed within-window run: %v", err)
	}

	// Seed an old non-skipped run for the outside-window agent.
	// 2*PostSyncDebounceWindow ensures the row is well outside the boundary
	// even under slow test-execution clock drift.
	oldAge := 2 * service.PostSyncDebounceWindow
	if _, err := pool.Exec(ctx,
		`INSERT INTO workflow_runs (workflow_id, "trigger", status, started_at)
		 VALUES ($1, 'webhook', 'success', NOW() - $2::interval)`,
		defOutWindow.ID, oldAge.String(),
	); err != nil {
		t.Fatalf("seed outside-window run: %v", err)
	}

	// Channel buffered to 4 to catch unexpected extra fires without blocking.
	fired := make(chan string, 4)
	runner := agent.RunnerFunc(func(_ context.Context, spec agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		fired <- spec.WorkflowID
		return agent.RunResult{Status: agent.StatusSuccess, DurationMs: 1}, nil
	})
	orch := service.NewOrchestrator(svc, runner, 3, encKey, slog.Default())

	orch.FireSyncCompleteAgents(ctx)

	// Wait for exactly one fire: the outside-window agent.
	var got []string
	select {
	case id := <-fired:
		got = append(got, id)
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout: expected one runner fire (outside-window agent); got none")
	}

	// Short tail to detect any unexpected second fire (within-window agent
	// must be debounced row-lessly, not dispatched).
	select {
	case extraID := <-fired:
		t.Fatalf("unexpected extra fire from agent %q; within-window agent should be coalesced silently", extraID)
	case <-time.After(300 * time.Millisecond):
		// pass -- no extras
	}

	// Confirm the single fire came from the outside-window agent.
	if got[0] != defOutWindow.ID {
		t.Errorf("fired agent ID = %q, want %q (outside-window agent)", got[0], defOutWindow.ID)
	}
}

// TestT11_FireSyncCompleteAgents_SkippedRunDoesNotDebounce verifies that a
// 'skipped' run inside PostSyncDebounceWindow does NOT anchor the debounce.
// ExistsRecentRunForDefinition uses "status <> 'skipped'" in its WHERE
// clause, so skipped rows are invisible to the check and the definition
// remains eligible for dispatch on the next sync-complete event.
func TestT11_FireSyncCompleteAgents_SkippedRunDoesNotDebounce(t *testing.T) {
	svc, _, pool := newService(t)
	encKey := seedSubscriptionAuth(t, svc)
	ctx := context.Background()

	def := mustCreateWebhookAgent(t, svc, "t11-skip-no-debounce", true, true)

	// Seed a skipped run inside the debounce window.
	// A skipped status must NOT suppress the next sync-complete dispatch.
	if _, err := pool.Exec(ctx,
		`INSERT INTO workflow_runs (workflow_id, "trigger", status, started_at)
		 VALUES ($1, 'webhook', 'skipped', NOW())`,
		def.ID,
	); err != nil {
		t.Fatalf("seed skipped run: %v", err)
	}

	fired := make(chan struct{}, 2)
	runner := agent.RunnerFunc(func(_ context.Context, _ agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		fired <- struct{}{}
		return agent.RunResult{Status: agent.StatusSuccess, DurationMs: 1}, nil
	})
	orch := service.NewOrchestrator(svc, runner, 3, encKey, slog.Default())

	orch.FireSyncCompleteAgents(ctx)

	// Must fire: skipped rows are excluded from the debounce predicate.
	select {
	case <-fired:
		// pass -- correctly dispatched despite a skipped run being in the window
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: definition should have been dispatched (skipped-only run must not debounce)")
	}

	// No spurious double-fire expected.
	select {
	case <-fired:
		t.Fatal("unexpected second fire")
	case <-time.After(200 * time.Millisecond):
		// pass
	}
}

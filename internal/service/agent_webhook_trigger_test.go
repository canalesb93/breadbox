//go:build integration && !lite

package service_test

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"breadbox/internal/agent"
	"breadbox/internal/service"
)

// TestFireSyncCompleteAgents_OnlyFiresEligible confirms the post-sync hook
// dispatches the agents that opted in (enabled + trigger_on_sync_complete)
// and ignores everyone else. Builds a mix of three agents:
//   - one eligible (enabled=true, trigger_on_sync_complete=true)
//   - one disabled (enabled=false, trigger_on_sync_complete=true)
//   - one no-webhook (enabled=true, trigger_on_sync_complete=false)
// Verifies exactly one run lands.
func TestFireSyncCompleteAgents_OnlyFiresEligible(t *testing.T) {
	svc, _, _ := newService(t)
	encKey := seedSubscriptionAuth(t, svc)

	enabledWebhook := mustCreateWebhookAgent(t, svc, "wh-enabled", true, true)
	mustCreateWebhookAgent(t, svc, "wh-disabled", false, true)
	mustCreateWebhookAgent(t, svc, "wh-cron-only", true, false)

	fired := make(chan string, 4)
	runner := agent.RunnerFunc(func(_ context.Context, spec agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		fired <- spec.AgentDefinitionID
		return agent.RunResult{Status: agent.StatusSuccess, DurationMs: 1}, nil
	})
	orch := service.NewOrchestrator(svc, runner, 3, encKey, slog.Default())

	orch.FireSyncCompleteAgents(context.Background())

	// Deterministic wait: we expect exactly 1 fire (only the eligible
	// agent qualifies). Block until it lands or fail after a generous
	// timeout. Then sweep the channel briefly to catch any unexpected
	// extra fires the bug-this-test-pins might produce. Replaces the
	// iter-30 time.After polling pattern flagged as LOW #6 in iter-32
	// audit — the previous shape was timing-sensitive under CI load.
	var got []string
	select {
	case agentID := <-fired:
		got = append(got, agentID)
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for the eligible webhook fire; got %v", got)
	}
	// Tight tail to catch extras (if any agent that shouldn't fire does).
	select {
	case extra := <-fired:
		t.Fatalf("unexpected extra fire from agent %q (only enabled+webhook should run)", extra)
	case <-time.After(200 * time.Millisecond):
		// pass — no extras
	}

	if got[0] != enabledWebhook.ID {
		t.Errorf("fired agent ID = %q, want %q (the enabled+webhook one)",
			got[0], enabledWebhook.ID)
	}
}

// TestFireSyncCompleteAgents_NoEligibleAgents_NoOp verifies the hook is
// silent when no agents are configured for it. Confirms the cheap path
// doesn't spawn goroutines or log noise.
func TestFireSyncCompleteAgents_NoEligibleAgents_NoOp(t *testing.T) {
	svc, _, _ := newService(t)
	encKey := seedSubscriptionAuth(t, svc)
	mustCreateWebhookAgent(t, svc, "wh-none-cron", true, false)

	fired := make(chan struct{}, 1)
	runner := agent.RunnerFunc(func(_ context.Context, _ agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		fired <- struct{}{}
		return agent.RunResult{}, nil
	})
	orch := service.NewOrchestrator(svc, runner, 3, encKey, slog.Default())

	orch.FireSyncCompleteAgents(context.Background())

	select {
	case <-fired:
		t.Fatal("runner fired for an agent that didn't opt in")
	case <-time.After(200 * time.Millisecond):
		// pass — no goroutines fired
	}
}

func mustCreateWebhookAgent(t *testing.T, svc *service.Service, slug string, enabled, webhook bool) *service.AgentDefinitionResponse {
	t.Helper()
	def, err := svc.CreateAgentDefinition(context.Background(), service.CreateAgentDefinitionParams{
		Name:                  strings.ToUpper(slug),
		Slug:                  slug,
		Prompt:                "Webhook test prompt for " + slug + " — needs >50 characters to pass validation, padding here.",
		ToolScope:             "read_only",
		AllowedTools:          []string{},
		Model:                 "claude-haiku-4-5",
		MaxTurns:              1,
		Enabled:               enabled,
		TriggerOnSyncComplete: webhook,
	})
	if err != nil {
		t.Fatalf("create %s: %v", slug, err)
	}
	return def
}

// TestFireSyncCompleteAgents_DebouncesRecentRun confirms the post-sync
// hook coalesces: an eligible agent that already ran (non-skipped) within
// PostSyncDebounceWindow is NOT dispatched again on a fresh sync-complete.
func TestFireSyncCompleteAgents_DebouncesRecentRun(t *testing.T) {
	svc, _, pool := newService(t)
	encKey := seedSubscriptionAuth(t, svc)
	def := mustCreateWebhookAgent(t, svc, "wh-debounce", true, true)

	// A recent non-skipped run anchors the debounce window.
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO agent_runs (agent_definition_id,"trigger",status,started_at)
		 VALUES ($1,'webhook','success', NOW())`, def.ID); err != nil {
		t.Fatalf("seed recent run: %v", err)
	}

	fired := make(chan struct{}, 2)
	runner := agent.RunnerFunc(func(_ context.Context, _ agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		fired <- struct{}{}
		return agent.RunResult{Status: agent.StatusSuccess, DurationMs: 1}, nil
	})
	orch := service.NewOrchestrator(svc, runner, 3, encKey, slog.Default())

	orch.FireSyncCompleteAgents(context.Background())

	select {
	case <-fired:
		t.Fatal("runner fired for an agent that ran within the debounce window")
	case <-time.After(300 * time.Millisecond):
		// pass — debounced, no dispatch
	}
}

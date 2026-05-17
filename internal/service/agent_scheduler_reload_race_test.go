//go:build integration && !lite

package service_test

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"breadbox/internal/agent"
	"breadbox/internal/service"
)

// TestAgentScheduler_Reload_ConcurrentCRUD_NoDuplicateEntries pins the
// iter-34 fix for audit BLOCKER #2. Before iter-34, two concurrent
// Reload() calls could interleave their "remove all + re-register"
// phases such that AddFunc ran twice per slug, producing duplicate
// cron entries whose EntryIDs were silently leaked (entryIDs[slug]
// could only store the last one — the others fired forever, no way to
// cancel them).
//
// The fix serializes the whole Reload critical section with reloadMu.
// This test fires N concurrent Reloads against the live scheduler +
// service and asserts the underlying cron.Cron has exactly the same
// number of per-agent entries it would have after one sequential
// Reload — no duplicates, no leaks.
func TestAgentScheduler_Reload_ConcurrentCRUD_NoDuplicateEntries(t *testing.T) {
	svc, _, _ := newService(t)
	encKey := seedSubscriptionAuth(t, svc)

	// Seed 3 enabled, cron-scheduled agents — each adds one cron entry
	// per Reload pass.
	for _, slug := range []string{"sched-race-a", "sched-race-b", "sched-race-c"} {
		cron := "0 * * * *"
		_, err := svc.CreateAgentDefinition(context.Background(), service.CreateAgentDefinitionParams{
			Name:         slug,
			Slug:         slug,
			Prompt:       "Concurrent reload race test prompt for " + slug + " — padding to satisfy validation length requirement.",
			ScheduleCron: &cron,
			ToolScope:    "read_only",
			AllowedTools: []string{},
			Model:        "claude-haiku-4-5",
			MaxTurns:     1,
			Enabled:      true,
		})
		if err != nil {
			t.Fatalf("create %s: %v", slug, err)
		}
	}

	orch := service.NewOrchestrator(svc, agent.RunnerFunc(func(_ context.Context, _ agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		return agent.RunResult{}, nil
	}), 1, encKey, slog.Default())
	sched := service.NewAgentScheduler(orch, svc, slog.Default())

	// Start the cron loop so AddFunc/Remove actually take effect.
	sched.Start(context.Background())
	defer sched.Stop()

	// Baseline: after Start, cron has 1 cleanup entry + 3 agent entries = 4.
	const expectedAgentEntries = 3
	const cleanupEntries = 1

	// Fire 8 concurrent Reload calls — simulates a burst of CRUD
	// mutations all triggering OnDefinitionChanged at once. Before
	// iter-34 this routinely produced duplicate cron entries.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sched.Reload(context.Background())
		}()
	}
	wg.Wait()

	gotEntries := sched.EntryCountForTest()
	wantEntries := expectedAgentEntries + cleanupEntries
	if gotEntries != wantEntries {
		t.Errorf("after 8 concurrent Reloads, cron has %d entries (want %d) — likely duplicate-entry leak from concurrent CRUD",
			gotEntries, wantEntries)
	}
}

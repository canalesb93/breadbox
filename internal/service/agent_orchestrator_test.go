//go:build integration && !lite

package service_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"breadbox/internal/agent"
	"breadbox/internal/appconfig"
	"breadbox/internal/service"
)

// seedSubscriptionAuth writes a fake subscription token so AssembleJobSpec
// doesn't error on auth-not-configured. Returns the test enc key.
func seedSubscriptionAuth(t *testing.T, svc *service.Service) []byte {
	t.Helper()
	tok := "sk-ant-oat01-FAKE-TEST-TOKEN-FOR-INTEGRATION"
	if _, err := svc.UpdateAgentSettings(context.Background(), service.UpdateAgentSettingsParams{
		SubscriptionToken: &tok,
	}, devEncKey, ""); err != nil {
		t.Fatalf("seed subscription auth: %v", err)
	}
	return devEncKey
}

func TestOrchestratorRunNow_Success(t *testing.T) {
	svc, _, _ := newService(t)
	encKey := seedSubscriptionAuth(t, svc)
	def := mustCreateAgentDefinition(t, svc, "orch-success", true)

	fake := agent.RunnerFunc(func(ctx context.Context, spec agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		return agent.RunResult{
			Status:       agent.StatusSuccess,
			TotalCostUSD: 0.0123,
			InputTokens:  100,
			OutputTokens: 50,
			TurnCount:    2,
			NumToolCalls: 1,
			SessionID:    "sess-test",
			DurationMs:   42,
		}, nil
	})

	orch := service.NewOrchestrator(svc, fake, 1, encKey, slog.Default())
	runResp, err := orch.RunNow(context.Background(), def, "")
	if err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	if runResp.Status != agent.StatusSuccess {
		t.Errorf("Status = %q, want success", runResp.Status)
	}
	if runResp.TotalCostUSD == nil || *runResp.TotalCostUSD != 0.0123 {
		t.Errorf("TotalCostUSD = %v, want 0.0123", runResp.TotalCostUSD)
	}
	if runResp.SessionID == nil || *runResp.SessionID != "sess-test" {
		t.Errorf("SessionID = %v, want sess-test", runResp.SessionID)
	}

	// Verify the row was persisted with the same fields.
	persisted, err := svc.GetAgentRun(context.Background(), runResp.ShortID)
	if err != nil {
		t.Fatalf("GetAgentRun: %v", err)
	}
	if persisted.Status != agent.StatusSuccess {
		t.Errorf("persisted Status = %q, want success", persisted.Status)
	}
}

func TestOrchestratorRunNow_ConcurrencyLocked(t *testing.T) {
	svc, _, _ := newService(t)
	encKey := seedSubscriptionAuth(t, svc)
	def := mustCreateAgentDefinition(t, svc, "orch-locked", true)

	started := make(chan struct{})
	release := make(chan struct{})

	slow := agent.RunnerFunc(func(ctx context.Context, _ agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		close(started)
		<-release
		return agent.RunResult{Status: agent.StatusSuccess}, nil
	})

	orch := service.NewOrchestrator(svc, slow, 1, encKey, slog.Default())

	var wg sync.WaitGroup
	wg.Add(1)
	var firstRun *service.AgentRunResponse
	var firstErr error
	go func() {
		defer wg.Done()
		firstRun, firstErr = orch.RunNow(context.Background(), def, "")
	}()

	<-started // first goroutine holds the slot

	_, secondErr := orch.RunNow(context.Background(), def, "")
	if !errors.Is(secondErr, agent.ErrConcurrencyLocked) {
		t.Errorf("second RunNow err = %v, want ErrConcurrencyLocked", secondErr)
	}

	close(release)
	wg.Wait()

	if firstErr != nil {
		t.Errorf("first RunNow err = %v, want nil", firstErr)
	}
	if firstRun == nil || firstRun.Status != agent.StatusSuccess {
		t.Errorf("first run status = %v, want success", firstRun)
	}
}

func TestOrchestratorRunOrSkip_LeavesSkippedRow(t *testing.T) {
	svc, _, _ := newService(t)
	encKey := seedSubscriptionAuth(t, svc)
	def := mustCreateAgentDefinition(t, svc, "orch-skip", true)

	started := make(chan struct{})
	release := make(chan struct{})

	slow := agent.RunnerFunc(func(ctx context.Context, _ agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		close(started)
		<-release
		return agent.RunResult{Status: agent.StatusSuccess}, nil
	})
	orch := service.NewOrchestrator(svc, slow, 1, encKey, slog.Default())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = orch.RunNow(context.Background(), def, "")
	}()
	<-started

	skippedRun, err := orch.RunOrSkip(context.Background(), def, "cron")
	if !errors.Is(err, agent.ErrConcurrencyLocked) {
		t.Errorf("RunOrSkip err = %v, want ErrConcurrencyLocked", err)
	}
	if skippedRun == nil {
		t.Fatal("expected a skipped run row, got nil")
	}
	if skippedRun.Status != "skipped" {
		t.Errorf("skipped Status = %q, want skipped", skippedRun.Status)
	}
	if skippedRun.Trigger != "cron" {
		t.Errorf("skipped Trigger = %q, want cron", skippedRun.Trigger)
	}

	close(release)
	wg.Wait()

	// Verify the skipped row is in the run history.
	persisted, err := svc.GetAgentRun(context.Background(), skippedRun.ShortID)
	if err != nil {
		t.Fatalf("GetAgentRun(skipped): %v", err)
	}
	if persisted.Status != "skipped" {
		t.Errorf("persisted skipped Status = %q, want skipped", persisted.Status)
	}
}

func TestOrchestratorRunNow_AuthNotConfigured(t *testing.T) {
	svc, _, _ := newService(t)
	// Deliberately do NOT seed auth — orchestrator should surface ErrAuthNotConfigured.
	def := mustCreateAgentDefinition(t, svc, "orch-noauth", true)
	fake := agent.RunnerFunc(func(ctx context.Context, _ agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		t.Error("runner should not be invoked when auth is missing")
		return agent.RunResult{}, nil
	})
	orch := service.NewOrchestrator(svc, fake, 1, devEncKey, slog.Default())
	_, err := orch.RunNow(context.Background(), def, "")
	if !errors.Is(err, agent.ErrAuthNotConfigured) {
		t.Errorf("err = %v, want ErrAuthNotConfigured", err)
	}
}

func TestOrchestratorRunNow_RunnerErrorPersisted(t *testing.T) {
	svc, _, _ := newService(t)
	encKey := seedSubscriptionAuth(t, svc)
	def := mustCreateAgentDefinition(t, svc, "orch-error", true)

	fake := agent.RunnerFunc(func(ctx context.Context, _ agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		return agent.RunResult{
			Status:     agent.StatusError,
			DurationMs: 5,
			Err:        errors.New("sidecar said no"),
		}, errors.New("sidecar said no")
	})
	orch := service.NewOrchestrator(svc, fake, 1, encKey, slog.Default())

	runResp, err := orch.RunNow(context.Background(), def, "")
	if err == nil {
		t.Fatal("expected error from RunNow on runner failure")
	}
	if runResp == nil || runResp.Status != agent.StatusError {
		t.Errorf("expected error-status row, got %v", runResp)
	}
}

func TestOrchestratorRunNow_MintRevokeRoundTrip(t *testing.T) {
	svc, _, _ := newService(t)
	encKey := seedSubscriptionAuth(t, svc)
	def := mustCreateAgentDefinition(t, svc, "orch-mintrevoke", true)

	var capturedToken string
	fake := agent.RunnerFunc(func(ctx context.Context, spec agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		// Find the BREADBOX_API_KEY env var in the MCP server config.
		if mcp, ok := spec.MCPServers["breadbox"]; ok {
			capturedToken = mcp.Env["BREADBOX_API_KEY"]
		}
		return agent.RunResult{Status: agent.StatusSuccess, DurationMs: 1}, nil
	})
	orch := service.NewOrchestrator(svc, fake, 1, encKey, slog.Default())

	runResp, err := orch.RunNow(context.Background(), def, "")
	if err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	if capturedToken == "" {
		t.Error("expected BREADBOX_API_KEY to be set in JobSpec MCP env")
	}

	// Confirm the minted key was revoked after the run.
	keys, err := svc.ListAPIKeys(context.Background())
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	wantName := "agent:" + def.Slug + ":" + runResp.ShortID
	for _, k := range keys {
		if k.Name == wantName && k.RevokedAt == nil {
			t.Errorf("minted key %q should be revoked, but RevokedAt is nil", wantName)
		}
	}
	// Touch the unused appconfig reader to keep the import live in case
	// the test file evolves to read settings directly.
	_ = appconfig.String(context.Background(), svc.Queries, appconfig.KeyAgentAuthMode, "")
}

func TestOrchestratorRunNow_PromptPrefix_PrependsToSpecAndPersists(t *testing.T) {
	svc, _, _ := newService(t)
	encKey := seedSubscriptionAuth(t, svc)
	def := mustCreateAgentDefinition(t, svc, "orch-prefix", true)
	const prefix = "Focus on Amazon Prime transactions only — ignore the rest."

	var capturedPrompt string
	fake := agent.RunnerFunc(func(_ context.Context, spec agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		capturedPrompt = spec.Prompt
		return agent.RunResult{
			Status:       agent.StatusSuccess,
			TotalCostUSD: 0.01,
			TurnCount:    1,
			DurationMs:   10,
		}, nil
	})

	orch := service.NewOrchestrator(svc, fake, 1, encKey, slog.Default())
	runResp, err := orch.RunNow(context.Background(), def, prefix)
	if err != nil {
		t.Fatalf("RunNow: %v", err)
	}

	// The sidecar saw a prompt with our prefix at the top.
	if !strings.HasPrefix(capturedPrompt, "Operator note for this run:\n"+prefix+"\n\n") {
		t.Errorf("spec.Prompt missing expected operator-note header:\n%s", capturedPrompt)
	}
	if !strings.Contains(capturedPrompt, def.Prompt) {
		t.Errorf("spec.Prompt should still contain the agent's stored prompt; got:\n%s", capturedPrompt)
	}

	// The run row carries the prefix for the audit trail.
	if runResp.PromptPrefix == nil || *runResp.PromptPrefix != prefix {
		t.Errorf("response PromptPrefix = %v, want %q", runResp.PromptPrefix, prefix)
	}
	persisted, err := svc.GetAgentRun(context.Background(), runResp.ShortID)
	if err != nil {
		t.Fatalf("GetAgentRun: %v", err)
	}
	if persisted.PromptPrefix == nil || *persisted.PromptPrefix != prefix {
		t.Errorf("persisted PromptPrefix = %v, want %q", persisted.PromptPrefix, prefix)
	}
}

func TestOrchestratorRunNow_EmptyPrefix_LeavesPromptUnchanged(t *testing.T) {
	svc, _, _ := newService(t)
	encKey := seedSubscriptionAuth(t, svc)
	def := mustCreateAgentDefinition(t, svc, "orch-no-prefix", true)

	var capturedPrompt string
	fake := agent.RunnerFunc(func(_ context.Context, spec agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		capturedPrompt = spec.Prompt
		return agent.RunResult{Status: agent.StatusSuccess, TurnCount: 1, DurationMs: 5}, nil
	})

	orch := service.NewOrchestrator(svc, fake, 1, encKey, slog.Default())
	runResp, err := orch.RunNow(context.Background(), def, "")
	if err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	if capturedPrompt != def.Prompt {
		t.Errorf("empty prefix should leave prompt untouched; got %q, want %q", capturedPrompt, def.Prompt)
	}
	if runResp.PromptPrefix != nil {
		t.Errorf("PromptPrefix should be nil when none supplied, got %v", *runResp.PromptPrefix)
	}
}

func TestOrchestratorRunNow_MaxTurnsHit_PersistedAsHitCap(t *testing.T) {
	svc, _, _ := newService(t)
	encKey := seedSubscriptionAuth(t, svc)
	def := mustCreateAgentDefinition(t, svc, "orch-cap-turns", true)

	fake := agent.RunnerFunc(func(_ context.Context, _ agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		// Sidecar's contract: max_turns → success status + ErrMaxTurnsReached on err.
		return agent.RunResult{
			Status:     agent.StatusSuccess,
			TurnCount:  10,
			DurationMs: 50,
		}, agent.ErrMaxTurnsReached
	})

	orch := service.NewOrchestrator(svc, fake, 1, encKey, slog.Default())
	runResp, runErr := orch.RunNow(context.Background(), def, "")
	// Orchestrator returns the runner's error untouched — but the row should
	// be persisted with hit_cap set.
	if !errors.Is(runErr, agent.ErrMaxTurnsReached) {
		t.Fatalf("RunNow err = %v, want ErrMaxTurnsReached", runErr)
	}
	if runResp == nil {
		t.Fatal("expected run row")
	}
	if runResp.HitCap == nil || *runResp.HitCap != "max_turns" {
		t.Errorf("HitCap = %v, want max_turns", runResp.HitCap)
	}
	// Status stays success — the sidecar terminated cleanly within bounds.
	if runResp.Status != agent.StatusSuccess {
		t.Errorf("Status = %q, want success", runResp.Status)
	}

	// Verify the row was actually persisted.
	persisted, err := svc.GetAgentRun(context.Background(), runResp.ShortID)
	if err != nil {
		t.Fatalf("GetAgentRun: %v", err)
	}
	if persisted.HitCap == nil || *persisted.HitCap != "max_turns" {
		t.Errorf("persisted HitCap = %v, want max_turns", persisted.HitCap)
	}
}

func TestOrchestratorRunNow_BudgetHit_PersistedAsHitCap(t *testing.T) {
	svc, _, _ := newService(t)
	encKey := seedSubscriptionAuth(t, svc)
	def := mustCreateAgentDefinition(t, svc, "orch-cap-budget", true)

	fake := agent.RunnerFunc(func(_ context.Context, _ agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		// Sidecar's contract: budget_exceeded → error status + ErrBudgetExceeded.
		return agent.RunResult{
			Status:       agent.StatusError,
			TotalCostUSD: 1.5,
			DurationMs:   30,
		}, agent.ErrBudgetExceeded
	})

	orch := service.NewOrchestrator(svc, fake, 1, encKey, slog.Default())
	runResp, runErr := orch.RunNow(context.Background(), def, "")
	if !errors.Is(runErr, agent.ErrBudgetExceeded) {
		t.Fatalf("RunNow err = %v, want ErrBudgetExceeded", runErr)
	}
	if runResp == nil {
		t.Fatal("expected run row")
	}
	if runResp.HitCap == nil || *runResp.HitCap != "max_budget" {
		t.Errorf("HitCap = %v, want max_budget", runResp.HitCap)
	}
	// Status is error — the cap aborted the run mid-way.
	if runResp.Status != agent.StatusError {
		t.Errorf("Status = %q, want error", runResp.Status)
	}
}

func TestOrchestratorRunNow_CleanSuccess_LeavesHitCapNil(t *testing.T) {
	svc, _, _ := newService(t)
	encKey := seedSubscriptionAuth(t, svc)
	def := mustCreateAgentDefinition(t, svc, "orch-no-cap", true)

	fake := agent.RunnerFunc(func(_ context.Context, _ agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		return agent.RunResult{Status: agent.StatusSuccess, TurnCount: 3, DurationMs: 12}, nil
	})

	orch := service.NewOrchestrator(svc, fake, 1, encKey, slog.Default())
	runResp, err := orch.RunNow(context.Background(), def, "")
	if err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	if runResp.HitCap != nil {
		t.Errorf("HitCap should be nil on clean success, got %q", *runResp.HitCap)
	}
}

// TestOrchestratorRunNow_TripleConcurrency exercises the iter-29 default
// (max_concurrent=3). Fires 3 RunNow calls concurrently; all should
// complete cleanly with no semaphore lockout. Verifies mint-and-revoke,
// per-run row creation, and end-state run count all behave under
// contention. The 4th concurrent attempt should still lock — the cap is
// a real ceiling, not a no-op.
func TestOrchestratorRunNow_TripleConcurrency(t *testing.T) {
	svc, _, _ := newService(t)
	encKey := seedSubscriptionAuth(t, svc)
	def := mustCreateAgentDefinition(t, svc, "orch-3x", true)

	var running atomic.Int32
	var maxObserved atomic.Int32
	release := make(chan struct{})
	started := make(chan struct{}, 4)

	runner := agent.RunnerFunc(func(ctx context.Context, _ agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		now := running.Add(1)
		defer running.Add(-1)
		for {
			old := maxObserved.Load()
			if now <= old || maxObserved.CompareAndSwap(old, now) {
				break
			}
		}
		started <- struct{}{}
		<-release
		return agent.RunResult{Status: agent.StatusSuccess, DurationMs: 1}, nil
	})

	// Cap=3 — should let the first three Runs all run in parallel.
	orch := service.NewOrchestrator(svc, runner, 3, encKey, slog.Default())

	var wg sync.WaitGroup
	results := make([]error, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := orch.RunNow(context.Background(), def, "")
			results[idx] = err
		}(i)
	}

	// Wait for all three goroutines to enter the runner before releasing.
	for i := 0; i < 3; i++ {
		<-started
	}

	// A 4th concurrent attempt should hit ErrConcurrencyLocked while the
	// other three are still running.
	_, fourthErr := orch.RunNow(context.Background(), def, "")
	if !errors.Is(fourthErr, agent.ErrConcurrencyLocked) {
		t.Errorf("4th RunNow under cap=3 should return ErrConcurrencyLocked, got %v", fourthErr)
	}

	close(release)
	wg.Wait()

	for i, err := range results {
		if err != nil {
			t.Errorf("RunNow #%d err = %v, want nil", i, err)
		}
	}
	if got := maxObserved.Load(); got != 3 {
		t.Errorf("max concurrent runs observed = %d, want 3 (cap=3 should NOT serialize)", got)
	}

	// Every successful run minted an api_key and revoked it. After all three
	// complete, no `agent:orch-3x:*` key should remain unrevoked.
	keys, err := svc.ListAPIKeys(context.Background())
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	for _, k := range keys {
		if strings.HasPrefix(k.Name, "agent:orch-3x:") && k.RevokedAt == nil {
			t.Errorf("unrevoked minted key after concurrent runs: %s", k.Name)
		}
	}
}

// TestOrchestratorRunNowWith_PromptOverride_ReplacesDefPrompt pins the
// iter-45 "Test this prompt" flow: when an operator passes
// RunOverrides.PromptOverride, the sidecar receives THAT text as
// spec.Prompt instead of def.Prompt. Operator can dry-fire an unsaved
// edit-form prompt without round-tripping through Save.
func TestOrchestratorRunNowWith_PromptOverride_ReplacesDefPrompt(t *testing.T) {
	svc, _, _ := newService(t)
	encKey := seedSubscriptionAuth(t, svc)
	def := mustCreateAgentDefinition(t, svc, "orch-prompt-override", true)
	const override = "Operator's unsaved test prompt — definitely not the stored one, used for dry-fire only."

	var capturedPrompt string
	fake := agent.RunnerFunc(func(_ context.Context, spec agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		capturedPrompt = spec.Prompt
		return agent.RunResult{Status: agent.StatusSuccess, TurnCount: 1, DurationMs: 10}, nil
	})

	orch := service.NewOrchestrator(svc, fake, 1, encKey, slog.Default())
	_, err := orch.RunNowWith(context.Background(), def, service.RunOverrides{
		PromptOverride: override,
	})
	if err != nil {
		t.Fatalf("RunNowWith: %v", err)
	}
	if capturedPrompt != override {
		t.Errorf("spec.Prompt = %q, want override exactly (no prefix wrapping)", capturedPrompt)
	}
}

// TestOrchestratorRunNowWith_OverrideBeatsPrefix verifies the precedence
// rule: when both PromptOverride AND PromptPrefix are set, the override
// wins (the operator is testing a freshly-typed prompt, prepending a
// prefix wouldn't match what they actually want to test).
func TestOrchestratorRunNowWith_OverrideBeatsPrefix(t *testing.T) {
	svc, _, _ := newService(t)
	encKey := seedSubscriptionAuth(t, svc)
	def := mustCreateAgentDefinition(t, svc, "orch-prompt-prec", true)

	var capturedPrompt string
	fake := agent.RunnerFunc(func(_ context.Context, spec agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		capturedPrompt = spec.Prompt
		return agent.RunResult{Status: agent.StatusSuccess, TurnCount: 1, DurationMs: 5}, nil
	})

	orch := service.NewOrchestrator(svc, fake, 1, encKey, slog.Default())
	_, err := orch.RunNowWith(context.Background(), def, service.RunOverrides{
		PromptPrefix:   "this prefix should be ignored",
		PromptOverride: "and the override should win clean",
	})
	if err != nil {
		t.Fatalf("RunNowWith: %v", err)
	}
	if capturedPrompt != "and the override should win clean" {
		t.Errorf("override should win exactly; got %q", capturedPrompt)
	}
	if strings.Contains(capturedPrompt, "this prefix should be ignored") {
		t.Errorf("prefix leaked through despite override being set: %q", capturedPrompt)
	}
}

// TestOrchestratorRunNowAsyncWith_PanicInGoroutineIsRecovered verifies that
// a panic from the runner (in the async-dispatch goroutine) does NOT
// crash the breadbox process and DOES mark the run row as errored with a
// recognizable message. Before the panic-recover deferred handler was
// added, this scenario would propagate out and SIGABRT the server.
func TestOrchestratorRunNowAsyncWith_PanicInGoroutineIsRecovered(t *testing.T) {
	svc, _, _ := newService(t)
	encKey := seedSubscriptionAuth(t, svc)
	def := mustCreateAgentDefinition(t, svc, "orch-panic", true)

	// RunNowAsyncWith preflights agent.LocateBinary before spawning the
	// goroutine — in CI / fresh envs no sidecar is installed, so without
	// an override the preflight returns ErrBinaryNotFound as a sync error
	// and the panic path we want to exercise never runs. Stage any
	// existing file (we'll never actually exec it; the panickyRunner
	// substitutes for the real Sidecar).
	t.Setenv("BREADBOX_AGENT_BIN", "/bin/sh")

	panickyRunner := agent.RunnerFunc(func(ctx context.Context, spec agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		panic("runner exploded mid-stream")
	})

	orch := service.NewOrchestrator(svc, panickyRunner, 1, encKey, slog.Default())

	resp, err := orch.RunNowAsyncWith(context.Background(), def, service.RunOverrides{})
	if err != nil {
		t.Fatalf("RunNowAsyncWith: unexpected sync error: %v", err)
	}
	if resp.Status != "in_progress" {
		t.Fatalf("initial Status = %q, want in_progress (async hand-off)", resp.Status)
	}

	// Poll the run row until it transitions away from in_progress (the
	// goroutine's recover should mark it error). 5s is generous — the
	// panic + DB write should complete in tens of ms.
	deadline := time.Now().Add(5 * time.Second)
	var final *service.AgentRunResponse
	for time.Now().Before(deadline) {
		got, gerr := svc.GetAgentRun(context.Background(), resp.ShortID)
		if gerr != nil {
			t.Fatalf("GetAgentRun: %v", gerr)
		}
		if got.Status != "in_progress" {
			final = got
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if final == nil {
		t.Fatal("run row stayed in_progress past 5s — panic-recover handler didn't update the row")
	}
	if final.Status != "error" {
		t.Errorf("final Status = %q, want error", final.Status)
	}
	if final.ErrorMessage == nil || !strings.Contains(*final.ErrorMessage, "panic") {
		t.Errorf("ErrorMessage = %v, want substring 'panic'", final.ErrorMessage)
	}

	// Critical: the orchestrator must still be functional after a panic.
	// Run a second job to prove the goroutine's deferred semaphore release
	// fired even on the panic path.
	cleanRunner := agent.RunnerFunc(func(ctx context.Context, spec agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		return agent.RunResult{Status: agent.StatusSuccess, DurationMs: 1}, nil
	})
	orch2 := service.NewOrchestrator(svc, cleanRunner, 1, encKey, slog.Default())
	if _, err := orch2.RunNow(context.Background(), def, ""); err != nil {
		t.Errorf("post-panic RunNow failed (semaphore leaked?): %v", err)
	}
}

func TestHouseholdCostSince(t *testing.T) {
	svc, _, pool := newService(t)
	ctx := context.Background()
	def := mustCreateAgentDefinition(t, svc, "cost-since", true)

	ins := func(status string, cost string, age string) {
		_, err := pool.Exec(ctx,
			`INSERT INTO workflow_runs (agent_definition_id,"trigger",status,total_cost_usd,started_at)
			 VALUES ($1,'cron',$2,$3, NOW() - $4::interval)`,
			def.ID, status, cost, age)
		if err != nil {
			t.Fatalf("insert run: %v", err)
		}
	}
	ins("success", "0.06", "1 hour")   // in window
	ins("success", "0.04", "2 hours")  // in window
	ins("skipped", "0.50", "1 hour")   // excluded: skipped
	ins("success", "1.00", "60 days")  // excluded: outside window

	spent, err := svc.HouseholdCostSince(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("HouseholdCostSince: %v", err)
	}
	if spent < 0.099 || spent > 0.101 {
		t.Fatalf("spent = %v, want ~0.10 (skipped + out-of-window excluded)", spent)
	}
}

func TestRunOrSkip_HouseholdCeiling(t *testing.T) {
	svc, _, pool := newService(t)
	encKey := seedSubscriptionAuth(t, svc)
	ctx := context.Background()
	def := mustCreateAgentDefinition(t, svc, "ceiling", true)

	// Set a $0.10 household ceiling and push 30-day spend over it.
	ceiling := 0.10
	if _, err := svc.UpdateAgentSettings(ctx, service.UpdateAgentSettingsParams{GlobalMaxBudgetUSD: &ceiling}, encKey, ""); err != nil {
		t.Fatalf("set ceiling: %v", err)
	}
	for _, c := range []string{"0.07", "0.06"} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO workflow_runs (agent_definition_id,"trigger",status,total_cost_usd,started_at)
			 VALUES ($1,'cron','success',$2, NOW())`, def.ID, c); err != nil {
			t.Fatalf("insert run: %v", err)
		}
	}

	noop := agent.RunnerFunc(func(ctx context.Context, _ agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		t.Error("runner should NOT fire when over the household ceiling")
		return agent.RunResult{Status: agent.StatusSuccess}, nil
	})
	orch := service.NewOrchestrator(svc, noop, 1, encKey, slog.Default())

	resp, err := orch.RunOrSkip(ctx, def, "cron")
	if !errors.Is(err, service.ErrBudgetCeilingReached) {
		t.Fatalf("RunOrSkip err = %v, want ErrBudgetCeilingReached", err)
	}
	if resp == nil || resp.Status != "skipped" {
		t.Fatalf("want a skipped row, got %+v", resp)
	}
}

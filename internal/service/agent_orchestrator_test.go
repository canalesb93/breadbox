//go:build integration && !lite

package service_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

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
	}, devEncKey); err != nil {
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

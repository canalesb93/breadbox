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

// TestOrchestratorRunNow_PersistsErrorMessage pins that a runner error
// (sidecar crash, scanner failure) reaches agent_runs.error_message via
// CompleteAgentRunDB — without it, the row shows status='error' but no
// reason, and operators have to grep container logs.
func TestOrchestratorRunNow_PersistsErrorMessage(t *testing.T) {
	svc, _, _ := newService(t)
	encKey := seedSubscriptionAuth(t, svc)

	def, err := svc.CreateAgentDefinition(context.Background(), service.CreateAgentDefinitionParams{
		Name:         "orch-err-msg",
		Slug:         "orch-err-msg",
		Prompt:       "error message persistence regression test prompt — must exceed validation length so padding lives here.",
		ToolScope:    "read_only",
		AllowedTools: []string{},
		Model:        "claude-haiku-4-5",
		MaxTurns:     10,
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	const errText = "agent: sidecar exited non-zero: exit=exit status 127 stderr=Error loading shared library libstdc++.so.6"
	runner := agent.RunnerFunc(func(_ context.Context, _ agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		e := errors.New(errText)
		return agent.RunResult{
			Status:     agent.StatusError,
			Err:        e,
			DurationMs: 5,
		}, e
	})

	orch := service.NewOrchestrator(svc, runner, 3, encKey, slog.Default())
	runResp, _ := orch.RunNow(context.Background(), def, "")
	if runResp == nil {
		t.Fatalf("RunNow returned nil run response")
	}

	persisted, err := svc.GetAgentRun(context.Background(), runResp.ShortID)
	if err != nil {
		t.Fatalf("GetAgentRun: %v", err)
	}
	if persisted.Status != "error" {
		t.Fatalf("Status = %q, want %q", persisted.Status, "error")
	}
	if persisted.ErrorMessage == nil {
		t.Fatalf("ErrorMessage = nil, want runner error text (regression: orchestrator must persist result.Err to agent_runs.error_message)")
	}
	if !strings.Contains(*persisted.ErrorMessage, "libstdc++") {
		t.Errorf("ErrorMessage = %q, want it to contain runner error text", *persisted.ErrorMessage)
	}
}

// TestOrchestratorRunNow_SuccessLeavesErrorMessageEmpty guards the other
// direction: a clean success must not paint error_message with stale
// text from a prior code path.
func TestOrchestratorRunNow_SuccessLeavesErrorMessageEmpty(t *testing.T) {
	svc, _, _ := newService(t)
	encKey := seedSubscriptionAuth(t, svc)

	def, err := svc.CreateAgentDefinition(context.Background(), service.CreateAgentDefinitionParams{
		Name:         "orch-err-msg-ok",
		Slug:         "orch-err-msg-ok",
		Prompt:       "success path error_message guard — must exceed validation length so this padding is here.",
		ToolScope:    "read_only",
		AllowedTools: []string{},
		Model:        "claude-haiku-4-5",
		MaxTurns:     10,
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	runner := agent.RunnerFunc(func(_ context.Context, _ agent.JobSpec, _ agent.EventHandler) (agent.RunResult, error) {
		return agent.RunResult{Status: agent.StatusSuccess, TurnCount: 1, DurationMs: 5}, nil
	})

	orch := service.NewOrchestrator(svc, runner, 3, encKey, slog.Default())
	runResp, err := orch.RunNow(context.Background(), def, "")
	if err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	persisted, err := svc.GetAgentRun(context.Background(), runResp.ShortID)
	if err != nil {
		t.Fatalf("GetAgentRun: %v", err)
	}
	if persisted.ErrorMessage != nil {
		t.Errorf("ErrorMessage = %q, want nil on success", *persisted.ErrorMessage)
	}
}

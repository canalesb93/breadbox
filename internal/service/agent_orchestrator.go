//go:build !lite

package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"breadbox/internal/agent"
	"breadbox/internal/pgconv"
)

// Orchestrator drives one agent run end-to-end: acquires concurrency,
// mints a scoped API key, assembles the JobSpec, runs the sidecar, persists
// the resulting agent_runs row, and revokes the key.
type Orchestrator struct {
	svc    *Service
	runner agent.Runner
	sem    *agent.Semaphore
	encKey []byte
	logger *slog.Logger

	// sched is the agent scheduler this orchestrator notifies on CRUD changes.
	// Wired via AttachScheduler after construction (avoids a chicken-and-egg).
	sched *AgentScheduler
}

// NewOrchestrator constructs an Orchestrator.
// maxConcurrent < 1 is clamped to 1.
func NewOrchestrator(svc *Service, runner agent.Runner, maxConcurrent int, encKey []byte, logger *slog.Logger) *Orchestrator {
	return &Orchestrator{
		svc:    svc,
		runner: runner,
		sem:    agent.NewSemaphore(maxConcurrent),
		encKey: encKey,
		logger: logger,
	}
}

// AttachScheduler wires the scheduler so NotifyDefinitionChanged can trigger
// Reload. Call after both Orchestrator and Scheduler are built.
func (o *Orchestrator) AttachScheduler(s *AgentScheduler) {
	o.sched = s
}

// NotifyDefinitionChanged tells the attached scheduler (if any) to reload
// its registrations from DB. Safe to call from any goroutine; nop if no
// scheduler is attached.
func (o *Orchestrator) NotifyDefinitionChanged() {
	if o.sched != nil {
		o.sched.Reload(context.Background())
	}
}

// RunNow executes one agent run synchronously, for "run now" requests.
// Returns ErrConcurrencyLocked WITHOUT creating a run row when the semaphore
// is full — the caller (HTTP handler) maps to 503 and the user retries.
// Returns the resulting agent_runs row on any other outcome (success or error).
func (o *Orchestrator) RunNow(ctx context.Context, def *AgentDefinitionResponse) (*AgentRunResponse, error) {
	if err := o.sem.Acquire(ctx); err != nil {
		return nil, err
	}
	defer o.sem.Release()
	return o.runLocked(ctx, def, "manual")
}

// RunOrSkip is the entry point for scheduled (cron) runs. Always leaves
// a row in agent_runs — either completed/errored, or 'skipped' when the
// semaphore was full. Returns ErrConcurrencyLocked alongside the skipped
// row so the scheduler can log appropriately.
func (o *Orchestrator) RunOrSkip(ctx context.Context, def *AgentDefinitionResponse, trigger string) (*AgentRunResponse, error) {
	if err := o.sem.Acquire(ctx); err != nil {
		// Leave a 'skipped' row so the run history shows the missed fire.
		defUUID, perr := pgconv.ParseUUID(def.ID)
		if perr != nil {
			return nil, fmt.Errorf("orchestrator: parse def id: %w", perr)
		}
		runRow, crerr := o.svc.CreateAgentRunDB(ctx, defUUID, trigger)
		if crerr != nil {
			return nil, fmt.Errorf("orchestrator: create skipped run row: %w (acquire err: %v)", crerr, err)
		}
		_ = o.svc.MarkAgentRunSkippedDB(ctx, runRow.ID, "another run was in progress")
		resp := AgentRunFromRow(runRow)
		resp.Status = "skipped"
		return &resp, err
	}
	defer o.sem.Release()
	return o.runLocked(ctx, def, trigger)
}

// runLocked assumes the caller holds the semaphore.
func (o *Orchestrator) runLocked(ctx context.Context, def *AgentDefinitionResponse, trigger string) (*AgentRunResponse, error) {
	defUUID, err := pgconv.ParseUUID(def.ID)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: parse def id: %w", err)
	}

	runRow, err := o.svc.CreateAgentRunDB(ctx, defUUID, trigger)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: create run row: %w", err)
	}
	runResp := AgentRunFromRow(runRow)

	keyResult, err := o.svc.MintRunAPIKey(ctx, def, runResp.ShortID)
	if err != nil {
		o.logger.Warn("orchestrator: mint api key failed",
			"agent", def.Slug, "run", runResp.ShortID, "error", err)
		_ = o.svc.MarkAgentRunErrorDB(ctx, runRow.ID, fmt.Sprintf("mint api key: %v", err), "")
		return &runResp, fmt.Errorf("orchestrator: mint api key: %w", err)
	}
	defer func() {
		// Always revoke. Use a fresh ctx with timeout so a cancelled parent
		// doesn't prevent cleanup.
		revokeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if rerr := o.svc.RevokeAPIKey(revokeCtx, keyResult.ID); rerr != nil {
			o.logger.Warn("orchestrator: revoke api key failed",
				"agent", def.Slug, "run", runResp.ShortID,
				"key_id", keyResult.ID, "error", rerr)
		}
	}()

	spec, err := o.svc.AssembleJobSpec(ctx, def, &runResp, keyResult.PlaintextKey, o.encKey)
	if err != nil {
		o.logger.Warn("orchestrator: assemble spec failed",
			"agent", def.Slug, "run", runResp.ShortID, "error", err)
		_ = o.svc.MarkAgentRunErrorDB(ctx, runRow.ID, fmt.Sprintf("assemble spec: %v", err), "")
		return &runResp, fmt.Errorf("orchestrator: assemble spec: %w", err)
	}

	o.logger.Info("orchestrator: run starting",
		"agent", def.Slug, "run", runResp.ShortID, "trigger", trigger, "model", def.Model)

	// Event handler emits one structured slog line per sidecar NDJSON event.
	// Useful for tracing without OTel; cheap (Debug level by default — the
	// transcript file on disk is the canonical replay surface).
	handler := func(ev agent.Event) error {
		o.logger.Debug("orchestrator: sidecar event",
			"agent", def.Slug, "run", runResp.ShortID, "event_type", ev.Type)
		return nil
	}
	result, runErr := o.runner.Run(ctx, *spec, handler)

	completedRow, completeErr := o.svc.CompleteAgentRunDB(ctx, runRow.ID, result)
	if completeErr != nil {
		o.logger.Error("orchestrator: persist completed run failed",
			"agent", def.Slug, "run", runResp.ShortID, "error", completeErr)
		if runErr != nil {
			return &runResp, runErr
		}
		return &runResp, completeErr
	}

	finalResp := AgentRunFromRow(completedRow)

	switch {
	case errors.Is(runErr, agent.ErrAuthNotConfigured),
		errors.Is(runErr, agent.ErrBinaryNotFound):
		// Surface the upstream error so the handler can produce a clearer
		// error code than the generic agent_runs.error_message.
		return &finalResp, runErr
	case runErr != nil:
		o.logger.Warn("orchestrator: run finished with error",
			"agent", def.Slug, "run", runResp.ShortID,
			"status", result.Status, "error", runErr)
		return &finalResp, runErr
	}

	o.logger.Info("orchestrator: run finished",
		"agent", def.Slug, "run", runResp.ShortID,
		"status", result.Status, "cost_usd", result.TotalCostUSD,
		"duration_ms", result.DurationMs, "turns", result.TurnCount)
	return &finalResp, nil
}

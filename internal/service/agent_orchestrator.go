//go:build !lite

package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"breadbox/internal/agent"
	"breadbox/internal/appconfig"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5/pgtype"
)

// applyPromptPrefix renders the operator-supplied prefix in front of the
// agent's stored prompt. Split out so tests can pin the exact format and
// the orchestrator stays readable.
func applyPromptPrefix(prefix, original string) string {
	if prefix == "" {
		return original
	}
	return "Operator note for this run:\n" + prefix + "\n\n" + original
}

// capFromRunErr maps the sidecar's safety-cap sentinels onto the discrete
// hit_cap string the DB stores. Returns "" for any other error (or nil).
// max_turns is success-tagged by the sidecar (clean termination); budget
// is error-tagged (mid-run abort) — capFromRunErr preserves that nuance
// by leaving status untouched and only adding the hit_cap signal.
func capFromRunErr(err error) string {
	switch {
	case errors.Is(err, agent.ErrMaxTurnsReached):
		return "max_turns"
	case errors.Is(err, agent.ErrBudgetExceeded):
		return "max_budget"
	default:
		return ""
	}
}

// checkHouseholdCeiling enforces the household's aggregate spend ceiling
// before a run starts. When KeyAgentGlobalMaxBudgetUSD is set and the
// rolling 30-day spend has reached it, returns ErrBudgetCeilingReached
// (wrapped with the spent/ceiling figures). No ceiling (unset or <= 0)
// returns nil. A cost-sum query error fails OPEN — a transient DB hiccup
// must not wedge every workflow run — and is logged.
func (o *Orchestrator) checkHouseholdCeiling(ctx context.Context) error {
	ceiling := readOptionalFloat(ctx, o.svc.Queries, appconfig.KeyAgentGlobalMaxBudgetUSD)
	if ceiling == nil || *ceiling <= 0 {
		return nil
	}
	spent, err := o.svc.HouseholdCostSince(ctx, time.Now().Add(-HouseholdCeilingWindow))
	if err != nil {
		o.logger.Warn("orchestrator: household ceiling check failed; allowing run", "error", err)
		return nil
	}
	if spent >= *ceiling {
		return fmt.Errorf("%w ($%.2f of $%.2f in 30 days)", ErrBudgetCeilingReached, spent, *ceiling)
	}
	return nil
}

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

	// inFlight maps an in-progress run's short_id to its run-context CancelFunc,
	// so CancelRun can abort a specific run mid-flight (the run-detail "Cancel
	// run" button). Registered when the async run goroutine starts and removed
	// when it exits. Only this process's runs appear here.
	inFlightMu sync.Mutex
	inFlight   map[string]context.CancelFunc
}

// NewOrchestrator constructs an Orchestrator.
// maxConcurrent < 1 is clamped to 1.
func NewOrchestrator(svc *Service, runner agent.Runner, maxConcurrent int, encKey []byte, logger *slog.Logger) *Orchestrator {
	return &Orchestrator{
		svc:      svc,
		runner:   runner,
		sem:      agent.NewSemaphore(maxConcurrent),
		encKey:   encKey,
		logger:   logger,
		inFlight: make(map[string]context.CancelFunc),
	}
}

// registerInFlight records an in-progress run's CancelFunc so CancelRun can
// abort it. unregisterInFlight removes it (deferred on goroutine exit).
func (o *Orchestrator) registerInFlight(shortID string, cancel context.CancelFunc) {
	o.inFlightMu.Lock()
	o.inFlight[shortID] = cancel
	o.inFlightMu.Unlock()
}

func (o *Orchestrator) unregisterInFlight(shortID string) {
	o.inFlightMu.Lock()
	delete(o.inFlight, shortID)
	o.inFlightMu.Unlock()
}

// CancelRun aborts an in-progress run by cancelling its run context, which
// SIGKILLs the sidecar process group (see internal/agent/sidecar.go). The run
// goroutine then persists a terminal 'cancelled' status. Returns true if the
// run was found in-flight in THIS process; false means it already finished (or
// is unknown here), and the caller should treat that as a no-op / conflict.
func (o *Orchestrator) CancelRun(shortID string) bool {
	o.inFlightMu.Lock()
	cancel, ok := o.inFlight[shortID]
	o.inFlightMu.Unlock()
	if ok {
		cancel()
	}
	return ok
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

// SmokeTest runs the diagnostic round-trip via the orchestrator's existing
// runner. Bypasses the concurrency semaphore (diagnostic, not a real run)
// and never touches agent_runs / api_keys. Used by the admin Settings
// "Test connection" button and the `breadbox agent test` CLI alike.
func (o *Orchestrator) SmokeTest(ctx context.Context) (*agent.SmokeResult, error) {
	binaryPath := appconfig.String(ctx, o.svc.Queries, appconfig.KeyAgentRuntimePath, "")
	return agent.SmokeTest(ctx, o.svc.Queries, o.encKey, o.runner, binaryPath)
}

// RunOverrides carries the optional per-call mutations the operator may
// apply to a manual run. Both fields default to "use the def value"; set
// them sparingly:
//
//   - PromptPrefix prepends to def.Prompt for this fire only (iter-23).
//   - PromptOverride replaces def.Prompt entirely for this fire (iter-45),
//     enabling the "Test this prompt" flow on the edit form. Takes
//     precedence over PromptPrefix when both are set — the override is
//     the full prompt the operator wants to test, and prepending a prefix
//     wouldn't match the typed value any longer.
//
// Cron + webhook paths always pass the zero value; only manual runs from
// the admin UI / CLI / HTTP API construct non-empty overrides.
type RunOverrides struct {
	PromptPrefix   string
	PromptOverride string
}

// RunNow executes one agent run synchronously, for "run now" requests.
// Returns ErrConcurrencyLocked WITHOUT creating a run row when the semaphore
// is full — the caller (HTTP handler) maps to 503 and the user retries.
// Returns the resulting agent_runs row on any other outcome (success or error).
//
// promptPrefix is the operator-supplied per-run prefix that gets prepended to
// the agent's stored prompt for this fire only. Empty string disables the
// prefix; cron callers always pass "". For a full prompt override (the
// iter-45 "Test this prompt" flow), use RunNowWith instead.
func (o *Orchestrator) RunNow(ctx context.Context, def *AgentDefinitionResponse, promptPrefix string) (*AgentRunResponse, error) {
	return o.RunNowWith(ctx, def, RunOverrides{PromptPrefix: promptPrefix})
}

// RunNowWith is the full-shape variant of RunNow that accepts the operator's
// RunOverrides struct. Used by the iter-45 "Test this prompt" button on
// the agent edit page to dry-fire an unsaved prompt without mutating the
// stored definition.
func (o *Orchestrator) RunNowWith(ctx context.Context, def *AgentDefinitionResponse, ov RunOverrides) (*AgentRunResponse, error) {
	if err := o.checkHouseholdCeiling(ctx); err != nil {
		return nil, err
	}
	if err := o.sem.Acquire(ctx); err != nil {
		return nil, err
	}
	defer o.sem.Release()
	return o.runLocked(ctx, def, "manual", ov)
}

// FireSyncCompleteAgents dispatches a webhook-triggered run for every
// agent with trigger_on_sync_complete=true. A definition that already ran
// (non-skipped) within PostSyncDebounceWindow is silently skipped here —
// row-less, since a coalesced trigger isn't a "missed" run.
func (o *Orchestrator) FireSyncCompleteAgents(ctx context.Context) {
	// Use a fresh context for the lookup — the incoming ctx is the sync
	// engine's, and a cancelled webhook request or timed-out sync would
	// silently no-op the entire webhook trigger here (audit HIGH #3 from
	// iter-32). Per-dispatched goroutine below ALSO uses a fresh ctx for
	// the same reason. 30s is plenty for the unindexed-by-default lookup
	// even with a few dozen agents.
	lookupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	defs, err := o.svc.ListAgentDefinitionsForSyncWebhook(lookupCtx)
	if err != nil {
		o.logger.Warn("orchestrator: list sync-webhook agents failed", "error", err)
		return
	}
	if len(defs) == 0 {
		return
	}
	o.logger.Info("orchestrator: dispatching sync-webhook agents",
		"agent_count", len(defs))
	debounceSince := time.Now().Add(-PostSyncDebounceWindow)
	for i := range defs {
		def := defs[i] // capture range value before goroutine
		// Debounce: skip a definition that already ran recently. Fail open
		// — an EXISTS query error must not suppress the run.
		if recent, derr := o.svc.RecentRunExistsForDefinition(lookupCtx, def.ID, debounceSince); derr == nil && recent {
			o.logger.Info("orchestrator: debouncing sync-webhook agent (ran within window)",
				"agent", def.Slug, "window", PostSyncDebounceWindow.String())
			continue
		}
		go func() {
			// New ctx with timeout so a stuck run doesn't outlive shutdown.
			runCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			if _, rerr := o.RunOrSkip(runCtx, &def, "webhook"); rerr != nil {
				o.logger.Warn("orchestrator: sync-webhook run failed",
					"agent", def.Slug, "error", rerr)
			}
		}()
	}
}

// RunOrSkip is the entry point for scheduled (cron) runs. Always leaves
// a row in agent_runs — either completed/errored, or 'skipped' when the
// semaphore was full. Returns ErrConcurrencyLocked alongside the skipped
// row so the scheduler can log appropriately.
func (o *Orchestrator) RunOrSkip(ctx context.Context, def *AgentDefinitionResponse, trigger string) (*AgentRunResponse, error) {
	// Household ceiling first — a cron fire that's over budget leaves a
	// 'skipped' row (reason = the ceiling message) so the miss is visible.
	if cerr := o.checkHouseholdCeiling(ctx); cerr != nil {
		defUUID, perr := pgconv.ParseUUID(def.ID)
		if perr != nil {
			return nil, fmt.Errorf("orchestrator: parse def id: %w", perr)
		}
		runRow, crerr := o.svc.CreateAgentRunDB(ctx, defUUID, trigger, def.Model)
		if crerr != nil {
			return nil, fmt.Errorf("orchestrator: create ceiling-skipped run row: %w (ceiling: %v)", crerr, cerr)
		}
		_ = o.svc.MarkAgentRunSkippedDB(ctx, runRow.ID, cerr.Error())
		resp := AgentRunFromRow(runRow)
		resp.Status = "skipped"
		return &resp, cerr
	}
	if err := o.sem.Acquire(ctx); err != nil {
		// Leave a 'skipped' row so the run history shows the missed fire.
		defUUID, perr := pgconv.ParseUUID(def.ID)
		if perr != nil {
			return nil, fmt.Errorf("orchestrator: parse def id: %w", perr)
		}
		runRow, crerr := o.svc.CreateAgentRunDB(ctx, defUUID, trigger, def.Model)
		if crerr != nil {
			return nil, fmt.Errorf("orchestrator: create skipped run row: %w (acquire err: %v)", crerr, err)
		}
		_ = o.svc.MarkAgentRunSkippedDB(ctx, runRow.ID, "another run was in progress")
		resp := AgentRunFromRow(runRow)
		resp.Status = "skipped"
		return &resp, err
	}
	defer o.sem.Release()
	return o.runLocked(ctx, def, trigger, RunOverrides{})
}

// RunNowAsyncWith is the non-blocking variant of RunNowWith for the admin
// "Run now" button. It does enough work synchronously to fail fast for
// operator-visible mistakes (auth missing, binary missing, concurrency locked),
// then spawns a goroutine for the sidecar invocation + completion + revoke.
// The caller gets back the in_progress agent_runs row right away so the UI
// can close the dialog and stream the live transcript.
//
// Returned errors mirror RunNowWith's: ErrConcurrencyLocked, ErrAuthNotConfigured,
// ErrBinaryNotFound, or a generic spec-assembly failure. Any post-spawn
// failure (sidecar crash, model error, hit-cap) lands on the agent_runs row
// instead — the HTTP request has already returned 201.
func (o *Orchestrator) RunNowAsyncWith(ctx context.Context, def *AgentDefinitionResponse, ov RunOverrides) (*AgentRunResponse, error) {
	// Preflight: surface the obvious operator misconfigurations (no token,
	// no binary) as a typed error here so the HTTP handler can map to 422
	// instead of returning an in_progress row that's destined to fail.
	binaryPath := appconfig.String(ctx, o.svc.Queries, appconfig.KeyAgentRuntimePath, "")
	if _, err := agent.LocateBinary(binaryPath); err != nil {
		return nil, err
	}
	if err := o.checkHouseholdCeiling(ctx); err != nil {
		return nil, err
	}

	if err := o.sem.Acquire(ctx); err != nil {
		return nil, err
	}

	resp, prepErr, runFn := o.prepareRun(ctx, def, "manual", ov)
	if prepErr != nil {
		// Prep failed before we handed control to the goroutine — release
		// the semaphore inline so we don't deadlock subsequent runs.
		o.sem.Release()
		return resp, prepErr
	}

	// Hand the slow work off. The goroutine owns: sidecar invocation,
	// row completion, semaphore release, key revoke. A fresh context with
	// a generous timeout decouples the run from the HTTP request lifecycle
	// (the admin UI closes the dialog and starts polling the transcript).
	go func() {
		runCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		// Expose this run's cancel to CancelRun for the duration of the work, so
		// the run-detail "Cancel run" button can abort it mid-flight.
		o.registerInFlight(resp.ShortID, cancel)
		defer o.unregisterInFlight(resp.ShortID)
		defer o.sem.Release()
		// Panic recovery — without this a panic from anywhere downstream
		// (sidecar NDJSON parser, slog handler, DB driver) takes the whole
		// `breadbox serve` process down. Log and mark the run errored so
		// the row doesn't lie about its state and operators can see what
		// happened. The semaphore release above is already deferred so the
		// concurrency slot is reclaimed either way.
		defer func() {
			if r := recover(); r != nil {
				o.logger.Error("orchestrator: panic in async run goroutine",
					"agent", def.Slug, "run", resp.ShortID,
					"panic", r, "stack", string(debug.Stack()))
				// Best-effort row update under a fresh ctx. Ignore failure —
				// we're already in disaster recovery and can't surface more.
				recoverCtx, recoverCancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer recoverCancel()
				_ = o.svc.MarkAgentRunErrorDB(recoverCtx, runUUIDFromResp(resp),
					fmt.Sprintf("orchestrator panic: %v", r), "")
			}
		}()
		runFn(runCtx)
	}()
	return resp, nil
}

// runUUIDFromResp extracts the run row's UUID from the response, mirroring
// what o.prepareRun captured but accessible from the goroutine's recover
// path (which doesn't have direct closure-access to runRow).
func runUUIDFromResp(resp *AgentRunResponse) pgtype.UUID {
	u, _ := pgconv.ParseUUID(resp.ID)
	return u
}

// prepareRun does the synchronous prep portion of runLocked and returns a
// closure that does the slow work. Used by RunNowAsyncWith so the HTTP
// request can return as soon as the run row exists.
//
// The returned closure assumes the orchestrator already holds the semaphore;
// it does NOT release it (the caller wraps the goroutine with the release).
func (o *Orchestrator) prepareRun(ctx context.Context, def *AgentDefinitionResponse, trigger string, ov RunOverrides) (*AgentRunResponse, error, func(context.Context)) {
	promptPrefix := ov.PromptPrefix
	defUUID, err := pgconv.ParseUUID(def.ID)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: parse def id: %w", err), nil
	}

	runRow, err := o.svc.CreateAgentRunDB(ctx, defUUID, trigger, def.Model)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: create run row: %w", err), nil
	}
	if promptPrefix != "" {
		if perr := o.svc.SetAgentRunPromptPrefixDB(ctx, runRow.ID, promptPrefix); perr != nil {
			o.logger.Warn("orchestrator: persist prompt prefix failed",
				"agent", def.Slug, "run", runRow.ShortID, "error", perr)
		} else {
			runRow.PromptPrefix = pgtype.Text{String: promptPrefix, Valid: true}
		}
	}
	runResp := AgentRunFromRow(runRow)

	keyResult, err := o.svc.MintRunAPIKey(ctx, def, runResp.ShortID)
	if err != nil {
		o.logger.Warn("orchestrator: mint api key failed",
			"agent", def.Slug, "run", runResp.ShortID, "error", err)
		_ = o.svc.MarkAgentRunErrorDB(ctx, runRow.ID, fmt.Sprintf("mint api key: %v", err), "")
		return &runResp, fmt.Errorf("orchestrator: mint api key: %w", err), nil
	}

	spec, err := o.svc.AssembleJobSpec(ctx, def, &runResp, keyResult.PlaintextKey, o.encKey)
	if err != nil {
		o.logger.Warn("orchestrator: assemble spec failed",
			"agent", def.Slug, "run", runResp.ShortID, "error", err)
		_ = o.svc.MarkAgentRunErrorDB(ctx, runRow.ID, fmt.Sprintf("assemble spec: %v", err), "")
		// Revoke the key we just minted before bailing.
		revokeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = o.svc.RevokeAPIKey(revokeCtx, keyResult.ID)
		return &runResp, fmt.Errorf("orchestrator: assemble spec: %w", err), nil
	}

	switch {
	case ov.PromptOverride != "":
		spec.Prompt = ov.PromptOverride
	case promptPrefix != "":
		spec.Prompt = applyPromptPrefix(promptPrefix, spec.Prompt)
	}

	run := func(runCtx context.Context) {
		defer func() {
			revokeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if rerr := o.svc.RevokeAPIKey(revokeCtx, keyResult.ID); rerr != nil {
				o.logger.Warn("orchestrator: revoke api key failed",
					"agent", def.Slug, "run", runResp.ShortID,
					"key_id", keyResult.ID, "error", rerr)
			}
		}()

		o.logger.Info("orchestrator: run starting",
			"agent", def.Slug, "run", runResp.ShortID, "trigger", trigger, "model", def.Model)

		handler := func(ev agent.Event) error {
			o.logger.Debug("orchestrator: sidecar event",
				"agent", def.Slug, "run", runResp.ShortID, "event_type", ev.Type)
			return nil
		}
		result, runErr := o.runner.Run(runCtx, *spec, handler)

		// Operator cancellation: CancelRun cancels runCtx (context.Canceled),
		// which SIGKILLs the sidecar and surfaces here as a generic run error.
		// Re-tag it as the terminal 'cancelled' status so the row reads as an
		// intentional stop, not a failure. The 30-minute ceiling cancels with
		// context.DeadlineExceeded instead, so this only catches operator (and
		// shutdown) cancellation — timeout keeps its own status.
		if errors.Is(runCtx.Err(), context.Canceled) {
			result.Status = agent.StatusCancelled
			result.Err = nil
			runErr = nil
		}

		// Use a FRESH context for persist + hit-cap updates so a cancelled
		// runCtx (timeout, server shutdown, or future caller passing a
		// shorter parent) doesn't prevent the row from being completed.
		// Mirrors the runLocked path; same reasoning as the deferred revoke.
		persistCtx, persistCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer persistCancel()
		if _, completeErr := o.svc.CompleteAgentRunDB(persistCtx, runRow.ID, result, def.MaxTurns); completeErr != nil {
			o.logger.Error("orchestrator: persist completed run failed",
				"agent", def.Slug, "run", runResp.ShortID, "error", completeErr)
			return
		}
		if cap := capFromRunErr(runErr); cap != "" {
			if _, err := o.svc.SetAgentRunHitCapDB(persistCtx, runRow.ID, cap); err != nil {
				o.logger.Warn("orchestrator: persist hit_cap failed",
					"agent", def.Slug, "run", runResp.ShortID, "cap", cap, "error", err)
			}
		}
		if runErr != nil {
			o.logger.Warn("orchestrator: run finished with error",
				"agent", def.Slug, "run", runResp.ShortID,
				"status", result.Status, "error", runErr)
			return
		}
		o.logger.Info("orchestrator: run finished",
			"agent", def.Slug, "run", runResp.ShortID,
			"status", result.Status, "cost_usd", result.TotalCostUSD,
			"duration_ms", result.DurationMs, "turns", result.TurnCount)
	}
	return &runResp, nil, run
}

// runLocked assumes the caller holds the semaphore.
func (o *Orchestrator) runLocked(ctx context.Context, def *AgentDefinitionResponse, trigger string, ov RunOverrides) (*AgentRunResponse, error) {
	promptPrefix := ov.PromptPrefix
	defUUID, err := pgconv.ParseUUID(def.ID)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: parse def id: %w", err)
	}

	runRow, err := o.svc.CreateAgentRunDB(ctx, defUUID, trigger, def.Model)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: create run row: %w", err)
	}
	if promptPrefix != "" {
		if perr := o.svc.SetAgentRunPromptPrefixDB(ctx, runRow.ID, promptPrefix); perr != nil {
			o.logger.Warn("orchestrator: persist prompt prefix failed",
				"agent", def.Slug, "run", runRow.ShortID, "error", perr)
		} else {
			runRow.PromptPrefix = pgtype.Text{String: promptPrefix, Valid: true}
		}
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
	if err == nil {
		// Override semantics: a full PromptOverride wins outright (the
		// operator is testing a freshly-typed prompt, not annotating the
		// stored one). Otherwise fall through to the iter-23 prefix prepend.
		switch {
		case ov.PromptOverride != "":
			spec.Prompt = ov.PromptOverride
		case promptPrefix != "":
			spec.Prompt = applyPromptPrefix(promptPrefix, spec.Prompt)
		}
	}
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

	// Persist the completion under a FRESH context. The request-scoped ctx may
	// already be cancelled — e.g. when the admin "Run now" still went through
	// the sync path and the exe.dev proxy killed the HTTP request mid-run,
	// or when the bun-compiled sidecar got SIGKILL'd via CommandContext but
	// its orphaned Node child kept running and finished the work anyway
	// (see incident: run RK7U4E06, 2026-05-25). In either case we want the
	// DB write to succeed so the row reflects reality. The async dispatcher
	// already passes a fresh ctx so this is a belt-and-suspenders for any
	// remaining sync callers + future-proofing.
	persistCtx, persistCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer persistCancel()
	completedRow, completeErr := o.svc.CompleteAgentRunDB(persistCtx, runRow.ID, result, def.MaxTurns)
	if completeErr != nil {
		o.logger.Error("orchestrator: persist completed run failed",
			"agent", def.Slug, "run", runResp.ShortID, "error", completeErr)
		if runErr != nil {
			return &runResp, runErr
		}
		return &runResp, completeErr
	}

	// Detect cap exhaustion. The sidecar surfaces it through runErr after
	// having already classified status (max_turns → success, budget → error),
	// so we record the cap separately for the audit trail without rewriting
	// status. Same fresh-ctx reasoning as CompleteAgentRunDB above.
	if cap := capFromRunErr(runErr); cap != "" {
		if capRow, err := o.svc.SetAgentRunHitCapDB(persistCtx, runRow.ID, cap); err == nil {
			completedRow = capRow
		} else {
			o.logger.Warn("orchestrator: persist hit_cap failed",
				"agent", def.Slug, "run", runResp.ShortID, "cap", cap, "error", err)
		}
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

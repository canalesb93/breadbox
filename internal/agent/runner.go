//go:build !lite

package agent

import "context"

// Status values for RunResult.Status.
const (
	StatusSuccess = "success"
	StatusError   = "error"
	StatusTimeout = "timeout"
	StatusSkipped = "skipped"
	// StatusCancelled is an operator-initiated stop mid-run (the run-detail
	// "Cancel run" button). Distinct from error (a failure) and timeout (the
	// run ceiling) — the operator chose to abort, so it reads as intentional.
	StatusCancelled = "cancelled"
)

// RunResult is the structured outcome of one agent run.
// Populated from the sidecar's terminal "result" event; zeroed cost/token
// fields when Status != StatusSuccess.
type RunResult struct {
	Status string

	// Usage / cost (from the SDK's result event)
	TotalCostUSD        float64
	InputTokens         int
	OutputTokens        int
	CacheReadTokens     int
	CacheCreationTokens int
	TurnCount           int
	NumToolCalls        int
	SessionID           string

	// Wall-clock duration measured Go-side.
	DurationMs int64

	// Non-nil when Status != StatusSuccess. Wraps the sidecar's exit reason.
	Err error

	// Absolute path to the NDJSON transcript on disk.
	TranscriptPath string
}

// EventHandler is invoked for each parsed event as the sidecar streams them.
// Returning a non-nil error cancels the run (kills the subprocess).
type EventHandler func(Event) error

// Runner executes one agent job. The canonical implementation is *Sidecar;
// tests inject fakes.
type Runner interface {
	Run(ctx context.Context, spec JobSpec, handler EventHandler) (RunResult, error)
}

// RunnerFunc adapts a plain function to the Runner interface, useful for
// fake runners in tests.
type RunnerFunc func(ctx context.Context, spec JobSpec, handler EventHandler) (RunResult, error)

// Run implements Runner.
func (f RunnerFunc) Run(ctx context.Context, spec JobSpec, handler EventHandler) (RunResult, error) {
	return f(ctx, spec, handler)
}

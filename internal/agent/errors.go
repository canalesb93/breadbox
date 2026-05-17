//go:build !lite

package agent

import "errors"

var (
	// ErrBinaryNotFound is returned when the sidecar binary cannot be located.
	ErrBinaryNotFound = errors.New("agent: breadbox-agent binary not found")

	// ErrBudgetExceeded is returned when the SDK reports budget exhaustion.
	ErrBudgetExceeded = errors.New("agent: run exceeded budget cap")

	// ErrMaxTurnsReached is returned when the SDK stops at the turn ceiling.
	ErrMaxTurnsReached = errors.New("agent: max turns reached")

	// ErrConcurrencyLocked is returned when another run holds the
	// server-wide concurrency mutex.
	ErrConcurrencyLocked = errors.New("agent: another run is already in progress")

	// ErrAuthNotConfigured is returned when neither subscription_token nor
	// anthropic_api_key has been set in app_config.
	ErrAuthNotConfigured = errors.New("agent: authentication not configured")

	// ErrSidecarFailed is the generic wrap for a non-zero sidecar exit
	// when no specific "error" event was emitted.
	ErrSidecarFailed = errors.New("agent: sidecar exited non-zero")
)

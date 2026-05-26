//go:build !lite

package agent

import (
	"errors"
	"fmt"
	"strings"
)

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
	// when no specific "error" event was emitted. RunError wraps this so
	// `errors.Is(err, ErrSidecarFailed)` still works for legacy callers.
	ErrSidecarFailed = errors.New("agent: sidecar exited non-zero")
)

// RunErrorCode is a stable string drawn from the SDK's error-event subtype.
// Orchestrator / admin UI use these to drive retry policy + render
// friendlier messages without parsing free-form text. New codes can be
// added freely — callers must default to RunErrorCodeUnknown for the
// "didn't recognize it" branch.
const (
	// RunErrorCodeAuth — Anthropic rejected the credential (invalid /
	// expired / wrong account). Operator action: rotate the token in
	// Settings → Agents.
	RunErrorCodeAuth = "auth_error"

	// RunErrorCodeAPI — Anthropic API returned an upstream error (5xx,
	// overloaded, rate limited). Often transient; safe to retry.
	RunErrorCodeAPI = "api_error"

	// RunErrorCodeNetwork — DNS / TLS / connection failure reaching
	// Anthropic. Almost always transient.
	RunErrorCodeNetwork = "network_error"

	// RunErrorCodeTool — an MCP tool call failed in a way that the SDK
	// surfaced as a top-level error rather than a tool_result.
	RunErrorCodeTool = "tool_error"

	// RunErrorCodeSpecInvalid — the sidecar rejected the JobSpec (zod
	// validation failed). Indicates a Go/TS schema drift.
	RunErrorCodeSpecInvalid = "spec_invalid"

	// RunErrorCodeInterrupted — sidecar received SIGTERM / SIGINT before
	// the SDK stream completed. Maps to ctx cancellation in practice.
	RunErrorCodeInterrupted = "interrupted"

	// RunErrorCodeUnknown — non-zero exit with no recognizable error
	// event. Inspect stderr / the transcript for context.
	RunErrorCodeUnknown = "unknown"
)

// RunError describes a sidecar failure with a stable code suitable for
// retry policy + admin UI rendering. Wraps ErrSidecarFailed so legacy
// `errors.Is(err, ErrSidecarFailed)` checks keep working.
type RunError struct {
	Code    string // see RunErrorCode* constants
	Message string // human-readable; safe to surface in admin UI
	Stderr  string // sidecar process stderr (truncated by caller); empty when not relevant
}

// Error renders the error in a stable shape: "agent: <code>: <message> [stderr=…]".
func (e *RunError) Error() string {
	var b strings.Builder
	b.WriteString("agent: ")
	b.WriteString(e.Code)
	b.WriteString(": ")
	b.WriteString(e.Message)
	if e.Stderr != "" {
		b.WriteString(" [stderr=")
		b.WriteString(e.Stderr)
		b.WriteString("]")
	}
	return b.String()
}

// Unwrap returns ErrSidecarFailed so existing callers that do
// `errors.Is(err, ErrSidecarFailed)` continue to match.
func (e *RunError) Unwrap() error { return ErrSidecarFailed }

// ClassifyRunError builds a RunError from a sidecar exit by inspecting
// the optional structured error event (lastError) + the process's
// stderr. lastError may be the zero value if no error event was emitted
// before exit.
//
// Heuristics for the no-event branch are deliberately conservative —
// when in doubt, return RunErrorCodeUnknown rather than misclassify.
func ClassifyRunError(lastError ErrorPayload, stderr string) *RunError {
	stderrTrimmed := truncate(stderr, 2_000)

	// Path 1: the sidecar emitted a structured error event with a code.
	if lastError.Code != "" {
		return &RunError{
			Code:    normalizeCode(lastError.Code),
			Message: lastError.Message,
			Stderr:  stderrTrimmed,
		}
	}

	// Path 2: error event with a message but no code — heuristics on the
	// message (which is operator-controlled text from the SDK).
	if lastError.Message != "" {
		code := codeFromMessage(lastError.Message)
		return &RunError{
			Code:    code,
			Message: lastError.Message,
			Stderr:  stderrTrimmed,
		}
	}

	// Path 3: no event at all. Inspect stderr for known patterns; default
	// to unknown.
	code := codeFromMessage(stderr)
	return &RunError{
		Code:    code,
		Message: fmt.Sprintf("sidecar exited non-zero (%s)", code),
		Stderr:  stderrTrimmed,
	}
}

// normalizeCode maps SDK-emitted code strings to the canonical RunErrorCode*
// values. Unrecognized inputs return RunErrorCodeUnknown rather than passing
// through — the admin UI should never see a code outside this allowlist.
func normalizeCode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "auth_error", "auth", "unauthorized", "permission_denied":
		return RunErrorCodeAuth
	case "api_error", "overloaded", "rate_limited", "server_error":
		return RunErrorCodeAPI
	case "network_error", "connection_error", "fetch_failed", "enotfound":
		return RunErrorCodeNetwork
	case "tool_error", "mcp_error":
		return RunErrorCodeTool
	case "spec_invalid", "validation_error":
		return RunErrorCodeSpecInvalid
	case "interrupted", "sigterm", "sigint":
		return RunErrorCodeInterrupted
	default:
		return RunErrorCodeUnknown
	}
}

// codeFromMessage applies cheap substring heuristics to text — sidecar
// stderr or an SDK-supplied message — to classify common failure modes
// without a structured code field. Conservative by design.
func codeFromMessage(msg string) string {
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "unauthorized") ||
		strings.Contains(low, "invalid_api_key") ||
		strings.Contains(low, "invalid api key"):
		return RunErrorCodeAuth
	case strings.Contains(low, "overloaded") ||
		strings.Contains(low, "rate limit") ||
		strings.Contains(low, "rate_limit"):
		return RunErrorCodeAPI
	case strings.Contains(low, "enotfound") ||
		strings.Contains(low, "econnrefused") ||
		strings.Contains(low, "fetch failed") ||
		strings.Contains(low, "dns") ||
		strings.Contains(low, "tls"):
		return RunErrorCodeNetwork
	case strings.Contains(low, "spec parse") ||
		strings.Contains(low, "zod"):
		return RunErrorCodeSpecInvalid
	case strings.Contains(low, "sigterm") || strings.Contains(low, "sigint"):
		return RunErrorCodeInterrupted
	default:
		return RunErrorCodeUnknown
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…(truncated)"
}

//go:build !headless && !lite

package pages

import "strings"

// AgentRunFriendlyError maps known raw error_message strings on an
// agent_run row to operator-friendly text. Returns "" when no mapping
// applies — callers should fall back to the raw message.
//
// Mirrors the shape of internal/sync/errors.go (sync logs) but is a
// separate list because the failure modes differ: the only overlap
// today is the startup orphan-cleanup message, which both subsystems
// write verbatim and both want to surface as "safe to retry".
func AgentRunFriendlyError(raw string) string {
	lower := strings.ToLower(raw)
	for _, m := range AgentRunFriendlyErrorMappings {
		if strings.Contains(lower, m.pattern) {
			return m.message
		}
	}
	return ""
}

type agentRunErrorMapping struct {
	pattern string
	message string
}

var AgentRunFriendlyErrorMappings = []agentRunErrorMapping{
	{
		pattern: "interrupted by server restart",
		message: "Interrupted by a server restart — safe to re-run.",
	},
}

// FilterTranscriptForDisplay drops events that are present in the
// NDJSON file but add no signal to the rendered transcript. Currently
// just `result` events where every numeric field is zero: the SDK
// sometimes emits an init-shaped result envelope before the actual
// usage payload, and rendering both as "Final result" bubbles is
// confusing (the operator sees a row of zeros followed by the real one).
//
// Callers: the initial server render and the /-/agents/runs/{id}/live
// poll endpoint — both feed events through here so the live patch
// matches what the operator already saw on first paint.
func FilterTranscriptForDisplay(events []TranscriptEvent) []TranscriptEvent {
	if len(events) == 0 {
		return events
	}
	out := make([]TranscriptEvent, 0, len(events))
	for _, ev := range events {
		if ev.Type == "result" && resultEventIsEmpty(ev) {
			continue
		}
		out = append(out, ev)
	}
	return out
}

func resultEventIsEmpty(ev TranscriptEvent) bool {
	return ev.CostUSD == 0 &&
		ev.TokensIn == 0 &&
		ev.TokensOut == 0 &&
		ev.CacheRead == 0 &&
		ev.CacheWrite == 0
}

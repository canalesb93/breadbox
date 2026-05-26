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
// NDJSON file but add no signal to the rendered transcript, AND
// enriches tool_result events with the name of the tool_use they're
// answering. Two responsibilities in one pass because both touch
// every event and the live endpoint + the initial render must agree
// on the result either way.
//
// Filtering: `result` events where every numeric field is zero. The
// Claude Agent SDK sometimes emits an init-shaped result envelope
// before the actual usage payload, and rendering both as "Final
// result" bubbles is confusing.
//
// Enrichment: builds an id->name index from tool_use events and
// writes the tool name onto every tool_result whose ToolUseID
// matches. The chat-thread row then reads "tool result —
// query_transactions" instead of an anonymous "tool result".
//
// Callers: the initial server render and the /-/agents/runs/{id}/live
// poll endpoint.
func FilterTranscriptForDisplay(events []TranscriptEvent) []TranscriptEvent {
	if len(events) == 0 {
		return events
	}
	// Pass 1: build the id->name index across the whole slice.
	toolNames := make(map[string]string, 8)
	for _, ev := range events {
		if ev.Type == "tool_use" && ev.ToolUseID != "" && ev.ToolName != "" {
			toolNames[ev.ToolUseID] = ev.ToolName
		}
	}

	// Pass 2: filter + enrich.
	out := make([]TranscriptEvent, 0, len(events))
	for _, ev := range events {
		if ev.Type == "result" && resultEventIsEmpty(ev) {
			continue
		}
		if ev.Type == "tool_result" && ev.ToolName == "" && ev.ToolUseID != "" {
			if name, ok := toolNames[ev.ToolUseID]; ok {
				ev.ToolName = name
			}
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

// ToolUsageEntry is one row in the sidebar's "Tools used" breakdown:
// the stripped tool name (no mcp__breadbox__ prefix) and how many
// times this run invoked it.
type ToolUsageEntry struct {
	Name  string
	Count int
}

// ComputeToolUsage walks the transcript and returns per-tool call
// counts, sorted by count DESC then by name ASC. Drives the sidebar's
// "Tools used" card. We count tool_use events (not tool_result) so a
// run that issued a call but never got a result still shows up — the
// operator probably wants to see the call attempt.
func ComputeToolUsage(events []TranscriptEvent) []ToolUsageEntry {
	if len(events) == 0 {
		return nil
	}
	counts := make(map[string]int, 8)
	for _, ev := range events {
		if ev.Type != "tool_use" {
			continue
		}
		name := stripMCPPrefix(ev.ToolName)
		if name == "" {
			continue
		}
		counts[name]++
	}
	out := make([]ToolUsageEntry, 0, len(counts))
	for name, c := range counts {
		out = append(out, ToolUsageEntry{Name: name, Count: c})
	}
	// Sort: count DESC, then name ASC.
	sortToolUsage(out)
	return out
}

func sortToolUsage(s []ToolUsageEntry) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0; j-- {
			a, b := s[j-1], s[j]
			if a.Count > b.Count || (a.Count == b.Count && a.Name <= b.Name) {
				break
			}
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// stripMCPPrefix is the canonical mirror of the templ's
// agentRunToolDisplay helper, lifted out here so non-templ code (the
// usage rollup) doesn't need to duplicate the constant. Kept private
// because callers should never need to know about the prefix
// convention.
func stripMCPPrefix(name string) string {
	const mcp = "mcp__breadbox__"
	if len(name) > len(mcp) && name[:len(mcp)] == mcp {
		return name[len(mcp):]
	}
	return name
}

// TranscriptHasResultEvent reports whether the transcript contains a
// (non-empty) `result` event — used by the sticky header to decide
// whether to render the "Jump to result" anchor button.
func TranscriptHasResultEvent(events []TranscriptEvent) bool {
	for _, ev := range events {
		if ev.Type == "result" {
			return true
		}
	}
	return false
}

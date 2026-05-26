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

// FilterTranscriptForDisplay reshapes the raw NDJSON event slice into
// the form the run-detail page actually renders:
//
//  1. Drops zero-valued `result` envelopes (the SDK occasionally emits
//     an init-shaped result before the real usage payload).
//  2. Pairs each `tool_use` with its matching `tool_result` (by
//     ToolUseID), folding the result JSON onto the tool_use event
//     and dropping the now-redundant tool_result. The chat-thread row
//     then renders a single expandable showing call → result inline.
//  3. Enriches any *orphan* tool_result (a tool_result whose ToolUseID
//     doesn't match any tool_use in this transcript — rare, but
//     possible if the file is truncated) with its tool name when
//     resolvable.
//
// Callers: the initial server render and the /-/agents/runs/{id}/live
// poll endpoint. Pairing must run on both so the in_progress→success
// transition doesn't visually re-split a previously-paired row.
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

	// Pass 2: find the LATER tool_result for each tool_use ID. We use
	// "later" so a same-turn tool_use+tool_result pair is matched, but
	// a runaway agent that re-uses an ID couldn't fold backward.
	resultByID := make(map[string]TranscriptEvent, len(toolNames))
	resultIndexByID := make(map[string]int, len(toolNames))
	for i, ev := range events {
		if ev.Type != "tool_result" || ev.ToolUseID == "" {
			continue
		}
		// Keep the FIRST tool_result we see for each id — multiple
		// results for the same id should never happen, but defensive.
		if _, exists := resultByID[ev.ToolUseID]; !exists {
			resultByID[ev.ToolUseID] = ev
			resultIndexByID[ev.ToolUseID] = i
		}
	}

	// Pass 3: filter + enrich + pair.
	out := make([]TranscriptEvent, 0, len(events))
	for i, ev := range events {
		if ev.Type == "result" && resultEventIsEmpty(ev) {
			continue
		}
		if ev.Type == "tool_use" && ev.ToolUseID != "" {
			if r, ok := resultByID[ev.ToolUseID]; ok {
				// Fold the result JSON onto the tool_use event. The
				// templ's "tool_use" branch checks ToolResultJSON and
				// renders the result section when set.
				ev.ToolResultJSON = r.ToolResultJSON
				if !r.Timestamp.IsZero() {
					ev.ToolResultAt = r.Timestamp
				}
			}
			out = append(out, ev)
			continue
		}
		if ev.Type == "tool_result" && ev.ToolUseID != "" {
			// If this tool_result is paired with an earlier tool_use,
			// the tool_use branch already folded it in — skip the row.
			if idx, paired := resultIndexByID[ev.ToolUseID]; paired && idx == i {
				// Look for a preceding tool_use with the same ID in `out`
				// — if it's there, we've already emitted the combined
				// row, so this tool_result is redundant.
				if hasEarlierToolUse(events[:i], ev.ToolUseID) {
					continue
				}
			}
			// Orphan tool_result (no preceding tool_use) — enrich name
			// if we know it and keep the row.
			if ev.ToolName == "" {
				if name, ok := toolNames[ev.ToolUseID]; ok {
					ev.ToolName = name
				}
			}
			out = append(out, ev)
			continue
		}
		out = append(out, ev)
	}
	return out
}

// hasEarlierToolUse reports whether any tool_use event with the given
// ID appears in `prior`. Used by the pair-folding pass to decide
// whether a tool_result is now redundant or still an orphan worth
// rendering.
func hasEarlierToolUse(prior []TranscriptEvent, id string) bool {
	for _, ev := range prior {
		if ev.Type == "tool_use" && ev.ToolUseID == id {
			return true
		}
	}
	return false
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

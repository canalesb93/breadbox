//go:build !headless && !lite

package pages

import (
	"time"

	"breadbox/internal/templates/components"
)

// AgentRunRowProps is the one-row shape shared across the runs page,
// per-run detail header, and the design sandbox. The canonical type
// lives in `internal/templates/components` so any caller that wants to
// render an agent run uses the same fields; this alias keeps existing
// references in the admin handler compiling without churn.
type AgentRunRowProps = components.AgentRunRowProps

// AgentRunDetailProps powers the per-run page at /agents/runs/{shortId}.
// SystemPrompt / Prompt are pulled from the agent definition so the
// transcript card can show "this is what the agent was told to do" without
// fetching anything else. PromptPrefix is the operator-supplied per-run
// override (nullable on the row).
type AgentRunDetailProps struct {
	Run AgentRunRowProps

	SystemPrompt string
	Prompt       string
	PromptPrefix string

	// TranscriptPath is the path that was used to read the NDJSON file (or
	// the fallback path attempted when transcript_path is NULL). Shown in
	// the truncated banner so operators can read the file directly.
	TranscriptPath string

	Transcript []TranscriptEvent

	// Truncated is set when the NDJSON file had more than 500 lines; the
	// viewer renders only the first 500 and shows a banner with the file
	// path so operators can grep the rest.
	Truncated bool

	// Error is a non-blocking message (e.g. "transcript file missing on
	// disk"). The page still renders with run metadata + a placeholder.
	Error string

	CSRFToken string
}

// TranscriptEvent is a unified representation of one NDJSON line in the
// SDK transcript. The Go-side parser in agent_runs_page.go fans out by
// SDK event "type" and projects the interesting fields into this shape,
// always preserving the raw JSON in RawJSON so unfamiliar payloads can
// still be inspected.
//
// Server-rendered and deliberately minimal (no turn grouping, no
// search). The point is readability over richness.
type TranscriptEvent struct {
	// Type classifies the event. Recognised values:
	//   "assistant" — model text reply
	//   "user"      — user message (often a tool-result envelope)
	//   "tool_use"  — assistant tool invocation
	//   "tool_result" — tool response coming back
	//   "result"    — final cost/usage summary
	//   "error"     — sidecar/SDK error event
	//   "system"    — system event (cost_cap_hit, etc.)
	//   "raw"       — fallback when JSON shape is unrecognised
	Type string

	// Role is the original SDK "role" when present ("assistant" | "user").
	// Helpful for distinguishing assistant text from user feedback.
	Role string

	// Text is the assistant/user message body extracted from content
	// blocks of type "text".
	Text string

	// ToolName / ToolInputJSON are populated for type=="tool_use".
	// ToolResultJSON is populated for type=="tool_result".
	ToolName       string
	ToolInputJSON  string
	ToolResultJSON string

	// ToolUseID is the SDK-assigned correlation ID. tool_use events
	// carry their own ID; tool_result events carry the ID of the
	// tool_use they're answering. The enrichment pass in
	// FilterTranscriptForDisplay writes the tool's name onto every
	// tool_result whose ToolUseID matches a known tool_use, so the
	// rendered row can read "tool result — query_transactions" instead
	// of an anonymous "tool result".
	ToolUseID string

	// ToolResultAt is the timestamp of the matching tool_result, set
	// only on tool_use events that have been paired with a result by
	// FilterTranscriptForDisplay. The chat-thread row uses it to
	// surface a "→ <time>" caption on the combined call + result row.
	ToolResultAt time.Time

	// CostUSD / token counts are populated for type=="result".
	CostUSD     float64
	TokensIn    int64
	TokensOut   int64
	CacheRead   int64
	CacheWrite  int64

	// RawJSON is the un-parsed original line. Always set so callers can
	// always render something readable when the parser doesn't recognise
	// the shape.
	RawJSON string

	// Timestamp is the SDK "ts" field (Unix millis) converted to time.Time
	// when present; otherwise the zero value.
	Timestamp time.Time
}

//go:build !headless && !lite

package pages

import "time"

// AgentRunsListProps is the view-model for the v1 admin run-history page.
// One templ component (AgentRunsList) handles both modes:
//   - "global": cross-agent run history at /agents/runs.
//   - "agent":  per-agent run history at /agents/{slug}/runs.
//
// The handler in internal/admin/agent_runs_page.go picks the mode based on
// whether chi.URLParam(r, "slug") is set, then fills in the right subset of
// fields (AgentOptions is only used in global mode; AgentSlug/AgentName only
// in agent mode).
type AgentRunsListProps struct {
	// Mode is "global" or "agent". Drives the title, breadcrumb, agent
	// column visibility, and the presence of the agent-filter select.
	Mode string

	// AgentSlug / AgentName are populated only when Mode == "agent".
	AgentSlug string
	AgentName string

	Filters AgentRunsFilterProps
	Rows    []AgentRunRowProps

	// Total is the count of rows returned by the current filter set; we
	// don't have a cheap COUNT(*) at this layer, so callers can pass
	// len(Rows) and rely on "Showing X to Y" + has_more wording.
	Total int

	Limit  int
	Offset int

	// AgentOptions populates the agent-filter dropdown on the global page.
	// Empty for Mode == "agent".
	AgentOptions []AgentRunsAgentOption

	CSRFToken string
}

// AgentRunsFilterProps captures the currently-active URL filters so the
// form re-renders with the same selection on reload.
type AgentRunsFilterProps struct {
	Status    string
	Trigger   string
	HitCap    string
	AgentSlug string
	Start     string
	End       string
}

// AgentRunsAgentOption is one entry in the global-page agent filter.
type AgentRunsAgentOption struct {
	Slug string
	Name string
}

// AgentRunRowProps is one row in the runs table. The handler builds these
// from service.AgentRunResponse / AgentRunWithAgentResponse, expanding the
// pointer fields into plain values (zeroes when nil) so the templ doesn't
// have to nil-check on every cell.
type AgentRunRowProps struct {
	ShortID    string
	AgentSlug  string
	AgentName  string
	Status     string
	Trigger    string
	StartedAt  time.Time
	FinishedAt *time.Time
	DurationMs int64
	CostUSD    float64
	TokensIn   int64
	TokensOut  int64
	HitCap     string
	Turns      int
	Note       string
}

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

	// RefreshSeconds, when > 0, emits a <meta http-equiv="refresh"> tag
	// that auto-reloads the page. Set this for in_progress runs so the
	// transcript fills in as the SDK writes lines.
	RefreshSeconds int
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

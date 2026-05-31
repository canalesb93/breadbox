//go:build !headless && !lite

package pages

import (
	"strings"
	"testing"
)

func TestAgentRunFriendlyError(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		wantEmpty bool
		// substr must appear in the friendly message when wantEmpty is false.
		substr string
	}{
		{
			name:   "opaque claude-code exit (the run Ricardo hit)",
			raw:    "agent: unknown: Claude Code process exited with code 1",
			substr: "expired or invalid Anthropic credential",
		},
		{
			name:   "zero-message stream end",
			raw:    "agent: auth_error: agent SDK stream ended without yielding any messages — likely an invalid auth credential",
			substr: "no output",
		},
		{
			name:   "typed auth_error",
			raw:    "agent: auth_error: 401 unauthorized [stderr=…]",
			substr: "rejected the credential",
		},
		{
			name:   "typed api_error",
			raw:    "agent: api_error: overloaded",
			substr: "transient",
		},
		{
			name:   "typed network_error",
			raw:    "agent: network_error: fetch failed: ENOTFOUND",
			substr: "outbound connectivity",
		},
		{
			name:   "spec_invalid is a bug",
			raw:    "agent: spec_invalid: zod parse failed",
			substr: "Breadbox bug",
		},
		{
			name:   "missing runner binary",
			raw:    "agent: breadbox-agent binary not found",
			substr: "isn't installed",
		},
		{
			name:   "concurrency skip",
			raw:    "agent: another run is already in progress",
			substr: "another run was already in progress",
		},
		{
			name:   "budget cap",
			raw:    "agent: run exceeded budget cap",
			substr: "budget cap",
		},
		{
			name:   "server restart still mapped",
			raw:    "run interrupted by server restart",
			substr: "safe to re-run",
		},
		{
			name:      "genuinely unknown falls through to raw",
			raw:       "agent: unknown: some brand-new failure mode nobody mapped",
			wantEmpty: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := AgentRunFriendlyError(c.raw)
			if c.wantEmpty {
				if got != "" {
					t.Errorf("expected no friendly mapping, got %q", got)
				}
				return
			}
			if got == "" {
				t.Fatalf("expected a friendly mapping for %q, got empty", c.raw)
			}
			if !strings.Contains(strings.ToLower(got), strings.ToLower(c.substr)) {
				t.Errorf("friendly message %q does not contain %q", got, c.substr)
			}
		})
	}
}

func TestFilterTranscriptForDisplay_DropsEmptyResult(t *testing.T) {
	in := []TranscriptEvent{
		{Type: "assistant", Text: "hello"},
		{Type: "result"}, // zero everywhere → drop
		{Type: "assistant", Text: "world"},
		{Type: "result", CostUSD: 0.05, TokensIn: 12, TokensOut: 4},
	}
	out := FilterTranscriptForDisplay(in)
	if len(out) != 3 {
		t.Fatalf("expected 3 events after filter, got %d: %+v", len(out), out)
	}
	if out[0].Type != "assistant" || out[0].Text != "hello" {
		t.Errorf("expected first assistant event preserved, got %+v", out[0])
	}
	if out[1].Type != "assistant" || out[1].Text != "world" {
		t.Errorf("expected second assistant event preserved, got %+v", out[1])
	}
	if out[2].Type != "result" || out[2].CostUSD == 0 {
		t.Errorf("expected non-empty result preserved, got %+v", out[2])
	}
}

func TestFilterTranscriptForDisplay_KeepsNonResult(t *testing.T) {
	in := []TranscriptEvent{
		{Type: "tool_use", ToolName: "x"},
		{Type: "tool_result"},
		{Type: "error", Text: "oops"},
	}
	out := FilterTranscriptForDisplay(in)
	if len(out) != len(in) {
		t.Fatalf("expected no filtering on non-result events, got %d / %d", len(out), len(in))
	}
}

func TestFilterTranscriptForDisplay_KeepsResultWithJustCacheData(t *testing.T) {
	// A result event that only carries cache stats (no cost, no input/output
	// tokens) is still meaningful — surface it.
	in := []TranscriptEvent{{Type: "result", CacheRead: 100}}
	out := FilterTranscriptForDisplay(in)
	if len(out) != 1 {
		t.Fatalf("expected result with cache_read preserved, got %+v", out)
	}
}

func TestFilterTranscriptForDisplay_PairFoldDropsRedundantResultRows(t *testing.T) {
	// Same shape as before, but now we expect the tool_use rows to
	// absorb their matching tool_result and drop the redundant rows.
	in := []TranscriptEvent{
		{Type: "tool_use", ToolUseID: "toolu_a", ToolName: "query_transactions"},
		{Type: "tool_use", ToolUseID: "toolu_b", ToolName: "list_categories"},
		{Type: "tool_result", ToolUseID: "toolu_a", ToolResultJSON: `{"count": 47}`},
		{Type: "tool_result", ToolUseID: "toolu_b", ToolResultJSON: `[]`},
	}
	out := FilterTranscriptForDisplay(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 paired events, got %d: %+v", len(out), out)
	}
	if out[0].ToolName != "query_transactions" || out[0].ToolResultJSON != `{"count": 47}` {
		t.Errorf("expected first row to fold in tool_result for toolu_a, got %+v", out[0])
	}
	if out[1].ToolName != "list_categories" || out[1].ToolResultJSON != `[]` {
		t.Errorf("expected second row to fold in tool_result for toolu_b, got %+v", out[1])
	}
}

func TestComputeToolUsage(t *testing.T) {
	in := []TranscriptEvent{
		{Type: "tool_use", ToolName: "mcp__breadbox__query_transactions"},
		{Type: "tool_use", ToolName: "mcp__breadbox__query_transactions"},
		{Type: "tool_use", ToolName: "mcp__breadbox__list_categories"},
		{Type: "tool_result", ToolName: "mcp__breadbox__query_transactions"}, // not counted
		{Type: "assistant", Text: "hi"},
		{Type: "tool_use", ToolName: "Bash"}, // non-mcp tool, kept as-is
	}
	got := ComputeToolUsage(in)
	if len(got) != 3 {
		t.Fatalf("expected 3 unique tools, got %d: %+v", len(got), got)
	}
	// First entry: query_transactions, 2 calls.
	if got[0].Name != "query_transactions" || got[0].Count != 2 {
		t.Errorf("expected query_transactions=2 first, got %+v", got[0])
	}
	// Tie-break — name ASC among count=1 entries: Bash < list_categories.
	if got[1].Name != "Bash" || got[1].Count != 1 {
		t.Errorf("expected Bash=1 second, got %+v", got[1])
	}
	if got[2].Name != "list_categories" || got[2].Count != 1 {
		t.Errorf("expected list_categories=1 third, got %+v", got[2])
	}
}

func TestFilterTranscriptForDisplay_PairsToolUseWithToolResult(t *testing.T) {
	in := []TranscriptEvent{
		{Type: "tool_use", ToolUseID: "toolu_a", ToolName: "query_transactions", ToolInputJSON: `{"limit":10}`},
		{Type: "tool_use", ToolUseID: "toolu_b", ToolName: "list_categories", ToolInputJSON: `{}`},
		{Type: "tool_result", ToolUseID: "toolu_a", ToolResultJSON: `{"count":47}`},
		{Type: "tool_result", ToolUseID: "toolu_b", ToolResultJSON: `[]`},
	}
	out := FilterTranscriptForDisplay(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 paired tool_use rows, got %d: %+v", len(out), out)
	}
	if out[0].Type != "tool_use" || out[0].ToolName != "query_transactions" || out[0].ToolResultJSON != `{"count":47}` {
		t.Errorf("first row: expected paired query_transactions, got %+v", out[0])
	}
	if out[1].Type != "tool_use" || out[1].ToolName != "list_categories" || out[1].ToolResultJSON != `[]` {
		t.Errorf("second row: expected paired list_categories, got %+v", out[1])
	}
}

func TestFilterTranscriptForDisplay_KeepsOrphanToolUseUnpaired(t *testing.T) {
	in := []TranscriptEvent{
		{Type: "tool_use", ToolUseID: "toolu_pending", ToolName: "query_transactions", ToolInputJSON: `{"limit":10}`},
	}
	out := FilterTranscriptForDisplay(in)
	if len(out) != 1 {
		t.Fatalf("expected 1 row, got %d", len(out))
	}
	if out[0].ToolResultJSON != "" {
		t.Errorf("orphan tool_use should not get a result fold-in, got %q", out[0].ToolResultJSON)
	}
}

func TestTranscriptHasResultEvent(t *testing.T) {
	cases := []struct {
		name string
		in   []TranscriptEvent
		want bool
	}{
		{"empty", nil, false},
		{"no result", []TranscriptEvent{{Type: "assistant"}, {Type: "tool_use"}}, false},
		{"has result", []TranscriptEvent{{Type: "assistant"}, {Type: "result", CostUSD: 0.01}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := TranscriptHasResultEvent(c.in); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestFilterTranscriptForDisplay_LeavesOrphanToolResultAlone(t *testing.T) {
	// A tool_result whose ToolUseID has no matching tool_use should keep
	// an empty ToolName. The renderer drops the name pill in that case.
	in := []TranscriptEvent{
		{Type: "tool_result", ToolUseID: "toolu_unknown"},
	}
	out := FilterTranscriptForDisplay(in)
	if len(out) != 1 {
		t.Fatalf("expected 1 event, got %d", len(out))
	}
	if out[0].ToolName != "" {
		t.Errorf("expected empty ToolName for orphan tool_result, got %q", out[0].ToolName)
	}
}

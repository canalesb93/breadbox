//go:build !headless && !lite

package pages

import "testing"

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

func TestFilterTranscriptForDisplay_EnrichesToolResultWithName(t *testing.T) {
	in := []TranscriptEvent{
		{Type: "tool_use", ToolUseID: "toolu_a", ToolName: "query_transactions"},
		{Type: "tool_use", ToolUseID: "toolu_b", ToolName: "list_categories"},
		{Type: "tool_result", ToolUseID: "toolu_a", ToolResultJSON: `{"count": 47}`},
		{Type: "tool_result", ToolUseID: "toolu_b", ToolResultJSON: `[]`},
	}
	out := FilterTranscriptForDisplay(in)
	if len(out) != 4 {
		t.Fatalf("expected 4 events, got %d", len(out))
	}
	if out[2].ToolName != "query_transactions" {
		t.Errorf("expected first tool_result enriched with query_transactions, got %q", out[2].ToolName)
	}
	if out[3].ToolName != "list_categories" {
		t.Errorf("expected second tool_result enriched with list_categories, got %q", out[3].ToolName)
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

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

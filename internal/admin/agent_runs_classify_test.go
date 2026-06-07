//go:build !headless && !lite

package admin

import (
	"encoding/json"
	"strings"
	"testing"
)

// The SDK's first transcript line is a type:"system" / subtype:"init"
// event that carries session metadata but no human "message". It used
// to render as an empty "System" row; classifyTranscriptEvent now
// summarizes the metadata instead.
func TestClassifySystemInitEvent(t *testing.T) {
	raw := `{"type":"system","ts":1717000000000,"data":{` +
		`"type":"system","subtype":"init","model":"claude-opus-4-8",` +
		`"tools":["Read","Bash","Grep"],` +
		`"mcp_servers":[{"name":"breadbox","status":"connected"}],` +
		`"cwd":"/app","permissionMode":"dontAsk"}}`

	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	ev := classifyTranscriptEvent(obj, raw)
	if ev.Type != "system" {
		t.Fatalf("Type = %q, want system", ev.Type)
	}
	if ev.Text == "" {
		t.Fatal("init event rendered an empty System body (the bug)")
	}
	for _, want := range []string{"claude-opus-4-8", "3 tools", "breadbox (connected)"} {
		if !strings.Contains(ev.Text, want) {
			t.Errorf("summary %q missing %q", ev.Text, want)
		}
	}
}

// A cost_cap_hit event carries a real message; it must still win over
// the init branch.
func TestClassifySystemMessageEvent(t *testing.T) {
	raw := `{"type":"cost_cap_hit","data":{"message":"Budget exceeded"}}`
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ev := classifyTranscriptEvent(obj, raw)
	if ev.Type != "system" || ev.Text != "Budget exceeded" {
		t.Fatalf("got Type=%q Text=%q, want system / Budget exceeded", ev.Type, ev.Text)
	}
}

// An init event with no recognisable metadata still gets a non-empty
// fallback label rather than a blank row.
func TestClassifySystemInitFallback(t *testing.T) {
	raw := `{"type":"system","data":{"subtype":"init"}}`
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ev := classifyTranscriptEvent(obj, raw)
	if ev.Text != "Session initialized" {
		t.Fatalf("fallback Text = %q, want Session initialized", ev.Text)
	}
}

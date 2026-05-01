package mcp

import (
	"bytes"
	"encoding/json"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestBuildServerAllTools verifies that all MCP tool input schemas are valid.
// This catches invalid jsonschema struct tags (e.g., "enum=..." syntax) that
// cause panics in google/jsonschema-go when AddTool infers the schema.
func TestBuildServerAllTools(t *testing.T) {
	// NewMCPServer with nil service — we only need tool registration, not execution.
	s := NewMCPServer(nil, "test")

	// BuildServer with read_write + full_access so ALL tools (read + write) are registered.
	// If any tool's input struct has an invalid jsonschema tag, AddTool will panic.
	server := s.BuildServer(MCPServerConfig{
		Mode:        "read_write",
		APIKeyScope: "full_access",
	})
	if server == nil {
		t.Fatal("BuildServer returned nil")
	}
}

func TestCompactIDs_SimpleObject(t *testing.T) {
	input := map[string]any{
		"id":       "9466ab98-0de2-41a0-847b-6740bb519cdc",
		"short_id": "k7Xm9pQ2",
		"name":     "Alice",
	}
	compactIDs(input)
	if input["id"] != "k7Xm9pQ2" {
		t.Fatalf("expected id to be short_id value, got %v", input["id"])
	}
	if _, exists := input["short_id"]; exists {
		t.Fatal("expected short_id field to be removed")
	}
	if input["name"] != "Alice" {
		t.Fatal("expected other fields to be preserved")
	}
}

func TestCompactIDs_NestedObjects(t *testing.T) {
	input := map[string]any{
		"id":       "uuid-1",
		"short_id": "aaaa1111",
		"category": map[string]any{
			"id":       "uuid-2",
			"short_id": "bbbb2222",
			"slug":     "food",
		},
	}
	compactIDs(input)
	if input["id"] != "aaaa1111" {
		t.Fatalf("expected top-level id compacted, got %v", input["id"])
	}
	cat := input["category"].(map[string]any)
	if cat["id"] != "bbbb2222" {
		t.Fatalf("expected nested id compacted, got %v", cat["id"])
	}
	if _, exists := cat["short_id"]; exists {
		t.Fatal("expected nested short_id removed")
	}
}

func TestCompactIDs_Array(t *testing.T) {
	input := []any{
		map[string]any{"id": "uuid-1", "short_id": "aaaa1111"},
		map[string]any{"id": "uuid-2", "short_id": "bbbb2222"},
	}
	compactIDs(input)
	for i, item := range input {
		m := item.(map[string]any)
		if _, exists := m["short_id"]; exists {
			t.Fatalf("item %d: expected short_id removed", i)
		}
	}
	if input[0].(map[string]any)["id"] != "aaaa1111" {
		t.Fatal("expected first item id compacted")
	}
}

func TestCompactIDs_NoShortID(t *testing.T) {
	// Objects without short_id should be untouched
	input := map[string]any{
		"id":   "uuid-only",
		"name": "test",
	}
	compactIDs(input)
	if input["id"] != "uuid-only" {
		t.Fatalf("expected id unchanged when no short_id, got %v", input["id"])
	}
}

// TestToolRegistryScopeContract locks the tool-count carving against
// accidental future regressions. The MCP overhaul (#997) collapsed the tool
// surface; this test guards that BuildServer-time filtering keeps read_only
// API keys to the read-classified subset and full_access keeps everything.
// A regression that flips a tool's classification (e.g. accidentally marking
// update_transactions as ToolRead) would show up here as a count mismatch
// long before it surfaced as a security issue in production.
func TestToolRegistryScopeContract(t *testing.T) {
	s := NewMCPServer(nil, "test")
	defs := s.AllToolDefs()

	var reads, writes int
	readNames := map[string]struct{}{}
	for _, td := range defs {
		switch td.Classification {
		case ToolRead:
			reads++
			readNames[td.Tool.Name] = struct{}{}
		case ToolWrite:
			writes++
		default:
			t.Errorf("tool %q: unexpected classification %q", td.Tool.Name, td.Classification)
		}
	}

	// Anchor the explicit canonical set for read tools — this is the surface
	// area read-only API keys are allowed to exercise. Includes the seven
	// reference-data mirrors (get_overview / list_* / get_sync_status) that
	// shadow the bounded reference resources for clients without resources
	// support — see tools_reads.go.
	wantReads := []string{
		"query_transactions",
		"count_transactions",
		"transaction_summary",
		"list_annotations",
		"preview_rule",
		"get_overview",
		"list_accounts",
		"list_categories",
		"list_users",
		"list_tags",
		"get_sync_status",
		"list_transaction_rules",
	}
	if len(readNames) != len(wantReads) {
		t.Errorf("read tool count = %d, want %d (got %v)", len(readNames), len(wantReads), readNames)
	}
	for _, name := range wantReads {
		if _, ok := readNames[name]; !ok {
			t.Errorf("read tool %q missing from registry", name)
		}
	}

	// Total must equal reads + writes (no third classification leaked in).
	if got := reads + writes; got != len(defs) {
		t.Errorf("classification accounting drift: reads=%d writes=%d total=%d", reads, writes, len(defs))
	}

	// Simulate BuildServer's filter for each scope and pin the visible count.
	// read_only keys see read-classified tools only.
	visibleForReadOnly := 0
	for _, td := range defs {
		if td.Classification == ToolWrite {
			continue // BuildServer drops these for read_only scope.
		}
		visibleForReadOnly++
	}
	if visibleForReadOnly != len(wantReads) {
		t.Errorf("read_only scope visible tools = %d, want %d", visibleForReadOnly, len(wantReads))
	}
	// full_access keys see the entire registry.
	if len(defs) != reads+writes {
		t.Errorf("full_access scope visible tools = %d, want %d (entire registry)", len(defs), reads+writes)
	}
}

// TestToolAnnotationsContract pins the per-tool MCP metadata that hosts use
// to render the connector picker (Title) and decide whether to confirm a
// call (Annotations.DestructiveHint). A registration that forgets to set
// Title would render as the tool's snake_case name in Claude.ai's Settings →
// Connectors → Configure tools list; one that drops DestructiveHint=true
// from a delete_* tool would let the host invoke it without prompting.
//
// Asserts:
//   - every tool has a non-empty Title.
//   - every read tool has Annotations.ReadOnlyHint=true.
//   - the canonical destructive set (delete_*) carries
//     DestructiveHint=true. Reversible writes default to false.
func TestToolAnnotationsContract(t *testing.T) {
	s := NewMCPServer(nil, "test")
	defs := s.AllToolDefs()

	wantDestructive := map[string]bool{
		"delete_tag":              true,
		"delete_transaction_rule": true,
	}

	for _, td := range defs {
		name := td.Tool.Name
		if td.Tool.Title == "" {
			t.Errorf("tool %q: missing Title (would render as snake_case in connector picker)", name)
		}
		if td.Tool.Annotations == nil {
			t.Errorf("tool %q: missing Annotations", name)
			continue
		}
		ann := td.Tool.Annotations

		// Read tools must advertise ReadOnlyHint=true so hosts that suppress
		// confirmation prompts based on it can flow read calls through
		// without friction.
		if td.Classification == ToolRead && !ann.ReadOnlyHint {
			t.Errorf("tool %q: ToolRead must set Annotations.ReadOnlyHint=true", name)
		}

		// Destructive contract: the canonical destructive set must opt in;
		// every other write must explicitly opt out (DestructiveHint=false)
		// so hosts don't fall back to the spec's true default.
		if td.Classification == ToolWrite && name != "create_session" {
			want := wantDestructive[name]
			if ann.DestructiveHint == nil {
				t.Errorf("tool %q: write tool must set DestructiveHint explicitly", name)
				continue
			}
			if got := *ann.DestructiveHint; got != want {
				t.Errorf("tool %q: DestructiveHint = %v, want %v", name, got, want)
			}
		}
	}
}

// TestJSONResult_StructuredContent locks the dual-output contract on
// jsonResult: every tool response carries the JSON in BOTH the TextContent
// block (for backwards compatibility with pre-2025-06-18 clients) AND the
// StructuredContent slot (for newer clients that validate against
// OutputSchema). The two views must be byte-identical after compaction so
// hosts that branch on either field see the same data.
//
// Catches a regression where someone re-orders the unmarshal step or skips
// compaction on one path — agents would silently see different short_id
// values across the two views.
func TestJSONResult_StructuredContent(t *testing.T) {
	type row struct {
		ID      string `json:"id"`
		ShortID string `json:"short_id"`
		Name    string `json:"name"`
	}
	type result struct {
		Rows []row `json:"rows"`
	}

	v := result{Rows: []row{
		{ID: "long-uuid-1", ShortID: "Aa11Bb22", Name: "First"},
		{ID: "long-uuid-2", ShortID: "Cc33Dd44", Name: "Second"},
	}}

	res, _, err := jsonResult(v)
	if err != nil {
		t.Fatalf("jsonResult: %v", err)
	}
	if res.StructuredContent == nil {
		t.Fatal("StructuredContent missing")
	}
	if len(res.Content) != 1 {
		t.Fatalf("Content len = %d, want 1", len(res.Content))
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("Content[0] type = %T, want *TextContent", res.Content[0])
	}

	// Round-trip the structured value back to bytes and compare against the
	// text block. They must be the same JSON — same fields, same compacted
	// short_id value on every row.
	structuredBytes, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured: %v", err)
	}
	if string(structuredBytes) != tc.Text {
		t.Errorf("StructuredContent / TextContent drift:\n  text:       %s\n  structured: %s",
			tc.Text, string(structuredBytes))
	}

	// Both views must show the compacted id (Aa11Bb22), not the long UUID.
	if !bytes.Contains(structuredBytes, []byte(`"id":"Aa11Bb22"`)) {
		t.Errorf("StructuredContent did not compact id: %s", string(structuredBytes))
	}
	if bytes.Contains(structuredBytes, []byte("short_id")) {
		t.Errorf("StructuredContent still carries short_id field: %s", string(structuredBytes))
	}
}

func TestCompactIDs_FullRoundTrip(t *testing.T) {
	type testStruct struct {
		ID      string `json:"id"`
		ShortID string `json:"short_id"`
		Name    string `json:"name"`
	}
	original := testStruct{ID: "long-uuid", ShortID: "Ab12Cd34", Name: "Test"}

	data, _ := json.Marshal(original)
	var raw any
	json.Unmarshal(data, &raw)
	compactIDs(raw)
	result, _ := json.Marshal(raw)

	var out map[string]any
	json.Unmarshal(result, &out)

	if out["id"] != "Ab12Cd34" {
		t.Fatalf("expected compacted id, got %v", out["id"])
	}
	if _, exists := out["short_id"]; exists {
		t.Fatal("expected short_id removed from output")
	}
}

// TestTransportBindingHelpers locks the contract for the transport-bound
// audit session helpers introduced in PR 07. resolveTransportID must always
// return a non-empty string (so every call lands on a row, even stdio with
// no native session id), metaReason must read tools/call._meta.reason
// without panicking on nil paths, and contextWithAuditSession round-trips
// through auditSessionFromContext. A regression that drops the stdio
// fallback would leave stdio tool calls unbound.
func TestTransportBindingHelpers(t *testing.T) {
	s := NewMCPServer(nil, "test")

	// resolveTransportID with nil request → stdio fallback.
	if got := s.resolveTransportID(nil); got == "" {
		t.Error("resolveTransportID(nil): empty fallback id")
	}
	// resolveTransportID with no Session → stdio fallback.
	if got := s.resolveTransportID(&mcpsdk.CallToolRequest{}); got == "" {
		t.Error("resolveTransportID(no-session): empty fallback id")
	}
	// Two NewMCPServer instances should differ on stdio fallback so concurrent
	// stdio invocations don't collide on the same audit row. (shortid is
	// random; we just assert non-empty + uniqueness when generation succeeds.)
	s2 := NewMCPServer(nil, "test2")
	if s.stdioFallbackTransportID == s2.stdioFallbackTransportID {
		// Acceptable only if shortid generation failed and both fell back
		// to "stdio-fallback". Don't fail the test on that path.
		if s.stdioFallbackTransportID != "stdio-fallback" {
			t.Errorf("stdio fallbacks collided: %q", s.stdioFallbackTransportID)
		}
	}

	// metaReason: nil-safe + reads the optional reason key.
	if got := metaReason(nil); got != "" {
		t.Errorf("metaReason(nil) = %q, want \"\"", got)
	}
	emptyReq := &mcpsdk.CallToolRequest{Params: &mcpsdk.CallToolParamsRaw{}}
	if got := metaReason(emptyReq); got != "" {
		t.Errorf("metaReason(no-meta) = %q, want \"\"", got)
	}
	withReasonReq := &mcpsdk.CallToolRequest{Params: &mcpsdk.CallToolParamsRaw{
		Meta: mcpsdk.Meta{"reason": "weekly review"},
	}}
	if got := metaReason(withReasonReq); got != "weekly review" {
		t.Errorf("metaReason(with-meta) = %q, want \"weekly review\"", got)
	}

	// auditSession context round-trip.
	ctx := contextWithAuditSession(t.Context(), "abc12345")
	if got := auditSessionFromContext(ctx); got != "abc12345" {
		t.Errorf("auditSessionFromContext = %q, want \"abc12345\"", got)
	}
	// Empty session id → no-op (don't store empty values on ctx).
	emptyCtx := contextWithAuditSession(t.Context(), "")
	if got := auditSessionFromContext(emptyCtx); got != "" {
		t.Errorf("auditSessionFromContext(empty) = %q, want \"\"", got)
	}
}

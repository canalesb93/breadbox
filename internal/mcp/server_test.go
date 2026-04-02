package mcp

import (
	"encoding/json"
	"testing"
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

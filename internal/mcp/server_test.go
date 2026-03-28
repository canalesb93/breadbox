package mcp

import (
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

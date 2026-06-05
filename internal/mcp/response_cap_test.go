//go:build !lite

package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// bigPayload builds a value whose JSON encoding exceeds n bytes.
func bigPayload(n int) map[string]any {
	return map[string]any{"blob": strings.Repeat("x", n)}
}

func TestJSONResult_ResponseCap(t *testing.T) {
	orig := maxResponseBytes
	t.Cleanup(func() { maxResponseBytes = orig })

	// Cap at 1 KB; a ~5 KB payload must be rejected with RESPONSE_TOO_LARGE.
	maxResponseBytes = 1000
	res, _, err := jsonResult(bigPayload(5000))
	if err != nil {
		t.Fatalf("jsonResult returned a transport error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected IsError for oversized response")
	}
	var payload map[string]string
	if uerr := json.Unmarshal([]byte(textOf(t, res)), &payload); uerr != nil {
		t.Fatalf("decode error payload: %v", uerr)
	}
	if payload["code"] != "RESPONSE_TOO_LARGE" {
		t.Errorf("code = %q, want RESPONSE_TOO_LARGE", payload["code"])
	}

	// A small payload under the cap passes.
	res, _, _ = jsonResult(map[string]any{"ok": true})
	if res.IsError {
		t.Errorf("small payload should not error: %s", textOf(t, res))
	}
}

func TestJSONResult_CapDisabledWhenZero(t *testing.T) {
	orig := maxResponseBytes
	t.Cleanup(func() { maxResponseBytes = orig })

	maxResponseBytes = 0 // disabled
	res, _, _ := jsonResult(bigPayload(50_000))
	if res.IsError {
		t.Errorf("cap=0 must disable the limit, got error: %s", textOf(t, res))
	}
}

func TestResolveMaxResponseBytes(t *testing.T) {
	t.Setenv("BREADBOX_MCP_MAX_RESPONSE_BYTES", "")
	if got := resolveMaxResponseBytes(); got != defaultMaxResponseBytes {
		t.Errorf("empty env → %d, want default %d", got, defaultMaxResponseBytes)
	}

	t.Setenv("BREADBOX_MCP_MAX_RESPONSE_BYTES", "5000")
	if got := resolveMaxResponseBytes(); got != 5000 {
		t.Errorf("env=5000 → %d, want 5000", got)
	}

	t.Setenv("BREADBOX_MCP_MAX_RESPONSE_BYTES", "0")
	if got := resolveMaxResponseBytes(); got != 0 {
		t.Errorf("env=0 → %d, want 0 (disabled)", got)
	}

	t.Setenv("BREADBOX_MCP_MAX_RESPONSE_BYTES", "garbage")
	if got := resolveMaxResponseBytes(); got != defaultMaxResponseBytes {
		t.Errorf("invalid env → %d, want default %d", got, defaultMaxResponseBytes)
	}

	t.Setenv("BREADBOX_MCP_MAX_RESPONSE_BYTES", "-10")
	if got := resolveMaxResponseBytes(); got != defaultMaxResponseBytes {
		t.Errorf("negative env → %d, want default %d (rejected)", got, defaultMaxResponseBytes)
	}
}

// textOf extracts the first text content block from a tool result.
func textOf(t *testing.T, res *mcpsdk.CallToolResult) string {
	t.Helper()
	if res == nil || len(res.Content) == 0 {
		t.Fatal("tool result has no content")
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}
	return tc.Text
}

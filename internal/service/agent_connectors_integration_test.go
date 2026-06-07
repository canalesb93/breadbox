//go:build integration && !lite

package service_test

import (
	"context"
	"strings"
	"testing"

	"breadbox/internal/service"
)

// TestConnectorLibrary_MultiHeaderEnableAssemble exercises the global-library +
// per-workflow-enablement model with multiple custom headers, a note, and the
// JSON import: create with two headers + note → masked read → enable on a
// workflow → AssembleJobSpec mounts both decrypted headers + injects the note →
// blank-value edit carries a header forward → delete drops it → import by JSON.
func TestConnectorLibrary_MultiHeaderEnableAssemble(t *testing.T) {
	svc, _, _ := newService(t)
	svc.EncryptionKey = devEncKey
	ctx := context.Background()

	// 1. Create with two headers + a note.
	view, err := svc.CreateConnector(ctx, service.ConnectorLibraryInput{
		Name:      "gmail",
		URL:       "https://gmail-mcp.example.com/mcp",
		Transport: "http",
		Note:      "Search receipts by sender and amount.",
		Headers: []service.ConnectorHeaderInput{
			{Name: "Authorization", Value: "Bearer super-secret"},
			{Name: "X-Workspace", Value: "home"},
		},
	})
	if err != nil {
		t.Fatalf("create connector: %v", err)
	}
	if view.ShortID == "" || !view.HasSecret || len(view.Headers) != 2 || view.Note == "" {
		t.Fatalf("unexpected create view: %+v", view)
	}
	// The view must never carry header values.
	for _, h := range view.Headers {
		_ = h.Name // ConnectorHeaderView has no value field by design
	}

	// Duplicate name rejected.
	if _, err := svc.CreateConnector(ctx, service.ConnectorLibraryInput{Name: "gmail", URL: "https://x/mcp"}); err == nil {
		t.Fatalf("expected conflict on duplicate connector name")
	}

	// 2. Enable on a workflow.
	def, err := svc.CreateAgentDefinition(ctx, service.CreateAgentDefinitionParams{
		Name:       "Connector agent",
		Slug:       "conn-agent",
		Prompt:     "Look up receipts in Gmail.",
		ToolScope:  "read_write",
		Model:      "claude-opus-4-7",
		MaxTurns:   10,
		Connectors: []string{"gmail"},
	})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	// 3. AssembleJobSpec mounts both headers (decrypted) + injects the note.
	seedSubscriptionAuth(t, svc)
	run := &service.AgentRunResponse{ID: def.ID}
	spec, err := svc.AssembleJobSpec(ctx, def, run, "bb_test_key", devEncKey)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	gmail, ok := spec.MCPServers["gmail"]
	if !ok {
		t.Fatalf("gmail not mounted: %+v", spec.MCPServers)
	}
	if gmail.Headers["Authorization"] != "Bearer super-secret" || gmail.Headers["X-Workspace"] != "home" {
		t.Fatalf("headers not decrypted: %+v", gmail.Headers)
	}
	if !containsStr(spec.AllowedTools, "mcp__gmail") {
		t.Fatalf("allowedTools missing mcp__gmail: %v", spec.AllowedTools)
	}
	if !strings.Contains(spec.SystemPrompt, "Search receipts by sender") {
		t.Fatalf("connector note not injected into system prompt")
	}

	// 4. Edit: keep Authorization (blank value), change X-Workspace.
	if _, err := svc.UpdateConnector(ctx, view.ShortID, service.ConnectorLibraryInput{
		Name:      "gmail",
		URL:       "https://gmail-mcp.example.com/mcp",
		Transport: "http",
		Headers: []service.ConnectorHeaderInput{
			{Name: "Authorization", Value: ""},      // keep
			{Name: "X-Workspace", Value: "work"},    // change
		},
	}); err != nil {
		t.Fatalf("update connector: %v", err)
	}
	spec2, _ := svc.AssembleJobSpec(ctx, def, run, "bb_test_key", devEncKey)
	if got := spec2.MCPServers["gmail"]; got.Headers["Authorization"] != "Bearer super-secret" || got.Headers["X-Workspace"] != "work" {
		t.Fatalf("carry-forward/edit failed: %+v", got.Headers)
	}

	// 5. Delete → assembly no longer mounts it.
	if err := svc.DeleteConnector(ctx, view.ShortID); err != nil {
		t.Fatalf("delete connector: %v", err)
	}
	spec3, _ := svc.AssembleJobSpec(ctx, def, run, "bb_test_key", devEncKey)
	if _, ok := spec3.MCPServers["gmail"]; ok {
		t.Fatalf("deleted connector should not mount")
	}

	// 6. Import by JSON — creates an HTTP connector, skips stdio.
	created, skipped, err := svc.ImportConnectorsJSON(ctx, `{
		"mcpServers": {
			"calendar": { "type": "http", "url": "https://cal/mcp", "headers": { "Authorization": "Bearer cal" } },
			"local": { "command": "npx", "args": ["-y", "x"] }
		}
	}`)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(created) != 1 || created[0].Name != "calendar" {
		t.Fatalf("import created: %+v", created)
	}
	if len(skipped) != 1 || skipped[0] != "local" {
		t.Fatalf("import skipped: %v", skipped)
	}
}

func containsStr(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

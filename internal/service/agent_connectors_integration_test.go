//go:build integration && !lite

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/service"
)

// TestConnectorLibrary_CRUDEnableAssemble exercises the global-library +
// per-workflow-enablement model end-to-end: create a library connector with a
// secret → masked read → enable it on a workflow → AssembleJobSpec mounts the
// decrypted HTTP server → blank-secret edit carries the secret forward →
// delete removes it from assembly.
func TestConnectorLibrary_CRUDEnableAssemble(t *testing.T) {
	svc, _, _ := newService(t)
	svc.EncryptionKey = devEncKey
	ctx := context.Background()

	// 1. Create a library connector with a secret.
	view, err := svc.CreateConnector(ctx, service.ConnectorLibraryInput{
		Name:       "gmail",
		URL:        "https://gmail-mcp.example.com/mcp",
		HeaderName: "Authorization",
		Secret:     "Bearer super-secret-token",
	})
	if err != nil {
		t.Fatalf("create connector: %v", err)
	}
	if view.ShortID == "" || !view.HasSecret || view.Name != "gmail" {
		t.Fatalf("unexpected create view: %+v", view)
	}

	// Duplicate name rejected.
	if _, err := svc.CreateConnector(ctx, service.ConnectorLibraryInput{Name: "gmail", URL: "https://x/mcp"}); err == nil {
		t.Fatalf("expected conflict on duplicate connector name")
	}

	// List returns it, masked.
	list, err := svc.ListConnectors(ctx)
	if err != nil || len(list) != 1 || !list[0].HasSecret {
		t.Fatalf("list connectors: %v (%+v)", err, list)
	}

	// 2. Create a workflow that enables it (+ a non-existent name to prove
	// assembly skips unknown connectors).
	def, err := svc.CreateAgentDefinition(ctx, service.CreateAgentDefinitionParams{
		Name:       "Connector agent",
		Slug:       "conn-agent",
		Prompt:     "Look up receipts in Gmail.",
		ToolScope:  "read_write",
		Model:      "claude-opus-4-7",
		MaxTurns:   10,
		Connectors: []string{"gmail", "ghost"},
	})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if len(def.Connectors) != 2 {
		t.Fatalf("want 2 enabled names, got %+v", def.Connectors)
	}

	// 3. AssembleJobSpec mounts gmail (decrypted) but skips the unknown "ghost".
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
	if gmail.Type != "http" || gmail.Headers["Authorization"] != "Bearer super-secret-token" {
		t.Fatalf("gmail server wrong: %+v", gmail)
	}
	if _, ok := spec.MCPServers["ghost"]; ok {
		t.Fatalf("unknown connector should be skipped")
	}
	if _, ok := spec.MCPServers["breadbox"]; !ok {
		t.Fatalf("breadbox server should still be present")
	}
	if !containsStr(spec.AllowedTools, "mcp__gmail") {
		t.Fatalf("allowedTools missing mcp__gmail: %v", spec.AllowedTools)
	}

	// 4. Edit the connector with a blank secret → carries forward; assembly
	// still decrypts to the original value.
	if _, err := svc.UpdateConnector(ctx, view.ShortID, service.ConnectorLibraryInput{
		Name:       "gmail",
		URL:        "https://gmail-mcp.example.com/v2/mcp",
		HeaderName: "Authorization",
		Secret:     "", // keep
	}); err != nil {
		t.Fatalf("update connector: %v", err)
	}
	spec2, err := svc.AssembleJobSpec(ctx, def, run, "bb_test_key", devEncKey)
	if err != nil {
		t.Fatalf("assemble after edit: %v", err)
	}
	if got := spec2.MCPServers["gmail"]; got.URL != "https://gmail-mcp.example.com/v2/mcp" || got.Headers["Authorization"] != "Bearer super-secret-token" {
		t.Fatalf("carry-forward failed: %+v", got)
	}

	// 5. Delete the connector → assembly no longer mounts it (stale enabled
	// name is harmlessly skipped).
	if err := svc.DeleteConnector(ctx, view.ShortID); err != nil {
		t.Fatalf("delete connector: %v", err)
	}
	spec3, err := svc.AssembleJobSpec(ctx, def, run, "bb_test_key", devEncKey)
	if err != nil {
		t.Fatalf("assemble after delete: %v", err)
	}
	if _, ok := spec3.MCPServers["gmail"]; ok {
		t.Fatalf("deleted connector should not mount")
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

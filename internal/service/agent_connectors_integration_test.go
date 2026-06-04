//go:build integration && !lite

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/service"
)

// TestConnectors_CRUDAndAssembly exercises the full custom-connector path:
// create with a secret → masked read → blank-secret edit carries the secret
// forward → AssembleJobSpec mounts the decrypted HTTP server → clearing the set.
func TestConnectors_CRUDAndAssembly(t *testing.T) {
	svc, _, _ := newService(t)
	svc.EncryptionKey = devEncKey
	ctx := context.Background()

	def, err := svc.CreateAgentDefinition(ctx, service.CreateAgentDefinitionParams{
		Name:      "Connector agent",
		Slug:      "conn-agent",
		Prompt:    "Look up receipts in Gmail to enrich transactions.",
		ToolScope: "read_write",
		Model:     "claude-opus-4-7",
		MaxTurns:  10,
		Connectors: []service.ConnectorInput{{
			Name:       "gmail",
			URL:        "https://gmail-mcp.example.com/mcp",
			HeaderName: "Authorization",
			Secret:     "Bearer super-secret-token",
		}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(def.Connectors) != 1 {
		t.Fatalf("want 1 connector, got %d", len(def.Connectors))
	}
	c := def.Connectors[0]
	if c.Name != "gmail" || c.URL != "https://gmail-mcp.example.com/mcp" || c.HeaderName != "Authorization" {
		t.Fatalf("unexpected connector view: %+v", c)
	}
	if !c.HasSecret {
		t.Fatalf("expected HasSecret=true on create")
	}

	// Read-back is masked (no secret field exists on the view).
	got, err := svc.GetAgentDefinition(ctx, "conn-agent")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Connectors) != 1 || !got.Connectors[0].HasSecret {
		t.Fatalf("read-back connector missing/!HasSecret: %+v", got.Connectors)
	}

	// Edit with a blank secret → the stored secret must carry forward.
	updated, err := svc.UpdateAgentDefinition(ctx, "conn-agent", service.UpdateAgentDefinitionParams{
		Connectors: &[]service.ConnectorInput{{
			Name:       "gmail",
			URL:        "https://gmail-mcp.example.com/mcp",
			HeaderName: "Authorization",
			Secret:     "", // keep existing
		}},
	})
	if err != nil {
		t.Fatalf("update carry-forward: %v", err)
	}
	if len(updated.Connectors) != 1 || !updated.Connectors[0].HasSecret {
		t.Fatalf("carry-forward lost the secret: %+v", updated.Connectors)
	}

	// AssembleJobSpec must mount the connector as an HTTP server with the
	// decrypted header and allow-list its tools.
	seedSubscriptionAuth(t, svc)
	run := &service.AgentRunResponse{ID: updated.ID}
	spec, err := svc.AssembleJobSpec(ctx, updated, run, "bb_test_key", devEncKey)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	gmail, ok := spec.MCPServers["gmail"]
	if !ok {
		t.Fatalf("gmail server not mounted: %+v", spec.MCPServers)
	}
	if gmail.Type != "http" || gmail.URL != "https://gmail-mcp.example.com/mcp" {
		t.Fatalf("unexpected gmail server: %+v", gmail)
	}
	if gmail.Headers["Authorization"] != "Bearer super-secret-token" {
		t.Fatalf("header not decrypted at assembly: %+v", gmail.Headers)
	}
	if _, ok := spec.MCPServers["breadbox"]; !ok {
		t.Fatalf("breadbox server should still be present")
	}
	if !containsStr(spec.AllowedTools, "mcp__gmail") {
		t.Fatalf("allowedTools missing mcp__gmail: %v", spec.AllowedTools)
	}

	// Clearing the set (present-but-empty) removes all connectors.
	cleared, err := svc.UpdateAgentDefinition(ctx, "conn-agent", service.UpdateAgentDefinitionParams{
		Connectors: &[]service.ConnectorInput{},
	})
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if len(cleared.Connectors) != 0 {
		t.Fatalf("expected connectors cleared, got %+v", cleared.Connectors)
	}

	// Omitting connectors on a later edit must leave them untouched (still 0).
	noTouch, err := svc.UpdateAgentDefinition(ctx, "conn-agent", service.UpdateAgentDefinitionParams{})
	if err != nil {
		t.Fatalf("no-touch update: %v", err)
	}
	if len(noTouch.Connectors) != 0 {
		t.Fatalf("omitting connectors changed the set: %+v", noTouch.Connectors)
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

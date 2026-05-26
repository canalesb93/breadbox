//go:build integration && !lite

package service_test

import (
	"context"
	"sync"
	"testing"

	"breadbox/internal/pgconv"
	"breadbox/internal/service"
)

// TestEnsureMCPClientAgentKey_LookupOrCreate verifies the happy path
// — first call inserts the per-client row with actor_type='agent'
// and the expected fingerprint shape, repeat calls return the same
// id, and the row is filtered out of ListAPIKeys.
func TestEnsureMCPClientAgentKey_LookupOrCreate(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	client := service.MCPClientInfo{
		Name:       "claude",
		Title:      "Claude Desktop",
		Version:    "1.4.0",
		WebsiteURL: "https://claude.ai",
	}

	first, err := svc.EnsureMCPClientAgentKey(ctx, client, "stdio")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if first == nil {
		t.Fatal("first call returned nil row")
	}
	if first.ActorType != "agent" {
		t.Fatalf("ActorType = %q, want agent", first.ActorType)
	}
	if !first.ActorName.Valid || first.ActorName.String != "Claude Desktop" {
		t.Fatalf("ActorName = %+v, want Claude Desktop", first.ActorName)
	}
	wantFP := "claude_desktop@claude.ai@stdio"
	if !first.ClientFingerprint.Valid || first.ClientFingerprint.String != wantFP {
		t.Fatalf("ClientFingerprint = %+v, want %q", first.ClientFingerprint, wantFP)
	}

	second, err := svc.EnsureMCPClientAgentKey(ctx, client, "stdio")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	firstID := pgconv.FormatUUID(first.ID)
	secondID := pgconv.FormatUUID(second.ID)
	if secondID != firstID {
		t.Fatalf("second call returned a different row: first=%s second=%s", firstID, secondID)
	}

	// /settings/api-keys must not list this auto-managed row.
	listed, err := svc.ListAPIKeys(ctx)
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	for _, k := range listed {
		if k.ID == firstID {
			t.Fatalf("auto-managed key leaked into ListAPIKeys: %+v", k)
		}
	}
}

// TestEnsureMCPClientAgentKey_ConcurrentInsertRace fires two
// EnsureMCPClientAgentKey calls in parallel for the same fingerprint.
// The partial unique index on api_keys.client_fingerprint guarantees
// at most one row; the losing goroutine must re-read the winner's row
// and both callers must end up with the same id.
func TestEnsureMCPClientAgentKey_ConcurrentInsertRace(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	client := service.MCPClientInfo{
		Name:  "race-tester",
		Title: "Race Tester",
	}

	var wg sync.WaitGroup
	var keyA, keyB string
	wg.Add(2)
	go func() {
		defer wg.Done()
		k, err := svc.EnsureMCPClientAgentKey(ctx, client, "stdio")
		if err == nil && k != nil {
			keyA = pgconv.FormatUUID(k.ID)
		}
	}()
	go func() {
		defer wg.Done()
		k, err := svc.EnsureMCPClientAgentKey(ctx, client, "stdio")
		if err == nil && k != nil {
			keyB = pgconv.FormatUUID(k.ID)
		}
	}()
	wg.Wait()

	if keyA == "" || keyB == "" {
		t.Fatalf("both concurrent calls expected to succeed: a=%q b=%q", keyA, keyB)
	}
	if keyA != keyB {
		t.Fatalf("concurrent calls returned different rows: a=%q b=%q (partial unique index should have collapsed them)", keyA, keyB)
	}
}

// TestEnsureMCPClientAgentKey_FallbackToLocalMCP covers the path
// where clientInfo carries no usable label at all — the helper
// resolves to the relabelled "Local MCP" fallback singleton instead
// of inserting yet another row.
func TestEnsureMCPClientAgentKey_FallbackToLocalMCP(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	row, err := svc.EnsureMCPClientAgentKey(ctx, service.MCPClientInfo{}, "stdio")
	if err != nil {
		t.Fatalf("EnsureMCPClientAgentKey on empty clientInfo: %v", err)
	}
	if row == nil {
		t.Fatal("expected fallback row, got nil")
	}
	if !row.ClientFingerprint.Valid || row.ClientFingerprint.String != service.MCPClientFallbackFingerprint {
		t.Fatalf("ClientFingerprint = %+v, want %q", row.ClientFingerprint, service.MCPClientFallbackFingerprint)
	}
	if !row.ActorName.Valid || row.ActorName.String != service.MCPClientFallbackActorName {
		t.Fatalf("ActorName = %+v, want %q", row.ActorName, service.MCPClientFallbackActorName)
	}
	if row.ActorType != "agent" {
		t.Fatalf("ActorType = %q, want agent (the migration must have relabelled the singleton)", row.ActorType)
	}
}

//go:build integration && !lite

package service_test

import (
	"context"
	"strings"
	"testing"

	"breadbox/internal/service"
)

// TestCreateAPIKey_StoresActorColumns covers the new actor_type/actor_name
// surface added in PR-03 of the CLI/headless sprint. The migration adds the
// columns with a CHECK (actor_type IN ...) constraint; this test verifies
// the service writes them through and that invalid values are rejected at
// the API surface (the service rejects bad values before the DB even sees
// them).
func TestCreateAPIKey_StoresActorColumns(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	cases := []struct {
		name      string
		actorType string
		actorName string
	}{
		// Empty ActorType defaults to "user" — the safe default for
		// human-facing entry points (dashboard, REST). Agent runtime keys
		// must opt in explicitly so the startup CleanupOrphanedAgentApiKeys
		// sweep doesn't reap user keys after the 1-hour grace.
		{"user default", "", ""},
		{"explicit agent", "agent", "bot-alpha"},
		{"user actor", "user", "ricardo"},
		{"system actor", "system", "stdio"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.CreateAPIKey(ctx, service.CreateAPIKeyParams{
				Name:      "test-" + tc.name,
				Scope:     "full_access",
				ActorType: tc.actorType,
				ActorName: tc.actorName,
			})
			if err != nil {
				t.Fatalf("CreateAPIKey: %v", err)
			}
			wantType := tc.actorType
			if wantType == "" {
				wantType = "user"
			}
			if got.ActorType != wantType {
				t.Errorf("ActorType = %q want %q", got.ActorType, wantType)
			}
			if tc.actorName == "" {
				if got.ActorName != nil {
					t.Errorf("ActorName = %v want nil", *got.ActorName)
				}
			} else {
				if got.ActorName == nil || *got.ActorName != tc.actorName {
					t.Errorf("ActorName = %v want %q", got.ActorName, tc.actorName)
				}
			}
			if !strings.HasPrefix(got.PlaintextKey, "bb_") {
				t.Errorf("plaintext key prefix = %q", got.PlaintextKey)
			}
		})
	}
}

// TestListAPIKeys_ExcludesAgentKeys verifies that ListAPIKeys returns only
// user-manageable credentials. Agent machinery — per-run keys minted+revoked
// on every workflow run, and auto-managed MCP-client identity rows — is hidden
// so the mint/revoke churn never appears in /settings/api-keys, the REST list,
// or `breadbox keys list`.
//
// Crucially the filter is STRUCTURAL, not `actor_type <> 'agent'`: the
// actor_type column defaults to 'agent' (migration 20260512061200), so any key
// created before that migration — including legitimate user keys — is stamped
// 'agent'. The legacy-user-key case below is the regression guard: such a key
// must stay visible and revocable. See the ListApiKeys query comment.
func TestListAPIKeys_ExcludesAgentKeys(t *testing.T) {
	svc, _, pool := newService(t)
	ctx := context.Background()

	mk := func(name, actorType string) {
		t.Helper()
		if _, err := svc.CreateAPIKey(ctx, service.CreateAPIKeyParams{
			Name:      name,
			Scope:     "full_access",
			ActorType: actorType,
		}); err != nil {
			t.Fatalf("CreateAPIKey(%s/%s): %v", name, actorType, err)
		}
	}
	mk("user-key", "user")
	mk("system-key", "system")
	mk("agent:reviewer:RUN12345", "agent") // run-key shape → hidden

	// A legitimate user key created BEFORE the actor-columns migration: the
	// DEFAULT 'agent' backfilled it to actor_type='agent', but it has a normal
	// name, no workflow_id, and no client_fingerprint. It must NOT be hidden.
	if _, err := pool.Exec(ctx,
		`INSERT INTO api_keys (name, key_hash, key_prefix, scope, actor_type)
		 VALUES ('legacy-deploy-key', 'legacyhash01', 'bb_legacy01', 'full_access', 'agent')`,
	); err != nil {
		t.Fatalf("insert legacy key: %v", err)
	}

	// An auto-managed MCP-client identity row (client_fingerprint set) → hidden.
	if _, err := pool.Exec(ctx,
		`INSERT INTO api_keys (name, key_hash, key_prefix, scope, actor_type, client_fingerprint)
		 VALUES ('mcp-client:claude@@stdio', 'mcphash01', 'bb_mcp0001', 'full_access', 'agent', 'claude@@stdio')`,
	); err != nil {
		t.Fatalf("insert mcp-client key: %v", err)
	}

	keys, err := svc.ListAPIKeys(ctx)
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}

	present := make(map[string]bool, len(keys))
	for _, k := range keys {
		present[k.Name] = true
	}

	wantVisible := []string{"user-key", "system-key", "legacy-deploy-key"}
	for _, name := range wantVisible {
		if !present[name] {
			t.Errorf("ListAPIKeys hid %q — it should be user-manageable", name)
		}
	}
	wantHidden := []string{"agent:reviewer:RUN12345", "mcp-client:claude@@stdio"}
	for _, name := range wantHidden {
		if present[name] {
			t.Errorf("ListAPIKeys leaked agent machinery %q — should be hidden", name)
		}
	}
}

// TestCreateAPIKey_RejectsBadActorType ensures the service guards the enum
// before the CHECK constraint runs — so callers see a clean error rather
// than a Postgres error.
func TestCreateAPIKey_RejectsBadActorType(t *testing.T) {
	svc, _, _ := newService(t)
	_, err := svc.CreateAPIKey(context.Background(), service.CreateAPIKeyParams{
		Name:      "nope",
		ActorType: "admin",
	})
	if err == nil {
		t.Fatal("expected error for invalid actor_type, got nil")
	}
	if !strings.Contains(err.Error(), "actor_type") {
		t.Errorf("error %v: expected actor_type complaint", err)
	}
}

// TestCheckConstraintEnforced reaches around the service layer to confirm
// the database CHECK constraint is actually live (defense in depth).
func TestCheckConstraintEnforced(t *testing.T) {
	svc, _, pool := newService(t)
	_ = svc
	_, err := pool.Exec(context.Background(), `
		INSERT INTO api_keys (name, key_hash, key_prefix, scope, actor_type)
		VALUES ('bad', 'deadbeef', 'bb_test_x', 'full_access', 'rogue')
	`)
	if err == nil {
		t.Fatal("expected CHECK constraint violation, got nil")
	}
	if !strings.Contains(err.Error(), "check") && !strings.Contains(err.Error(), "actor_type") {
		t.Errorf("error %v: expected CHECK-constraint complaint", err)
	}
}

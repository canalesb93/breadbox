//go:build integration

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
		{"agent default", "", ""},
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
				wantType = "agent"
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

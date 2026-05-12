package service

import (
	"context"
	"testing"

	"breadbox/internal/db"

	"github.com/jackc/pgx/v5/pgtype"
)

// mustUUID builds a pgtype.UUID from a 16-byte literal so tests can stamp
// a deterministic id without spelling out hex. Not exposed because
// ActorFromContext's behavior is identical for any valid UUID.
func mustUUID(byteFill byte) pgtype.UUID {
	var u pgtype.UUID
	for i := range u.Bytes {
		u.Bytes[i] = byteFill
	}
	u.Valid = true
	return u
}

func TestSystemActor(t *testing.T) {
	got := SystemActor()
	if got.Type != "system" {
		t.Errorf("Type = %q, want %q", got.Type, "system")
	}
	if got.ID != "" {
		t.Errorf("ID = %q, want empty", got.ID)
	}
	if got.Name != "Breadbox" {
		t.Errorf("Name = %q, want %q", got.Name, "Breadbox")
	}
}

func TestActorFromContext_Empty(t *testing.T) {
	got := ActorFromContext(context.Background())
	if got != SystemActor() {
		t.Errorf("empty context: got %+v, want SystemActor()", got)
	}
}

func TestActorFromContext_AgentFromAPIKey(t *testing.T) {
	key := &db.ApiKey{
		ID:        mustUUID(0x01),
		Name:      "CI bot",
		KeyPrefix: "bb_test",
		Scope:     "full_access",
		ActorType: "agent",
	}
	ctx := ContextWithAPIKey(context.Background(), key)
	got := ActorFromContext(ctx)

	if got.Type != "agent" {
		t.Errorf("Type = %q, want %q", got.Type, "agent")
	}
	if got.ID == "" {
		t.Errorf("ID is empty; expected a stringified UUID")
	}
	if got.Name != "CI bot" {
		t.Errorf("Name = %q, want %q", got.Name, "CI bot")
	}
}

func TestActorFromContext_UserActorType(t *testing.T) {
	key := &db.ApiKey{
		ID:        mustUUID(0x02),
		Name:      "CLI Bootstrap",
		KeyPrefix: "bb_cli",
		Scope:     "full_access",
		ActorType: "user",
		ActorName: pgtype.Text{String: "ricardo", Valid: true},
	}
	ctx := ContextWithAPIKey(context.Background(), key)
	got := ActorFromContext(ctx)
	if got.Type != "user" {
		t.Errorf("Type = %q want user", got.Type)
	}
	if got.Name != "ricardo" {
		t.Errorf("Name = %q want ricardo (actor_name should win over key.Name)", got.Name)
	}
}

func TestActorFromContext_SystemActorTypeFromKey(t *testing.T) {
	key := &db.ApiKey{
		ID:        mustUUID(0x03),
		Name:      "MCP Stdio",
		KeyPrefix: "bb_stdio_singleton",
		Scope:     "full_access",
		ActorType: "system",
		ActorName: pgtype.Text{String: "stdio", Valid: true},
	}
	ctx := ContextWithAPIKey(context.Background(), key)
	got := ActorFromContext(ctx)
	if got.Type != "system" {
		t.Errorf("Type = %q want system", got.Type)
	}
	if got.Name != "stdio" {
		t.Errorf("Name = %q want stdio", got.Name)
	}
}

func TestActorFromContext_LegacyContextDefaultsToAgent(t *testing.T) {
	ctx := ContextWithAPIKeyLegacy(context.Background(), "key-123", "CI bot")
	got := ActorFromContext(ctx)
	if got.Type != "agent" {
		t.Errorf("Type = %q want agent", got.Type)
	}
	if got.ID != "key-123" {
		t.Errorf("ID = %q want key-123", got.ID)
	}
	if got.Name != "CI bot" {
		t.Errorf("Name = %q want CI bot", got.Name)
	}
}

func TestActorFromContext_BlankIDFallsBackToSystem(t *testing.T) {
	ctx := ContextWithAPIKeyLegacy(context.Background(), "", "ignored")
	got := ActorFromContext(ctx)
	if got != SystemActor() {
		t.Errorf("blank id: got %+v, want SystemActor()", got)
	}
}

func TestContextWithAPIKey_NilIsNoop(t *testing.T) {
	parent := context.Background()
	ctx := ContextWithAPIKey(parent, nil)
	if ActorFromContext(ctx) != SystemActor() {
		t.Error("nil key should fall back to SystemActor")
	}
}

func TestContextWithAPIKey_NameFallback(t *testing.T) {
	// actor_name unset → falls back to key.Name → falls back to key.KeyPrefix.
	key := &db.ApiKey{
		ID:        mustUUID(0x04),
		KeyPrefix: "bb_fallback",
		Scope:     "full_access",
		ActorType: "agent",
	}
	got := ActorFromContext(ContextWithAPIKey(context.Background(), key))
	if got.Name != "bb_fallback" {
		t.Errorf("expected prefix fallback, got Name=%q", got.Name)
	}
}

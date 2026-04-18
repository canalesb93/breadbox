package service

import (
	"context"
	"testing"
)

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

func TestActorFromContext_AgentWithAPIKey(t *testing.T) {
	ctx := ContextWithAPIKey(context.Background(), "key-123", "CI bot")
	got := ActorFromContext(ctx)

	if got.Type != "agent" {
		t.Errorf("Type = %q, want %q", got.Type, "agent")
	}
	if got.ID != "key-123" {
		t.Errorf("ID = %q, want %q", got.ID, "key-123")
	}
	if got.Name != "CI bot" {
		t.Errorf("Name = %q, want %q", got.Name, "CI bot")
	}
}

func TestActorFromContext_BlankIDFallsBackToSystem(t *testing.T) {
	// An empty API key ID should NOT promote the actor to agent — otherwise
	// audit rows lose attribution. Falls back to system instead.
	ctx := ContextWithAPIKey(context.Background(), "", "ignored")
	got := ActorFromContext(ctx)
	if got != SystemActor() {
		t.Errorf("blank id: got %+v, want SystemActor()", got)
	}
}

func TestContextWithAPIKey_DoesNotMutateParent(t *testing.T) {
	parent := context.Background()
	_ = ContextWithAPIKey(parent, "key-1", "one")
	if ActorFromContext(parent) != SystemActor() {
		t.Error("parent context was mutated by ContextWithAPIKey")
	}
}

func TestContextWithAPIKey_NestedOverrides(t *testing.T) {
	outer := ContextWithAPIKey(context.Background(), "key-outer", "outer")
	inner := ContextWithAPIKey(outer, "key-inner", "inner")

	if a := ActorFromContext(inner); a.ID != "key-inner" || a.Name != "inner" {
		t.Errorf("inner actor = %+v, want id=key-inner name=inner", a)
	}
	if a := ActorFromContext(outer); a.ID != "key-outer" || a.Name != "outer" {
		t.Errorf("outer actor = %+v, want id=key-outer name=outer", a)
	}
}

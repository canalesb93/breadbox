//go:build integration && !lite

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/service"
)

// TestMintRunAPIKey_CarriesDefinitionIdentity is the core of the
// identity-convergence fix: a minted per-run key must carry the agent's
// DISPLAY name (def.Name) as actor_name and a durable agent_definition_id
// link, so every write the run makes resolves to one canonical identity
// (definition name + slug-seeded avatar) instead of the SDK's clientInfo.
func TestMintRunAPIKey_CarriesDefinitionIdentity(t *testing.T) {
	svc, _, _ := newService(t)
	def := mustCreateAgentDefinition(t, svc, "ident-mint", true)
	ctx := context.Background()

	res, err := svc.MintRunAPIKey(ctx, def, "Run12345")
	if err != nil {
		t.Fatalf("MintRunAPIKey: %v", err)
	}

	// actor_name is the display name, not the slug — the feed reads
	// "Test ident-mint", never "ident-mint" or "claude-code".
	key, err := svc.ValidateAPIKey(ctx, res.PlaintextKey)
	if err != nil {
		t.Fatalf("ValidateAPIKey: %v", err)
	}
	if !key.ActorName.Valid || key.ActorName.String != def.Name {
		t.Errorf("actor_name = %q, want def.Name %q", key.ActorName.String, def.Name)
	}
	if !key.AgentDefinitionID.Valid {
		t.Fatalf("agent_definition_id not set on minted run key")
	}

	// The key resolves to the definition's slug — the avatar seed shared
	// across every surface the agent's activity appears on.
	slug, ok := svc.ResolveAgentSlugForActor(ctx, res.ID)
	if !ok {
		t.Fatalf("ResolveAgentSlugForActor: expected ok for a run key")
	}
	if slug != def.Slug {
		t.Errorf("resolved slug = %q, want def.Slug %q", slug, def.Slug)
	}
}

// TestResolveAgentSlugForActor_NonAgentKey confirms the resolver returns
// ok=false for keys with no definition link (human keys, external
// mcp-client identities) so render code falls back cleanly rather than
// inventing an agent identity.
func TestResolveAgentSlugForActor_NonAgentKey(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	res, err := svc.CreateAPIKey(ctx, service.CreateAPIKeyParams{
		Name:      "human-key-ident",
		Scope:     "full_access",
		ActorType: "user",
	})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if slug, ok := svc.ResolveAgentSlugForActor(ctx, res.ID); ok {
		t.Errorf("expected ok=false for a non-agent key, got slug=%q", slug)
	}
	if _, ok := svc.ResolveAgentSlugForActor(ctx, ""); ok {
		t.Errorf("expected ok=false for an empty actor id")
	}
}

// TestResolveAgentSlugForActor_FallbackFromKeyName covers the
// slug-from-name fallback: an agent key with NO agent_definition_id link
// (definition deleted via ON DELETE SET NULL, or a run key minted before
// the FK existed) still resolves to its slug from the immutable key name,
// so the agent's avatar keeps rendering instead of falling to the bot tile.
func TestResolveAgentSlugForActor_FallbackFromKeyName(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	res, err := svc.CreateAPIKey(ctx, service.CreateAPIKeyParams{
		Name:      "agent:orphan-slug:Run99",
		Scope:     "full_access",
		ActorType: "agent",
		// deliberately no AgentDefinitionID — simulates a deleted/pre-FK link
	})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	slug, ok := svc.ResolveAgentSlugForActor(ctx, res.ID)
	if !ok || slug != "orphan-slug" {
		t.Fatalf("fallback resolve = (%q,%v), want (orphan-slug,true)", slug, ok)
	}
}

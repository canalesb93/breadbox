//go:build integration && !lite

package cli

import (
	"log/slog"
	"testing"

	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

func TestResolveStdioActorKey_UsesBreadboxAPIKeyEnv(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	svc := service.New(queries, pool, nil, slog.Default())
	ctx := t.Context()

	created, err := svc.CreateAPIKey(ctx, service.CreateAPIKeyParams{
		Name:      "agent:reviewer:abc123de",
		Scope:     "full_access",
		ActorType: "agent",
		ActorName: "reviewer",
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	t.Setenv("BREADBOX_API_KEY", created.PlaintextKey)

	got, scope, err := resolveStdioActorKey(ctx, svc, queries)
	if err != nil {
		t.Fatalf("resolveStdioActorKey: %v", err)
	}
	if got.ActorType != "agent" {
		t.Errorf("ActorType = %q, want %q", got.ActorType, "agent")
	}
	if !got.ActorName.Valid || got.ActorName.String != "reviewer" {
		t.Errorf("ActorName = %+v, want valid=true string=%q", got.ActorName, "reviewer")
	}
	if scope != "full_access" {
		t.Errorf("scope = %q, want %q", scope, "full_access")
	}
}

func TestResolveStdioActorKey_PropagatesReadOnlyScope(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	svc := service.New(queries, pool, nil, slog.Default())
	ctx := t.Context()

	created, err := svc.CreateAPIKey(ctx, service.CreateAPIKeyParams{
		Name:      "agent:viewer:def45678",
		Scope:     "read_only",
		ActorType: "agent",
		ActorName: "viewer",
	})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	t.Setenv("BREADBOX_API_KEY", created.PlaintextKey)

	_, scope, err := resolveStdioActorKey(ctx, svc, queries)
	if err != nil {
		t.Fatalf("resolveStdioActorKey: %v", err)
	}
	if scope != "read_only" {
		t.Errorf("scope = %q, want %q", scope, "read_only")
	}
}

func TestResolveStdioActorKey_FallsBackToStdioSingleton(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	svc := service.New(queries, pool, nil, slog.Default())
	ctx := t.Context()

	t.Setenv("BREADBOX_API_KEY", "")

	got, scope, err := resolveStdioActorKey(ctx, svc, queries)
	if err != nil {
		t.Fatalf("resolveStdioActorKey: %v", err)
	}
	if got.ActorType != "system" {
		t.Errorf("ActorType = %q, want %q", got.ActorType, "system")
	}
	if !got.ActorName.Valid || got.ActorName.String != "stdio" {
		t.Errorf("ActorName = %+v, want valid=true string=%q", got.ActorName, "stdio")
	}
	if scope != "full_access" {
		t.Errorf("scope = %q, want %q", scope, "full_access")
	}
}

func TestResolveStdioActorKey_InvalidKeyFallsBack(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	svc := service.New(queries, pool, nil, slog.Default())
	ctx := t.Context()

	t.Setenv("BREADBOX_API_KEY", "bb_not_a_real_key_12345")

	got, _, err := resolveStdioActorKey(ctx, svc, queries)
	if err != nil {
		t.Fatalf("resolveStdioActorKey: %v", err)
	}
	if got.ActorType != "system" {
		t.Errorf("ActorType = %q, want %q (stdio singleton fallback)", got.ActorType, "system")
	}
}

//go:build integration

// Package auth_test exercises the CLI's auth-bootstrap flow at the
// service layer: minting a `user`-typed key, verifying the actor columns
// land on the row, and confirming the returned plaintext is the same one
// stored by SHA-256 hash. The cobra command itself writes to hosts.toml;
// that layer is covered by the unit tests in internal/cli/config.
package auth_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"strings"
	"testing"

	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

func TestMain(m *testing.M) {
	testutil.RunWithDB(m)
}

func TestBootstrap_MintsUserKey(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	svc := service.New(queries, pool, nil, slog.Default())
	ctx := context.Background()

	result, err := svc.CreateAPIKey(ctx, service.CreateAPIKeyParams{
		Name:      "cli-bootstrap",
		Scope:     "full_access",
		ActorType: "user",
		ActorName: "cli-bootstrap",
	})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if !strings.HasPrefix(result.PlaintextKey, "bb_") {
		t.Fatalf("plaintext key missing bb_ prefix: %q", result.PlaintextKey)
	}
	if result.ActorType != "user" {
		t.Errorf("actor_type = %q want user", result.ActorType)
	}
	if result.ActorName == nil || *result.ActorName != "cli-bootstrap" {
		t.Errorf("actor_name = %v want cli-bootstrap", result.ActorName)
	}

	// Round-trip: the plaintext should validate via the same SHA-256
	// hash path the middleware uses.
	hash := sha256.Sum256([]byte(result.PlaintextKey))
	row, err := queries.GetApiKeyByHash(ctx, hex.EncodeToString(hash[:]))
	if err != nil {
		t.Fatalf("GetApiKeyByHash: %v", err)
	}
	if row.ActorType != "user" {
		t.Errorf("row.ActorType = %q want user", row.ActorType)
	}
	if !row.ActorName.Valid || row.ActorName.String != "cli-bootstrap" {
		t.Errorf("row.ActorName = %v want cli-bootstrap", row.ActorName)
	}
}

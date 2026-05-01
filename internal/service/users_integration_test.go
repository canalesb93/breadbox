//go:build integration

// Integration coverage for service.GetUser. The method is the user-shaped
// mirror of GetAccount and is the resolver behind breadbox://user/{short_id}.
// Locks: lookup by UUID, lookup by short_id, ErrNotFound for unknown ids,
// ErrNotFound for malformed (non-UUID, non-short_id) input.
//
// TestMain lives in integration_test.go for this package; do not duplicate it.

package service_test

import (
	"context"
	"errors"
	"testing"

	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

func TestGetUser_ByUUIDAndShortID(t *testing.T) {
	svc, q, _ := newService(t)
	ctx := context.Background()

	created := testutil.MustCreateUser(t, q, "Alice")

	// Pull short_id + canonical UUID via the service so we test the real
	// boundary, not the row struct.
	users, err := svc.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	uuidStr := users[0].ID
	shortID := users[0].ShortID
	if shortID == "" || uuidStr == "" {
		t.Fatalf("missing identifiers: id=%q short=%q", uuidStr, shortID)
	}
	_ = created

	t.Run("by uuid", func(t *testing.T) {
		got, err := svc.GetUser(ctx, uuidStr)
		if err != nil {
			t.Fatalf("GetUser(uuid): %v", err)
		}
		if got.Name != "Alice" {
			t.Errorf("name = %q, want Alice", got.Name)
		}
		if got.ShortID != shortID {
			t.Errorf("short_id mismatch: got %q, want %q", got.ShortID, shortID)
		}
	})

	t.Run("by short_id", func(t *testing.T) {
		got, err := svc.GetUser(ctx, shortID)
		if err != nil {
			t.Fatalf("GetUser(short_id): %v", err)
		}
		if got.ID != uuidStr {
			t.Errorf("id mismatch: got %q, want %q", got.ID, uuidStr)
		}
	})

	t.Run("unknown short_id returns ErrNotFound", func(t *testing.T) {
		_, err := svc.GetUser(ctx, "zzzzzzzz")
		if !errors.Is(err, service.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("malformed id returns ErrNotFound", func(t *testing.T) {
		// Not a valid UUID and not an 8-char short_id — resolver short-circuits
		// to the parse path and the service maps the failure to ErrNotFound.
		_, err := svc.GetUser(ctx, "not-an-id")
		if !errors.Is(err, service.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})
}

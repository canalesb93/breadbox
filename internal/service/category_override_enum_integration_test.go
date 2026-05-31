//go:build integration && !lite

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

// TestCategoryOverrideEnum locks in the source-enum values on category_override
// (PR 2a — the boolean->enum retype). A freshly-synced row is 'none'; the
// binary lock toggle pins it to 'user'; unlocking returns it to 'none'.
func TestCategoryOverrideEnum(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "txn_cov", "Trader Joe's", 4200, "2026-02-01")
	id := txn.ShortID

	// Fresh transaction defaults to 'none' (not boolean false, not null).
	if got := mustOverride(t, svc, id); got != service.CategoryOverrideNone {
		t.Fatalf("fresh category_override = %q, want %q", got, service.CategoryOverrideNone)
	}

	// Locking via the (binary) override flag stamps 'user'.
	if err := svc.SetCategoryOverrideFlag(ctx, id, true, service.SystemActor()); err != nil {
		t.Fatalf("SetCategoryOverrideFlag(true): %v", err)
	}
	if got := mustOverride(t, svc, id); got != service.CategoryOverrideUser {
		t.Fatalf("after lock category_override = %q, want %q", got, service.CategoryOverrideUser)
	}

	// Unlocking returns it to 'none' so rules (and agents) can act again.
	if err := svc.SetCategoryOverrideFlag(ctx, id, false, service.SystemActor()); err != nil {
		t.Fatalf("SetCategoryOverrideFlag(false): %v", err)
	}
	if got := mustOverride(t, svc, id); got != service.CategoryOverrideNone {
		t.Fatalf("after unlock category_override = %q, want %q", got, service.CategoryOverrideNone)
	}
}

func mustOverride(t *testing.T, svc *service.Service, id string) string {
	t.Helper()
	resp, err := svc.GetTransaction(context.Background(), id)
	if err != nil {
		t.Fatalf("GetTransaction(%q): %v", id, err)
	}
	return resp.CategoryOverride
}

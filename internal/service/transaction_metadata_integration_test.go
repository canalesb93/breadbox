//go:build integration && !lite

package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

// metaOf reads a transaction's metadata back as a map for assertions.
func metaOf(t *testing.T, svc *service.Service, id string) map[string]any {
	t.Helper()
	resp, err := svc.GetTransaction(context.Background(), id)
	if err != nil {
		t.Fatalf("GetTransaction(%q): %v", id, err)
	}
	var m map[string]any
	if err := json.Unmarshal(resp.Metadata, &m); err != nil {
		t.Fatalf("unmarshal metadata %q: %v", string(resp.Metadata), err)
	}
	return m
}

func TestTransactionMetadata_Lifecycle(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "txn_meta", "Blue Bottle", 650, "2026-01-15")
	id := txn.ShortID

	// Fresh transaction starts with an empty object, never null.
	if m := metaOf(t, svc, id); len(m) != 0 {
		t.Fatalf("new transaction metadata = %v, want empty", m)
	}

	// Set upserts a single key.
	if err := svc.SetTransactionMetadata(ctx, id, "tax_deductible", true); err != nil {
		t.Fatalf("SetTransactionMetadata: %v", err)
	}
	if err := svc.SetTransactionMetadata(ctx, id, "trip", "japan-2026"); err != nil {
		t.Fatalf("SetTransactionMetadata: %v", err)
	}
	m := metaOf(t, svc, id)
	if m["tax_deductible"] != true {
		t.Fatalf("tax_deductible = %v, want true", m["tax_deductible"])
	}
	if m["trip"] != "japan-2026" {
		t.Fatalf("trip = %v, want japan-2026", m["trip"])
	}

	// Set overwrites one key without disturbing the other.
	if err := svc.SetTransactionMetadata(ctx, id, "trip", "iceland-2026"); err != nil {
		t.Fatalf("SetTransactionMetadata overwrite: %v", err)
	}
	m = metaOf(t, svc, id)
	if m["trip"] != "iceland-2026" || m["tax_deductible"] != true {
		t.Fatalf("after overwrite metadata = %v", m)
	}

	// Remove deletes one key only.
	if err := svc.RemoveTransactionMetadata(ctx, id, "tax_deductible"); err != nil {
		t.Fatalf("RemoveTransactionMetadata: %v", err)
	}
	m = metaOf(t, svc, id)
	if _, ok := m["tax_deductible"]; ok {
		t.Fatalf("tax_deductible still present after remove: %v", m)
	}
	if m["trip"] != "iceland-2026" {
		t.Fatalf("trip lost after removing a different key: %v", m)
	}

	// Removing an absent key is a no-op success.
	if err := svc.RemoveTransactionMetadata(ctx, id, "does_not_exist"); err != nil {
		t.Fatalf("RemoveTransactionMetadata(absent): %v", err)
	}

	// Replace swaps the entire object atomically.
	if err := svc.ReplaceTransactionMetadata(ctx, id, map[string]any{"a": float64(1), "b": "x"}); err != nil {
		t.Fatalf("ReplaceTransactionMetadata: %v", err)
	}
	m = metaOf(t, svc, id)
	if len(m) != 2 || m["a"] != float64(1) || m["b"] != "x" {
		t.Fatalf("after replace metadata = %v", m)
	}

	// Clear resets to empty object.
	if err := svc.ClearTransactionMetadata(ctx, id); err != nil {
		t.Fatalf("ClearTransactionMetadata: %v", err)
	}
	if m := metaOf(t, svc, id); len(m) != 0 {
		t.Fatalf("after clear metadata = %v, want empty", m)
	}
}

func TestTransactionMetadata_Validation(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "txn_val", "Costco", 12000, "2026-01-16")
	id := txn.ShortID

	if err := svc.SetTransactionMetadata(ctx, id, "", "v"); !errors.Is(err, service.ErrInvalidParameter) {
		t.Fatalf("empty key err = %v, want ErrInvalidParameter", err)
	}
	if err := svc.SetTransactionMetadata(ctx, id, strings.Repeat("k", 200), "v"); !errors.Is(err, service.ErrInvalidParameter) {
		t.Fatalf("long key err = %v, want ErrInvalidParameter", err)
	}
	if err := svc.SetTransactionMetadata(ctx, "nonexistent", "k", "v"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("missing txn err = %v, want ErrNotFound", err)
	}
	// The metadata ops never touch a first-class field: setting metadata must
	// not change the category override flag.
	if err := svc.SetTransactionMetadata(ctx, id, "note", "hello"); err != nil {
		t.Fatalf("SetTransactionMetadata: %v", err)
	}
	resp, err := svc.GetTransaction(ctx, id)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if resp.CategoryOverride != "none" {
		t.Fatalf("metadata write flipped category_override")
	}
}

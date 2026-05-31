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

// T12metaOf reads a transaction's metadata back as a map for assertions.
// Named with T12 prefix to avoid collision with metaOf in the sibling file.
func T12metaOf(t *testing.T, svc *service.Service, id string) map[string]any {
	t.Helper()
	resp, err := svc.GetTransaction(context.Background(), id)
	if err != nil {
		t.Fatalf("T12metaOf GetTransaction(%q): %v", id, err)
	}
	var m map[string]any
	if err := json.Unmarshal(resp.Metadata, &m); err != nil {
		t.Fatalf("T12metaOf unmarshal metadata %q: %v", string(resp.Metadata), err)
	}
	return m
}

// TestT12_SetMetadata_ValueSizeCap verifies that a value exceeding 4 KiB is
// rejected with ErrInvalidParameter.  The existing suite only validates key
// constraints; this covers the value ceiling.
func TestT12_SetMetadata_ValueSizeCap(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "t12_valcap", "ValCap Co", 100, "2026-02-01")
	id := txn.ShortID

	// A 4096-char string serialises to 4098 bytes (chars + surrounding quotes) - over 4096-byte cap.
	bigVal := strings.Repeat("x", 4096)
	err := svc.SetTransactionMetadata(ctx, id, "big", bigVal)
	if !errors.Is(err, service.ErrInvalidParameter) {
		t.Fatalf("4096-char string: err = %v, want ErrInvalidParameter", err)
	}

	// A 4094-char string serialises to 4096 bytes (4094 chars + 2 quotes) - exactly at the cap.
	okVal := strings.Repeat("y", 4094)
	if err := svc.SetTransactionMetadata(ctx, id, "ok", okVal); err != nil {
		t.Fatalf("4094-char string (serialised = 4096 bytes): %v", err)
	}
}

// TestT12_ReplaceMetadata_ObjectSizeCap verifies that replacing the entire blob
// with an object whose serialised size exceeds 8 KiB is rejected.
func TestT12_ReplaceMetadata_ObjectSizeCap(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "t12_objcap", "ObjCap Co", 200, "2026-02-02")
	id := txn.ShortID

	// Build an object whose JSON is comfortably over the 8 KiB ceiling.
	// Each entry serialises to ~407 bytes; 30 entries ≈ 12 KiB (> 8192).
	big := map[string]any{}
	for i := 0; i < 30; i++ {
		// Each key is ~100 chars + a unique suffix; each value is ~300 chars.
		key := strings.Repeat("k", 100) + string(rune('a'+i))
		big[key] = strings.Repeat("v", 300)
	}
	if err := svc.ReplaceTransactionMetadata(ctx, id, big); !errors.Is(err, service.ErrInvalidParameter) {
		t.Fatalf("oversized object: err = %v, want ErrInvalidParameter", err)
	}
}

// TestT12_ReplaceMetadata_KeyValidation confirms that ReplaceTransactionMetadata
// enforces key constraints the same way SetTransactionMetadata does.
func TestT12_ReplaceMetadata_KeyValidation(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "t12_replkey", "ReplKey Co", 300, "2026-02-03")
	id := txn.ShortID

	// Empty key inside the replacement object.
	if err := svc.ReplaceTransactionMetadata(ctx, id, map[string]any{"": "v"}); !errors.Is(err, service.ErrInvalidParameter) {
		t.Fatalf("empty key in replace: err = %v, want ErrInvalidParameter", err)
	}

	// Overlong key inside the replacement object.
	longKey := strings.Repeat("k", 200)
	if err := svc.ReplaceTransactionMetadata(ctx, id, map[string]any{longKey: "v"}); !errors.Is(err, service.ErrInvalidParameter) {
		t.Fatalf("long key in replace: err = %v, want ErrInvalidParameter", err)
	}

	// After both failures, metadata is still the original empty object.
	if m := T12metaOf(t, svc, id); len(m) != 0 {
		t.Fatalf("metadata unexpectedly mutated after validation failure: %v", m)
	}
}

// TestT12_RemoveMetadata_MissingTransaction verifies that RemoveTransactionMetadata
// returns ErrNotFound for a non-existent transaction -- the existing suite only
// tests this path for SetTransactionMetadata.
func TestT12_RemoveMetadata_MissingTransaction(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	err := svc.RemoveTransactionMetadata(ctx, "nonexistent_remove", "key")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("RemoveTransactionMetadata(missing): err = %v, want ErrNotFound", err)
	}
}

// TestT12_ReplaceMetadata_MissingTransaction verifies that ReplaceTransactionMetadata
// returns ErrNotFound for a non-existent transaction.
func TestT12_ReplaceMetadata_MissingTransaction(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	err := svc.ReplaceTransactionMetadata(ctx, "nonexistent_replace", map[string]any{"k": "v"})
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("ReplaceTransactionMetadata(missing): err = %v, want ErrNotFound", err)
	}
}

// TestT12_ClearMetadata_MissingTransaction verifies that ClearTransactionMetadata
// returns ErrNotFound for a non-existent transaction.
func TestT12_ClearMetadata_MissingTransaction(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	err := svc.ClearTransactionMetadata(ctx, "nonexistent_clear")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("ClearTransactionMetadata(missing): err = %v, want ErrNotFound", err)
	}
}

// TestT12_SetMetadata_JSONValueTypes verifies that the scoped Set op correctly
// round-trips diverse JSON value types: nested object, array, integer, float,
// and null.
func TestT12_SetMetadata_JSONValueTypes(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "t12_jsontypes", "JSONTypes Co", 500, "2026-02-04")
	id := txn.ShortID

	type valueCase struct {
		key      string
		val      any
		assertFn func(got any)
	}
	cases := []valueCase{
		{
			key: "nested",
			val: map[string]any{"inner": "value"},
			assertFn: func(got any) {
				m, ok := got.(map[string]any)
				if !ok || m["inner"] != "value" {
					t.Errorf("nested: got %v (%T)", got, got)
				}
			},
		},
		{
			key: "arr",
			val: []any{float64(1), "two", false},
			assertFn: func(got any) {
				arr, ok := got.([]any)
				if !ok || len(arr) != 3 {
					t.Errorf("arr: got %v (%T)", got, got)
				}
			},
		},
		{
			key: "intval",
			val: float64(42),
			assertFn: func(got any) {
				if got != float64(42) {
					t.Errorf("intval: got %v (%T)", got, got)
				}
			},
		},
		{
			key: "floatval",
			val: 3.14,
			assertFn: func(got any) {
				f, ok := got.(float64)
				if !ok || f < 3.13 || f > 3.15 {
					t.Errorf("floatval: got %v (%T)", got, got)
				}
			},
		},
		{
			key: "nullval",
			val: nil,
			assertFn: func(got any) {
				if got != nil {
					t.Errorf("nullval: got %v (%T), want nil", got, got)
				}
			},
		},
	}

	for _, c := range cases {
		if err := svc.SetTransactionMetadata(ctx, id, c.key, c.val); err != nil {
			t.Fatalf("SetTransactionMetadata(%q): %v", c.key, err)
		}
	}

	m := T12metaOf(t, svc, id)
	for _, c := range cases {
		c.assertFn(m[c.key])
	}
}

// TestT12_SetMetadata_IsolatesFirstClassFields verifies that scoped Set ops
// never touch first-class transaction columns (amount, provider_name).
func TestT12_SetMetadata_IsolatesFirstClassFields(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "t12_iso", "ISO Merchant", 4200, "2026-02-05")
	id := txn.ShortID

	// Set a metadata key.
	if err := svc.SetTransactionMetadata(ctx, id, "note", "business expense"); err != nil {
		t.Fatalf("SetTransactionMetadata: %v", err)
	}

	// Read first-class fields directly from DB to avoid any service-layer
	// mapping surprises.
	var (
		providerName string
		amountRaw    string // NUMERIC comes back as text via ::text cast
	)
	if err := pool.QueryRow(ctx,
		"SELECT provider_name, amount::text FROM transactions WHERE id = $1",
		txn.ID,
	).Scan(&providerName, &amountRaw); err != nil {
		t.Fatalf("QueryRow first-class fields: %v", err)
	}
	if providerName != "ISO Merchant" {
		t.Errorf("provider_name mutated: got %q, want %q", providerName, "ISO Merchant")
	}
	// amount stored as NUMERIC(12,2): 4200 cents -> 42.00
	if amountRaw != "42.00" {
		t.Errorf("amount mutated: got %q, want %q", amountRaw, "42.00")
	}
}

// TestT12_ClearMetadata_Idempotent verifies that clearing an already-empty
// metadata blob succeeds and leaves the column as an empty object.
func TestT12_ClearMetadata_Idempotent(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "t12_clearidm", "ClearIdm Co", 700, "2026-02-06")
	id := txn.ShortID

	// First clear on an already-empty blob.
	if err := svc.ClearTransactionMetadata(ctx, id); err != nil {
		t.Fatalf("ClearTransactionMetadata (first, empty): %v", err)
	}
	if m := T12metaOf(t, svc, id); len(m) != 0 {
		t.Fatalf("after first clear: metadata = %v, want empty", m)
	}

	// Set a key, clear, then clear again.
	if err := svc.SetTransactionMetadata(ctx, id, "x", 1); err != nil {
		t.Fatalf("SetTransactionMetadata: %v", err)
	}
	if err := svc.ClearTransactionMetadata(ctx, id); err != nil {
		t.Fatalf("ClearTransactionMetadata (after set): %v", err)
	}
	if err := svc.ClearTransactionMetadata(ctx, id); err != nil {
		t.Fatalf("ClearTransactionMetadata (second, empty again): %v", err)
	}
	if m := T12metaOf(t, svc, id); len(m) != 0 {
		t.Fatalf("after double-clear: metadata = %v, want empty", m)
	}
}

// TestT12_MetadataOps_ShortIDResolution verifies that all four scoped ops
// accept a short_id in place of a UUID.
func TestT12_MetadataOps_ShortIDResolution(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "t12_shortid", "ShortID Co", 800, "2026-02-07")
	shortID := txn.ShortID

	if len(shortID) != 8 {
		t.Fatalf("expected 8-char short_id, got %q (len=%d)", shortID, len(shortID))
	}

	// Set via short_id.
	if err := svc.SetTransactionMetadata(ctx, shortID, "s", "short"); err != nil {
		t.Fatalf("SetTransactionMetadata(short_id): %v", err)
	}

	// Remove via short_id.
	if err := svc.RemoveTransactionMetadata(ctx, shortID, "s"); err != nil {
		t.Fatalf("RemoveTransactionMetadata(short_id): %v", err)
	}

	// Replace via short_id.
	if err := svc.ReplaceTransactionMetadata(ctx, shortID, map[string]any{"r": true}); err != nil {
		t.Fatalf("ReplaceTransactionMetadata(short_id): %v", err)
	}

	m := T12metaOf(t, svc, shortID)
	if m["r"] != true {
		t.Fatalf("after ReplaceTransactionMetadata(short_id): got %v", m)
	}

	// Clear via short_id.
	if err := svc.ClearTransactionMetadata(ctx, shortID); err != nil {
		t.Fatalf("ClearTransactionMetadata(short_id): %v", err)
	}
	if m := T12metaOf(t, svc, shortID); len(m) != 0 {
		t.Fatalf("after ClearTransactionMetadata(short_id): %v", m)
	}
}

// TestT12_ReplaceMetadata_NilEquivalentToEmpty verifies that nil and an empty
// map produce the same result: an empty metadata object (not SQL NULL).
func TestT12_ReplaceMetadata_NilEquivalentToEmpty(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "t12_nil", "Nil Co", 900, "2026-02-08")
	id := txn.ShortID

	// Seed a key first.
	if err := svc.SetTransactionMetadata(ctx, id, "tmp", 1); err != nil {
		t.Fatalf("SetTransactionMetadata: %v", err)
	}

	// Replace with nil.
	if err := svc.ReplaceTransactionMetadata(ctx, id, nil); err != nil {
		t.Fatalf("ReplaceTransactionMetadata(nil): %v", err)
	}

	// metadata column must be the empty JSON object, not SQL NULL.
	var raw []byte
	if err := pool.QueryRow(ctx,
		"SELECT metadata FROM transactions WHERE id = $1", txn.ID,
	).Scan(&raw); err != nil {
		t.Fatalf("QueryRow metadata: %v", err)
	}
	if string(raw) != "{}" {
		t.Fatalf("metadata after nil replace = %q, want %q", string(raw), "{}")
	}
}

// TestT12_MetadataOps_NoBleedIntoOtherColumns confirms that performing all
// four metadata ops does not mutate category_id or the transaction_tags join
// table.
func TestT12_MetadataOps_NoBleedIntoOtherColumns(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "t12_metaonly", "MetaOnly Co", 1100, "2026-02-09")
	id := txn.ShortID

	// Baseline: no category, no tags.
	var catIDBase *string
	var tagCountBase int
	if err := pool.QueryRow(ctx,
		`SELECT category_id::text,
		        (SELECT COUNT(*) FROM transaction_tags WHERE transaction_id = t.id)
		   FROM transactions t WHERE t.id = $1`,
		txn.ID,
	).Scan(&catIDBase, &tagCountBase); err != nil {
		t.Fatalf("baseline query: %v", err)
	}
	if catIDBase != nil {
		t.Fatalf("unexpected category before metadata ops: %v", *catIDBase)
	}

	// Perform all four scoped metadata ops.
	if err := svc.SetTransactionMetadata(ctx, id, "flag", true); err != nil {
		t.Fatalf("SetTransactionMetadata: %v", err)
	}
	if err := svc.RemoveTransactionMetadata(ctx, id, "flag"); err != nil {
		t.Fatalf("RemoveTransactionMetadata: %v", err)
	}
	if err := svc.ReplaceTransactionMetadata(ctx, id, map[string]any{"a": 1}); err != nil {
		t.Fatalf("ReplaceTransactionMetadata: %v", err)
	}
	if err := svc.ClearTransactionMetadata(ctx, id); err != nil {
		t.Fatalf("ClearTransactionMetadata: %v", err)
	}

	// category_id and tag count must be unchanged after all ops.
	var catIDAfter *string
	var tagCountAfter int
	if err := pool.QueryRow(ctx,
		`SELECT category_id::text,
		        (SELECT COUNT(*) FROM transaction_tags WHERE transaction_id = t.id)
		   FROM transactions t WHERE t.id = $1`,
		txn.ID,
	).Scan(&catIDAfter, &tagCountAfter); err != nil {
		t.Fatalf("after-ops query: %v", err)
	}
	if catIDAfter != nil {
		t.Fatalf("category mutated by metadata ops: %v", *catIDAfter)
	}
	if tagCountAfter != tagCountBase {
		t.Fatalf("tag count changed by metadata ops: %d -> %d", tagCountBase, tagCountAfter)
	}
}

//go:build integration && !lite

package sync_test

import (
	"context"
	"testing"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/provider"
	"breadbox/internal/testutil"

	"github.com/shopspring/decimal"
)

// TestSync_RecurrenceCondition_AddsTag drives the canonical detector-free
// subscription rule through a real sync: amount ≈ X ± Y AND day_of_month ≈ D ± N
// → add_tag. Only the in-window charge should be tagged.
func TestSync_RecurrenceCondition_AddsTag(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)
	tag := testutil.MustCreateTag(t, queries, "subscription", "Subscription")

	// amount ≈ 15.49 ± 0.50 AND day_of_month ≈ 14 ± 3.
	testutil.MustCreateTransactionRule(
		t, queries, "Netflix subscription",
		[]byte(`{"and":[
			{"field":"amount","op":"approx","value":15.49,"tolerance":0.5},
			{"field":"day_of_month","op":"approx","value":14,"tolerance":3}
		]}`),
		[]byte(`[{"type":"add_tag","tag_slug":"subscription"}]`),
		"on_create",
	)

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Added: []provider.Transaction{
				{ // in window: $15.49 on the 16th
					ExternalID:        "txn_match",
					AccountExternalID: "ext_acct_1",
					Amount:            decimal.NewFromFloat(15.49),
					Date:              time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC),
					Name:              "Netflix",
					ISOCurrencyCode:   "USD",
				},
				{ // out-of-window day: $15.49 on the 25th
					ExternalID:        "txn_offday",
					AccountExternalID: "ext_acct_1",
					Amount:            decimal.NewFromFloat(15.49),
					Date:              time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC),
					Name:              "Netflix",
					ISOCurrencyCode:   "USD",
				},
				{ // out-of-window amount: $99.00 on the 14th
					ExternalID:        "txn_offamt",
					AccountExternalID: "ext_acct_1",
					Amount:            decimal.NewFromFloat(99.00),
					Date:              time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC),
					Name:              "Spotify",
					ISOCurrencyCode:   "USD",
				},
			},
			Cursor: "cursor_1",
		},
	}
	engine := newEngine(t, pool, queries, map[string]provider.Provider{"plaid": mock})
	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	tagged := func(extID string) bool {
		var n int
		if err := pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM transaction_tags tt
			JOIN transactions t ON tt.transaction_id = t.id
			WHERE t.provider_transaction_id = $1 AND tt.tag_id = $2`, extID, tag.ID).Scan(&n); err != nil {
			t.Fatalf("count tags for %s: %v", extID, err)
		}
		return n == 1
	}

	if !tagged("txn_match") {
		t.Errorf("in-window charge should be tagged during sync")
	}
	if tagged("txn_offday") {
		t.Errorf("out-of-window day should NOT be tagged")
	}
	if tagged("txn_offamt") {
		t.Errorf("out-of-window amount should NOT be tagged")
	}
}

//go:build integration && !lite

// Sync-path coverage for the flag / unflag rule actions (rules-as-universal-
// substrate, P1). A rule with a flag action must set transactions.flagged_at
// during sync (mirroring the flag_transaction MCP tool); unflag must clear it.
package sync_test

import (
	"context"
	"testing"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/provider"
	"breadbox/internal/testutil"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

// A flag rule sets flagged_at on a freshly-synced matching transaction.
func TestRule_Flag_DuringSync(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	testutil.MustCreateTransactionRule(
		t, queries, "Flag big charges",
		[]byte(`{"field": "provider_name","op":"contains","value":"FlagMe"}`),
		[]byte(`[{"type":"flag"}]`),
		"on_create",
	)

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_flag")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_flag", "Checking")

	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Added: []provider.Transaction{{
				ExternalID:        "txn_flagme",
				AccountExternalID: "ext_acct_flag",
				Amount:            decimal.NewFromFloat(250.00),
				Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
				Name:              "FlagMe Charge",
				ISOCurrencyCode:   "USD",
			}},
			Cursor: "cursor_1",
		},
	}
	engine := newEngine(t, pool, queries, map[string]provider.Provider{"plaid": mock})
	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("sync: %v", err)
	}

	var flaggedAt pgtype.Timestamptz
	if err := pool.QueryRow(ctx,
		"SELECT flagged_at FROM transactions WHERE provider_transaction_id = 'txn_flagme'",
	).Scan(&flaggedAt); err != nil {
		t.Fatalf("query flagged_at: %v", err)
	}
	if !flaggedAt.Valid {
		t.Errorf("flag: expected flagged_at to be set during sync, got NULL")
	}
}

// An unflag rule clears flagged_at on a re-synced (changed) transaction that
// was previously flagged.
func TestRule_Unflag_DuringSync(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	testutil.MustCreateTransactionRule(
		t, queries, "Clear flag",
		[]byte(`{"field": "provider_name","op":"contains","value":"UnflagMe"}`),
		[]byte(`[{"type":"unflag"}]`),
		"always",
	)

	user := testutil.MustCreateUser(t, queries, "Bob")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_unflag")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_unflag", "Checking")

	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Added: []provider.Transaction{{
				ExternalID:        "txn_unflagme",
				AccountExternalID: "ext_acct_unflag",
				Amount:            decimal.NewFromFloat(12.00),
				Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
				Name:              "UnflagMe Charge",
				ISOCurrencyCode:   "USD",
				Pending:           true,
			}},
			Cursor: "cursor_1",
		},
	}
	engine := newEngine(t, pool, queries, map[string]provider.Provider{"plaid": mock})
	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Pre-flag the row so the next sync's unflag has something to clear.
	if _, err := pool.Exec(ctx,
		"UPDATE transactions SET flagged_at = NOW() WHERE provider_transaction_id = 'txn_unflagme'",
	); err != nil {
		t.Fatalf("pre-flag: %v", err)
	}

	// Second sync modifies the txn (pending flips) so the "always" rule re-fires.
	mock.syncResult = provider.SyncResult{
		Modified: []provider.Transaction{{
			ExternalID:        "txn_unflagme",
			AccountExternalID: "ext_acct_unflag",
			Amount:            decimal.NewFromFloat(12.00),
			Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
			Name:              "UnflagMe Charge",
			ISOCurrencyCode:   "USD",
			Pending:           false,
		}},
		Cursor: "cursor_2",
	}
	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	var flaggedAt pgtype.Timestamptz
	if err := pool.QueryRow(ctx,
		"SELECT flagged_at FROM transactions WHERE provider_transaction_id = 'txn_unflagme'",
	).Scan(&flaggedAt); err != nil {
		t.Fatalf("query flagged_at: %v", err)
	}
	if flaggedAt.Valid {
		t.Errorf("unflag: expected flagged_at cleared during sync, still set: %v", flaggedAt.Time)
	}
}

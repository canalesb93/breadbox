//go:build integration && !lite

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

// TestSync_RawResyncPreservesManualCategory is the P3-T8 / P0-T5 guard: after a
// user (or rule) sets a transaction's category, a raw re-sync of the SAME,
// UNCHANGED provider transaction must PRESERVE the existing category_id. The
// UpsertTransaction on-conflict clause sets category_id = transactions.category_id,
// so a re-sync never clobbers a manual edit. Provenance/override columns were
// removed in P3 — this preserve-on-resync behavior is the only thing standing
// between a user's manual category and the next sync.
func TestSync_RawResyncPreservesManualCategory(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)
	manualCat, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug: "manual_pick", DisplayName: "Manual Pick",
	})
	if err != nil {
		t.Fatalf("create manual category: %v", err)
	}

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

	txns := []provider.Transaction{
		{
			ExternalID:        "txn_resync",
			AccountExternalID: "ext_acct_1",
			Amount:            decimal.NewFromFloat(12.34),
			Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
			Name:              "Some Merchant",
			ISOCurrencyCode:   "USD",
		},
	}

	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Added:  txns,
			Cursor: "cursor_1",
		},
	}
	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)

	// First sync: the transaction is created (no rule, lands uncategorized).
	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// A user manually categorizes the transaction.
	if _, err := pool.Exec(ctx,
		"UPDATE transactions SET category_id = $1 WHERE provider_transaction_id = 'txn_resync'",
		manualCat.ID,
	); err != nil {
		t.Fatalf("set manual category: %v", err)
	}

	// Second sync: the SAME provider transaction comes back unchanged. The
	// on-conflict upsert must preserve the manual category_id.
	mock.syncResult = provider.SyncResult{
		Added:  txns,
		Cursor: "cursor_2",
	}
	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	var gotCatID pgtype.UUID
	if err := pool.QueryRow(ctx,
		"SELECT category_id FROM transactions WHERE provider_transaction_id = 'txn_resync'",
	).Scan(&gotCatID); err != nil {
		t.Fatalf("query category_id after re-sync: %v", err)
	}
	if gotCatID != manualCat.ID {
		t.Errorf("raw re-sync clobbered the manual category; expected %v, got %v", manualCat.ID, gotCatID)
	}
}

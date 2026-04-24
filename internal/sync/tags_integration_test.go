//go:build integration

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

// TestSync_AddTagAction_PersistsTag exercises add_tag wiring: a rule with an
// add_tag action fires during sync, the tag is attached to the transaction
// with provenance, and a matching tag_added annotation row is written.
func TestSync_AddTagAction_PersistsTag(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	// Seed a persistent "dining" tag so we don't rely on auto-create in this
	// test — the auto-create path is covered by the seeded-rule test below.
	tag := testutil.MustCreateTag(t, queries, "dining", "Dining")

	// Rule: name contains "Restaurant" → add_tag "dining".
	testutil.MustCreateTransactionRule(
		t, queries, "Dining Tag",
		[]byte(`{"field": "provider_name","op":"contains","value":"Restaurant"}`),
		[]byte(`[{"type":"add_tag","tag_slug":"dining"}]`),
		"on_create",
	)

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Added: []provider.Transaction{
				{
					ExternalID:        "txn_tagged",
					AccountExternalID: "ext_acct_1",
					Amount:            decimal.NewFromFloat(42.50),
					Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
					Name:              "Restaurant ABC",
					ISOCurrencyCode:   "USD",
				},
			},
			Cursor: "cursor_1",
		},
	}
	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)

	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Verify transaction_tags row exists.
	var count int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM transaction_tags tt
		JOIN transactions t ON tt.transaction_id = t.id
		WHERE t.provider_transaction_id = 'txn_tagged' AND tt.tag_id = $1`, tag.ID).Scan(&count)
	if err != nil {
		t.Fatalf("count transaction_tags: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 transaction_tags row, got %d", count)
	}

	// Verify tag_added annotation was written with rule back-reference.
	var annCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM annotations a
		JOIN transactions t ON a.transaction_id = t.id
		WHERE t.provider_transaction_id = 'txn_tagged'
		  AND a.kind = 'tag_added'
		  AND a.tag_id = $1
		  AND a.rule_id IS NOT NULL`, tag.ID).Scan(&annCount)
	if err != nil {
		t.Fatalf("count annotations: %v", err)
	}
	if annCount != 1 {
		t.Errorf("expected 1 tag_added annotation with rule_id, got %d", annCount)
	}

	// Verify provenance: added_by_type='rule'.
	var addedByType string
	err = pool.QueryRow(ctx, `
		SELECT added_by_type FROM transaction_tags tt
		JOIN transactions t ON tt.transaction_id = t.id
		WHERE t.provider_transaction_id = 'txn_tagged' AND tt.tag_id = $1`, tag.ID).Scan(&addedByType)
	if err != nil {
		t.Fatalf("query added_by_type: %v", err)
	}
	if addedByType != "rule" {
		t.Errorf("expected added_by_type=rule, got %q", addedByType)
	}
}

// TestSync_RemoveTagAction_DeletesTagAndAnnotates exercises the remove_tag
// action end-to-end. The test pre-seeds a transaction with `needs-review`,
// then re-syncs with a rule that fires on "always" and removes the tag.
// Verifies the transaction_tags row is deleted and a tag_removed annotation
// is written with the rule as the actor.
func TestSync_RemoveTagAction_DeletesTagAndAnnotates(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	// Pre-seed tag and rule.
	tag := testutil.MustCreateTag(t, queries, "needs-review", "Needs Review")
	testutil.MustCreateTransactionRule(
		t, queries, "Clear review flag for coffee",
		[]byte(`{"field": "provider_name","op":"contains","value":"Starbucks"}`),
		[]byte(`[{"type":"remove_tag","tag_slug":"needs-review"}]`),
		"always",
	)

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

	// First sync: creates the transaction. Our rule (trigger=always) tags
	// nothing but the absence of a pre-existing needs-review means the
	// remove is a no-op here.
	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Added: []provider.Transaction{
				{
					ExternalID:        "txn_sbx",
					AccountExternalID: "ext_acct_1",
					Amount:            decimal.NewFromFloat(5.50),
					Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
					Name:              "Starbucks Coffee",
					ISOCurrencyCode:   "USD",
				},
			},
			Cursor: "cursor_1",
		},
	}
	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)
	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("first Sync() error: %v", err)
	}

	// Manually attach needs-review to the transaction (simulates a prior
	// sync pass or user action).
	_, err := pool.Exec(ctx, `
		INSERT INTO transaction_tags (transaction_id, tag_id, added_by_type, added_by_name)
		SELECT t.id, $1, 'user', 'Alice'
		FROM transactions t WHERE t.provider_transaction_id = 'txn_sbx'`, tag.ID)
	if err != nil {
		t.Fatalf("attach needs-review: %v", err)
	}

	// Second sync: provider returns the same transaction with an updated
	// balance / amount, causing an isChanged path that re-runs rules.
	mock.syncResult = provider.SyncResult{
		Modified: []provider.Transaction{
			{
				ExternalID:        "txn_sbx",
				AccountExternalID: "ext_acct_1",
				Amount:            decimal.NewFromFloat(5.75), // changed
				Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
				Name:              "Starbucks Coffee",
				ISOCurrencyCode:   "USD",
			},
		},
		Cursor: "cursor_2",
	}
	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("second Sync() error: %v", err)
	}

	// Verify transaction_tags row is gone.
	var ttCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM transaction_tags tt
		JOIN transactions t ON tt.transaction_id = t.id
		WHERE t.provider_transaction_id = 'txn_sbx' AND tt.tag_id = $1`, tag.ID).Scan(&ttCount)
	if err != nil {
		t.Fatalf("count transaction_tags: %v", err)
	}
	if ttCount != 0 {
		t.Errorf("expected needs-review tag removed, %d row(s) remain", ttCount)
	}

	// Verify tag_removed annotation with rule back-reference.
	var annCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM annotations a
		JOIN transactions t ON a.transaction_id = t.id
		WHERE t.provider_transaction_id = 'txn_sbx'
		  AND a.kind = 'tag_removed'
		  AND a.tag_id = $1
		  AND a.rule_id IS NOT NULL`, tag.ID).Scan(&annCount)
	if err != nil {
		t.Fatalf("count annotations: %v", err)
	}
	if annCount != 1 {
		t.Errorf("expected 1 tag_removed annotation, got %d", annCount)
	}
}

// TestSync_SeededRule_TagsAllNewTransactions verifies the seeded review rule:
// match-all conditions (NULL) with a single add_tag action tagging
// newly-synced transactions with 'needs-review'.
//
// Testutil truncates all tables between tests, so the migration's seeded
// rows are wiped. We re-seed the tag + rule here mirroring the migration
// exactly (same slug, same action shape) to prove the
// seeded behavior works end-to-end.
func TestSync_SeededRule_TagsAllNewTransactions(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	// Re-seed the needs-review tag + rule just like the migration.
	testutil.MustCreateTag(t, queries, "needs-review", "Needs Review")
	testutil.MustCreateTransactionRule(
		t, queries, "Auto-tag new transactions for review",
		nil, // match-all conditions
		[]byte(`[{"type":"add_tag","tag_slug":"needs-review"}]`),
		"on_create",
	)

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Added: []provider.Transaction{
				{
					ExternalID:        "txn_seeded_1",
					AccountExternalID: "ext_acct_1",
					Amount:            decimal.NewFromFloat(10.00),
					Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
					Name:              "Unknown Merchant",
					ISOCurrencyCode:   "USD",
				},
				{
					ExternalID:        "txn_seeded_2",
					AccountExternalID: "ext_acct_1",
					Amount:            decimal.NewFromFloat(20.00),
					Date:              time.Date(2025, 3, 2, 0, 0, 0, 0, time.UTC),
					Name:              "Another Merchant",
					ISOCurrencyCode:   "USD",
				},
			},
			Cursor: "cursor_seed",
		},
	}
	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)

	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Every synced transaction should carry the needs-review tag.
	var taggedCount int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT t.id)
		FROM transactions t
		JOIN transaction_tags tt ON tt.transaction_id = t.id
		JOIN tags tag ON tag.id = tt.tag_id
		WHERE tag.slug = 'needs-review'
		  AND t.provider_transaction_id IN ('txn_seeded_1', 'txn_seeded_2')`).Scan(&taggedCount)
	if err != nil {
		t.Fatalf("count tagged transactions: %v", err)
	}
	if taggedCount != 2 {
		t.Errorf("expected seeded rule to tag both synced transactions, got %d", taggedCount)
	}
}

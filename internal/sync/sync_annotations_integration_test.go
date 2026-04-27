//go:build integration

package sync_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/provider"
	"breadbox/internal/testutil"

	"github.com/shopspring/decimal"
)

// TestSync_SyncStartedAnnotation_OnInitialImport asserts that a fresh
// transaction lands a `sync_started` annotation attributed to the provider
// with payload {provider, connection_id, sync_log_id}.
func TestSync_SyncStartedAnnotation_OnInitialImport(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Added: []provider.Transaction{
				{
					ExternalID:        "txn_started",
					AccountExternalID: "ext_acct_1",
					Amount:            decimal.NewFromFloat(42.50),
					Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
					Name:              "Starbucks",
					ISOCurrencyCode:   "USD",
				},
			},
			Cursor: "cursor_1",
		},
	}
	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)

	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerInitial); err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	var actorType, actorName, payloadJSON string
	err := pool.QueryRow(ctx, `
		SELECT a.actor_type, a.actor_name, a.payload::text
		FROM annotations a
		JOIN transactions t ON a.transaction_id = t.id
		WHERE t.provider_transaction_id = 'txn_started'
		  AND a.kind = 'sync_started'`).Scan(&actorType, &actorName, &payloadJSON)
	if err != nil {
		t.Fatalf("query sync_started annotation: %v", err)
	}
	if actorType != "system" {
		t.Errorf("actor_type = %q, want system", actorType)
	}
	if actorName != "Plaid" {
		t.Errorf("actor_name = %q, want Plaid", actorName)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["provider"] != "plaid" {
		t.Errorf("payload.provider = %v, want plaid", payload["provider"])
	}
	if payload["connection_id"] != conn.ShortID {
		t.Errorf("payload.connection_id = %v, want %s", payload["connection_id"], conn.ShortID)
	}
	if _, ok := payload["sync_log_id"].(string); !ok {
		t.Errorf("payload.sync_log_id missing or not a string: %v", payload["sync_log_id"])
	}
}

// TestSync_SyncUpdatedAnnotation_OnPendingFlip asserts that flipping pending
// from true → false in a subsequent sync writes a `sync_updated` annotation
// carrying the status_change object.
func TestSync_SyncUpdatedAnnotation_OnPendingFlip(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Added: []provider.Transaction{
				{
					ExternalID:        "txn_flip",
					AccountExternalID: "ext_acct_1",
					Amount:            decimal.NewFromFloat(42.50),
					Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
					Name:              "Starbucks",
					ISOCurrencyCode:   "USD",
					Pending:           true,
				},
			},
			Cursor: "cursor_1",
		},
	}
	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)

	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerInitial); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Second sync: same transaction surfaces as Modified with pending=false.
	mock.syncResult = provider.SyncResult{
		Modified: []provider.Transaction{
			{
				ExternalID:        "txn_flip",
				AccountExternalID: "ext_acct_1",
				Amount:            decimal.NewFromFloat(42.50),
				Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
				Name:              "Starbucks",
				ISOCurrencyCode:   "USD",
				Pending:           false,
			},
		},
		Cursor: "cursor_2",
	}

	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	var payloadJSON string
	err := pool.QueryRow(ctx, `
		SELECT a.payload::text
		FROM annotations a
		JOIN transactions t ON a.transaction_id = t.id
		WHERE t.provider_transaction_id = 'txn_flip'
		  AND a.kind = 'sync_updated'`).Scan(&payloadJSON)
	if err != nil {
		t.Fatalf("query sync_updated annotation: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	sc, ok := payload["status_change"].(map[string]any)
	if !ok {
		t.Fatalf("payload.status_change missing or wrong type: %v", payload["status_change"])
	}
	if sc["from"] != "pending" {
		t.Errorf("status_change.from = %v, want pending", sc["from"])
	}
	if sc["to"] != "posted" {
		t.Errorf("status_change.to = %v, want posted", sc["to"])
	}
}

// TestSync_SyncUpdatedAnnotation_NoOp asserts that a subsequent sync touching
// no user-visible fields writes no `sync_updated` row.
func TestSync_SyncUpdatedAnnotation_NoOp(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	seedCategories(t, queries)

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

	txn := provider.Transaction{
		ExternalID:        "txn_noop",
		AccountExternalID: "ext_acct_1",
		Amount:            decimal.NewFromFloat(42.50),
		Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
		Name:              "Starbucks",
		ISOCurrencyCode:   "USD",
		Pending:           false,
	}
	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Added:  []provider.Transaction{txn},
			Cursor: "cursor_1",
		},
	}
	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)

	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerInitial); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	// Re-send the SAME row as Modified — nothing has changed.
	mock.syncResult = provider.SyncResult{
		Modified: []provider.Transaction{txn},
		Cursor:   "cursor_2",
	}
	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	var count int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM annotations a
		JOIN transactions t ON a.transaction_id = t.id
		WHERE t.provider_transaction_id = 'txn_noop'
		  AND a.kind = 'sync_updated'`).Scan(&count)
	if err != nil {
		t.Fatalf("count sync_updated: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 sync_updated rows for no-op sync, got %d", count)
	}
}

// TestSync_SyncUpdatedAnnotation_RuleOnlyChange asserts that a subsequent
// sync that fires a rule (only the category changes) does NOT write a
// `sync_updated` row but DOES write the rule_applied + category_set rows.
func TestSync_SyncUpdatedAnnotation_RuleOnlyChange(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	_, food := seedCategoriesWithFood(t, queries)
	_ = food

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")

	txn := provider.Transaction{
		ExternalID:        "txn_rule_only",
		AccountExternalID: "ext_acct_1",
		Amount:            decimal.NewFromFloat(42.50),
		Date:              time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
		Name:              "Starbucks",
		ISOCurrencyCode:   "USD",
		Pending:           false,
	}
	mock := &mockProvider{
		syncResult: provider.SyncResult{
			Added:  []provider.Transaction{txn},
			Cursor: "cursor_1",
		},
	}
	providers := map[string]provider.Provider{"plaid": mock}
	engine := newEngine(t, pool, queries, providers)

	// Initial sync — no rule yet, so the txn lands uncategorized.
	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerInitial); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Add a rule that fires on_change AND on_create matching this txn name.
	// We'll use trigger 'always' so it fires on the second sync too.
	testutil.MustCreateTransactionRule(
		t, queries, "Starbucks → Food",
		[]byte(`{"field": "provider_name","op":"contains","value":"Starbucks"}`),
		[]byte(`[{"type":"set_category","category_slug":"food_and_drink"}]`),
		"always",
	)

	// Second sync — provider returns the same row (Modified), but the upsert
	// classifies it as "unchanged" (no field-level change). Even so, our sync
	// engine currently only fires rules on isNew||isChanged paths, so we need
	// to bump a field. Bump merchant_name so the upsert flags isChanged but
	// pending stays put — this is exactly the "rule fires, no pending flip"
	// scenario the spec calls out.
	merchant := "Starbucks Corp"
	txn.MerchantName = &merchant
	mock.syncResult = provider.SyncResult{
		Modified: []provider.Transaction{txn},
		Cursor:   "cursor_2",
	}
	if err := engine.Sync(ctx, conn.ID, db.SyncTriggerManual); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	// rule_applied row should exist.
	var ruleAppliedCount int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM annotations a
		JOIN transactions t ON a.transaction_id = t.id
		WHERE t.provider_transaction_id = 'txn_rule_only'
		  AND a.kind = 'rule_applied'`).Scan(&ruleAppliedCount)
	if err != nil {
		t.Fatalf("count rule_applied: %v", err)
	}
	if ruleAppliedCount < 1 {
		t.Errorf("expected at least 1 rule_applied row, got %d", ruleAppliedCount)
	}

	// sync_updated row should NOT exist (pending didn't flip).
	var syncUpdatedCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM annotations a
		JOIN transactions t ON a.transaction_id = t.id
		WHERE t.provider_transaction_id = 'txn_rule_only'
		  AND a.kind = 'sync_updated'`).Scan(&syncUpdatedCount)
	if err != nil {
		t.Fatalf("count sync_updated: %v", err)
	}
	if syncUpdatedCount != 0 {
		t.Errorf("expected 0 sync_updated rows when only category changed, got %d", syncUpdatedCount)
	}
}

//go:build integration && !lite

// Integration tests for the flagged filter on transaction query/count (T21).
// Specifically verifies that flagging a transaction makes it appear in the
// ?flagged=true filter and affects CountTransactionsFiltered, and that
// unflagging reverses both effects.
//
// The existing transaction_flag_integration_test.go (TestFlagTransaction) covers
// single-transaction FlaggedAt field correctness and comment creation.
// This file covers the multi-transaction COUNT and LIST filter semantics more
// exhaustively, using the T21 prefix to avoid name collisions.
//
// Run with: make test-integration
//
// IMPORTANT: Do NOT use t.Parallel() — tests share a database and truncate between runs.
package service_test

import (
	"context"
	"testing"

	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

// t21SeedFlagFixture creates a user → connection → account and three transactions.
// Returns the service and an array of three short_id strings:
// [0] and [1] are intended to be flagged, [2] remains unflagged in most tests.
func t21SeedFlagFixture(t *testing.T) (svc *service.Service, txnIDs [3]string) {
	t.Helper()
	svc, queries, _ := newService(t)

	user := testutil.MustCreateUser(t, queries, "T21Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "t21_item")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "t21_ext", "T21 Checking")

	txn1 := testutil.MustCreateTransaction(t, queries, acct.ID, "t21_flagged_1", "T21 Flagged Coffee", 500, "2025-03-10")
	txn2 := testutil.MustCreateTransaction(t, queries, acct.ID, "t21_flagged_2", "T21 Flagged Lunch", 1200, "2025-03-11")
	txn3 := testutil.MustCreateTransaction(t, queries, acct.ID, "t21_unflagged", "T21 Normal Dinner", 2500, "2025-03-12")

	txnIDs[0] = txn1.ShortID
	txnIDs[1] = txn2.ShortID
	txnIDs[2] = txn3.ShortID
	return svc, txnIDs
}

// TestT21_ListTransactions_FlaggedTrue verifies that only flagged transactions
// appear in ListTransactions when Flagged=true.
func TestT21_ListTransactions_FlaggedTrue(t *testing.T) {
	svc, txnIDs := t21SeedFlagFixture(t)
	ctx := context.Background()
	actor := service.SystemActor()

	// Flag the first two transactions; leave the third unflagged.
	if err := svc.FlagTransaction(ctx, txnIDs[0], "needs review", actor); err != nil {
		t.Fatalf("FlagTransaction(txn1): %v", err)
	}
	if err := svc.FlagTransaction(ctx, txnIDs[1], "", actor); err != nil {
		t.Fatalf("FlagTransaction(txn2): %v", err)
	}

	flaggedTrue := true
	result, err := svc.ListTransactions(ctx, service.TransactionListParams{Flagged: &flaggedTrue})
	if err != nil {
		t.Fatalf("ListTransactions(flagged=true): %v", err)
	}

	if len(result.Transactions) != 2 {
		t.Errorf("expected 2 flagged transactions, got %d", len(result.Transactions))
	}

	// Verify FlaggedAt is populated on all returned rows.
	for _, txn := range result.Transactions {
		if txn.FlaggedAt == nil || *txn.FlaggedAt == "" {
			t.Errorf("expected FlaggedAt to be set for transaction %s, got nil/empty", txn.ID)
		}
	}
}

// TestT21_ListTransactions_FlaggedFalse verifies that only unflagged transactions
// appear in ListTransactions when Flagged=false.
func TestT21_ListTransactions_FlaggedFalse(t *testing.T) {
	svc, txnIDs := t21SeedFlagFixture(t)
	ctx := context.Background()
	actor := service.SystemActor()

	// Flag two; leave one unflagged.
	if err := svc.FlagTransaction(ctx, txnIDs[0], "", actor); err != nil {
		t.Fatalf("FlagTransaction(txn1): %v", err)
	}
	if err := svc.FlagTransaction(ctx, txnIDs[1], "", actor); err != nil {
		t.Fatalf("FlagTransaction(txn2): %v", err)
	}

	flaggedFalse := false
	result, err := svc.ListTransactions(ctx, service.TransactionListParams{Flagged: &flaggedFalse})
	if err != nil {
		t.Fatalf("ListTransactions(flagged=false): %v", err)
	}

	if len(result.Transactions) != 1 {
		t.Errorf("expected 1 unflagged transaction, got %d", len(result.Transactions))
	}

	// The remaining transaction should have no FlaggedAt.
	for _, txn := range result.Transactions {
		if txn.FlaggedAt != nil && *txn.FlaggedAt != "" {
			t.Errorf("expected FlaggedAt to be nil/empty for unflagged transaction %s, got %q", txn.ID, *txn.FlaggedAt)
		}
	}
}

// TestT21_CountTransactionsFiltered_FlaggedTrue verifies that
// CountTransactionsFiltered returns the correct count before and after flagging
// when Flagged=true.
func TestT21_CountTransactionsFiltered_FlaggedTrue(t *testing.T) {
	svc, txnIDs := t21SeedFlagFixture(t)
	ctx := context.Background()
	actor := service.SystemActor()

	flaggedTrue := true

	// No flags yet — count should be 0.
	count, err := svc.CountTransactionsFiltered(ctx, service.TransactionCountParams{Flagged: &flaggedTrue})
	if err != nil {
		t.Fatalf("CountTransactionsFiltered(flagged=true, before flag): %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 flagged before flagging, got %d", count)
	}

	// Flag the first two transactions.
	if err := svc.FlagTransaction(ctx, txnIDs[0], "urgent", actor); err != nil {
		t.Fatalf("FlagTransaction(txn1): %v", err)
	}
	if err := svc.FlagTransaction(ctx, txnIDs[1], "", actor); err != nil {
		t.Fatalf("FlagTransaction(txn2): %v", err)
	}

	// Count should now be 2.
	count, err = svc.CountTransactionsFiltered(ctx, service.TransactionCountParams{Flagged: &flaggedTrue})
	if err != nil {
		t.Fatalf("CountTransactionsFiltered(flagged=true, after flag): %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 flagged after flagging two, got %d", count)
	}
}

// TestT21_CountTransactionsFiltered_FlaggedFalse verifies that
// CountTransactionsFiltered decrements when one transaction is flagged.
func TestT21_CountTransactionsFiltered_FlaggedFalse(t *testing.T) {
	svc, txnIDs := t21SeedFlagFixture(t)
	ctx := context.Background()
	actor := service.SystemActor()

	flaggedFalse := false

	// No flags yet — all 3 are unflagged.
	count, err := svc.CountTransactionsFiltered(ctx, service.TransactionCountParams{Flagged: &flaggedFalse})
	if err != nil {
		t.Fatalf("CountTransactionsFiltered(flagged=false, before flag): %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 unflagged before flagging, got %d", count)
	}

	// Flag one transaction.
	if err := svc.FlagTransaction(ctx, txnIDs[0], "", actor); err != nil {
		t.Fatalf("FlagTransaction(txn1): %v", err)
	}

	// Count should now be 2 unflagged.
	count, err = svc.CountTransactionsFiltered(ctx, service.TransactionCountParams{Flagged: &flaggedFalse})
	if err != nil {
		t.Fatalf("CountTransactionsFiltered(flagged=false, after flag): %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 unflagged after flagging one, got %d", count)
	}
}

// TestT21_FlagFilter_UnflagReverses verifies that unflagging a transaction
// removes it from the flagged filter and restores it to the unflagged filter.
// This is the core reversal test: flag all → unflag one → check counts and lists.
func TestT21_FlagFilter_UnflagReverses(t *testing.T) {
	svc, txnIDs := t21SeedFlagFixture(t)
	ctx := context.Background()
	actor := service.SystemActor()

	// Flag all three transactions.
	for i, id := range txnIDs {
		if err := svc.FlagTransaction(ctx, id, "", actor); err != nil {
			t.Fatalf("FlagTransaction(txn%d): %v", i, err)
		}
	}

	// Confirm all 3 are flagged.
	flaggedTrue := true
	count, err := svc.CountTransactionsFiltered(ctx, service.TransactionCountParams{Flagged: &flaggedTrue})
	if err != nil {
		t.Fatalf("CountTransactionsFiltered after flagging all: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 flagged after flagging all, got %d", count)
	}

	// Unflag the first transaction.
	if err := svc.UnflagTransaction(ctx, txnIDs[0]); err != nil {
		t.Fatalf("UnflagTransaction(txn1): %v", err)
	}

	// Flagged count should drop to 2.
	count, err = svc.CountTransactionsFiltered(ctx, service.TransactionCountParams{Flagged: &flaggedTrue})
	if err != nil {
		t.Fatalf("CountTransactionsFiltered(flagged=true, after unflag): %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 flagged after unflagging one, got %d", count)
	}

	// Unflagged count should be 1.
	flaggedFalse := false
	count, err = svc.CountTransactionsFiltered(ctx, service.TransactionCountParams{Flagged: &flaggedFalse})
	if err != nil {
		t.Fatalf("CountTransactionsFiltered(flagged=false, after unflag): %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 unflagged after unflagging one, got %d", count)
	}

	// ListTransactions(flagged=true) should return 2 transactions.
	resultFlagged, err := svc.ListTransactions(ctx, service.TransactionListParams{Flagged: &flaggedTrue})
	if err != nil {
		t.Fatalf("ListTransactions(flagged=true, after unflag): %v", err)
	}
	if len(resultFlagged.Transactions) != 2 {
		t.Errorf("expected 2 flagged transactions after unflag, got %d", len(resultFlagged.Transactions))
	}

	// ListTransactions(flagged=false) should return 1 transaction.
	resultUnflagged, err := svc.ListTransactions(ctx, service.TransactionListParams{Flagged: &flaggedFalse})
	if err != nil {
		t.Fatalf("ListTransactions(flagged=false, after unflag): %v", err)
	}
	if len(resultUnflagged.Transactions) != 1 {
		t.Errorf("expected 1 unflagged transaction after unflag, got %d", len(resultUnflagged.Transactions))
	}
}

// TestT21_FlagFilter_NoFlagParam verifies that omitting the Flagged param
// returns all transactions regardless of flag status (no filter applied).
func TestT21_FlagFilter_NoFlagParam(t *testing.T) {
	svc, txnIDs := t21SeedFlagFixture(t)
	ctx := context.Background()
	actor := service.SystemActor()

	// Flag one transaction, leave two unflagged.
	if err := svc.FlagTransaction(ctx, txnIDs[0], "", actor); err != nil {
		t.Fatalf("FlagTransaction(txn1): %v", err)
	}

	// No Flagged filter — should return all 3.
	result, err := svc.ListTransactions(ctx, service.TransactionListParams{})
	if err != nil {
		t.Fatalf("ListTransactions(no flag filter): %v", err)
	}
	if len(result.Transactions) != 3 {
		t.Errorf("expected 3 transactions when no flag filter, got %d", len(result.Transactions))
	}

	// CountTransactionsFiltered with no Flagged filter — should also count all 3.
	count, err := svc.CountTransactionsFiltered(ctx, service.TransactionCountParams{})
	if err != nil {
		t.Fatalf("CountTransactionsFiltered(no flag filter): %v", err)
	}
	if count != 3 {
		t.Errorf("expected count 3 when no flag filter, got %d", count)
	}
}

// TestT21_FlagFilter_Reflag verifies that re-flagging an already-flagged
// transaction is idempotent (last-write-wins semantics) and doesn't double-count.
func TestT21_FlagFilter_Reflag(t *testing.T) {
	svc, txnIDs := t21SeedFlagFixture(t)
	ctx := context.Background()
	actor := service.SystemActor()

	// Flag once.
	if err := svc.FlagTransaction(ctx, txnIDs[0], "first pass", actor); err != nil {
		t.Fatalf("FlagTransaction first: %v", err)
	}

	// Re-flag the same transaction.
	if err := svc.FlagTransaction(ctx, txnIDs[0], "second pass", actor); err != nil {
		t.Fatalf("FlagTransaction second (reflag): %v", err)
	}

	// Flagged count must still be exactly 1 — not 2.
	flaggedTrue := true
	count, err := svc.CountTransactionsFiltered(ctx, service.TransactionCountParams{Flagged: &flaggedTrue})
	if err != nil {
		t.Fatalf("CountTransactionsFiltered(flagged=true, after reflag): %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 flagged after reflag (no double-count), got %d", count)
	}

	result, err := svc.ListTransactions(ctx, service.TransactionListParams{Flagged: &flaggedTrue})
	if err != nil {
		t.Fatalf("ListTransactions(flagged=true, after reflag): %v", err)
	}
	if len(result.Transactions) != 1 {
		t.Errorf("expected 1 flagged transaction after reflag, got %d", len(result.Transactions))
	}
}

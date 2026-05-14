//go:build integration

package cli_test

import (
	"context"
	"testing"
	"time"

	"breadbox/internal/client"
	"breadbox/internal/testutil"
)

// strPtr is a tiny helper for *string fields in UpdateTransactionsOp.
func strPtr(s string) *string { return &s }

func TestTransactions_ListFiltersByAccount(t *testing.T) {
	env := setupEnv(t)
	q := env.Queries

	user := testutil.MustCreateUser(t, q, "Tester")
	conn := testutil.MustCreateConnection(t, q, user.ID, "ext-conn-tx-1")
	a := testutil.MustCreateAccount(t, q, conn.ID, "ext-acct-tx-1", "Checking")
	b := testutil.MustCreateAccount(t, q, conn.ID, "ext-acct-tx-2", "Savings")

	today := time.Now().UTC().Format("2006-01-02")
	testutil.MustCreateTransaction(t, q, a.ID, "ext-tx-1", "Coffee", 500, today)
	testutil.MustCreateTransaction(t, q, b.ID, "ext-tx-2", "Rent", 100000, today)

	res, err := env.Client.ListTransactions(context.Background(),
		client.TransactionFilters{Account: a.ID.String()},
		"", 0, "")
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(res.Transactions) != 1 {
		t.Fatalf("got %d transactions, want 1 (filter by account)", len(res.Transactions))
	}
	if res.Transactions[0].ProviderName != "Coffee" {
		t.Errorf("name = %q, want Coffee", res.Transactions[0].ProviderName)
	}
}

func TestTransactions_AtomicUpdateSetsCategory(t *testing.T) {
	env := setupEnv(t)
	q := env.Queries

	user := testutil.MustCreateUser(t, q, "Tester")
	conn := testutil.MustCreateConnection(t, q, user.ID, "ext-conn-tx-3")
	a := testutil.MustCreateAccount(t, q, conn.ID, "ext-acct-tx-3", "Checking")
	cat := testutil.MustCreateCategory(t, q, "food", "Food")
	_ = cat

	today := time.Now().UTC().Format("2006-01-02")
	txn := testutil.MustCreateTransaction(t, q, a.ID, "ext-tx-3", "Coffee", 500, today)

	res, err := env.Client.UpdateTransactions(context.Background(), client.UpdateTransactionsRequest{
		Operations: []client.UpdateTransactionOp{{
			TransactionID: txn.ID.String(),
			CategorySlug:  strPtr("food"),
		}},
	})
	if err != nil {
		t.Fatalf("UpdateTransactions: %v", err)
	}
	if res.Succeeded != 1 {
		t.Fatalf("succeeded = %d, want 1: %+v", res.Succeeded, res)
	}

	got, err := env.Client.GetTransaction(context.Background(), txn.ID.String(), "")
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if got.Category == nil || got.Category.Slug == nil || *got.Category.Slug != "food" {
		t.Fatalf("category slug not applied: %#v", got.Category)
	}
}

func TestTransactions_DeleteRestoreRoundtrip(t *testing.T) {
	env := setupEnv(t)
	q := env.Queries

	user := testutil.MustCreateUser(t, q, "Tester")
	conn := testutil.MustCreateConnection(t, q, user.ID, "ext-conn-tx-4")
	a := testutil.MustCreateAccount(t, q, conn.ID, "ext-acct-tx-4", "Checking")
	today := time.Now().UTC().Format("2006-01-02")
	txn := testutil.MustCreateTransaction(t, q, a.ID, "ext-tx-4", "Bookstore", 1500, today)

	if err := env.Client.DeleteTransaction(context.Background(), txn.ID.String()); err != nil {
		t.Fatalf("DeleteTransaction: %v", err)
	}

	// After delete, the row is filtered from list paths.
	res, err := env.Client.ListTransactions(context.Background(),
		client.TransactionFilters{Account: a.ID.String()}, "", 0, "")
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(res.Transactions) != 0 {
		t.Fatalf("expected 0 transactions after delete, got %d", len(res.Transactions))
	}

	// Restore (path id must be UUID because short_id resolution filters deleted rows).
	if err := env.Client.RestoreTransaction(context.Background(), txn.ID.String()); err != nil {
		t.Fatalf("RestoreTransaction: %v", err)
	}
	res, err = env.Client.ListTransactions(context.Background(),
		client.TransactionFilters{Account: a.ID.String()}, "", 0, "")
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(res.Transactions) != 1 {
		t.Fatalf("expected 1 transaction after restore, got %d", len(res.Transactions))
	}
}

func TestTransactions_TagUntagRoundtrip(t *testing.T) {
	env := setupEnv(t)
	q := env.Queries

	user := testutil.MustCreateUser(t, q, "Tester")
	conn := testutil.MustCreateConnection(t, q, user.ID, "ext-conn-tx-5")
	a := testutil.MustCreateAccount(t, q, conn.ID, "ext-acct-tx-5", "Checking")
	today := time.Now().UTC().Format("2006-01-02")
	txn := testutil.MustCreateTransaction(t, q, a.ID, "ext-tx-5", "Gas Station", 5000, today)

	if _, err := env.Client.AddTransactionTag(context.Background(), txn.ID.String(), "vehicle"); err != nil {
		t.Fatalf("AddTransactionTag: %v", err)
	}
	// Tags are populated by the list-path enrichment, not the get-path —
	// filter the list by the tag to verify attachment.
	res, err := env.Client.ListTransactions(context.Background(),
		client.TransactionFilters{Tags: []string{"vehicle"}}, "", 0, "")
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	foundTagged := false
	for _, r := range res.Transactions {
		if r.ID == txn.ID.String() {
			foundTagged = true
		}
	}
	if !foundTagged {
		t.Fatalf("expected tagged transaction in tag-filtered list, got %d rows", len(res.Transactions))
	}

	if _, err := env.Client.RemoveTransactionTag(context.Background(), txn.ID.String(), "vehicle"); err != nil {
		t.Fatalf("RemoveTransactionTag: %v", err)
	}
	res, err = env.Client.ListTransactions(context.Background(),
		client.TransactionFilters{Tags: []string{"vehicle"}}, "", 0, "")
	if err != nil {
		t.Fatalf("ListTransactions after untag: %v", err)
	}
	if len(res.Transactions) != 0 {
		t.Fatalf("expected 0 tagged transactions after removal, got %d", len(res.Transactions))
	}
}

func TestTransactions_CommentsRoundtrip(t *testing.T) {
	env := setupEnv(t)
	q := env.Queries

	user := testutil.MustCreateUser(t, q, "Tester")
	conn := testutil.MustCreateConnection(t, q, user.ID, "ext-conn-tx-6")
	a := testutil.MustCreateAccount(t, q, conn.ID, "ext-acct-tx-6", "Checking")
	today := time.Now().UTC().Format("2006-01-02")
	txn := testutil.MustCreateTransaction(t, q, a.ID, "ext-tx-6", "Grocery", 7500, today)

	com, err := env.Client.CreateComment(context.Background(), txn.ID.String(), "test comment")
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if com.Content != "test comment" {
		t.Errorf("content = %q, want %q", com.Content, "test comment")
	}

	coms, err := env.Client.ListComments(context.Background(), txn.ID.String())
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(coms) != 1 || coms[0].ID != com.ID {
		t.Fatalf("expected 1 comment matching created id, got %#v", coms)
	}
}

//go:build integration && !lite

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

func TestFlagTransaction(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "txn_flag", "Suspicious Co", 99900, "2026-04-01")
	other := testutil.MustCreateTransaction(t, queries, acctID, "txn_plain", "Normal Co", 1200, "2026-04-02")
	id := txn.ShortID
	agent := service.Actor{Type: "agent", ID: "agent-1", Name: "Routine Reviewer"}

	// Fresh transaction is not flagged.
	if got, _ := svc.GetTransaction(ctx, id); got.FlaggedAt != nil {
		t.Fatalf("fresh transaction FlaggedAt = %v, want nil", *got.FlaggedAt)
	}

	// Flag with a reason -> flagged_at set + the reason recorded as a comment.
	if err := svc.FlagTransaction(ctx, id, "amount looks high for this merchant", agent); err != nil {
		t.Fatalf("FlagTransaction: %v", err)
	}
	got, err := svc.GetTransaction(ctx, id)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if got.FlaggedAt == nil {
		t.Fatalf("FlaggedAt is nil after flagging")
	}
	comments, err := svc.ListComments(ctx, id)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	var foundReason bool
	for _, c := range comments {
		if c.Content == "amount looks high for this merchant" {
			foundReason = true
		}
	}
	if !foundReason {
		t.Fatalf("flag reason was not recorded as a comment; comments=%+v", comments)
	}

	// query_transactions(flagged=true) returns the flagged row, not the other.
	tru, fls := true, false
	flagged, err := svc.ListTransactions(ctx, service.TransactionListParams{Flagged: &tru, Limit: 50})
	if err != nil {
		t.Fatalf("ListTransactions(flagged=true): %v", err)
	}
	if !containsTxn(flagged.Transactions, id) || containsTxn(flagged.Transactions, other.ShortID) {
		t.Fatalf("flagged=true returned wrong set: %v", txnIDs(flagged.Transactions))
	}
	unflaggedList, err := svc.ListTransactions(ctx, service.TransactionListParams{Flagged: &fls, Limit: 50})
	if err != nil {
		t.Fatalf("ListTransactions(flagged=false): %v", err)
	}
	if containsTxn(unflaggedList.Transactions, id) || !containsTxn(unflaggedList.Transactions, other.ShortID) {
		t.Fatalf("flagged=false returned wrong set: %v", txnIDs(unflaggedList.Transactions))
	}

	// Unflag clears it.
	if err := svc.UnflagTransaction(ctx, id); err != nil {
		t.Fatalf("UnflagTransaction: %v", err)
	}
	if got, _ := svc.GetTransaction(ctx, id); got.FlaggedAt != nil {
		t.Fatalf("FlaggedAt = %v after unflag, want nil", *got.FlaggedAt)
	}

	// Flagging a missing transaction is ErrNotFound.
	if err := svc.FlagTransaction(ctx, "nonexistent", "", agent); err == nil {
		t.Fatalf("FlagTransaction(missing) = nil, want error")
	}
}

func containsTxn(rows []service.TransactionResponse, shortID string) bool {
	for _, r := range rows {
		if r.ShortID == shortID || r.ID == shortID {
			return true
		}
	}
	return false
}

func txnIDs(rows []service.TransactionResponse) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ShortID)
	}
	return out
}

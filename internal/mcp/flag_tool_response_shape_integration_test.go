//go:build integration && !lite

package mcp

// Regression harness for flagging through update_transactions (T18).
//
// flag_transaction / unflag_transaction were folded into update_transactions
// as the per-op `flagged` field. These tests lock that the folded path still
// has the same observable behavior: flagged_at is set/cleared in the DB, an
// accompanying comment lands as the flag reason, and query_transactions
// (flagged=…) filters on it.
//
// Coverage:
//   - flagged:true sets flagged_at (side-effect)
//   - flagged:true + comment records the reason as a comment annotation
//   - flagged:false clears flagged_at
//   - flagged:false on an already-unflagged row is a no-op success
//   - an op with a nonexistent transaction_id fails that op (not a panic)
//   - query_transactions(flagged=true) surfaces only the flagged row

import (
	"testing"

	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

// T18FlagFreshTxn creates a fresh transaction on the seeded primary account
// and returns its UUID string.
func T18FlagFreshTxn(t *testing.T, f *fixtures) string {
	t.Helper()
	q := f.svc.svc.Queries
	accts, err := f.svc.svc.ListAccounts(f.ctx, nil)
	if err != nil || len(accts) == 0 {
		t.Fatalf("T18FlagFreshTxn: ListAccounts: err=%v n=%d", err, len(accts))
	}
	var primaryShort string
	for _, a := range accts {
		if a.Name == "Primary Credit Card" {
			primaryShort = a.ShortID
			break
		}
	}
	if primaryShort == "" {
		t.Fatalf("T18FlagFreshTxn: could not find Primary Credit Card in accounts")
	}
	primaryUUID, err := q.GetAccountUUIDByShortID(f.ctx, primaryShort)
	if err != nil {
		t.Fatalf("T18FlagFreshTxn: GetAccountUUIDByShortID: %v", err)
	}
	txn := testutil.MustCreateTransaction(t, q, primaryUUID, "txn_t18_flag", "T18 Merchant", 7700, "2026-04-20")
	return formatUUIDTest(t, txn.ID)
}

// t18Flag drives a single flag/unflag op through update_transactions and
// returns the decoded response envelope.
func t18Flag(t *testing.T, f *fixtures, txnID string, flagged bool, comment string) map[string]any {
	t.Helper()
	op := transactionOperationInput{TransactionID: txnID, Flagged: &flagged}
	if comment != "" {
		op.Comment = &comment
	}
	res, _, err := f.svc.handleUpdateTransactions(f.ctx, nil, updateTransactionsInput{
		Operations: []transactionOperationInput{op},
	})
	return decodeToolResult[map[string]any](t, "T18:update_transactions flag", res, err)
}

// TestT18FlagSetsFlaggedAt asserts that flagged:true via update_transactions
// actually sets flagged_at in the DB.
func TestT18FlagSetsFlaggedAt(t *testing.T) {
	f := seedFixtures(t)
	txnID := T18FlagFreshTxn(t, f)

	before, err := f.svc.svc.GetTransaction(f.ctx, txnID)
	if err != nil {
		t.Fatalf("T18:flag: GetTransaction before: %v", err)
	}
	if before.FlaggedAt != nil {
		t.Fatalf("T18:flag: fresh transaction FlaggedAt=%v, want nil", *before.FlaggedAt)
	}

	out := t18Flag(t, f, txnID, true, "")
	requireKeys(t, "T18:flag", out, "results", "succeeded", "failed")
	if succeeded, _ := out["succeeded"].(float64); succeeded != 1 {
		t.Errorf("T18:flag: succeeded=%v, want 1", out["succeeded"])
	}

	after, err := f.svc.svc.GetTransaction(f.ctx, txnID)
	if err != nil {
		t.Fatalf("T18:flag: GetTransaction after: %v", err)
	}
	if after.FlaggedAt == nil {
		t.Fatalf("T18:flag: FlaggedAt is nil after flagged:true — fold dropped the flag write")
	}
}

// TestT18FlagWithReason ensures a comment paired with flagged:true persists as
// a comment annotation on the timeline (the fold's replacement for the old
// flag `reason` field).
func TestT18FlagWithReason(t *testing.T) {
	f := seedFixtures(t)
	txnID := T18FlagFreshTxn(t, f)

	const reason = "T18: amount looks high for this merchant"
	t18Flag(t, f, txnID, true, reason)

	anns, err := f.svc.svc.ListAnnotations(f.ctx, txnID, service.ListAnnotationsParams{})
	if err != nil {
		t.Fatalf("T18:flag_with_reason: ListAnnotations: %v", err)
	}
	var foundReason bool
	for _, a := range anns {
		if a.Content == reason {
			foundReason = true
			break
		}
	}
	if !foundReason {
		t.Errorf("T18:flag_with_reason: reason %q not found in annotations; got %d annotation(s)", reason, len(anns))
	}
}

// TestT18UnflagClearsFlaggedAt asserts flagged:false clears flagged_at.
func TestT18UnflagClearsFlaggedAt(t *testing.T) {
	f := seedFixtures(t)
	txnID := T18FlagFreshTxn(t, f)

	t18Flag(t, f, txnID, true, "")
	mid, err := f.svc.svc.GetTransaction(f.ctx, txnID)
	if err != nil {
		t.Fatalf("T18:unflag: GetTransaction mid: %v", err)
	}
	if mid.FlaggedAt == nil {
		t.Fatalf("T18:unflag: FlaggedAt is nil after flag — seeding failed")
	}

	t18Flag(t, f, txnID, false, "")
	after, err := f.svc.svc.GetTransaction(f.ctx, txnID)
	if err != nil {
		t.Fatalf("T18:unflag: GetTransaction after: %v", err)
	}
	if after.FlaggedAt != nil {
		t.Errorf("T18:unflag: FlaggedAt=%v after flagged:false, want nil", *after.FlaggedAt)
	}
}

// TestT18UnflagIdempotent asserts flagged:false on a never-flagged row
// succeeds without error.
func TestT18UnflagIdempotent(t *testing.T) {
	f := seedFixtures(t)
	txnID := T18FlagFreshTxn(t, f)

	out := t18Flag(t, f, txnID, false, "")
	if succeeded, _ := out["succeeded"].(float64); succeeded != 1 {
		t.Errorf("T18:unflag_idempotent: succeeded=%v, want 1", out["succeeded"])
	}
}

// TestT18FlagNonexistentTransaction asserts that flagging an unknown id fails
// that op (failed=1) rather than panicking or silently succeeding.
func TestT18FlagNonexistentTransaction(t *testing.T) {
	f := seedFixtures(t)

	out := t18Flag(t, f, "zzzzzzzz", true, "")
	if failed, _ := out["failed"].(float64); failed != 1 {
		t.Errorf("T18:flag_nonexistent: failed=%v, want 1", out["failed"])
	}
}

// TestT18QueryTransactionsFlaggedFilter exercises query_transactions with
// flagged=true/false after flagging a fresh transaction via update_transactions.
func TestT18QueryTransactionsFlaggedFilter(t *testing.T) {
	f := seedFixtures(t)
	txnID := T18FlagFreshTxn(t, f)

	t18Flag(t, f, txnID, true, "")

	flaggedRecord, err := f.svc.svc.GetTransaction(f.ctx, txnID)
	if err != nil {
		t.Fatalf("T18:query_flagged: GetTransaction: %v", err)
	}
	flaggedShort := flaggedRecord.ShortID

	tru := true
	flaggedQRes, _, flaggedQErr := f.svc.handleQueryTransactions(f.ctx, nil, queryTransactionsInput{
		Flagged: &tru,
		Limit:   500,
	})
	flaggedQOut := decodeToolResult[map[string]any](t, "T18:query_flagged", flaggedQRes, flaggedQErr)
	requireKeys(t, "T18:query_flagged", flaggedQOut, "transactions", "has_more", "limit")

	flaggedTxns := asArray(t, "T18:query_flagged.transactions", flaggedQOut["transactions"])
	if len(flaggedTxns) == 0 {
		t.Fatal("T18:query_flagged: expected at least the flagged transaction, got 0 rows")
	}
	var foundInFlagged bool
	for _, raw := range flaggedTxns {
		row := asObject(t, "T18:query_flagged row", raw)
		requireKeys(t, "T18:query_flagged row", row, "id")
		if id, _ := row["id"].(string); id == flaggedShort {
			foundInFlagged = true
		}
	}
	if !foundInFlagged {
		t.Errorf("T18:query_flagged: short_id %q not found in flagged=true result set", flaggedShort)
	}

	fls := false
	unflaggedQRes, _, unflaggedQErr := f.svc.handleQueryTransactions(f.ctx, nil, queryTransactionsInput{
		Flagged: &fls,
		Limit:   500,
	})
	unflaggedQOut := decodeToolResult[map[string]any](t, "T18:query_unflagged", unflaggedQRes, unflaggedQErr)
	unflaggedTxns := asArray(t, "T18:query_unflagged.transactions", unflaggedQOut["transactions"])
	for _, raw := range unflaggedTxns {
		row := asObject(t, "T18:query_unflagged row", raw)
		if id, _ := row["id"].(string); id == flaggedShort {
			t.Errorf("T18:query_unflagged: flagged transaction %q appeared in flagged=false result set", flaggedShort)
		}
	}
}

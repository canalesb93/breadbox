//go:build integration && !lite

package mcp

// Regression harness for the flag_transaction / unflag_transaction MCP tool
// response shapes (T18).
//
// The goals mirror response_shapes_integration_test.go: lock the JSON envelope
// every caller / SDK generator relies on. The asserts are intentionally loose on
// values but strict on shape — if someone renames `flagged` → `is_flagged`, or
// drops `transaction_id` from the envelope, the test breaks in the same PR.
//
// Coverage:
//   - flag_transaction: required keys present (transaction_id, flagged=true)
//   - flag_transaction with reason: reason recorded as a comment annotation
//   - unflag_transaction: required keys present (transaction_id, flagged=false)
//   - flag_transaction -> GetTransaction: flagged_at reflected in side-effect
//   - unflag_transaction side-effect: flagged_at cleared (nil)
//   - Missing transaction_id: error envelope returned
//   - Missing transaction row: error envelope returned
//   - StructuredContent / TextContent parity (via decodeToolResult)
//   - query_transactions(flagged=true) surfaces only the flagged row

import (
	"testing"

	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

// T18FlagFreshTxn creates a fresh transaction on the seeded primary account
// and returns its UUID string. Keeps the test bodies from duplicating fixture
// plumbing and avoids naming collisions with the shared seedFixtures txn used
// by other parallel suites.
func T18FlagFreshTxn(t *testing.T, f *fixtures) string {
	t.Helper()
	// Reuse the queries handle from the fixture server to stay within the same
	// truncated DB state seedFixtures created.
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

// TestT18FlagTransactionResponseShape pins the flag_transaction tool's JSON
// envelope: required keys transaction_id and flagged=true must both be present
// for every successful call. Also locks the StructuredContent / TextContent
// parity contract (enforced by decodeToolResult).
func TestT18FlagTransactionResponseShape(t *testing.T) {
	f := seedFixtures(t)
	txnID := T18FlagFreshTxn(t, f)

	res, _, err := f.svc.handleFlagTransaction(f.ctx, nil, flagTransactionInput{
		TransactionID: txnID,
	})
	out := decodeToolResult[map[string]any](t, "T18:flag_transaction", res, err)

	requireKeys(t, "T18:flag_transaction", out, "transaction_id", "flagged")

	if flagged, _ := out["flagged"].(bool); !flagged {
		t.Errorf("T18:flag_transaction: flagged=%v, want true", out["flagged"])
	}
	if tid, _ := out["transaction_id"].(string); tid == "" {
		t.Errorf("T18:flag_transaction: transaction_id is empty")
	}
}

// TestT18FlagTransactionSideEffect asserts that flagging a transaction actually
// sets flagged_at in the DB. A MCP wrapper that returned the success envelope
// without calling the service would look correct in the tool response but fail
// here when we read back the transaction.
func TestT18FlagTransactionSideEffect(t *testing.T) {
	f := seedFixtures(t)
	txnID := T18FlagFreshTxn(t, f)

	// Confirm baseline: fresh transaction is unflagged.
	before, err := f.svc.svc.GetTransaction(f.ctx, txnID)
	if err != nil {
		t.Fatalf("T18:side-effect: GetTransaction before: %v", err)
	}
	if before.FlaggedAt != nil {
		t.Fatalf("T18:side-effect: fresh transaction FlaggedAt=%v, want nil", *before.FlaggedAt)
	}

	flagRes, _, flagErr := f.svc.handleFlagTransaction(f.ctx, nil, flagTransactionInput{
		TransactionID: txnID,
	})
	decodeToolResult[map[string]any](t, "T18:flag_transaction side-effect", flagRes, flagErr)

	after, err := f.svc.svc.GetTransaction(f.ctx, txnID)
	if err != nil {
		t.Fatalf("T18:side-effect: GetTransaction after: %v", err)
	}
	if after.FlaggedAt == nil {
		t.Fatalf("T18:side-effect: FlaggedAt is nil after flag_transaction — wrapper likely dropped the service call")
	}
}

// TestT18FlagTransactionWithReason ensures that passing a non-empty reason
// persists it as a comment annotation on the transaction timeline. The flag
// tool doc says "recorded as a comment annotation"; this test locks that the
// MCP wrapper actually forwards the reason string to the service.
func TestT18FlagTransactionWithReason(t *testing.T) {
	f := seedFixtures(t)
	txnID := T18FlagFreshTxn(t, f)

	const reason = "T18: amount looks high for this merchant"

	flagRes, _, flagErr := f.svc.handleFlagTransaction(f.ctx, nil, flagTransactionInput{
		TransactionID: txnID,
		Reason:        reason,
	})
	decodeToolResult[map[string]any](t, "T18:flag_with_reason", flagRes, flagErr)

	// The reason should surface as a comment annotation on the timeline.
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

// TestT18UnflagTransactionResponseShape pins the unflag_transaction tool's JSON
// envelope: required keys transaction_id and flagged=false must both be present.
// Also locks the StructuredContent / TextContent parity contract.
func TestT18UnflagTransactionResponseShape(t *testing.T) {
	f := seedFixtures(t)
	txnID := T18FlagFreshTxn(t, f)

	// Flag first so there is something to unflag.
	flagRes, _, flagErr := f.svc.handleFlagTransaction(f.ctx, nil, flagTransactionInput{
		TransactionID: txnID,
	})
	decodeToolResult[map[string]any](t, "T18:unflag setup flag", flagRes, flagErr)

	res, _, err := f.svc.handleUnflagTransaction(f.ctx, nil, unflagTransactionInput{
		TransactionID: txnID,
	})
	out := decodeToolResult[map[string]any](t, "T18:unflag_transaction", res, err)

	requireKeys(t, "T18:unflag_transaction", out, "transaction_id", "flagged")

	if flagged, _ := out["flagged"].(bool); flagged {
		t.Errorf("T18:unflag_transaction: flagged=%v, want false", out["flagged"])
	}
	if tid, _ := out["transaction_id"].(string); tid == "" {
		t.Errorf("T18:unflag_transaction: transaction_id is empty")
	}
}

// TestT18UnflagTransactionSideEffect asserts that unflag_transaction actually
// clears flagged_at in the DB. A wrapper that returned flagged=false without
// calling the service would be silently broken.
func TestT18UnflagTransactionSideEffect(t *testing.T) {
	f := seedFixtures(t)
	txnID := T18FlagFreshTxn(t, f)

	// Seed a flag first via the tool handler.
	flagRes, _, flagErr := f.svc.handleFlagTransaction(f.ctx, nil, flagTransactionInput{
		TransactionID: txnID,
	})
	decodeToolResult[map[string]any](t, "T18:unflag side-effect: flag setup", flagRes, flagErr)

	// Confirm it is now flagged in the DB.
	mid, err := f.svc.svc.GetTransaction(f.ctx, txnID)
	if err != nil {
		t.Fatalf("T18:unflag side-effect: GetTransaction mid: %v", err)
	}
	if mid.FlaggedAt == nil {
		t.Fatalf("T18:unflag side-effect: FlaggedAt is nil after flag — seeding failed")
	}

	// Now unflag via the MCP tool handler.
	unflagRes, _, unflagErr := f.svc.handleUnflagTransaction(f.ctx, nil, unflagTransactionInput{
		TransactionID: txnID,
	})
	decodeToolResult[map[string]any](t, "T18:unflag side-effect", unflagRes, unflagErr)

	after, err := f.svc.svc.GetTransaction(f.ctx, txnID)
	if err != nil {
		t.Fatalf("T18:unflag side-effect: GetTransaction after: %v", err)
	}
	if after.FlaggedAt != nil {
		t.Errorf("T18:unflag side-effect: FlaggedAt=%v after unflag, want nil — wrapper likely dropped the service call", *after.FlaggedAt)
	}
}

// TestT18UnflagIdempotent asserts that unflagging an already-unflagged
// transaction succeeds without error (the tool doc says "No-op if it isn't
// flagged"). The response shape contract still holds.
func TestT18UnflagIdempotent(t *testing.T) {
	f := seedFixtures(t)
	txnID := T18FlagFreshTxn(t, f)

	// txnID is already unflagged — calling unflag must succeed.
	res, _, err := f.svc.handleUnflagTransaction(f.ctx, nil, unflagTransactionInput{
		TransactionID: txnID,
	})
	out := decodeToolResult[map[string]any](t, "T18:unflag_idempotent", res, err)
	requireKeys(t, "T18:unflag_idempotent", out, "transaction_id", "flagged")
}

// TestT18FlagMissingTransactionID asserts that omitting transaction_id returns
// the error envelope (IsError=true) rather than a 500 or a nil crash.
func TestT18FlagMissingTransactionID(t *testing.T) {
	f := seedFixtures(t)

	res, _, _ := f.svc.handleFlagTransaction(f.ctx, nil, flagTransactionInput{
		TransactionID: "",
	})
	if res == nil || !res.IsError {
		t.Fatalf("T18:flag_missing_id: expected error envelope for empty transaction_id, got %+v", res)
	}
}

// TestT18UnflagMissingTransactionID mirrors the same contract for the unflag
// tool.
func TestT18UnflagMissingTransactionID(t *testing.T) {
	f := seedFixtures(t)

	res, _, _ := f.svc.handleUnflagTransaction(f.ctx, nil, unflagTransactionInput{
		TransactionID: "",
	})
	if res == nil || !res.IsError {
		t.Fatalf("T18:unflag_missing_id: expected error envelope for empty transaction_id, got %+v", res)
	}
}

// TestT18FlagNonexistentTransaction asserts that a syntactically valid but
// non-existent ID returns the error envelope instead of a nil result or panic.
func TestT18FlagNonexistentTransaction(t *testing.T) {
	f := seedFixtures(t)

	res, _, _ := f.svc.handleFlagTransaction(f.ctx, nil, flagTransactionInput{
		TransactionID: "zzzzzzzz",
	})
	if res == nil || !res.IsError {
		t.Fatalf("T18:flag_nonexistent: expected error envelope for unknown transaction, got %+v", res)
	}
}

// TestT18QueryTransactionsFlaggedFilter exercises query_transactions with
// flagged=true after seeding a flagged and an unflagged transaction. The
// filter must return only the flagged row — locking that the flagged_at field
// is actually populated and filterable by agents using the MCP tool.
func TestT18QueryTransactionsFlaggedFilter(t *testing.T) {
	f := seedFixtures(t)
	txnID := T18FlagFreshTxn(t, f)

	// Flag the fresh txn via the tool handler.
	flagRes, _, flagErr := f.svc.handleFlagTransaction(f.ctx, nil, flagTransactionInput{
		TransactionID: txnID,
	})
	decodeToolResult[map[string]any](t, "T18:query_flagged: flag setup", flagRes, flagErr)

	// Resolve the short_id so we can look for it in both result sets.
	flaggedRecord, err := f.svc.svc.GetTransaction(f.ctx, txnID)
	if err != nil {
		t.Fatalf("T18:query_flagged: GetTransaction: %v", err)
	}
	flaggedShort := flaggedRecord.ShortID

	// query_transactions(flagged=true) — must include our flagged txn.
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

	// query_transactions(flagged=false) — the flagged txn must NOT appear.
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

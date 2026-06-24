//go:build integration && !lite

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

// summaryCountForUser is a small helper: count transactions a user is the
// effective owner of, over a wide date window, via GetTransactionSummary.
func summaryCountForUser(t *testing.T, svc *service.Service, userUUID string) int64 {
	t.Helper()
	start := testutil.MustParseDate("2025-01-01")
	end := testutil.MustParseDate("2025-12-31")
	res, err := svc.GetTransactionSummary(context.Background(), service.TransactionSummaryParams{
		GroupBy:   "month",
		StartDate: &start,
		EndDate:   &end,
		UserID:    &userUUID,
	})
	if err != nil {
		t.Fatalf("GetTransactionSummary(user=%s): %v", userUUID, err)
	}
	return res.Totals.TransactionCount
}

// listCountForUser counts transactions returned by ListTransactions filtered
// to a user (exercises the WHERE-clause COALESCE chain).
func listCountForUser(t *testing.T, svc *service.Service, userUUID string) int {
	t.Helper()
	res, err := svc.ListTransactions(context.Background(), service.TransactionListParams{
		Limit:  100,
		UserID: &userUUID,
	})
	if err != nil {
		t.Fatalf("ListTransactions(user=%s): %v", userUUID, err)
	}
	return len(res.Transactions)
}

// TestUpdateAccount_OwnerOverride_RoutesAttribution is the core behavior: an
// account whose owner is overridden re-routes its existing transactions to the
// new owner across summaries and list queries, and clearing the override
// restores them to the connection owner — all at read time, no backfill.
func TestUpdateAccount_OwnerOverride_RoutesAttribution(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	connOwner := testutil.MustCreateUser(t, queries, "Connection Owner")
	other := testutil.MustCreateUser(t, queries, "Wife")
	conn := testutil.MustCreateConnection(t, queries, connOwner.ID, "item_owner_override")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_owner_override", "Her Checking")
	testutil.MustCreateTransaction(t, queries, acct.ID, "txn_owner_1", "Groceries", 4200, "2025-03-10")

	connOwnerID := pgconv.FormatUUID(connOwner.ID)
	otherID := pgconv.FormatUUID(other.ID)
	acctID := pgconv.FormatUUID(acct.ID)

	// Before any override: the connection owner owns the transaction.
	if got := summaryCountForUser(t, svc, connOwnerID); got != 1 {
		t.Fatalf("pre-override summary for conn owner: got %d, want 1", got)
	}
	if got := summaryCountForUser(t, svc, otherID); got != 0 {
		t.Fatalf("pre-override summary for other: got %d, want 0", got)
	}

	// Override the account's owner to the other member.
	otherShort := other.ShortID
	if _, err := svc.UpdateAccount(ctx, acctID, service.UpdateAccountParams{OwnerUserID: &otherShort}); err != nil {
		t.Fatalf("UpdateAccount set owner: %v", err)
	}

	// The transaction now belongs to the override owner — in both the summary
	// (WHERE) and list (WHERE) paths.
	if got := summaryCountForUser(t, svc, otherID); got != 1 {
		t.Errorf("post-override summary for other: got %d, want 1", got)
	}
	if got := summaryCountForUser(t, svc, connOwnerID); got != 0 {
		t.Errorf("post-override summary for conn owner: got %d, want 0", got)
	}
	if got := listCountForUser(t, svc, otherID); got != 1 {
		t.Errorf("post-override list for other: got %d, want 1", got)
	}
	if got := listCountForUser(t, svc, connOwnerID); got != 0 {
		t.Errorf("post-override list for conn owner: got %d, want 0", got)
	}

	// Clearing the override (empty string) returns the transaction to the
	// connection owner.
	empty := ""
	if _, err := svc.UpdateAccount(ctx, acctID, service.UpdateAccountParams{OwnerUserID: &empty}); err != nil {
		t.Fatalf("UpdateAccount clear owner: %v", err)
	}
	if got := summaryCountForUser(t, svc, connOwnerID); got != 1 {
		t.Errorf("post-clear summary for conn owner: got %d, want 1", got)
	}
	if got := summaryCountForUser(t, svc, otherID); got != 0 {
		t.Errorf("post-clear summary for other: got %d, want 0", got)
	}
}

// TestUpdateAccount_OwnerOverride_SetClearRoundtrip asserts the GetAccount
// response surfaces the override owner and that clearing returns to inherit.
func TestUpdateAccount_OwnerOverride_SetClearRoundtrip(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	connOwner := testutil.MustCreateUser(t, queries, "Owner")
	other := testutil.MustCreateUser(t, queries, "Override Member")
	conn := testutil.MustCreateConnection(t, queries, connOwner.ID, "item_owner_roundtrip")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_owner_roundtrip", "Acct")
	acctID := pgconv.FormatUUID(acct.ID)

	// Initially inherits — no override.
	got, err := svc.GetAccount(ctx, acctID)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if got.OwnerUserID != nil {
		t.Errorf("expected nil owner override initially, got %v", *got.OwnerUserID)
	}

	// Set via short_id.
	otherShort := other.ShortID
	if _, err := svc.UpdateAccount(ctx, acctID, service.UpdateAccountParams{OwnerUserID: &otherShort}); err != nil {
		t.Fatalf("UpdateAccount set: %v", err)
	}
	got, err = svc.GetAccount(ctx, acctID)
	if err != nil {
		t.Fatalf("GetAccount after set: %v", err)
	}
	if got.OwnerUserID == nil || *got.OwnerUserID != other.ShortID {
		t.Errorf("owner short_id: got %v, want %s", got.OwnerUserID, other.ShortID)
	}
	if got.OwnerUserName == nil || *got.OwnerUserName != "Override Member" {
		t.Errorf("owner name: got %v, want Override Member", got.OwnerUserName)
	}

	// Clear.
	empty := ""
	if _, err := svc.UpdateAccount(ctx, acctID, service.UpdateAccountParams{OwnerUserID: &empty}); err != nil {
		t.Fatalf("UpdateAccount clear: %v", err)
	}
	got, err = svc.GetAccount(ctx, acctID)
	if err != nil {
		t.Fatalf("GetAccount after clear: %v", err)
	}
	if got.OwnerUserID != nil {
		t.Errorf("expected nil owner override after clear, got %v", *got.OwnerUserID)
	}
}

// TestListAccountsByUser_RespectsOwnerOverride verifies "my accounts" follows
// the effective owner: an overridden account shows up under the new owner and
// disappears from the connection owner's list.
func TestListAccountsByUser_RespectsOwnerOverride(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	connOwner := testutil.MustCreateUser(t, queries, "Conn Owner")
	other := testutil.MustCreateUser(t, queries, "Other Member")
	conn := testutil.MustCreateConnection(t, queries, connOwner.ID, "item_accts_by_user")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_accts_by_user", "Shared Bridge Acct")
	acctID := pgconv.FormatUUID(acct.ID)
	connOwnerID := pgconv.FormatUUID(connOwner.ID)
	otherID := pgconv.FormatUUID(other.ID)

	// Before override: belongs to the connection owner only.
	if n := accountsForUser(t, svc, connOwnerID); n != 1 {
		t.Fatalf("pre-override accounts for conn owner: got %d, want 1", n)
	}
	if n := accountsForUser(t, svc, otherID); n != 0 {
		t.Fatalf("pre-override accounts for other: got %d, want 0", n)
	}

	otherShort := other.ShortID
	if _, err := svc.UpdateAccount(ctx, acctID, service.UpdateAccountParams{OwnerUserID: &otherShort}); err != nil {
		t.Fatalf("UpdateAccount set owner: %v", err)
	}

	if n := accountsForUser(t, svc, otherID); n != 1 {
		t.Errorf("post-override accounts for other: got %d, want 1", n)
	}
	if n := accountsForUser(t, svc, connOwnerID); n != 0 {
		t.Errorf("post-override accounts for conn owner: got %d, want 0", n)
	}
}

func accountsForUser(t *testing.T, svc *service.Service, userUUID string) int {
	t.Helper()
	accts, err := svc.ListAccounts(context.Background(), &userUUID)
	if err != nil {
		t.Fatalf("ListAccounts(user=%s): %v", userUUID, err)
	}
	return len(accts)
}

// TestListAccountsByUser_OwnerOverride_NullConnection guards the edge case
// where a connection was removed (connection_id SET NULL, account preserved for
// history) but the account carries an owner override. The effective-owner list
// must still surface it — ListAccountsByUser LEFT-JOINs bank_connections so the
// override resolves even without a connection.
func TestListAccountsByUser_OwnerOverride_NullConnection(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()

	connOwner := testutil.MustCreateUser(t, queries, "Conn Owner")
	other := testutil.MustCreateUser(t, queries, "Override Member")
	conn := testutil.MustCreateConnection(t, queries, connOwner.ID, "item_null_conn")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_null_conn", "Orphaned Acct")
	acctID := pgconv.FormatUUID(acct.ID)
	otherID := pgconv.FormatUUID(other.ID)

	// Reassign owner, then sever the connection (mimicking a removed bank).
	otherShort := other.ShortID
	if _, err := svc.UpdateAccount(ctx, acctID, service.UpdateAccountParams{OwnerUserID: &otherShort}); err != nil {
		t.Fatalf("UpdateAccount set owner: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE accounts SET connection_id = NULL WHERE id = $1`, acct.ID); err != nil {
		t.Fatalf("null out connection: %v", err)
	}

	if n := accountsForUser(t, svc, otherID); n != 1 {
		t.Errorf("override owner should still see the connection-less account: got %d, want 1", n)
	}
}

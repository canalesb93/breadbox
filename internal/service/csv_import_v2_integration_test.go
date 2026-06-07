//go:build integration && !lite

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

func countTxns(t *testing.T, svc *service.Service, accountID string) int {
	t.Helper()
	uid, err := pgconv.ParseUUID(accountID)
	if err != nil {
		t.Fatalf("parse account id: %v", err)
	}
	var n int
	if err := svc.Pool.QueryRow(context.Background(),
		"SELECT count(*) FROM transactions WHERE account_id = $1 AND deleted_at IS NULL", uid).Scan(&n); err != nil {
		t.Fatalf("count txns: %v", err)
	}
	return n
}

func TestImportV2_FullLifecycleAndProfileRedrop(t *testing.T) {
	svc, q, _ := newService(t)
	ctx := context.Background()
	user := testutil.MustCreateUser(t, q, "Alice")

	file1 := []byte("Date,Amount,Description\n2026-07-01,10.00,COFFEE\n2026-07-02,20.00,LUNCH\n")

	// Analyze — no accounts yet, so it awaits account resolution.
	an, err := svc.CreateImportSession(ctx, service.CreateImportSessionParams{
		UserID: pgconv.FormatUUID(user.ID), Filename: "chase.csv", Data: file1,
	})
	if err != nil {
		t.Fatalf("CreateImportSession: %v", err)
	}
	if an.Session.Status != "awaiting_account" {
		t.Fatalf("status = %q, want awaiting_account", an.Session.Status)
	}

	// Resolve by creating a new account.
	sess, err := svc.ResolveImportAccount(ctx, an.Session.ShortID, service.ResolveImportAccountParams{
		CreateNew: true, NewName: "Chase Checking",
	})
	if err != nil {
		t.Fatalf("ResolveImportAccount: %v", err)
	}
	if sess.Status != "previewed" || sess.ResolvedAccountID == "" {
		t.Fatalf("after resolve: status=%q account=%q", sess.Status, sess.ResolvedAccountID)
	}
	acctA := sess.ResolvedAccountID

	_, summary, err := svc.ListImportRows(ctx, sess.ShortID, "", 1, 100)
	if err != nil {
		t.Fatalf("ListImportRows: %v", err)
	}
	if summary.Counts["new"] != 2 || summary.IncludedCount != 2 {
		t.Fatalf("summary = %+v, want 2 new/included", summary)
	}

	// Apply.
	res, err := svc.ApplyImportSession(ctx, sess.ShortID, service.SystemActor())
	if err != nil {
		t.Fatalf("ApplyImportSession: %v", err)
	}
	if res.NewCount != 2 || res.UpdatedCount != 0 {
		t.Fatalf("apply result = %+v, want 2 new", res)
	}
	if n := countTxns(t, svc, acctA); n != 2 {
		t.Fatalf("account has %d txns, want 2", n)
	}

	// Re-drop the same file plus one new row. The saved profile should now
	// auto-resolve to the same account, dedupe the two, and stage one new row.
	file2 := []byte("Date,Amount,Description\n2026-07-01,10.00,COFFEE\n2026-07-02,20.00,LUNCH\n2026-07-03,30.00,DINNER\n")
	an2, err := svc.CreateImportSession(ctx, service.CreateImportSessionParams{
		UserID: pgconv.FormatUUID(user.ID), Filename: "chase.csv", Data: file2,
	})
	if err != nil {
		t.Fatalf("CreateImportSession redrop: %v", err)
	}
	if an2.Session.Status != "previewed" {
		t.Fatalf("redrop status = %q, want previewed (profile auto-resolve)", an2.Session.Status)
	}
	if an2.Session.ResolvedAccountID != acctA {
		t.Fatalf("redrop resolved to %q, want original account %q", an2.Session.ResolvedAccountID, acctA)
	}
	if an2.Summary.Counts["exact_dup"] != 2 || an2.Summary.Counts["new"] != 1 {
		t.Fatalf("redrop summary = %+v, want 2 exact_dup + 1 new", an2.Summary)
	}

	res2, err := svc.ApplyImportSession(ctx, an2.Session.ShortID, service.SystemActor())
	if err != nil {
		t.Fatalf("apply redrop: %v", err)
	}
	if res2.NewCount != 1 {
		t.Fatalf("redrop apply = %+v, want 1 new", res2)
	}
	if n := countTxns(t, svc, acctA); n != 3 {
		t.Fatalf("after redrop account has %d txns, want 3", n)
	}
}

func TestImportV2_ApplyIsIdempotent(t *testing.T) {
	svc, q, _ := newService(t)
	ctx := context.Background()
	user := testutil.MustCreateUser(t, q, "Alice")
	file := []byte("Date,Amount,Description\n2026-08-01,5.00,A\n2026-08-02,6.00,B\n")

	an, err := svc.CreateImportSession(ctx, service.CreateImportSessionParams{
		UserID: pgconv.FormatUUID(user.ID), Filename: "f.csv", Data: file,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	sess, err := svc.ResolveImportAccount(ctx, an.Session.ShortID, service.ResolveImportAccountParams{CreateNew: true, NewName: "Acct"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if _, err := svc.ApplyImportSession(ctx, sess.ShortID, service.SystemActor()); err != nil {
		t.Fatalf("apply1: %v", err)
	}
	// A fresh session over the same file into the same account must add nothing.
	an2, err := svc.CreateImportSession(ctx, service.CreateImportSessionParams{
		UserID: pgconv.FormatUUID(user.ID), Filename: "f.csv", Data: file,
	})
	if err != nil {
		t.Fatalf("create2: %v", err)
	}
	res2, err := svc.ApplyImportSession(ctx, an2.Session.ShortID, service.SystemActor())
	if err != nil {
		t.Fatalf("apply2: %v", err)
	}
	if res2.NewCount != 0 {
		t.Fatalf("second apply added %d rows, want 0 (idempotent)", res2.NewCount)
	}
	if n := countTxns(t, svc, sess.ResolvedAccountID); n != 2 {
		t.Fatalf("account has %d txns after double import, want 2", n)
	}
}

func TestImportV2_AccountChangeBeforeConfirm(t *testing.T) {
	svc, q, _ := newService(t)
	ctx := context.Background()
	user := testutil.MustCreateUser(t, q, "Alice")
	conn := testutil.MustCreateConnection(t, q, user.ID, "item_1")
	accA := testutil.MustCreateAccount(t, q, conn.ID, "extA", "Account A")
	accB := testutil.MustCreateAccount(t, q, conn.ID, "extB", "Account B")

	file := []byte("Date,Amount,Description\n2026-09-01,12.00,X\n")
	an, err := svc.CreateImportSession(ctx, service.CreateImportSessionParams{
		UserID: pgconv.FormatUUID(user.ID), Filename: "f.csv", Data: file,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Resolve to A, then change to B before applying.
	if _, err := svc.ResolveImportAccount(ctx, an.Session.ShortID, service.ResolveImportAccountParams{AccountID: pgconv.FormatUUID(accA.ID)}); err != nil {
		t.Fatalf("resolve A: %v", err)
	}
	if _, err := svc.ResolveImportAccount(ctx, an.Session.ShortID, service.ResolveImportAccountParams{AccountID: pgconv.FormatUUID(accB.ID)}); err != nil {
		t.Fatalf("resolve B: %v", err)
	}
	if _, err := svc.ApplyImportSession(ctx, an.Session.ShortID, service.SystemActor()); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if n := countTxns(t, svc, pgconv.FormatUUID(accB.ID)); n != 1 {
		t.Fatalf("account B has %d txns, want 1", n)
	}
	if n := countTxns(t, svc, pgconv.FormatUUID(accA.ID)); n != 0 {
		t.Fatalf("account A has %d txns, want 0 (import went to B)", n)
	}
}

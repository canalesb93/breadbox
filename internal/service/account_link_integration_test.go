//go:build integration

package service_test

import (
	"context"
	"errors"
	"testing"

	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/jackc/pgx/v5/pgtype"
)

// --- CreateAccountLink ---

func TestCreateAccountLink_Success(t *testing.T) {
	svc, queries, _ := newService(t)
	user1 := testutil.MustCreateUser(t, queries, "Cardholder")
	conn1 := testutil.MustCreateConnection(t, queries, user1.ID, "item_primary")
	primaryAcct := testutil.MustCreateAccount(t, queries, conn1.ID, "ext_primary", "Primary Card")

	user2 := testutil.MustCreateUser(t, queries, "Authorized User")
	conn2 := testutil.MustCreateConnection(t, queries, user2.ID, "item_dependent")
	dependentAcct := testutil.MustCreateAccount(t, queries, conn2.ID, "ext_dependent", "Dependent Card")

	link, err := svc.CreateAccountLink(context.Background(), service.CreateAccountLinkParams{
		PrimaryAccountID:   pgconv.FormatUUID(primaryAcct.ID),
		DependentAccountID: pgconv.FormatUUID(dependentAcct.ID),
		MatchStrategy:      "date_amount_name",
		MatchToleranceDays: 1,
	})
	if err != nil {
		t.Fatalf("CreateAccountLink: %v", err)
	}

	if link.ID == "" {
		t.Error("expected non-empty ID")
	}
	if link.PrimaryAccountID != pgconv.FormatUUID(primaryAcct.ID) {
		t.Errorf("primary account mismatch: got %s, want %s", link.PrimaryAccountID, pgconv.FormatUUID(primaryAcct.ID))
	}
	if link.DependentAccountID != pgconv.FormatUUID(dependentAcct.ID) {
		t.Errorf("dependent account mismatch")
	}
	if link.MatchStrategy != "date_amount_name" {
		t.Errorf("strategy: got %s, want date_amount_name", link.MatchStrategy)
	}
	if link.MatchToleranceDays != 1 {
		t.Errorf("tolerance: got %d, want 1", link.MatchToleranceDays)
	}
	if !link.Enabled {
		t.Error("expected link to be enabled by default")
	}

	// Verify dependent account is marked as linked.
	acct, err := queries.GetAccount(context.Background(), dependentAcct.ID)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if !acct.IsDependentLinked {
		t.Error("expected dependent account to be marked as linked")
	}
}

func TestCreateAccountLink_DefaultStrategy(t *testing.T) {
	svc, queries, _ := newService(t)
	user1 := testutil.MustCreateUser(t, queries, "Cardholder")
	conn1 := testutil.MustCreateConnection(t, queries, user1.ID, "item_primary")
	primaryAcct := testutil.MustCreateAccount(t, queries, conn1.ID, "ext_primary", "Primary Card")

	user2 := testutil.MustCreateUser(t, queries, "Authorized User")
	conn2 := testutil.MustCreateConnection(t, queries, user2.ID, "item_dependent")
	dependentAcct := testutil.MustCreateAccount(t, queries, conn2.ID, "ext_dependent", "Dependent Card")

	link, err := svc.CreateAccountLink(context.Background(), service.CreateAccountLinkParams{
		PrimaryAccountID:   pgconv.FormatUUID(primaryAcct.ID),
		DependentAccountID: pgconv.FormatUUID(dependentAcct.ID),
		// No strategy specified — should default to "date_amount_name"
	})
	if err != nil {
		t.Fatalf("CreateAccountLink: %v", err)
	}
	if link.MatchStrategy != "date_amount_name" {
		t.Errorf("expected default strategy, got %q", link.MatchStrategy)
	}
}

func TestCreateAccountLink_InvalidPrimaryAccount(t *testing.T) {
	svc, queries, _ := newService(t)
	user := testutil.MustCreateUser(t, queries, "User")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_1", "Real Account")

	_, err := svc.CreateAccountLink(context.Background(), service.CreateAccountLinkParams{
		PrimaryAccountID:   "00000000-0000-0000-0000-000000000001",
		DependentAccountID: pgconv.FormatUUID(acct.ID),
	})
	if err == nil {
		t.Fatal("expected error for nonexistent primary account")
	}
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestCreateAccountLink_InvalidDependentAccount(t *testing.T) {
	svc, queries, _ := newService(t)
	user := testutil.MustCreateUser(t, queries, "User")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_1", "Real Account")

	_, err := svc.CreateAccountLink(context.Background(), service.CreateAccountLinkParams{
		PrimaryAccountID:   pgconv.FormatUUID(acct.ID),
		DependentAccountID: "00000000-0000-0000-0000-000000000001",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent dependent account")
	}
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestCreateAccountLink_BadUUID(t *testing.T) {
	svc, _, _ := newService(t)

	_, err := svc.CreateAccountLink(context.Background(), service.CreateAccountLinkParams{
		PrimaryAccountID:   "not-a-uuid",
		DependentAccountID: "also-not-a-uuid",
	})
	if err == nil {
		t.Fatal("expected error for bad UUID")
	}
	if !errors.Is(err, service.ErrInvalidParameter) {
		t.Errorf("expected ErrInvalidParameter, got: %v", err)
	}
}

func TestCreateAccountLink_DuplicateLink(t *testing.T) {
	svc, queries, _ := newService(t)
	user1 := testutil.MustCreateUser(t, queries, "Cardholder")
	conn1 := testutil.MustCreateConnection(t, queries, user1.ID, "item_primary")
	primaryAcct := testutil.MustCreateAccount(t, queries, conn1.ID, "ext_primary", "Primary Card")

	user2 := testutil.MustCreateUser(t, queries, "Authorized User")
	conn2 := testutil.MustCreateConnection(t, queries, user2.ID, "item_dependent")
	dependentAcct := testutil.MustCreateAccount(t, queries, conn2.ID, "ext_dependent", "Dependent Card")

	params := service.CreateAccountLinkParams{
		PrimaryAccountID:   pgconv.FormatUUID(primaryAcct.ID),
		DependentAccountID: pgconv.FormatUUID(dependentAcct.ID),
	}

	_, err := svc.CreateAccountLink(context.Background(), params)
	if err != nil {
		t.Fatalf("first CreateAccountLink: %v", err)
	}

	// Creating the same link again should fail (unique constraint).
	_, err = svc.CreateAccountLink(context.Background(), params)
	if err == nil {
		t.Fatal("expected error for duplicate link")
	}
}

func TestCreateAccountLink_SelfLink(t *testing.T) {
	svc, queries, _ := newService(t)
	user := testutil.MustCreateUser(t, queries, "User")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_1", "Acct")

	acctID := pgconv.FormatUUID(acct.ID)
	_, err := svc.CreateAccountLink(context.Background(), service.CreateAccountLinkParams{
		PrimaryAccountID:   acctID,
		DependentAccountID: acctID,
	})
	if err == nil {
		t.Fatal("expected error for self-link")
	}
	if !errors.Is(err, service.ErrInvalidParameter) {
		t.Errorf("expected ErrInvalidParameter, got: %v", err)
	}
}

func TestCreateAccountLink_CircularLinkRejected(t *testing.T) {
	svc, queries, _ := newService(t)
	user1 := testutil.MustCreateUser(t, queries, "User A")
	conn1 := testutil.MustCreateConnection(t, queries, user1.ID, "item_a")
	acctA := testutil.MustCreateAccount(t, queries, conn1.ID, "ext_a", "Account A")

	user2 := testutil.MustCreateUser(t, queries, "User B")
	conn2 := testutil.MustCreateConnection(t, queries, user2.ID, "item_b")
	acctB := testutil.MustCreateAccount(t, queries, conn2.ID, "ext_b", "Account B")

	acctAID := pgconv.FormatUUID(acctA.ID)
	acctBID := pgconv.FormatUUID(acctB.ID)

	// Create A→B link (A is primary, B is dependent).
	_, err := svc.CreateAccountLink(context.Background(), service.CreateAccountLinkParams{
		PrimaryAccountID:   acctAID,
		DependentAccountID: acctBID,
	})
	if err != nil {
		t.Fatalf("first CreateAccountLink: %v", err)
	}

	// Attempt to create reverse B→A link (B is primary, A is dependent).
	// This would create a circular dependency where both accounts are marked
	// as dependent, excluding both from totals.
	_, err = svc.CreateAccountLink(context.Background(), service.CreateAccountLinkParams{
		PrimaryAccountID:   acctBID,
		DependentAccountID: acctAID,
	})
	if err == nil {
		t.Fatal("expected error for circular link (reverse direction)")
	}
	if !errors.Is(err, service.ErrInvalidParameter) {
		t.Errorf("expected ErrInvalidParameter, got: %v", err)
	}
}

// --- GetAccountLink ---

func TestGetAccountLink_Success(t *testing.T) {
	svc, queries, _ := newService(t)
	user1 := testutil.MustCreateUser(t, queries, "Cardholder")
	conn1 := testutil.MustCreateConnection(t, queries, user1.ID, "item_primary")
	primaryAcct := testutil.MustCreateAccount(t, queries, conn1.ID, "ext_primary", "Primary Card")

	user2 := testutil.MustCreateUser(t, queries, "Authorized User")
	conn2 := testutil.MustCreateConnection(t, queries, user2.ID, "item_dependent")
	dependentAcct := testutil.MustCreateAccount(t, queries, conn2.ID, "ext_dependent", "Dependent Card")

	created, err := svc.CreateAccountLink(context.Background(), service.CreateAccountLinkParams{
		PrimaryAccountID:   pgconv.FormatUUID(primaryAcct.ID),
		DependentAccountID: pgconv.FormatUUID(dependentAcct.ID),
		MatchToleranceDays: 2,
	})
	if err != nil {
		t.Fatalf("CreateAccountLink: %v", err)
	}

	got, err := svc.GetAccountLink(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetAccountLink: %v", err)
	}

	if got.ID != created.ID {
		t.Errorf("ID mismatch: got %s, want %s", got.ID, created.ID)
	}
	if got.PrimaryAccountName != "Primary Card" {
		t.Errorf("primary name: got %q, want %q", got.PrimaryAccountName, "Primary Card")
	}
	if got.DependentAccountName != "Dependent Card" {
		t.Errorf("dependent name: got %q, want %q", got.DependentAccountName, "Dependent Card")
	}
	if got.PrimaryUserName != "Cardholder" {
		t.Errorf("primary user name: got %q, want %q", got.PrimaryUserName, "Cardholder")
	}
	if got.DependentUserName != "Authorized User" {
		t.Errorf("dependent user name: got %q, want %q", got.DependentUserName, "Authorized User")
	}
	if got.MatchToleranceDays != 2 {
		t.Errorf("tolerance: got %d, want 2", got.MatchToleranceDays)
	}
}

func TestGetAccountLink_NotFound(t *testing.T) {
	svc, _, _ := newService(t)

	_, err := svc.GetAccountLink(context.Background(), "00000000-0000-0000-0000-000000000001")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestGetAccountLink_BadUUID(t *testing.T) {
	svc, _, _ := newService(t)

	_, err := svc.GetAccountLink(context.Background(), "not-a-uuid")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound for bad UUID, got: %v", err)
	}
}

// --- ListAccountLinks ---

func TestListAccountLinks_Empty(t *testing.T) {
	svc, _, _ := newService(t)
	links, err := svc.ListAccountLinks(context.Background())
	if err != nil {
		t.Fatalf("ListAccountLinks: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("expected 0 links, got %d", len(links))
	}
}

func TestListAccountLinks_WithData(t *testing.T) {
	svc, queries, _ := newService(t)
	user1 := testutil.MustCreateUser(t, queries, "User1")
	conn1 := testutil.MustCreateConnection(t, queries, user1.ID, "item_1")
	acct1 := testutil.MustCreateAccount(t, queries, conn1.ID, "ext_1", "Acct1")

	user2 := testutil.MustCreateUser(t, queries, "User2")
	conn2 := testutil.MustCreateConnection(t, queries, user2.ID, "item_2")
	acct2 := testutil.MustCreateAccount(t, queries, conn2.ID, "ext_2", "Acct2")

	user3 := testutil.MustCreateUser(t, queries, "User3")
	conn3 := testutil.MustCreateConnection(t, queries, user3.ID, "item_3")
	acct3 := testutil.MustCreateAccount(t, queries, conn3.ID, "ext_3", "Acct3")

	_, err := svc.CreateAccountLink(context.Background(), service.CreateAccountLinkParams{
		PrimaryAccountID:   pgconv.FormatUUID(acct1.ID),
		DependentAccountID: pgconv.FormatUUID(acct2.ID),
	})
	if err != nil {
		t.Fatalf("CreateAccountLink 1: %v", err)
	}
	_, err = svc.CreateAccountLink(context.Background(), service.CreateAccountLinkParams{
		PrimaryAccountID:   pgconv.FormatUUID(acct1.ID),
		DependentAccountID: pgconv.FormatUUID(acct3.ID),
	})
	if err != nil {
		t.Fatalf("CreateAccountLink 2: %v", err)
	}

	links, err := svc.ListAccountLinks(context.Background())
	if err != nil {
		t.Fatalf("ListAccountLinks: %v", err)
	}
	if len(links) != 2 {
		t.Errorf("expected 2 links, got %d", len(links))
	}
}

// --- UpdateAccountLink ---

func TestUpdateAccountLink_Success(t *testing.T) {
	svc, queries, _ := newService(t)
	user1 := testutil.MustCreateUser(t, queries, "User1")
	conn1 := testutil.MustCreateConnection(t, queries, user1.ID, "item_1")
	acct1 := testutil.MustCreateAccount(t, queries, conn1.ID, "ext_1", "Acct1")

	user2 := testutil.MustCreateUser(t, queries, "User2")
	conn2 := testutil.MustCreateConnection(t, queries, user2.ID, "item_2")
	acct2 := testutil.MustCreateAccount(t, queries, conn2.ID, "ext_2", "Acct2")

	created, err := svc.CreateAccountLink(context.Background(), service.CreateAccountLinkParams{
		PrimaryAccountID:   pgconv.FormatUUID(acct1.ID),
		DependentAccountID: pgconv.FormatUUID(acct2.ID),
		MatchToleranceDays: 0,
	})
	if err != nil {
		t.Fatalf("CreateAccountLink: %v", err)
	}

	newTolerance := 3
	updated, err := svc.UpdateAccountLink(context.Background(), created.ID, service.UpdateAccountLinkParams{
		MatchToleranceDays: &newTolerance,
	})
	if err != nil {
		t.Fatalf("UpdateAccountLink: %v", err)
	}
	if updated.MatchToleranceDays != 3 {
		t.Errorf("tolerance: got %d, want 3", updated.MatchToleranceDays)
	}
	// Other fields should be unchanged.
	if updated.MatchStrategy != "date_amount_name" {
		t.Errorf("strategy changed unexpectedly: %s", updated.MatchStrategy)
	}
	if !updated.Enabled {
		t.Error("enabled changed unexpectedly")
	}
}

func TestUpdateAccountLink_Disable(t *testing.T) {
	svc, queries, _ := newService(t)
	user1 := testutil.MustCreateUser(t, queries, "User1")
	conn1 := testutil.MustCreateConnection(t, queries, user1.ID, "item_1")
	acct1 := testutil.MustCreateAccount(t, queries, conn1.ID, "ext_1", "Acct1")

	user2 := testutil.MustCreateUser(t, queries, "User2")
	conn2 := testutil.MustCreateConnection(t, queries, user2.ID, "item_2")
	acct2 := testutil.MustCreateAccount(t, queries, conn2.ID, "ext_2", "Acct2")

	created, err := svc.CreateAccountLink(context.Background(), service.CreateAccountLinkParams{
		PrimaryAccountID:   pgconv.FormatUUID(acct1.ID),
		DependentAccountID: pgconv.FormatUUID(acct2.ID),
	})
	if err != nil {
		t.Fatalf("CreateAccountLink: %v", err)
	}

	disabled := false
	updated, err := svc.UpdateAccountLink(context.Background(), created.ID, service.UpdateAccountLinkParams{
		Enabled: &disabled,
	})
	if err != nil {
		t.Fatalf("UpdateAccountLink: %v", err)
	}
	if updated.Enabled {
		t.Error("expected link to be disabled")
	}

	// Dependent account should be unmarked (no other enabled links).
	acct, err := queries.GetAccount(context.Background(), acct2.ID)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if acct.IsDependentLinked {
		t.Error("expected dependent account to be unmarked when only link is disabled")
	}
}

func TestUpdateAccountLink_ReEnable(t *testing.T) {
	svc, queries, _ := newService(t)
	user1 := testutil.MustCreateUser(t, queries, "User1")
	conn1 := testutil.MustCreateConnection(t, queries, user1.ID, "item_1")
	acct1 := testutil.MustCreateAccount(t, queries, conn1.ID, "ext_1", "Acct1")

	user2 := testutil.MustCreateUser(t, queries, "User2")
	conn2 := testutil.MustCreateConnection(t, queries, user2.ID, "item_2")
	acct2 := testutil.MustCreateAccount(t, queries, conn2.ID, "ext_2", "Acct2")

	created, err := svc.CreateAccountLink(context.Background(), service.CreateAccountLinkParams{
		PrimaryAccountID:   pgconv.FormatUUID(acct1.ID),
		DependentAccountID: pgconv.FormatUUID(acct2.ID),
	})
	if err != nil {
		t.Fatalf("CreateAccountLink: %v", err)
	}

	// Disable then re-enable.
	disabled := false
	_, err = svc.UpdateAccountLink(context.Background(), created.ID, service.UpdateAccountLinkParams{
		Enabled: &disabled,
	})
	if err != nil {
		t.Fatalf("Disable: %v", err)
	}

	enabled := true
	_, err = svc.UpdateAccountLink(context.Background(), created.ID, service.UpdateAccountLinkParams{
		Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("Re-enable: %v", err)
	}

	// Dependent account should be marked as linked again.
	acct, err := queries.GetAccount(context.Background(), acct2.ID)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if !acct.IsDependentLinked {
		t.Error("expected dependent account to be re-marked as linked")
	}
}

func TestUpdateAccountLink_NotFound(t *testing.T) {
	svc, _, _ := newService(t)

	newTol := 1
	_, err := svc.UpdateAccountLink(context.Background(), "00000000-0000-0000-0000-000000000001", service.UpdateAccountLinkParams{
		MatchToleranceDays: &newTol,
	})
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// --- DeleteAccountLink ---

func TestDeleteAccountLink_Success(t *testing.T) {
	svc, queries, _ := newService(t)
	user1 := testutil.MustCreateUser(t, queries, "User1")
	conn1 := testutil.MustCreateConnection(t, queries, user1.ID, "item_1")
	acct1 := testutil.MustCreateAccount(t, queries, conn1.ID, "ext_1", "Acct1")

	user2 := testutil.MustCreateUser(t, queries, "User2")
	conn2 := testutil.MustCreateConnection(t, queries, user2.ID, "item_2")
	acct2 := testutil.MustCreateAccount(t, queries, conn2.ID, "ext_2", "Acct2")

	created, err := svc.CreateAccountLink(context.Background(), service.CreateAccountLinkParams{
		PrimaryAccountID:   pgconv.FormatUUID(acct1.ID),
		DependentAccountID: pgconv.FormatUUID(acct2.ID),
	})
	if err != nil {
		t.Fatalf("CreateAccountLink: %v", err)
	}

	err = svc.DeleteAccountLink(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("DeleteAccountLink: %v", err)
	}

	// Should be gone.
	_, err = svc.GetAccountLink(context.Background(), created.ID)
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got: %v", err)
	}

	// Dependent account should be unmarked.
	acct, err := queries.GetAccount(context.Background(), acct2.ID)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if acct.IsDependentLinked {
		t.Error("expected dependent account to be unmarked after link deletion")
	}
}

func TestDeleteAccountLink_NotFound(t *testing.T) {
	svc, _, _ := newService(t)

	err := svc.DeleteAccountLink(context.Background(), "00000000-0000-0000-0000-000000000001")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// --- ManualMatch ---

func TestManualMatch_Success(t *testing.T) {
	svc, queries, _ := newService(t)
	user1 := testutil.MustCreateUser(t, queries, "Cardholder")
	conn1 := testutil.MustCreateConnection(t, queries, user1.ID, "item_primary")
	acct1 := testutil.MustCreateAccount(t, queries, conn1.ID, "ext_primary", "Primary Card")

	user2 := testutil.MustCreateUser(t, queries, "Authorized User")
	conn2 := testutil.MustCreateConnection(t, queries, user2.ID, "item_dependent")
	acct2 := testutil.MustCreateAccount(t, queries, conn2.ID, "ext_dependent", "Dependent Card")

	link, err := svc.CreateAccountLink(context.Background(), service.CreateAccountLinkParams{
		PrimaryAccountID:   pgconv.FormatUUID(acct1.ID),
		DependentAccountID: pgconv.FormatUUID(acct2.ID),
	})
	if err != nil {
		t.Fatalf("CreateAccountLink: %v", err)
	}

	txn1 := testutil.MustCreateTransaction(t, queries, acct1.ID, "txn_p1", "Coffee Shop", 550, "2025-01-15")
	txn2 := testutil.MustCreateTransaction(t, queries, acct2.ID, "txn_d1", "Coffee Shop", 550, "2025-01-15")

	match, err := svc.ManualMatch(context.Background(), link.ID, pgconv.FormatUUID(txn1.ID), pgconv.FormatUUID(txn2.ID))
	if err != nil {
		t.Fatalf("ManualMatch: %v", err)
	}

	if match.ID == "" {
		t.Error("expected non-empty match ID")
	}
	if match.MatchConfidence != "confirmed" {
		t.Errorf("confidence: got %q, want %q", match.MatchConfidence, "confirmed")
	}
	if len(match.MatchedOn) != 1 || match.MatchedOn[0] != "manual" {
		t.Errorf("matched_on: got %v, want [manual]", match.MatchedOn)
	}
	if match.PrimaryTransactionID != pgconv.FormatUUID(txn1.ID) {
		t.Error("primary transaction ID mismatch")
	}
	if match.DependentTransactionID != pgconv.FormatUUID(txn2.ID) {
		t.Error("dependent transaction ID mismatch")
	}
}

func TestManualMatch_InvalidLinkID(t *testing.T) {
	svc, _, _ := newService(t)

	_, err := svc.ManualMatch(context.Background(), "not-a-uuid", "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002")
	if err == nil {
		t.Fatal("expected error for bad link UUID")
	}
	if !errors.Is(err, service.ErrInvalidParameter) {
		t.Errorf("expected ErrInvalidParameter, got: %v", err)
	}
}

func TestManualMatch_NonexistentLink(t *testing.T) {
	svc, _, _ := newService(t)

	_, err := svc.ManualMatch(context.Background(), "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002", "00000000-0000-0000-0000-000000000003")
	if err == nil {
		t.Fatal("expected error for nonexistent link")
	}
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// --- ConfirmMatch & RejectMatch ---

func TestConfirmMatch_Success(t *testing.T) {
	svc, queries, _ := newService(t)
	user1 := testutil.MustCreateUser(t, queries, "User1")
	conn1 := testutil.MustCreateConnection(t, queries, user1.ID, "item_1")
	acct1 := testutil.MustCreateAccount(t, queries, conn1.ID, "ext_1", "Acct1")

	user2 := testutil.MustCreateUser(t, queries, "User2")
	conn2 := testutil.MustCreateConnection(t, queries, user2.ID, "item_2")
	acct2 := testutil.MustCreateAccount(t, queries, conn2.ID, "ext_2", "Acct2")

	link, err := svc.CreateAccountLink(context.Background(), service.CreateAccountLinkParams{
		PrimaryAccountID:   pgconv.FormatUUID(acct1.ID),
		DependentAccountID: pgconv.FormatUUID(acct2.ID),
	})
	if err != nil {
		t.Fatalf("CreateAccountLink: %v", err)
	}

	txn1 := testutil.MustCreateTransaction(t, queries, acct1.ID, "txn_1", "Store", 1000, "2025-01-15")
	txn2 := testutil.MustCreateTransaction(t, queries, acct2.ID, "txn_2", "Store", 1000, "2025-01-15")

	match, err := svc.ManualMatch(context.Background(), link.ID, pgconv.FormatUUID(txn1.ID), pgconv.FormatUUID(txn2.ID))
	if err != nil {
		t.Fatalf("ManualMatch: %v", err)
	}

	// ManualMatch already creates as confirmed, but let's test the ConfirmMatch method works.
	err = svc.ConfirmMatch(context.Background(), match.ID)
	if err != nil {
		t.Fatalf("ConfirmMatch: %v", err)
	}
}

func TestConfirmMatch_NotFound(t *testing.T) {
	svc, _, _ := newService(t)

	err := svc.ConfirmMatch(context.Background(), "00000000-0000-0000-0000-000000000001")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestRejectMatch_Success(t *testing.T) {
	svc, queries, _ := newService(t)
	user1 := testutil.MustCreateUser(t, queries, "User1")
	conn1 := testutil.MustCreateConnection(t, queries, user1.ID, "item_1")
	acct1 := testutil.MustCreateAccount(t, queries, conn1.ID, "ext_1", "Acct1")

	user2 := testutil.MustCreateUser(t, queries, "User2")
	conn2 := testutil.MustCreateConnection(t, queries, user2.ID, "item_2")
	acct2 := testutil.MustCreateAccount(t, queries, conn2.ID, "ext_2", "Acct2")

	link, err := svc.CreateAccountLink(context.Background(), service.CreateAccountLinkParams{
		PrimaryAccountID:   pgconv.FormatUUID(acct1.ID),
		DependentAccountID: pgconv.FormatUUID(acct2.ID),
	})
	if err != nil {
		t.Fatalf("CreateAccountLink: %v", err)
	}

	txn1 := testutil.MustCreateTransaction(t, queries, acct1.ID, "txn_1", "Store", 1000, "2025-01-15")
	txn2 := testutil.MustCreateTransaction(t, queries, acct2.ID, "txn_2", "Store", 1000, "2025-01-15")

	match, err := svc.ManualMatch(context.Background(), link.ID, pgconv.FormatUUID(txn1.ID), pgconv.FormatUUID(txn2.ID))
	if err != nil {
		t.Fatalf("ManualMatch: %v", err)
	}

	err = svc.RejectMatch(context.Background(), match.ID)
	if err != nil {
		t.Fatalf("RejectMatch: %v", err)
	}

	// Match should be gone after rejection.
	matches, err := svc.ListTransactionMatches(context.Background(), link.ID)
	if err != nil {
		t.Fatalf("ListTransactionMatches: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 matches after reject, got %d", len(matches))
	}
}

func TestRejectMatch_NotFound(t *testing.T) {
	svc, _, _ := newService(t)

	err := svc.RejectMatch(context.Background(), "00000000-0000-0000-0000-000000000001")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// --- ListTransactionMatches ---

func TestListTransactionMatches_Empty(t *testing.T) {
	svc, queries, _ := newService(t)
	user1 := testutil.MustCreateUser(t, queries, "User1")
	conn1 := testutil.MustCreateConnection(t, queries, user1.ID, "item_1")
	acct1 := testutil.MustCreateAccount(t, queries, conn1.ID, "ext_1", "Acct1")

	user2 := testutil.MustCreateUser(t, queries, "User2")
	conn2 := testutil.MustCreateConnection(t, queries, user2.ID, "item_2")
	acct2 := testutil.MustCreateAccount(t, queries, conn2.ID, "ext_2", "Acct2")

	link, err := svc.CreateAccountLink(context.Background(), service.CreateAccountLinkParams{
		PrimaryAccountID:   pgconv.FormatUUID(acct1.ID),
		DependentAccountID: pgconv.FormatUUID(acct2.ID),
	})
	if err != nil {
		t.Fatalf("CreateAccountLink: %v", err)
	}

	matches, err := svc.ListTransactionMatches(context.Background(), link.ID)
	if err != nil {
		t.Fatalf("ListTransactionMatches: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matches))
	}
}

func TestListTransactionMatches_WithData(t *testing.T) {
	svc, queries, _ := newService(t)
	user1 := testutil.MustCreateUser(t, queries, "User1")
	conn1 := testutil.MustCreateConnection(t, queries, user1.ID, "item_1")
	acct1 := testutil.MustCreateAccount(t, queries, conn1.ID, "ext_1", "Acct1")

	user2 := testutil.MustCreateUser(t, queries, "User2")
	conn2 := testutil.MustCreateConnection(t, queries, user2.ID, "item_2")
	acct2 := testutil.MustCreateAccount(t, queries, conn2.ID, "ext_2", "Acct2")

	link, err := svc.CreateAccountLink(context.Background(), service.CreateAccountLinkParams{
		PrimaryAccountID:   pgconv.FormatUUID(acct1.ID),
		DependentAccountID: pgconv.FormatUUID(acct2.ID),
	})
	if err != nil {
		t.Fatalf("CreateAccountLink: %v", err)
	}

	txn1 := testutil.MustCreateTransaction(t, queries, acct1.ID, "txn_1", "Store A", 1000, "2025-01-15")
	txn2 := testutil.MustCreateTransaction(t, queries, acct2.ID, "txn_2", "Store A", 1000, "2025-01-15")

	_, err = svc.ManualMatch(context.Background(), link.ID, pgconv.FormatUUID(txn1.ID), pgconv.FormatUUID(txn2.ID))
	if err != nil {
		t.Fatalf("ManualMatch: %v", err)
	}

	matches, err := svc.ListTransactionMatches(context.Background(), link.ID)
	if err != nil {
		t.Fatalf("ListTransactionMatches: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].PrimaryTxnName != "Store A" {
		t.Errorf("primary txn name: got %q, want %q", matches[0].PrimaryTxnName, "Store A")
	}
	if matches[0].DependentTxnName != "Store A" {
		t.Errorf("dependent txn name: got %q, want %q", matches[0].DependentTxnName, "Store A")
	}
	if matches[0].Amount != 10.00 {
		t.Errorf("amount: got %f, want 10.00", matches[0].Amount)
	}
}

func TestListTransactionMatches_InvalidLinkID(t *testing.T) {
	svc, _, _ := newService(t)

	_, err := svc.ListTransactionMatches(context.Background(), "not-a-uuid")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound for bad UUID, got: %v", err)
	}
}

// --- DeleteAccountLink clears attribution ---

func TestDeleteAccountLink_ClearsAttribution(t *testing.T) {
	svc, queries, pool := newService(t)
	user1 := testutil.MustCreateUser(t, queries, "Cardholder")
	conn1 := testutil.MustCreateConnection(t, queries, user1.ID, "item_1")
	acct1 := testutil.MustCreateAccount(t, queries, conn1.ID, "ext_1", "Primary Card")

	user2 := testutil.MustCreateUser(t, queries, "Auth User")
	conn2 := testutil.MustCreateConnection(t, queries, user2.ID, "item_2")
	acct2 := testutil.MustCreateAccount(t, queries, conn2.ID, "ext_2", "Dependent Card")

	link, err := svc.CreateAccountLink(context.Background(), service.CreateAccountLinkParams{
		PrimaryAccountID:   pgconv.FormatUUID(acct1.ID),
		DependentAccountID: pgconv.FormatUUID(acct2.ID),
	})
	if err != nil {
		t.Fatalf("CreateAccountLink: %v", err)
	}

	txn1 := testutil.MustCreateTransaction(t, queries, acct1.ID, "txn_p1", "Coffee", 550, "2025-01-15")
	txn2 := testutil.MustCreateTransaction(t, queries, acct2.ID, "txn_d1", "Coffee", 550, "2025-01-15")

	_, err = svc.ManualMatch(context.Background(), link.ID, pgconv.FormatUUID(txn1.ID), pgconv.FormatUUID(txn2.ID))
	if err != nil {
		t.Fatalf("ManualMatch: %v", err)
	}

	// Verify attribution was set on primary transaction.
	var attrUserID pgtype.UUID
	err = pool.QueryRow(context.Background(), "SELECT attributed_user_id FROM transactions WHERE id = $1", txn1.ID).Scan(&attrUserID)
	if err != nil {
		t.Fatalf("query attributed_user_id: %v", err)
	}
	if !attrUserID.Valid {
		t.Fatal("expected attributed_user_id to be set after ManualMatch")
	}

	// Now delete the link.
	err = svc.DeleteAccountLink(context.Background(), link.ID)
	if err != nil {
		t.Fatalf("DeleteAccountLink: %v", err)
	}

	// Attribution should be cleared.
	err = pool.QueryRow(context.Background(), "SELECT attributed_user_id FROM transactions WHERE id = $1", txn1.ID).Scan(&attrUserID)
	if err != nil {
		t.Fatalf("query attributed_user_id after delete: %v", err)
	}
	if attrUserID.Valid {
		t.Error("expected attributed_user_id to be NULL after link deletion")
	}
}

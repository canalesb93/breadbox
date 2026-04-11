//go:build integration

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"golang.org/x/crypto/bcrypt"
)

// --- Login Accounts (consolidated auth_accounts) ---

func TestCreateLoginAccount_Success(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")

	member, err := svc.CreateLoginAccount(ctx, service.CreateLoginAccountParams{
		UserID:   pgconv.FormatUUID(user.ID),
		Username: "alice",
		Role:     "viewer",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if member.Username != "alice" {
		t.Errorf("expected username alice, got %s", member.Username)
	}
	if member.Role != "viewer" {
		t.Errorf("expected role viewer, got %s", member.Role)
	}
	if member.UserName != "Alice" {
		t.Errorf("expected user_name Alice, got %s", member.UserName)
	}
	if member.HasPassword {
		t.Errorf("expected has_password false for new account")
	}
}

func TestCreateLoginAccount_AdminRole(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Bob")

	member, err := svc.CreateLoginAccount(ctx, service.CreateLoginAccountParams{
		UserID:   pgconv.FormatUUID(user.ID),
		Username: "bob_admin",
		Role:     "admin",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if member.Role != "admin" {
		t.Errorf("expected role admin, got %s", member.Role)
	}
}

func TestCreateLoginAccount_EditorRole(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Carol")

	member, err := svc.CreateLoginAccount(ctx, service.CreateLoginAccountParams{
		UserID:   pgconv.FormatUUID(user.ID),
		Username: "carol_editor",
		Role:     "editor",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if member.Role != "editor" {
		t.Errorf("expected role editor, got %s", member.Role)
	}
}

func TestCreateLoginAccount_DuplicateUser(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")

	_, err := svc.CreateLoginAccount(ctx, service.CreateLoginAccountParams{
		UserID:   pgconv.FormatUUID(user.ID),
		Username: "alice1",
		Role:     "viewer",
	})
	if err != nil {
		t.Fatalf("unexpected error on first create: %v", err)
	}

	// Second account for same user should fail.
	_, err = svc.CreateLoginAccount(ctx, service.CreateLoginAccountParams{
		UserID:   pgconv.FormatUUID(user.ID),
		Username: "alice2",
		Role:     "viewer",
	})
	if err == nil {
		t.Fatal("expected error for duplicate user, got nil")
	}
}

func TestCreateLoginAccount_DuplicateUsername(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user1 := testutil.MustCreateUser(t, queries, "Alice")
	user2 := testutil.MustCreateUser(t, queries, "Bob")

	_, err := svc.CreateLoginAccount(ctx, service.CreateLoginAccountParams{
		UserID:   pgconv.FormatUUID(user1.ID),
		Username: "shared_name",
		Role:     "viewer",
	})
	if err != nil {
		t.Fatalf("unexpected error on first create: %v", err)
	}

	// Same username should fail.
	_, err = svc.CreateLoginAccount(ctx, service.CreateLoginAccountParams{
		UserID:   pgconv.FormatUUID(user2.ID),
		Username: "shared_name",
		Role:     "viewer",
	})
	if err == nil {
		t.Fatal("expected error for duplicate username, got nil")
	}
}

func TestCreateLoginAccount_UsernameConflictsWithExisting(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	// Create an admin account first (no user_id).
	hashedPw, _ := bcrypt.GenerateFromPassword([]byte("testpassword"), 10)
	_, err := queries.CreateAuthAccount(ctx, db.CreateAuthAccountParams{
		Username:       "admin_user",
		HashedPassword: hashedPw,
		Role:           "admin",
	})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}

	user := testutil.MustCreateUser(t, queries, "Alice")

	// Try to create a login account with same username as admin.
	_, err = svc.CreateLoginAccount(ctx, service.CreateLoginAccountParams{
		UserID:   pgconv.FormatUUID(user.ID),
		Username: "admin_user",
		Role:     "viewer",
	})
	if err == nil {
		t.Fatal("expected error for username conflict with admin, got nil")
	}
}

func TestCreateLoginAccount_InvalidRole(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")

	_, err := svc.CreateLoginAccount(ctx, service.CreateLoginAccountParams{
		UserID:   pgconv.FormatUUID(user.ID),
		Username: "alice",
		Role:     "superuser",
	})
	if err == nil {
		t.Fatal("expected error for invalid role, got nil")
	}
}

func TestListLoginAccounts_Empty(t *testing.T) {
	svc, _, _ := newService(t)

	members, err := svc.ListLoginAccounts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(members) != 0 {
		t.Fatalf("expected 0 accounts, got %d", len(members))
	}
}

func TestListLoginAccounts_WithData(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user1 := testutil.MustCreateUser(t, queries, "Alice")
	user2 := testutil.MustCreateUser(t, queries, "Bob")

	_, _ = svc.CreateLoginAccount(ctx, service.CreateLoginAccountParams{
		UserID: pgconv.FormatUUID(user1.ID), Username: "alice", Role: "viewer",
	})
	_, _ = svc.CreateLoginAccount(ctx, service.CreateLoginAccountParams{
		UserID: pgconv.FormatUUID(user2.ID), Username: "bob", Role: "admin",
	})

	members, err := svc.ListLoginAccounts(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(members))
	}
	// Ordered by user name.
	if members[0].UserName != "Alice" {
		t.Errorf("expected first account Alice, got %s", members[0].UserName)
	}
}

func TestUpdateLoginAccountRole(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	member, _ := svc.CreateLoginAccount(ctx, service.CreateLoginAccountParams{
		UserID: pgconv.FormatUUID(user.ID), Username: "alice", Role: "viewer",
	})

	if err := svc.UpdateLoginAccountRole(ctx, member.ID, "editor"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the role was updated.
	members, _ := svc.ListLoginAccounts(ctx)
	if len(members) != 1 || members[0].Role != "editor" {
		t.Errorf("expected role editor after update, got %s", members[0].Role)
	}
}

func TestUpdateLoginAccountRole_ToAdmin(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	member, _ := svc.CreateLoginAccount(ctx, service.CreateLoginAccountParams{
		UserID: pgconv.FormatUUID(user.ID), Username: "alice", Role: "viewer",
	})

	if err := svc.UpdateLoginAccountRole(ctx, member.ID, "admin"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	members, _ := svc.ListLoginAccounts(ctx)
	if len(members) != 1 || members[0].Role != "admin" {
		t.Errorf("expected role admin after update, got %s", members[0].Role)
	}
}

func TestUpdateLoginAccountRole_InvalidRole(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	member, _ := svc.CreateLoginAccount(ctx, service.CreateLoginAccountParams{
		UserID: pgconv.FormatUUID(user.ID), Username: "alice", Role: "viewer",
	})

	if err := svc.UpdateLoginAccountRole(ctx, member.ID, "superuser"); err == nil {
		t.Fatal("expected error for invalid role, got nil")
	}
}

func TestDeleteLoginAccount(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	member, _ := svc.CreateLoginAccount(ctx, service.CreateLoginAccountParams{
		UserID: pgconv.FormatUUID(user.ID), Username: "alice", Role: "viewer",
	})

	if err := svc.DeleteLoginAccount(ctx, member.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	members, _ := svc.ListLoginAccounts(ctx)
	if len(members) != 0 {
		t.Errorf("expected 0 accounts after delete, got %d", len(members))
	}

	// User should still exist.
	users, _ := svc.ListUsers(ctx)
	if len(users) != 1 {
		t.Errorf("expected user to survive login account deletion, got %d users", len(users))
	}
}

func TestWipeUserData(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_1", "Checking")
	testutil.MustCreateTransaction(t, queries, acct.ID, "txn_1", "Coffee", 500, "2026-01-15")
	testutil.MustCreateTransaction(t, queries, acct.ID, "txn_2", "Lunch", 1200, "2026-01-16")

	txnCount, err := svc.WipeUserData(ctx, pgconv.FormatUUID(user.ID))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if txnCount != 2 {
		t.Errorf("expected 2 transactions deleted, got %d", txnCount)
	}

	// Verify data is gone.
	conns, _ := svc.ListConnections(ctx, nil)
	if len(conns) != 0 {
		t.Errorf("expected 0 connections after wipe, got %d", len(conns))
	}

	// User should still exist.
	users, _ := svc.ListUsers(ctx)
	if len(users) != 1 {
		t.Errorf("expected user to survive data wipe, got %d users", len(users))
	}
}

func TestWipeUserData_NoData(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")

	txnCount, err := svc.WipeUserData(ctx, pgconv.FormatUUID(user.ID))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if txnCount != 0 {
		t.Errorf("expected 0 transactions deleted, got %d", txnCount)
	}
}

func TestWipeUserData_DoesNotAffectOtherUsers(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	alice := testutil.MustCreateUser(t, queries, "Alice")
	bob := testutil.MustCreateUser(t, queries, "Bob")

	aliceConn := testutil.MustCreateConnection(t, queries, alice.ID, "alice_item")
	aliceAcct := testutil.MustCreateAccount(t, queries, aliceConn.ID, "alice_ext", "Alice Checking")
	testutil.MustCreateTransaction(t, queries, aliceAcct.ID, "alice_txn", "Alice Coffee", 500, "2026-01-15")

	bobConn := testutil.MustCreateConnection(t, queries, bob.ID, "bob_item")
	bobAcct := testutil.MustCreateAccount(t, queries, bobConn.ID, "bob_ext", "Bob Checking")
	testutil.MustCreateTransaction(t, queries, bobAcct.ID, "bob_txn", "Bob Coffee", 600, "2026-01-15")

	// Wipe Alice's data.
	_, err := svc.WipeUserData(ctx, pgconv.FormatUUID(alice.ID))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Bob's data should be untouched.
	bobID := pgconv.FormatUUID(bob.ID)
	conns, _ := svc.ListConnections(ctx, &bobID)
	if len(conns) != 1 {
		t.Errorf("expected Bob to still have 1 connection, got %d", len(conns))
	}
}

// --- Auth Account Login Tests ---

func TestAuthAccount_LoginFlow(t *testing.T) {
	_, queries, _ := newService(t)
	ctx := context.Background()

	// Create an admin account (no user_id linked).
	hashedPw, _ := bcrypt.GenerateFromPassword([]byte("admin_pass_123"), 10)
	admin, err := queries.CreateAuthAccount(ctx, db.CreateAuthAccountParams{
		Username:       "superadmin",
		HashedPassword: hashedPw,
		Role:           "admin",
	})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}

	// Verify lookup by username.
	found, err := queries.GetAuthAccountByUsername(ctx, "superadmin")
	if err != nil {
		t.Fatalf("get by username: %v", err)
	}
	if found.ID != admin.ID {
		t.Error("ID mismatch on lookup")
	}
	if found.Role != "admin" {
		t.Errorf("expected role admin, got %s", found.Role)
	}
	if !found.UserID.Valid {
		// user_id should be NULL for initial admin.
	}

	// Verify password.
	if err := bcrypt.CompareHashAndPassword(found.HashedPassword, []byte("admin_pass_123")); err != nil {
		t.Error("password verification failed")
	}
}

func TestAuthAccount_ViewerWithUser(t *testing.T) {
	_, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "TestUser")

	account, err := queries.CreateAuthAccount(ctx, db.CreateAuthAccountParams{
		UserID:   user.ID,
		Username: "testviewer",
		Role:     "viewer",
	})
	if err != nil {
		t.Fatalf("create viewer: %v", err)
	}

	if !account.UserID.Valid {
		t.Error("expected user_id to be set")
	}
	if account.HashedPassword != nil {
		t.Error("expected nil password for new account")
	}
	if account.Role != "viewer" {
		t.Errorf("expected role viewer, got %s", account.Role)
	}
}

func TestAuthAccount_EditorRole(t *testing.T) {
	_, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "EditorUser")

	account, err := queries.CreateAuthAccount(ctx, db.CreateAuthAccountParams{
		UserID:   user.ID,
		Username: "testeditor",
		Role:     "editor",
	})
	if err != nil {
		t.Fatalf("create editor: %v", err)
	}

	if account.Role != "editor" {
		t.Errorf("expected role editor, got %s", account.Role)
	}

	// Verify lookup by user_id.
	found, err := queries.GetAuthAccountByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get by user_id: %v", err)
	}
	if found.Username != "testeditor" {
		t.Errorf("expected username testeditor, got %s", found.Username)
	}
}

func TestAuthAccount_CountAccounts(t *testing.T) {
	_, queries, _ := newService(t)
	ctx := context.Background()

	// Initially zero.
	count, err := queries.CountAuthAccounts(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}

	// Create one admin.
	hashedPw, _ := bcrypt.GenerateFromPassword([]byte("testpass123"), 10)
	_, _ = queries.CreateAuthAccount(ctx, db.CreateAuthAccountParams{
		Username:       "admin1",
		HashedPassword: hashedPw,
		Role:           "admin",
	})

	count, _ = queries.CountAuthAccounts(ctx)
	if count != 1 {
		t.Errorf("expected 1, got %d", count)
	}

	adminCount, _ := queries.CountAuthAdminAccounts(ctx)
	if adminCount != 1 {
		t.Errorf("expected 1 admin, got %d", adminCount)
	}

	// Create a viewer.
	user := testutil.MustCreateUser(t, queries, "Alice")
	_, _ = queries.CreateAuthAccount(ctx, db.CreateAuthAccountParams{
		UserID:   user.ID,
		Username: "alice",
		Role:     "viewer",
	})

	count, _ = queries.CountAuthAccounts(ctx)
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}

	adminCount, _ = queries.CountAuthAdminAccounts(ctx)
	if adminCount != 1 {
		t.Errorf("expected still 1 admin, got %d", adminCount)
	}
}

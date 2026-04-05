//go:build integration

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/db"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"golang.org/x/crypto/bcrypt"
)

// --- Member Accounts ---

func TestCreateMemberAccount_Success(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")

	member, err := svc.CreateMemberAccount(ctx, service.CreateMemberAccountParams{
		UserID:   formatUUID(user.ID),
		Username: "alice",
		Role:     "member",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if member.Username != "alice" {
		t.Errorf("expected username alice, got %s", member.Username)
	}
	if member.Role != "member" {
		t.Errorf("expected role member, got %s", member.Role)
	}
	if member.UserName != "Alice" {
		t.Errorf("expected user_name Alice, got %s", member.UserName)
	}
	if member.HasPassword {
		t.Errorf("expected has_password false for new member")
	}
}

func TestCreateMemberAccount_AdminRole(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Bob")

	member, err := svc.CreateMemberAccount(ctx, service.CreateMemberAccountParams{
		UserID:   formatUUID(user.ID),
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

func TestCreateMemberAccount_DuplicateUser(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")

	_, err := svc.CreateMemberAccount(ctx, service.CreateMemberAccountParams{
		UserID:   formatUUID(user.ID),
		Username: "alice1",
		Role:     "member",
	})
	if err != nil {
		t.Fatalf("unexpected error on first create: %v", err)
	}

	// Second account for same user should fail.
	_, err = svc.CreateMemberAccount(ctx, service.CreateMemberAccountParams{
		UserID:   formatUUID(user.ID),
		Username: "alice2",
		Role:     "member",
	})
	if err == nil {
		t.Fatal("expected error for duplicate user, got nil")
	}
}

func TestCreateMemberAccount_DuplicateUsername(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user1 := testutil.MustCreateUser(t, queries, "Alice")
	user2 := testutil.MustCreateUser(t, queries, "Bob")

	_, err := svc.CreateMemberAccount(ctx, service.CreateMemberAccountParams{
		UserID:   formatUUID(user1.ID),
		Username: "shared_name",
		Role:     "member",
	})
	if err != nil {
		t.Fatalf("unexpected error on first create: %v", err)
	}

	// Same username should fail.
	_, err = svc.CreateMemberAccount(ctx, service.CreateMemberAccountParams{
		UserID:   formatUUID(user2.ID),
		Username: "shared_name",
		Role:     "member",
	})
	if err == nil {
		t.Fatal("expected error for duplicate username, got nil")
	}
}

func TestCreateMemberAccount_UsernameConflictsWithAdmin(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	// Create an admin account first.
	hashedPw, _ := bcrypt.GenerateFromPassword([]byte("testpassword"), 10)
	_, err := queries.CreateAdminAccount(ctx, db.CreateAdminAccountParams{
		Username:       "admin_user",
		HashedPassword: hashedPw,
	})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}

	user := testutil.MustCreateUser(t, queries, "Alice")

	// Try to create a member with same username as admin.
	_, err = svc.CreateMemberAccount(ctx, service.CreateMemberAccountParams{
		UserID:   formatUUID(user.ID),
		Username: "admin_user",
		Role:     "member",
	})
	if err == nil {
		t.Fatal("expected error for username conflict with admin, got nil")
	}
}

func TestCreateMemberAccount_InvalidRole(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")

	_, err := svc.CreateMemberAccount(ctx, service.CreateMemberAccountParams{
		UserID:   formatUUID(user.ID),
		Username: "alice",
		Role:     "superuser",
	})
	if err == nil {
		t.Fatal("expected error for invalid role, got nil")
	}
}

func TestListMemberAccounts_Empty(t *testing.T) {
	svc, _, _ := newService(t)

	members, err := svc.ListMemberAccounts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(members) != 0 {
		t.Fatalf("expected 0 members, got %d", len(members))
	}
}

func TestListMemberAccounts_WithData(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user1 := testutil.MustCreateUser(t, queries, "Alice")
	user2 := testutil.MustCreateUser(t, queries, "Bob")

	_, _ = svc.CreateMemberAccount(ctx, service.CreateMemberAccountParams{
		UserID: formatUUID(user1.ID), Username: "alice", Role: "member",
	})
	_, _ = svc.CreateMemberAccount(ctx, service.CreateMemberAccountParams{
		UserID: formatUUID(user2.ID), Username: "bob", Role: "admin",
	})

	members, err := svc.ListMemberAccounts(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}
	// Ordered by user name.
	if members[0].UserName != "Alice" {
		t.Errorf("expected first member Alice, got %s", members[0].UserName)
	}
}

func TestUpdateMemberRole(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	member, _ := svc.CreateMemberAccount(ctx, service.CreateMemberAccountParams{
		UserID: formatUUID(user.ID), Username: "alice", Role: "member",
	})

	if err := svc.UpdateMemberRole(ctx, member.ID, "admin"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the role was updated.
	members, _ := svc.ListMemberAccounts(ctx)
	if len(members) != 1 || members[0].Role != "admin" {
		t.Errorf("expected role admin after update, got %s", members[0].Role)
	}
}

func TestUpdateMemberRole_InvalidRole(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	member, _ := svc.CreateMemberAccount(ctx, service.CreateMemberAccountParams{
		UserID: formatUUID(user.ID), Username: "alice", Role: "member",
	})

	if err := svc.UpdateMemberRole(ctx, member.ID, "superuser"); err == nil {
		t.Fatal("expected error for invalid role, got nil")
	}
}

func TestDeleteMemberAccount(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	member, _ := svc.CreateMemberAccount(ctx, service.CreateMemberAccountParams{
		UserID: formatUUID(user.ID), Username: "alice", Role: "member",
	})

	if err := svc.DeleteMemberAccount(ctx, member.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	members, _ := svc.ListMemberAccounts(ctx)
	if len(members) != 0 {
		t.Errorf("expected 0 members after delete, got %d", len(members))
	}

	// User should still exist.
	users, _ := svc.ListUsers(ctx)
	if len(users) != 1 {
		t.Errorf("expected user to survive member account deletion, got %d users", len(users))
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

	txnCount, err := svc.WipeUserData(ctx, formatUUID(user.ID))
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

	txnCount, err := svc.WipeUserData(ctx, formatUUID(user.ID))
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
	_, err := svc.WipeUserData(ctx, formatUUID(alice.ID))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Bob's data should be untouched.
	bobID := formatUUID(bob.ID)
	conns, _ := svc.ListConnections(ctx, &bobID)
	if len(conns) != 1 {
		t.Errorf("expected Bob to still have 1 connection, got %d", len(conns))
	}
}

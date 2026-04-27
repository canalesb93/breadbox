//go:build integration

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

// TestListAnnotations_PrefersProfileName verifies that user-attributed
// timeline rows render the joined users.name (a real profile name) instead
// of the email/username carried by Actor.Name at write time. ActorFromSession
// builds Actor.Name from auth_accounts.username — typically an email like
// "admin@example.com" — but the timeline reads better with "Alice Example"
// when the linked household member has a profile name set.
//
// Covers three branches of annotationFromActorRow:
//
//  1. actor_id == users.id, users.name set → ActorName resolves to users.name.
//  2. actor_id == auth_accounts.id (legacy initial-admin sessions),
//     users.name set → still resolves via the auth_accounts → users join.
//  3. actor_id references a hard-deleted user → no join match, falls back
//     to the value frozen into annotations.actor_name at write time.
//
// Non-user actors (agent, system) are exercised by every other comment-
// related integration test in this package — they bypass the join entirely.
func TestListAnnotations_PrefersProfileName(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := seedTransaction(t, queries, acctID, "ext_actor_name_1", "Coffee", 100, "2024-06-01")
	txnID := pgconv.FormatUUID(txn.ID)

	// --- Case 1: actor_id == users.id, users.name set ---
	alice := testutil.MustCreateUser(t, queries, "Alice Example")
	aliceActor := service.Actor{
		Type: "user",
		ID:   pgconv.FormatUUID(alice.ID),
		Name: "alice@example.com", // simulates ActorFromSession (auth username)
	}
	if _, err := svc.CreateComment(ctx, service.CreateCommentParams{
		TransactionID: txnID,
		Content:       "users.id branch",
		Actor:         aliceActor,
	}); err != nil {
		t.Fatalf("create comment (users.id branch): %v", err)
	}

	// --- Case 2: actor_id == auth_accounts.id, joined user has a name ---
	bob := testutil.MustCreateUser(t, queries, "Bob Example")
	bobAccount, err := queries.CreateAuthAccount(ctx, db.CreateAuthAccountParams{
		UserID:         bob.ID,
		Username:       "bob@example.com",
		HashedPassword: []byte("not-real"),
		Role:           "editor",
	})
	if err != nil {
		t.Fatalf("create auth account: %v", err)
	}
	bobActor := service.Actor{
		Type: "user",
		ID:   pgconv.FormatUUID(bobAccount.ID), // legacy: account id, not user id
		Name: "bob@example.com",
	}
	if _, err := svc.CreateComment(ctx, service.CreateCommentParams{
		TransactionID: txnID,
		Content:       "auth_accounts.id branch",
		Actor:         bobActor,
	}); err != nil {
		t.Fatalf("create comment (auth_accounts.id branch): %v", err)
	}

	// --- Case 3: actor_id points at a user we then hard-delete ---
	charlie := testutil.MustCreateUser(t, queries, "Charlie Example")
	charlieActor := service.Actor{
		Type: "user",
		ID:   pgconv.FormatUUID(charlie.ID),
		Name: "charlie@example.com",
	}
	if _, err := svc.CreateComment(ctx, service.CreateCommentParams{
		TransactionID: txnID,
		Content:       "fallback branch",
		Actor:         charlieActor,
	}); err != nil {
		t.Fatalf("create comment (fallback branch): %v", err)
	}
	// Drop Charlie. annotations.actor_id is plain TEXT — no FK — so the row
	// survives, but the join now misses on both sides, exercising the
	// fallback-to-stored-actor_name path. Raw SQL because there is no
	// production DeleteUser query (users are soft-handled elsewhere).
	if _, err := pool.Exec(ctx, "DELETE FROM users WHERE id = $1", charlie.ID); err != nil {
		t.Fatalf("delete charlie: %v", err)
	}

	anns, err := svc.ListAnnotations(ctx, txnID, service.ListAnnotationsParams{
		Kinds: []string{"comment"},
	})
	if err != nil {
		t.Fatalf("ListAnnotations: %v", err)
	}
	if len(anns) != 3 {
		t.Fatalf("expected 3 comments, got %d", len(anns))
	}

	// Comments are ordered by created_at ASC; insertion order matches.
	want := []struct {
		content string
		name    string
	}{
		{"users.id branch", "Alice Example"},
		{"auth_accounts.id branch", "Bob Example"},
		{"fallback branch", "charlie@example.com"},
	}
	for i, w := range want {
		if anns[i].Content != w.content {
			t.Fatalf("annotation[%d] content = %q, want %q", i, anns[i].Content, w.content)
		}
		if anns[i].ActorName != w.name {
			t.Errorf("annotation[%d] (%s) ActorName = %q, want %q",
				i, w.content, anns[i].ActorName, w.name)
		}
	}
}

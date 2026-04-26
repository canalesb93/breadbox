//go:build integration

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

// TestCreateTag_SlugValidation exercises the slug regex against good + bad inputs.
func TestCreateTag_SlugValidation(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	good := []string{"needs-review", "subscription:monthly", "a", "a1", "work:2026"}
	for _, slug := range good {
		_, err := svc.CreateTag(ctx, service.CreateTagParams{
			Slug:        slug,
			DisplayName: "Test",
		})
		if err != nil {
			t.Errorf("expected slug %q to be valid, got error: %v", slug, err)
		}
	}

	bad := []string{"", "Upper", "bad space", "-leading", "trailing-", "::double"}
	for _, slug := range bad {
		_, err := svc.CreateTag(ctx, service.CreateTagParams{
			Slug:        slug,
			DisplayName: "Test",
		})
		if err == nil {
			t.Errorf("expected slug %q to be rejected", slug)
		}
	}
}

// TestAddTransactionTag_AutoCreatesTag verifies that adding a tag whose slug
// doesn't exist creates it with a title-cased display name.
func TestAddTransactionTag_AutoCreatesTag(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "txn1", "Coffee", 500, "2026-03-01")

	// Slug does not exist yet.
	if _, err := queries.GetTagBySlug(ctx, "vacation-2026"); err == nil {
		t.Fatalf("precondition: expected vacation-2026 tag not to exist")
	}

	added, _, err := svc.AddTransactionTag(ctx, txn.ShortID, "vacation-2026", service.Actor{Type: "user", ID: "u1", Name: "Alice"})
	if err != nil {
		t.Fatalf("AddTransactionTag: %v", err)
	}
	if !added {
		t.Fatal("expected added=true")
	}
	// Tag should now exist.
	tag, err := queries.GetTagBySlug(ctx, "vacation-2026")
	if err != nil {
		t.Fatalf("tag was not created: %v", err)
	}
	if tag.DisplayName != "Vacation 2026" {
		t.Errorf("expected display name 'Vacation 2026', got %q", tag.DisplayName)
	}
}

// TestAddTransactionTag_WritesAnnotation verifies tag_added annotation is written.
func TestAddTransactionTag_WritesAnnotation(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "txn2", "Coffee", 500, "2026-03-01")

	_, _, err := svc.AddTransactionTag(ctx, txn.ShortID, "needs-review", service.Actor{Type: "agent", ID: "agent-1", Name: "ReviewBot"})
	if err != nil {
		t.Fatalf("AddTransactionTag: %v", err)
	}

	n := testutil.MustCountAnnotations(t, queries, txn.ID, "tag_added")
	if n != 1 {
		t.Errorf("expected 1 tag_added annotation, got %d", n)
	}

	// Idempotent second add: no new annotation.
	_, alreadyPresent, err := svc.AddTransactionTag(ctx, txn.ShortID, "needs-review", service.Actor{Type: "agent", ID: "agent-1", Name: "ReviewBot"})
	if err != nil {
		t.Fatalf("second AddTransactionTag: %v", err)
	}
	if !alreadyPresent {
		t.Error("expected alreadyPresent=true on second add")
	}
	n = testutil.MustCountAnnotations(t, queries, txn.ID, "tag_added")
	if n != 1 {
		t.Errorf("expected annotation count unchanged at 1 after idempotent add, got %d", n)
	}
}

// TestRemoveTransactionTag_NoNote verifies that a tag can be removed without
// a note — note is optional for all tags.
func TestRemoveTransactionTag_NoNote(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "txn3", "Coffee", 500, "2026-03-01")

	testutil.MustCreateTag(t, queries, "needs-review", "Needs Review")

	if _, _, err := svc.AddTransactionTag(ctx, txn.ShortID, "needs-review", service.Actor{Type: "user", Name: "A"}); err != nil {
		t.Fatalf("AddTransactionTag: %v", err)
	}

	removed, _, err := svc.RemoveTransactionTag(ctx, txn.ShortID, "needs-review", service.Actor{Type: "user", Name: "A"})
	if err != nil {
		t.Fatalf("RemoveTransactionTag without note should succeed: %v", err)
	}
	if !removed {
		t.Error("expected removed=true")
	}

	n := testutil.MustCountAnnotations(t, queries, txn.ID, "tag_removed")
	if n != 1 {
		t.Errorf("expected 1 tag_removed annotation, got %d", n)
	}
}

// TestRemoveTransactionTag_EmptyNoteSucceeds verifies tag removal works with
// an empty note across regular tags.
func TestRemoveTransactionTag_EmptyNoteSucceeds(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "txn4", "Coffee", 500, "2026-03-01")

	testutil.MustCreateTag(t, queries, "watchlist", "Watch")

	if _, _, err := svc.AddTransactionTag(ctx, txn.ShortID, "watchlist", service.Actor{Type: "user", Name: "A"}); err != nil {
		t.Fatalf("AddTransactionTag: %v", err)
	}
	removed, _, err := svc.RemoveTransactionTag(ctx, txn.ShortID, "watchlist", service.Actor{Type: "user", Name: "A"})
	if err != nil {
		t.Fatalf("RemoveTransactionTag: %v", err)
	}
	if !removed {
		t.Error("expected removed=true")
	}
}

// TestRemoveTransactionTag_AlreadyAbsent_NoError verifies removing a tag
// that is not attached returns alreadyAbsent=true with no error.
func TestRemoveTransactionTag_AlreadyAbsent_NoError(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "txn5", "Coffee", 500, "2026-03-01")

	testutil.MustCreateTag(t, queries, "unused", "Unused")

	removed, alreadyAbsent, err := svc.RemoveTransactionTag(ctx, txn.ShortID, "unused", service.Actor{Type: "user", Name: "A"})
	if err != nil {
		t.Fatalf("RemoveTransactionTag: %v", err)
	}
	if removed {
		t.Error("expected removed=false")
	}
	if !alreadyAbsent {
		t.Error("expected alreadyAbsent=true")
	}

	// Also: slug that doesn't even exist at all returns alreadyAbsent.
	removed, alreadyAbsent, err = svc.RemoveTransactionTag(ctx, txn.ShortID, "nonexistent", service.Actor{Type: "user", Name: "A"})
	if err != nil {
		t.Fatalf("RemoveTransactionTag nonexistent: %v", err)
	}
	if removed || !alreadyAbsent {
		t.Errorf("expected removed=false alreadyAbsent=true for nonexistent slug, got removed=%v alreadyAbsent=%v", removed, alreadyAbsent)
	}
}

// TestListTransactionTags returns tags with provenance.
func TestListTransactionTags(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := testutil.MustCreateTransaction(t, queries, acctID, "txn6", "Coffee", 500, "2026-03-01")

	if _, _, err := svc.AddTransactionTag(ctx, txn.ShortID, "needs-review", service.Actor{Type: "user", ID: "u1", Name: "Alice"}); err != nil {
		t.Fatalf("AddTransactionTag: %v", err)
	}

	tags, err := svc.ListTransactionTags(ctx, txn.ShortID)
	if err != nil {
		t.Fatalf("ListTransactionTags: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}
	if tags[0].Slug != "needs-review" {
		t.Errorf("expected slug=needs-review, got %q", tags[0].Slug)
	}
	if tags[0].AddedByType != "user" {
		t.Errorf("expected AddedByType=user, got %q", tags[0].AddedByType)
	}
	if tags[0].AddedByName != "Alice" {
		t.Errorf("expected AddedByName=Alice, got %q", tags[0].AddedByName)
	}
}

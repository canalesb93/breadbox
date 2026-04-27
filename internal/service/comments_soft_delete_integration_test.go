//go:build integration

package service_test

import (
	"context"
	"strings"
	"testing"

	"breadbox/internal/pgconv"
	"breadbox/internal/service"
)

// TestDeleteComment_SoftDeleteTombstone is the canonical integration test for
// the soft-delete behavior introduced by PR 4 of activity-log-v2. The contract
// is: DeleteComment marks the row deleted_at = NOW() instead of removing it,
// so the activity timeline (ListAnnotations) keeps the audit trail with
// IsDeleted=true and a "<Actor> deleted a comment" Summary, while the
// REST/MCP comments listing (ListComments) hides the row to preserve the
// prior external API semantics.
func TestDeleteComment_SoftDeleteTombstone(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := seedTransaction(t, queries, acctID, "ext_soft_delete_1", "Coffee", 100, "2024-06-01")
	txnID := pgconv.FormatUUID(txn.ID)

	actor := service.Actor{Type: "agent", ID: "key-soft-1", Name: "TombstoneBot"}
	comment, err := svc.CreateComment(ctx, service.CreateCommentParams{
		TransactionID: txnID,
		Content:       "About to be tombstoned",
		Actor:         actor,
	})
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}

	if err := svc.DeleteComment(ctx, comment.ID, actor); err != nil {
		t.Fatalf("DeleteComment failed: %v", err)
	}

	// ListComments hides the row — REST/MCP callers see the same external
	// shape as the prior hard-delete behavior.
	comments, err := svc.ListComments(ctx, txnID)
	if err != nil {
		t.Fatalf("ListComments after delete: %v", err)
	}
	if len(comments) != 0 {
		t.Errorf("ListComments after soft-delete: got %d comments, want 0", len(comments))
	}

	// ListAnnotations keeps the row, marks it deleted, and the enriched
	// Summary uses the tombstone phrasing rather than the original body.
	anns, err := svc.ListAnnotations(ctx, txnID, service.ListAnnotationsParams{Kinds: []string{"comment"}})
	if err != nil {
		t.Fatalf("ListAnnotations: %v", err)
	}
	if len(anns) != 1 {
		t.Fatalf("expected 1 annotation row preserved, got %d", len(anns))
	}
	got := anns[0]
	if !got.IsDeleted {
		t.Errorf("annotation IsDeleted = false, want true")
	}
	if got.ActorName != "TombstoneBot" {
		t.Errorf("ActorName = %q, want %q (actor must survive on tombstone)", got.ActorName, "TombstoneBot")
	}
	wantSummary := "TombstoneBot deleted a comment"
	if got.Summary != wantSummary {
		t.Errorf("Summary = %q, want %q", got.Summary, wantSummary)
	}
	if strings.Contains(got.Summary, "About to be tombstoned") {
		t.Errorf("tombstone Summary should not echo the original body; got %q", got.Summary)
	}
}

// TestDeleteComment_Idempotent re-deleting a tombstoned comment must not
// error. The underlying SoftDeleteAnnotation query filters non-tombstoned
// rows, so the second call is a no-op and still returns nil.
func TestDeleteComment_Idempotent(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := seedTransaction(t, queries, acctID, "ext_soft_delete_idem", "Coffee", 100, "2024-06-01")
	txnID := pgconv.FormatUUID(txn.ID)

	actor := service.Actor{Type: "agent", ID: "key-soft-2", Name: "IdemBot"}
	comment, err := svc.CreateComment(ctx, service.CreateCommentParams{
		TransactionID: txnID,
		Content:       "delete me twice",
		Actor:         actor,
	})
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}

	if err := svc.DeleteComment(ctx, comment.ID, actor); err != nil {
		t.Fatalf("first DeleteComment failed: %v", err)
	}
	if err := svc.DeleteComment(ctx, comment.ID, actor); err != nil {
		t.Fatalf("second DeleteComment must be idempotent, got: %v", err)
	}
}

// TestUpdateComment_TombstonedNotFound editing a tombstoned comment must
// fail — the content is retired and external API surfaces shouldn't allow
// a soft-deleted body to come back through an Update path.
func TestUpdateComment_TombstonedNotFound(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	acctID := seedTxnFixture(t, queries)
	txn := seedTransaction(t, queries, acctID, "ext_soft_delete_upd", "Coffee", 100, "2024-06-01")
	txnID := pgconv.FormatUUID(txn.ID)

	actor := service.Actor{Type: "agent", ID: "key-soft-3", Name: "EditBot"}
	comment, err := svc.CreateComment(ctx, service.CreateCommentParams{
		TransactionID: txnID,
		Content:       "first",
		Actor:         actor,
	})
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}

	if err := svc.DeleteComment(ctx, comment.ID, actor); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = svc.UpdateComment(ctx, comment.ID, service.UpdateCommentParams{
		Content: "resurrect",
		Actor:   actor,
	})
	if err != service.ErrNotFound {
		t.Errorf("UpdateComment on tombstone: got %v, want ErrNotFound", err)
	}
}

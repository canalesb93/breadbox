//go:build integration

package service_test

import (
	"context"
	"errors"
	"testing"

	"breadbox/internal/db"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/jackc/pgx/v5/pgtype"
)

// mustCreateCategory creates a category and fatals on error.
func mustCreateCategory(t *testing.T, q *db.Queries, slug, displayName string) db.Category {
	t.Helper()
	cat, err := q.InsertCategory(context.Background(), db.InsertCategoryParams{
		Slug:        slug,
		DisplayName: displayName,
	})
	if err != nil {
		t.Fatalf("mustCreateCategory(%q): %v", slug, err)
	}
	return cat
}

// mustEnqueueReview directly enqueues a review via DB for test setup.
func mustEnqueueReview(t *testing.T, q *db.Queries, txnID pgtype.UUID, reviewType string) db.ReviewQueue {
	t.Helper()
	review, err := q.EnqueueReview(context.Background(), db.EnqueueReviewParams{
		TransactionID: txnID,
		ReviewType:    reviewType,
	})
	if err != nil {
		t.Fatalf("mustEnqueueReview: %v", err)
	}
	return review
}

// reviewTestFixture creates user → connection → account → transaction and returns the transaction.
func reviewTestFixture(t *testing.T, q *db.Queries) db.Transaction {
	t.Helper()
	user := testutil.MustCreateUser(t, q, "Alice")
	conn := testutil.MustCreateConnection(t, q, user.ID, "item_review_1")
	acct := testutil.MustCreateAccount(t, q, conn.ID, "ext_review_1", "Checking")
	txn := testutil.MustCreateTransaction(t, q, acct.ID, "txn_review_1", "Coffee Shop", 550, "2025-01-15")
	return txn
}

var testActor = service.Actor{Type: "user", ID: "admin-1", Name: "Test Admin"}

// --- EnqueueManualReview ---

func TestEnqueueManualReview_Success(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	txn := reviewTestFixture(t, queries)
	txnID := formatUUID(txn.ID)

	review, err := svc.EnqueueManualReview(ctx, txnID, testActor)
	if err != nil {
		t.Fatalf("EnqueueManualReview: %v", err)
	}

	if review.TransactionID != txnID {
		t.Errorf("expected transaction_id=%s, got %s", txnID, review.TransactionID)
	}
	if review.ReviewType != "manual" {
		t.Errorf("expected review_type=manual, got %s", review.ReviewType)
	}
	if review.Status != "pending" {
		t.Errorf("expected status=pending, got %s", review.Status)
	}
}

func TestEnqueueManualReview_DuplicatePending(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	txn := reviewTestFixture(t, queries)
	txnID := formatUUID(txn.ID)

	// First enqueue succeeds
	_, err := svc.EnqueueManualReview(ctx, txnID, testActor)
	if err != nil {
		t.Fatalf("first EnqueueManualReview: %v", err)
	}

	// Second enqueue should fail with ErrReviewAlreadyPending
	_, err = svc.EnqueueManualReview(ctx, txnID, testActor)
	if !errors.Is(err, service.ErrReviewAlreadyPending) {
		t.Errorf("expected ErrReviewAlreadyPending, got %v", err)
	}
}

func TestEnqueueManualReview_SoftDeletedTransaction(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()

	txn := reviewTestFixture(t, queries)
	txnID := formatUUID(txn.ID)

	// Soft-delete the transaction
	_, err := pool.Exec(ctx, "UPDATE transactions SET deleted_at = NOW() WHERE id = $1", txn.ID)
	if err != nil {
		t.Fatalf("soft-delete txn: %v", err)
	}

	_, err = svc.EnqueueManualReview(ctx, txnID, testActor)
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound for soft-deleted transaction, got %v", err)
	}
}

func TestEnqueueManualReview_NonexistentTransaction(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	_, err := svc.EnqueueManualReview(ctx, "00000000-0000-0000-0000-000000000000", testActor)
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- GetReview ---

func TestGetReview_Success(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	txn := reviewTestFixture(t, queries)
	review := mustEnqueueReview(t, queries, txn.ID, "new_transaction")

	result, err := svc.GetReview(ctx, formatUUID(review.ID))
	if err != nil {
		t.Fatalf("GetReview: %v", err)
	}

	if result.ID != formatUUID(review.ID) {
		t.Errorf("expected id=%s, got %s", formatUUID(review.ID), result.ID)
	}
	if result.ReviewType != "new_transaction" {
		t.Errorf("expected review_type=new_transaction, got %s", result.ReviewType)
	}
	if result.Status != "pending" {
		t.Errorf("expected status=pending, got %s", result.Status)
	}
	if result.Transaction == nil {
		t.Error("expected transaction to be populated")
	} else if result.Transaction.Name != "Coffee Shop" {
		t.Errorf("expected transaction name=Coffee Shop, got %s", result.Transaction.Name)
	}
}

func TestGetReview_NotFound(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	_, err := svc.GetReview(ctx, "00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- ListReviews ---

func TestListReviews_Empty(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	result, err := svc.ListReviews(ctx, service.ReviewListParams{})
	if err != nil {
		t.Fatalf("ListReviews: %v", err)
	}
	if len(result.Reviews) != 0 {
		t.Errorf("expected 0 reviews, got %d", len(result.Reviews))
	}
	if result.Total != 0 {
		t.Errorf("expected total=0, got %d", result.Total)
	}
}

func TestListReviews_DefaultsPending(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	// Create two transactions with reviews in different states
	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_lr1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_lr1", "Checking")

	txn1 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_lr1", "Pending Review", 1000, "2025-01-15")
	txn2 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_lr2", "Resolved Review", 2000, "2025-01-16")

	mustEnqueueReview(t, queries, txn1.ID, "new_transaction")
	review2 := mustEnqueueReview(t, queries, txn2.ID, "uncategorized")

	// Resolve review2
	_, err := queries.UpdateReviewDecision(ctx, db.UpdateReviewDecisionParams{
		ID:     review2.ID,
		Status: "approved",
		ReviewerType: pgtype.Text{String: "user", Valid: true},
		ReviewerName: pgtype.Text{String: "Admin", Valid: true},
	})
	if err != nil {
		t.Fatalf("resolve review: %v", err)
	}

	// Default listing should only return pending
	result, err := svc.ListReviews(ctx, service.ReviewListParams{})
	if err != nil {
		t.Fatalf("ListReviews: %v", err)
	}
	if len(result.Reviews) != 1 {
		t.Fatalf("expected 1 pending review, got %d", len(result.Reviews))
	}
	if result.Reviews[0].Transaction.Name != "Pending Review" {
		t.Errorf("expected Pending Review, got %s", result.Reviews[0].Transaction.Name)
	}
}

func TestListReviews_FilterByType(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_ft1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_ft1", "Checking")

	txn1 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_ft1", "New Txn", 1000, "2025-01-15")
	txn2 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_ft2", "Uncat Txn", 2000, "2025-01-16")

	mustEnqueueReview(t, queries, txn1.ID, "new_transaction")
	mustEnqueueReview(t, queries, txn2.ID, "uncategorized")

	reviewType := "uncategorized"
	result, err := svc.ListReviews(ctx, service.ReviewListParams{
		ReviewType: &reviewType,
	})
	if err != nil {
		t.Fatalf("ListReviews: %v", err)
	}
	if len(result.Reviews) != 1 {
		t.Fatalf("expected 1 uncategorized review, got %d", len(result.Reviews))
	}
	if result.Reviews[0].ReviewType != "uncategorized" {
		t.Errorf("expected uncategorized, got %s", result.Reviews[0].ReviewType)
	}
}

func TestListReviews_InvalidStatus(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	invalid := "invalid_status"
	_, err := svc.ListReviews(ctx, service.ReviewListParams{Status: &invalid})
	if err == nil {
		t.Error("expected error for invalid status")
	}
}

func TestListReviews_StatusAll(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_sa1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_sa1", "Checking")

	txn1 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_sa1", "Txn1", 1000, "2025-01-15")
	txn2 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_sa2", "Txn2", 2000, "2025-01-16")

	mustEnqueueReview(t, queries, txn1.ID, "new_transaction")
	review2 := mustEnqueueReview(t, queries, txn2.ID, "uncategorized")

	// Resolve one
	_, err := queries.UpdateReviewDecision(ctx, db.UpdateReviewDecisionParams{
		ID:           review2.ID,
		Status:       "rejected",
		ReviewerType: pgtype.Text{String: "user", Valid: true},
		ReviewerName: pgtype.Text{String: "Admin", Valid: true},
	})
	if err != nil {
		t.Fatalf("resolve review: %v", err)
	}

	all := "all"
	result, err := svc.ListReviews(ctx, service.ReviewListParams{Status: &all})
	if err != nil {
		t.Fatalf("ListReviews: %v", err)
	}
	if len(result.Reviews) != 2 {
		t.Errorf("expected 2 reviews with status=all, got %d", len(result.Reviews))
	}
}

// --- SubmitReview ---

func TestSubmitReview_Approve(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	txn := reviewTestFixture(t, queries)
	review := mustEnqueueReview(t, queries, txn.ID, "new_transaction")

	result, err := svc.SubmitReview(ctx, service.SubmitReviewParams{
		ReviewID: formatUUID(review.ID),
		Decision: "approved",
		Actor:    testActor,
	})
	if err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}
	if result.Status != "approved" {
		t.Errorf("expected status=approved, got %s", result.Status)
	}
	if result.ReviewerName == nil || *result.ReviewerName != "Test Admin" {
		t.Error("expected reviewer_name to be Test Admin")
	}
	if result.ReviewedAt == nil {
		t.Error("expected reviewed_at to be set")
	}
}

func TestSubmitReview_Reject(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	txn := reviewTestFixture(t, queries)
	review := mustEnqueueReview(t, queries, txn.ID, "new_transaction")

	result, err := svc.SubmitReview(ctx, service.SubmitReviewParams{
		ReviewID: formatUUID(review.ID),
		Decision: "rejected",
		Actor:    testActor,
	})
	if err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}
	if result.Status != "rejected" {
		t.Errorf("expected status=rejected, got %s", result.Status)
	}
}

func TestSubmitReview_Skip(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	txn := reviewTestFixture(t, queries)
	review := mustEnqueueReview(t, queries, txn.ID, "new_transaction")

	result, err := svc.SubmitReview(ctx, service.SubmitReviewParams{
		ReviewID: formatUUID(review.ID),
		Decision: "skipped",
		Actor:    testActor,
	})
	if err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}
	if result.Status != "skipped" {
		t.Errorf("expected status=skipped, got %s", result.Status)
	}
}

func TestSubmitReview_InvalidDecision(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	txn := reviewTestFixture(t, queries)
	review := mustEnqueueReview(t, queries, txn.ID, "new_transaction")

	_, err := svc.SubmitReview(ctx, service.SubmitReviewParams{
		ReviewID: formatUUID(review.ID),
		Decision: "invalid",
		Actor:    testActor,
	})
	if !errors.Is(err, service.ErrInvalidDecision) {
		t.Errorf("expected ErrInvalidDecision, got %v", err)
	}
}

func TestSubmitReview_AlreadyResolved(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	txn := reviewTestFixture(t, queries)
	review := mustEnqueueReview(t, queries, txn.ID, "new_transaction")

	// First submit succeeds
	_, err := svc.SubmitReview(ctx, service.SubmitReviewParams{
		ReviewID: formatUUID(review.ID),
		Decision: "approved",
		Actor:    testActor,
	})
	if err != nil {
		t.Fatalf("first SubmitReview: %v", err)
	}

	// Second submit should fail
	_, err = svc.SubmitReview(ctx, service.SubmitReviewParams{
		ReviewID: formatUUID(review.ID),
		Decision: "rejected",
		Actor:    testActor,
	})
	if !errors.Is(err, service.ErrReviewAlreadyResolved) {
		t.Errorf("expected ErrReviewAlreadyResolved, got %v", err)
	}
}

func TestSubmitReview_WithCategoryOverride(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()

	txn := reviewTestFixture(t, queries)
	review := mustEnqueueReview(t, queries, txn.ID, "uncategorized")

	// Create a category
	cat := mustCreateCategory(t, queries, "food_and_drink", "Food & Drink")
	catID := formatUUID(cat.ID)

	// Approve with explicit category
	result, err := svc.SubmitReview(ctx, service.SubmitReviewParams{
		ReviewID:   formatUUID(review.ID),
		Decision:   "approved",
		CategoryID: &catID,
		Actor:      testActor,
	})
	if err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}
	if result.ResolvedCategoryID == nil || *result.ResolvedCategoryID != catID {
		t.Error("expected resolved_category_id to match the provided category")
	}

	// Verify the transaction was updated with the category and override flag
	var txnCatID pgtype.UUID
	var catOverride bool
	err = pool.QueryRow(ctx, "SELECT category_id, category_override FROM transactions WHERE id = $1", txn.ID).Scan(&txnCatID, &catOverride)
	if err != nil {
		t.Fatalf("query transaction: %v", err)
	}
	if !txnCatID.Valid {
		t.Error("expected transaction category_id to be set")
	}
	if formatUUID(txnCatID) != catID {
		t.Errorf("expected transaction category_id=%s, got %s", catID, formatUUID(txnCatID))
	}
	if !catOverride {
		t.Error("expected category_override=true after review approval with category")
	}
}

func TestSubmitReview_WithNote(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	txn := reviewTestFixture(t, queries)
	review := mustEnqueueReview(t, queries, txn.ID, "new_transaction")

	note := "This looks like a duplicate charge"
	result, err := svc.SubmitReview(ctx, service.SubmitReviewParams{
		ReviewID: formatUUID(review.ID),
		Decision: "rejected",
		Note:     &note,
		Actor:    testActor,
	})
	if err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}
	if result.ReviewNote == nil || *result.ReviewNote != note {
		t.Error("expected review_note to be set")
	}
}

func TestSubmitReview_NotFound(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	_, err := svc.SubmitReview(ctx, service.SubmitReviewParams{
		ReviewID: "00000000-0000-0000-0000-000000000000",
		Decision: "approved",
		Actor:    testActor,
	})
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- BulkSubmitReviews ---

func TestBulkSubmitReviews_Success(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_bulk1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_bulk1", "Checking")

	txn1 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_b1", "Txn1", 1000, "2025-01-15")
	txn2 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_b2", "Txn2", 2000, "2025-01-16")

	r1 := mustEnqueueReview(t, queries, txn1.ID, "new_transaction")
	r2 := mustEnqueueReview(t, queries, txn2.ID, "new_transaction")

	result, err := svc.BulkSubmitReviews(ctx, service.BulkSubmitReviewParams{
		Reviews: []service.BulkReviewItem{
			{ReviewID: formatUUID(r1.ID), Decision: "approved"},
			{ReviewID: formatUUID(r2.ID), Decision: "rejected"},
		},
		Actor: testActor,
	})
	if err != nil {
		t.Fatalf("BulkSubmitReviews: %v", err)
	}
	if result.Succeeded != 2 {
		t.Errorf("expected 2 succeeded, got %d", result.Succeeded)
	}
	if len(result.Failed) != 0 {
		t.Errorf("expected 0 failed, got %d", len(result.Failed))
	}
}

func TestBulkSubmitReviews_PartialFailure(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	txn := reviewTestFixture(t, queries)
	review := mustEnqueueReview(t, queries, txn.ID, "new_transaction")

	result, err := svc.BulkSubmitReviews(ctx, service.BulkSubmitReviewParams{
		Reviews: []service.BulkReviewItem{
			{ReviewID: formatUUID(review.ID), Decision: "approved"},
			{ReviewID: "00000000-0000-0000-0000-000000000000", Decision: "approved"}, // nonexistent
		},
		Actor: testActor,
	})
	if err != nil {
		t.Fatalf("BulkSubmitReviews: %v", err)
	}
	if result.Succeeded != 1 {
		t.Errorf("expected 1 succeeded, got %d", result.Succeeded)
	}
	if len(result.Failed) != 1 {
		t.Errorf("expected 1 failed, got %d", len(result.Failed))
	}
}

func TestBulkSubmitReviews_EmptyArray(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	_, err := svc.BulkSubmitReviews(ctx, service.BulkSubmitReviewParams{
		Reviews: []service.BulkReviewItem{},
		Actor:   testActor,
	})
	if !errors.Is(err, service.ErrInvalidParameter) {
		t.Errorf("expected ErrInvalidParameter for empty reviews, got %v", err)
	}
}

// --- DismissReview ---

func TestDismissReview_Success(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	txn := reviewTestFixture(t, queries)
	review := mustEnqueueReview(t, queries, txn.ID, "new_transaction")

	err := svc.DismissReview(ctx, formatUUID(review.ID), testActor)
	if err != nil {
		t.Fatalf("DismissReview: %v", err)
	}

	// Verify it's gone
	_, err = svc.GetReview(ctx, formatUUID(review.ID))
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound after dismiss, got %v", err)
	}
}

func TestDismissReview_AlreadyResolved(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	txn := reviewTestFixture(t, queries)
	review := mustEnqueueReview(t, queries, txn.ID, "new_transaction")

	// Resolve it first
	_, err := svc.SubmitReview(ctx, service.SubmitReviewParams{
		ReviewID: formatUUID(review.ID),
		Decision: "approved",
		Actor:    testActor,
	})
	if err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}

	// Now try to dismiss
	err = svc.DismissReview(ctx, formatUUID(review.ID), testActor)
	if !errors.Is(err, service.ErrReviewAlreadyResolved) {
		t.Errorf("expected ErrReviewAlreadyResolved, got %v", err)
	}
}

func TestDismissReview_NotFound(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	err := svc.DismissReview(ctx, "00000000-0000-0000-0000-000000000000", testActor)
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- DismissAllPendingReviews ---

func TestDismissAllPendingReviews(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_dap1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_dap1", "Checking")

	txn1 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_dap1", "Txn1", 1000, "2025-01-15")
	txn2 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_dap2", "Txn2", 2000, "2025-01-16")
	txn3 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_dap3", "Txn3", 3000, "2025-01-17")

	mustEnqueueReview(t, queries, txn1.ID, "new_transaction")
	mustEnqueueReview(t, queries, txn2.ID, "uncategorized")
	r3 := mustEnqueueReview(t, queries, txn3.ID, "new_transaction")

	// Resolve one so it should NOT be dismissed
	_, err := queries.UpdateReviewDecision(ctx, db.UpdateReviewDecisionParams{
		ID:           r3.ID,
		Status:       "approved",
		ReviewerType: pgtype.Text{String: "user", Valid: true},
		ReviewerName: pgtype.Text{String: "Admin", Valid: true},
	})
	if err != nil {
		t.Fatalf("resolve review: %v", err)
	}

	count, err := svc.DismissAllPendingReviews(ctx, testActor)
	if err != nil {
		t.Fatalf("DismissAllPendingReviews: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 dismissed, got %d", count)
	}

	// Verify: pending count should be 0
	all := "all"
	result, err := svc.ListReviews(ctx, service.ReviewListParams{Status: &all})
	if err != nil {
		t.Fatalf("ListReviews: %v", err)
	}
	// Only the resolved one should remain
	if len(result.Reviews) != 1 {
		t.Errorf("expected 1 remaining review (resolved), got %d", len(result.Reviews))
	}
}

// --- GetReviewCounts ---

func TestGetReviewCounts_Empty(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	counts, err := svc.GetReviewCounts(ctx)
	if err != nil {
		t.Fatalf("GetReviewCounts: %v", err)
	}
	if counts.Pending != 0 {
		t.Errorf("expected pending=0, got %d", counts.Pending)
	}
}

func TestGetReviewCounts_WithData(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_rc1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_rc1", "Checking")

	txn1 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_rc1", "Txn1", 1000, "2025-01-15")
	txn2 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_rc2", "Txn2", 2000, "2025-01-16")
	txn3 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_rc3", "Txn3", 3000, "2025-01-17")

	mustEnqueueReview(t, queries, txn1.ID, "new_transaction")
	mustEnqueueReview(t, queries, txn2.ID, "uncategorized")
	r3 := mustEnqueueReview(t, queries, txn3.ID, "new_transaction")

	// Approve r3 (this makes it approved today)
	_, err := svc.SubmitReview(ctx, service.SubmitReviewParams{
		ReviewID: formatUUID(r3.ID),
		Decision: "approved",
		Actor:    testActor,
	})
	if err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}

	counts, err := svc.GetReviewCounts(ctx)
	if err != nil {
		t.Fatalf("GetReviewCounts: %v", err)
	}
	if counts.Pending != 2 {
		t.Errorf("expected pending=2, got %d", counts.Pending)
	}
	if counts.ApprovedToday != 1 {
		t.Errorf("expected approved_today=1, got %d", counts.ApprovedToday)
	}
}

// --- GetReviewSummary ---

func TestGetReviewSummary_Empty(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	summary, err := svc.GetReviewSummary(ctx)
	if err != nil {
		t.Fatalf("GetReviewSummary: %v", err)
	}
	if summary.TotalPending != 0 {
		t.Errorf("expected total_pending=0, got %d", summary.TotalPending)
	}
	if len(summary.Groups) != 0 {
		t.Errorf("expected 0 groups, got %d", len(summary.Groups))
	}
}

func TestGetReviewSummary_GroupsByCategory(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_rs1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_rs1", "Checking")

	txn1 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_rs1", "Coffee", 500, "2025-01-15")
	txn2 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_rs2", "Lunch", 1200, "2025-01-16")
	txn3 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_rs3", "Dinner", 2500, "2025-01-17")

	// Set category_primary on txns
	_, err := pool.Exec(ctx, "UPDATE transactions SET category_primary = 'FOOD_AND_DRINK' WHERE id = $1", txn1.ID)
	if err != nil {
		t.Fatalf("update txn1: %v", err)
	}
	_, err = pool.Exec(ctx, "UPDATE transactions SET category_primary = 'FOOD_AND_DRINK' WHERE id = $1", txn2.ID)
	if err != nil {
		t.Fatalf("update txn2: %v", err)
	}
	_, err = pool.Exec(ctx, "UPDATE transactions SET category_primary = 'SHOPPING' WHERE id = $1", txn3.ID)
	if err != nil {
		t.Fatalf("update txn3: %v", err)
	}

	mustEnqueueReview(t, queries, txn1.ID, "new_transaction")
	mustEnqueueReview(t, queries, txn2.ID, "new_transaction")
	mustEnqueueReview(t, queries, txn3.ID, "new_transaction")

	summary, err := svc.GetReviewSummary(ctx)
	if err != nil {
		t.Fatalf("GetReviewSummary: %v", err)
	}
	if summary.TotalPending != 3 {
		t.Errorf("expected total_pending=3, got %d", summary.TotalPending)
	}
	if len(summary.Groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(summary.Groups))
	}
	// FOOD_AND_DRINK group should come first (count=2 > count=1)
	if len(summary.Groups) >= 1 && summary.Groups[0].CategoryPrimaryRaw != "FOOD_AND_DRINK" {
		t.Errorf("expected first group=FOOD_AND_DRINK, got %s", summary.Groups[0].CategoryPrimaryRaw)
	}
	if len(summary.Groups) >= 1 && summary.Groups[0].Count != 2 {
		t.Errorf("expected first group count=2, got %d", summary.Groups[0].Count)
	}
}

// --- AutoApproveCategorizedReviews ---

func TestAutoApproveCategorizedReviews_ApprovesWhenCategorized(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_aa1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_aa1", "Checking")

	txn1 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_aa1", "Categorized Txn", 1000, "2025-01-15")
	txn2 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_aa2", "Uncategorized Txn", 2000, "2025-01-16")

	// Create a category and assign it to txn1
	cat := mustCreateCategory(t, queries, "food_and_drink", "Food & Drink")
	_, err := pool.Exec(ctx, "UPDATE transactions SET category_id = $1 WHERE id = $2", cat.ID, txn1.ID)
	if err != nil {
		t.Fatalf("set category: %v", err)
	}

	mustEnqueueReview(t, queries, txn1.ID, "new_transaction")
	mustEnqueueReview(t, queries, txn2.ID, "new_transaction")

	result, err := svc.AutoApproveCategorizedReviews(ctx, testActor)
	if err != nil {
		t.Fatalf("AutoApproveCategorizedReviews: %v", err)
	}
	if result.Approved != 1 {
		t.Errorf("expected approved=1, got %d", result.Approved)
	}
	if result.Remaining != 1 {
		t.Errorf("expected remaining=1, got %d", result.Remaining)
	}
}

func TestAutoApproveCategorizedReviews_SkipsCategoryOverride(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_aa2")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_aa2", "Checking")

	txn := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_aa_co", "Override Txn", 1000, "2025-01-15")

	// Set category AND category_override=true (auto-approve should skip these)
	cat := mustCreateCategory(t, queries, "transport", "Transportation")
	_, err := pool.Exec(ctx, "UPDATE transactions SET category_id = $1, category_override = TRUE WHERE id = $2", cat.ID, txn.ID)
	if err != nil {
		t.Fatalf("set category with override: %v", err)
	}

	mustEnqueueReview(t, queries, txn.ID, "new_transaction")

	result, err := svc.AutoApproveCategorizedReviews(ctx, testActor)
	if err != nil {
		t.Fatalf("AutoApproveCategorizedReviews: %v", err)
	}
	// category_override=true means the query condition `category_override = FALSE` excludes it
	if result.Approved != 0 {
		t.Errorf("expected approved=0 (category_override=true should be skipped), got %d", result.Approved)
	}
}

func TestAutoApproveCategorizedReviews_NoneEligible(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	txn := reviewTestFixture(t, queries)
	// No category assigned — should not be auto-approved
	mustEnqueueReview(t, queries, txn.ID, "new_transaction")

	result, err := svc.AutoApproveCategorizedReviews(ctx, testActor)
	if err != nil {
		t.Fatalf("AutoApproveCategorizedReviews: %v", err)
	}
	if result.Approved != 0 {
		t.Errorf("expected approved=0, got %d", result.Approved)
	}
}

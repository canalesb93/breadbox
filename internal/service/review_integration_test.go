//go:build integration

package service_test

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/jackc/pgx/v5/pgtype"
)

// --- ListPendingReviews ---

func TestListPendingReviews_Empty(t *testing.T) {
	svc, _, _ := newService(t)

	result, err := svc.ListPendingReviews(context.Background(), service.PendingReviewParams{})
	if err != nil {
		t.Fatalf("ListPendingReviews: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(result.Items))
	}
	if result.TotalPending != 0 {
		t.Errorf("expected 0 total pending, got %d", result.TotalPending)
	}
}

func TestListPendingReviews_WithData(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	// Create user -> connection -> account -> transaction
	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_acct_1", "Checking")
	txn := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_001", "Starbucks", 450, "2025-01-15")

	// Enqueue a review manually via the DB query
	review, err := queries.EnqueueReview(ctx, db.EnqueueReviewParams{
		TransactionID: txn.ID,
		ReviewType:    "new_transaction",
	})
	if err != nil {
		t.Fatalf("EnqueueReview: %v", err)
	}

	result, err := svc.ListPendingReviews(ctx, service.PendingReviewParams{})
	if err != nil {
		t.Fatalf("ListPendingReviews: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if result.TotalPending != 1 {
		t.Errorf("expected 1 total pending, got %d", result.TotalPending)
	}

	item := result.Items[0]
	if item.ReviewID != formatUUID(review.ID) {
		t.Errorf("expected review ID %s, got %s", formatUUID(review.ID), item.ReviewID)
	}
	if item.TransactionID != formatUUID(txn.ID) {
		t.Errorf("expected transaction ID %s, got %s", formatUUID(txn.ID), item.TransactionID)
	}
	if item.ReviewType != "new_transaction" {
		t.Errorf("expected review type new_transaction, got %s", item.ReviewType)
	}
	// Verify transaction context is populated
	if item.Transaction.Name != "Starbucks" {
		t.Errorf("expected transaction name Starbucks, got %s", item.Transaction.Name)
	}
	if item.Transaction.Amount != 4.50 {
		t.Errorf("expected amount 4.50, got %f", item.Transaction.Amount)
	}
	if item.Transaction.AccountName == nil || *item.Transaction.AccountName != "Checking" {
		t.Errorf("expected account name Checking, got %v", item.Transaction.AccountName)
	}
	if item.Transaction.UserName == nil || *item.Transaction.UserName != "Alice" {
		t.Errorf("expected user name Alice, got %v", item.Transaction.UserName)
	}
}

func TestListPendingReviews_WithInstructions(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	// Create a user and a pending review so variables expand
	user := testutil.MustCreateUser(t, queries, "Alice")
	testutil.MustCreateUser(t, queries, "Bob")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_1", "Checking")
	txn := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_001", "Test", 100, "2025-01-15")

	_, err := queries.EnqueueReview(ctx, db.EnqueueReviewParams{
		TransactionID: txn.ID,
		ReviewType:    "uncategorized",
	})
	if err != nil {
		t.Fatalf("EnqueueReview: %v", err)
	}

	// Save instructions with template variables
	instructions := "Review {{total_pending}} pending items for family: {{family_members}}"
	if err := svc.SaveReviewInstructions(ctx, instructions, "custom"); err != nil {
		t.Fatalf("SaveReviewInstructions: %v", err)
	}

	result, err := svc.ListPendingReviews(ctx, service.PendingReviewParams{
		IncludeInstructions: true,
	})
	if err != nil {
		t.Fatalf("ListPendingReviews: %v", err)
	}

	if result.ReviewInstructions == nil {
		t.Fatal("expected review instructions to be non-nil")
	}
	inst := *result.ReviewInstructions

	// Variables should be expanded
	if strings.Contains(inst, "{{total_pending}}") {
		t.Error("expected {{total_pending}} to be expanded")
	}
	if !strings.Contains(inst, "1") {
		t.Errorf("expected expanded instructions to contain '1' for total_pending, got: %s", inst)
	}
	if strings.Contains(inst, "{{family_members}}") {
		t.Error("expected {{family_members}} to be expanded")
	}
	if !strings.Contains(inst, "Alice") || !strings.Contains(inst, "Bob") {
		t.Errorf("expected expanded instructions to contain Alice and Bob, got: %s", inst)
	}
}

// --- SubmitReviews (batch) ---

func TestSubmitReviews_Batch(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	// Create a category for override
	groceries, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug:        "food_and_drink_groceries",
		DisplayName: "Groceries",
	})
	if err != nil {
		t.Fatalf("InsertCategory: %v", err)
	}

	// Create test data: 3 transactions with pending reviews
	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_1", "Checking")

	txn1 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_a", "Starbucks", 500, "2025-01-15")
	txn2 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_b", "Whole Foods", 2500, "2025-01-14")
	txn3 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_c", "Shell Gas", 4000, "2025-01-13")

	r1, err := queries.EnqueueReview(ctx, db.EnqueueReviewParams{TransactionID: txn1.ID, ReviewType: "new_transaction"})
	if err != nil {
		t.Fatalf("EnqueueReview 1: %v", err)
	}
	r2, err := queries.EnqueueReview(ctx, db.EnqueueReviewParams{TransactionID: txn2.ID, ReviewType: "uncategorized"})
	if err != nil {
		t.Fatalf("EnqueueReview 2: %v", err)
	}
	r3, err := queries.EnqueueReview(ctx, db.EnqueueReviewParams{TransactionID: txn3.ID, ReviewType: "new_transaction"})
	if err != nil {
		t.Fatalf("EnqueueReview 3: %v", err)
	}

	actor := service.Actor{Type: "agent", ID: "test-agent", Name: "Test Agent"}
	overrideSlug := groceries.Slug
	comment := "Looks like groceries"

	decisions := []service.ReviewDecision{
		{ReviewID: formatUUID(r1.ID), Decision: "approve"},
		{ReviewID: formatUUID(r2.ID), Decision: "reject", OverrideCategorySlug: &overrideSlug, Comment: &comment},
		{ReviewID: formatUUID(r3.ID), Decision: "skip"},
	}

	result, err := svc.SubmitReviews(ctx, decisions, actor)
	if err != nil {
		t.Fatalf("SubmitReviews: %v", err)
	}

	if result.Submitted != 3 {
		t.Errorf("expected 3 submitted, got %d", result.Submitted)
	}
	if len(result.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result.Results))
	}

	// All should be accepted
	for i, r := range result.Results {
		if r.Status != "accepted" {
			t.Errorf("result[%d]: expected status accepted, got %s (error: %s)", i, r.Status, r.Error)
		}
	}

	// Verify pending count is now 0
	pending, err := svc.ListPendingReviews(ctx, service.PendingReviewParams{})
	if err != nil {
		t.Fatalf("ListPendingReviews after submit: %v", err)
	}
	if pending.TotalPending != 0 {
		t.Errorf("expected 0 pending after batch submit, got %d", pending.TotalPending)
	}

	// Verify the rejected review applied category override
	review2, err := svc.GetReview(ctx, formatUUID(r2.ID))
	if err != nil {
		t.Fatalf("GetReview r2: %v", err)
	}
	if review2.Status != "rejected" {
		t.Errorf("expected status rejected, got %s", review2.Status)
	}
	if review2.ResolvedCategory == nil || *review2.ResolvedCategory != groceries.Slug {
		t.Errorf("expected resolved category %s, got %v", groceries.Slug, review2.ResolvedCategory)
	}
	if review2.ReviewerType == nil || *review2.ReviewerType != "agent" {
		t.Errorf("expected reviewer type agent, got %v", review2.ReviewerType)
	}
	if review2.ReviewerName == nil || *review2.ReviewerName != "Test Agent" {
		t.Errorf("expected reviewer name Test Agent, got %v", review2.ReviewerName)
	}

	// Verify the skipped review
	review3, err := svc.GetReview(ctx, formatUUID(r3.ID))
	if err != nil {
		t.Fatalf("GetReview r3: %v", err)
	}
	if review3.Status != "skipped" {
		t.Errorf("expected status skipped, got %s", review3.Status)
	}
}

// --- SaveAndGetReviewInstructions ---

func TestSaveAndGetReviewInstructions(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	instructions := "Please review all transactions carefully. Focus on categorization accuracy."
	templateSlug := "strict_review"

	if err := svc.SaveReviewInstructions(ctx, instructions, templateSlug); err != nil {
		t.Fatalf("SaveReviewInstructions: %v", err)
	}

	raw, slug, err := svc.GetReviewInstructionsRaw(ctx)
	if err != nil {
		t.Fatalf("GetReviewInstructionsRaw: %v", err)
	}
	if raw != instructions {
		t.Errorf("expected raw instructions %q, got %q", instructions, raw)
	}
	if slug != templateSlug {
		t.Errorf("expected template slug %q, got %q", templateSlug, slug)
	}
}

func TestGetReviewInstructions_ExpandsVariables(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	// Create users for family_members expansion
	user := testutil.MustCreateUser(t, queries, "Alice")
	testutil.MustCreateUser(t, queries, "Bob")

	// Create a pending review so total_pending > 0
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_1", "Checking")
	txn := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_001", "Test", 100, "2025-01-15")
	_, err := queries.EnqueueReview(ctx, db.EnqueueReviewParams{
		TransactionID: txn.ID,
		ReviewType:    "new_transaction",
	})
	if err != nil {
		t.Fatalf("EnqueueReview: %v", err)
	}

	instructions := "There are {{total_pending}} pending reviews for {{family_members}}."
	if err := svc.SaveReviewInstructions(ctx, instructions, ""); err != nil {
		t.Fatalf("SaveReviewInstructions: %v", err)
	}

	expanded, err := svc.GetReviewInstructions(ctx)
	if err != nil {
		t.Fatalf("GetReviewInstructions: %v", err)
	}

	if strings.Contains(expanded, "{{total_pending}}") {
		t.Error("expected {{total_pending}} to be expanded")
	}
	if strings.Contains(expanded, "{{family_members}}") {
		t.Error("expected {{family_members}} to be expanded")
	}
	if !strings.Contains(expanded, "1") {
		t.Errorf("expected '1' for total_pending in: %s", expanded)
	}
	if !strings.Contains(expanded, "Alice") {
		t.Errorf("expected 'Alice' in expanded instructions: %s", expanded)
	}
	if !strings.Contains(expanded, "Bob") {
		t.Errorf("expected 'Bob' in expanded instructions: %s", expanded)
	}
}

// --- SaveWebhookConfig ---

func TestSaveWebhookConfig_AutoGeneratesSecret(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	result, err := svc.SaveWebhookConfig(ctx, service.WebhookConfig{
		URL:    "https://example.com/webhook",
		Events: []string{"review_items_added"},
	})
	if err != nil {
		t.Fatalf("SaveWebhookConfig: %v", err)
	}

	if result.Secret == "" {
		t.Error("expected auto-generated secret, got empty")
	}
	if len(result.Secret) < 32 {
		t.Errorf("expected secret length >= 32, got %d", len(result.Secret))
	}
	if !result.SecretConfigured {
		t.Error("expected SecretConfigured to be true")
	}

	// Verify config round-trips
	cfg, err := svc.GetWebhookConfig(ctx)
	if err != nil {
		t.Fatalf("GetWebhookConfig: %v", err)
	}
	if cfg.URL != "https://example.com/webhook" {
		t.Errorf("expected URL https://example.com/webhook, got %s", cfg.URL)
	}
	if !cfg.SecretConfigured {
		t.Error("expected SecretConfigured to be true on re-read")
	}
}

func TestSaveWebhookConfig_RequiresHTTPS(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	_, err := svc.SaveWebhookConfig(ctx, service.WebhookConfig{
		URL:    "http://example.com/webhook",
		Events: []string{"review_items_added"},
	})
	if err == nil {
		t.Fatal("expected error for http:// URL, got nil")
	}
	if !strings.Contains(err.Error(), "https://") {
		t.Errorf("expected error about https, got: %v", err)
	}
}

// --- Webhook Deliveries ---

func TestWebhookDelivery_InsertAndQuery(t *testing.T) {
	_, queries, _ := newService(t)
	ctx := context.Background()

	// Generate a delivery ID
	var deliveryID pgtype.UUID
	if _, err := rand.Read(deliveryID.Bytes[:]); err != nil {
		t.Fatalf("generate UUID: %v", err)
	}
	// Set version 4 bits
	deliveryID.Bytes[6] = (deliveryID.Bytes[6] & 0x0f) | 0x40
	deliveryID.Bytes[8] = (deliveryID.Bytes[8] & 0x3f) | 0x80
	deliveryID.Valid = true

	payload := []byte(`{"event":"review_items_added","count":3}`)

	delivery, err := queries.InsertWebhookDelivery(ctx, db.InsertWebhookDeliveryParams{
		Event:      "review_items_added",
		Url:        "https://example.com/webhook",
		Payload:    payload,
		DeliveryID: deliveryID,
	})
	if err != nil {
		t.Fatalf("InsertWebhookDelivery: %v", err)
	}

	if delivery.Event != "review_items_added" {
		t.Errorf("expected event review_items_added, got %s", delivery.Event)
	}
	if delivery.Url != "https://example.com/webhook" {
		t.Errorf("expected URL https://example.com/webhook, got %s", delivery.Url)
	}
	if delivery.Status != "pending" {
		t.Errorf("expected status pending, got %s", delivery.Status)
	}
	if delivery.Attempts != 0 {
		t.Errorf("expected 0 attempts, got %d", delivery.Attempts)
	}

	// Verify it appears in ListRecentWebhookDeliveries
	recent, err := queries.ListRecentWebhookDeliveries(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecentWebhookDeliveries: %v", err)
	}
	if len(recent) != 1 {
		t.Fatalf("expected 1 recent delivery, got %d", len(recent))
	}
	if recent[0].Event != "review_items_added" {
		t.Errorf("expected event review_items_added, got %s", recent[0].Event)
	}

	// Verify it appears in GetPendingWebhookDeliveries
	pending, err := queries.GetPendingWebhookDeliveries(ctx)
	if err != nil {
		t.Fatalf("GetPendingWebhookDeliveries: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending delivery, got %d", len(pending))
	}
	if pending[0].DeliveryID != deliveryID {
		t.Errorf("expected delivery ID to match")
	}

	// After marking as delivered, it should not appear in pending
	err = queries.UpdateWebhookDeliveryAttempt(ctx, db.UpdateWebhookDeliveryAttemptParams{
		ID:             delivery.ID,
		Status:         "success",
		ResponseStatus: pgtype.Int4{Int32: 200, Valid: true},
	})
	if err != nil {
		t.Fatalf("UpdateWebhookDeliveryAttempt: %v", err)
	}

	pending, err = queries.GetPendingWebhookDeliveries(ctx)
	if err != nil {
		t.Fatalf("GetPendingWebhookDeliveries after update: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 pending after success, got %d", len(pending))
	}
}

// --- ListPendingReviews filters ---

func TestListPendingReviews_FilterByAccountID(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct1 := testutil.MustCreateAccount(t, queries, conn.ID, "ext_1", "Checking")
	acct2 := testutil.MustCreateAccount(t, queries, conn.ID, "ext_2", "Savings")

	txn1 := testutil.MustCreateTransaction(t, queries, acct1.ID, "txn_1", "Coffee", 500, "2025-01-15")
	txn2 := testutil.MustCreateTransaction(t, queries, acct2.ID, "txn_2", "Rent", 100000, "2025-01-15")

	for _, txn := range []db.Transaction{txn1, txn2} {
		if _, err := queries.EnqueueReview(ctx, db.EnqueueReviewParams{
			TransactionID: txn.ID,
			ReviewType:    "new_transaction",
		}); err != nil {
			t.Fatalf("EnqueueReview: %v", err)
		}
	}

	acctID := formatUUID(acct1.ID)
	result, err := svc.ListPendingReviews(ctx, service.PendingReviewParams{
		AccountID: &acctID,
	})
	if err != nil {
		t.Fatalf("ListPendingReviews: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item filtered by account, got %d", len(result.Items))
	}
	if result.Items[0].Transaction.Name != "Coffee" {
		t.Errorf("expected Coffee, got %s", result.Items[0].Transaction.Name)
	}
	// TotalPending should still reflect all pending (unfiltered)
	if result.TotalPending != 2 {
		t.Errorf("expected total pending 2, got %d", result.TotalPending)
	}
}

func TestListPendingReviews_FilterByUserID(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	alice := testutil.MustCreateUser(t, queries, "Alice")
	bob := testutil.MustCreateUser(t, queries, "Bob")
	conn1 := testutil.MustCreateConnection(t, queries, alice.ID, "item_1")
	conn2 := testutil.MustCreateConnection(t, queries, bob.ID, "item_2")
	acct1 := testutil.MustCreateAccount(t, queries, conn1.ID, "ext_1", "Alice Checking")
	acct2 := testutil.MustCreateAccount(t, queries, conn2.ID, "ext_2", "Bob Checking")

	txn1 := testutil.MustCreateTransaction(t, queries, acct1.ID, "txn_1", "Alice Txn", 500, "2025-01-15")
	txn2 := testutil.MustCreateTransaction(t, queries, acct2.ID, "txn_2", "Bob Txn", 700, "2025-01-15")

	for _, txn := range []db.Transaction{txn1, txn2} {
		if _, err := queries.EnqueueReview(ctx, db.EnqueueReviewParams{
			TransactionID: txn.ID,
			ReviewType:    "new_transaction",
		}); err != nil {
			t.Fatalf("EnqueueReview: %v", err)
		}
	}

	bobID := formatUUID(bob.ID)
	result, err := svc.ListPendingReviews(ctx, service.PendingReviewParams{
		UserID: &bobID,
	})
	if err != nil {
		t.Fatalf("ListPendingReviews: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item filtered by user, got %d", len(result.Items))
	}
	if result.Items[0].Transaction.Name != "Bob Txn" {
		t.Errorf("expected Bob Txn, got %s", result.Items[0].Transaction.Name)
	}
}

func TestListPendingReviews_FilterBySince(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_1", "Checking")

	txn1 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_1", "Old", 500, "2025-01-10")
	txn2 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_2", "New", 700, "2025-01-15")

	if _, err := queries.EnqueueReview(ctx, db.EnqueueReviewParams{
		TransactionID: txn1.ID,
		ReviewType:    "new_transaction",
	}); err != nil {
		t.Fatalf("EnqueueReview 1: %v", err)
	}

	// Sleep briefly so second enqueue has a later created_at
	time.Sleep(10 * time.Millisecond)
	midpoint := time.Now()
	time.Sleep(10 * time.Millisecond)

	if _, err := queries.EnqueueReview(ctx, db.EnqueueReviewParams{
		TransactionID: txn2.ID,
		ReviewType:    "new_transaction",
	}); err != nil {
		t.Fatalf("EnqueueReview 2: %v", err)
	}

	result, err := svc.ListPendingReviews(ctx, service.PendingReviewParams{
		Since: &midpoint,
	})
	if err != nil {
		t.Fatalf("ListPendingReviews: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item since midpoint, got %d", len(result.Items))
	}
	if result.Items[0].Transaction.Name != "New" {
		t.Errorf("expected New, got %s", result.Items[0].Transaction.Name)
	}
}

func TestListPendingReviews_CursorPagination(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_1", "Checking")

	// Create 3 transactions with reviews
	for i := 0; i < 3; i++ {
		txn := testutil.MustCreateTransaction(t, queries, acct.ID,
			fmt.Sprintf("txn_%d", i), fmt.Sprintf("Item %d", i), int64(100*(i+1)), "2025-01-15")
		if _, err := queries.EnqueueReview(ctx, db.EnqueueReviewParams{
			TransactionID: txn.ID,
			ReviewType:    "new_transaction",
		}); err != nil {
			t.Fatalf("EnqueueReview %d: %v", i, err)
		}
		time.Sleep(5 * time.Millisecond) // ensure distinct created_at
	}

	// Page 1: limit 2
	page1, err := svc.ListPendingReviews(ctx, service.PendingReviewParams{Limit: 2})
	if err != nil {
		t.Fatalf("ListPendingReviews page 1: %v", err)
	}
	if len(page1.Items) != 2 {
		t.Fatalf("expected 2 items on page 1, got %d", len(page1.Items))
	}
	if !page1.HasMore {
		t.Error("expected HasMore=true on page 1")
	}
	if page1.NextCursor == "" {
		t.Fatal("expected non-empty cursor on page 1")
	}

	// Page 2: use cursor
	page2, err := svc.ListPendingReviews(ctx, service.PendingReviewParams{
		Limit:  2,
		Cursor: page1.NextCursor,
	})
	if err != nil {
		t.Fatalf("ListPendingReviews page 2: %v", err)
	}
	if len(page2.Items) != 1 {
		t.Fatalf("expected 1 item on page 2, got %d", len(page2.Items))
	}
	if page2.HasMore {
		t.Error("expected HasMore=false on page 2")
	}

	// No overlap between pages
	if page1.Items[0].ReviewID == page2.Items[0].ReviewID ||
		page1.Items[1].ReviewID == page2.Items[0].ReviewID {
		t.Error("page 2 items overlap with page 1")
	}
}

// --- SubmitReviews edge cases ---

func TestSubmitReviews_InvalidDecision(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_1", "Checking")
	txn := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_1", "Test", 100, "2025-01-15")

	r, err := queries.EnqueueReview(ctx, db.EnqueueReviewParams{
		TransactionID: txn.ID,
		ReviewType:    "new_transaction",
	})
	if err != nil {
		t.Fatalf("EnqueueReview: %v", err)
	}

	actor := service.Actor{Type: "agent", ID: "test", Name: "Test"}
	result, err := svc.SubmitReviews(ctx, []service.ReviewDecision{
		{ReviewID: formatUUID(r.ID), Decision: "invalid_value"},
	}, actor)
	if err != nil {
		t.Fatalf("SubmitReviews: %v", err)
	}
	if result.Results[0].Status != "error" {
		t.Errorf("expected error status, got %s", result.Results[0].Status)
	}
	if !strings.Contains(result.Results[0].Error, "invalid decision") {
		t.Errorf("expected invalid decision error, got: %s", result.Results[0].Error)
	}
}

func TestSubmitReviews_AlreadyResolved(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_1", "Checking")
	txn := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_1", "Test", 100, "2025-01-15")

	r, err := queries.EnqueueReview(ctx, db.EnqueueReviewParams{
		TransactionID: txn.ID,
		ReviewType:    "new_transaction",
	})
	if err != nil {
		t.Fatalf("EnqueueReview: %v", err)
	}

	actor := service.Actor{Type: "agent", ID: "test", Name: "Test"}

	// First submit succeeds
	_, err = svc.SubmitReviews(ctx, []service.ReviewDecision{
		{ReviewID: formatUUID(r.ID), Decision: "approve"},
	}, actor)
	if err != nil {
		t.Fatalf("First SubmitReviews: %v", err)
	}

	// Second submit on same review should return error per-item
	result, err := svc.SubmitReviews(ctx, []service.ReviewDecision{
		{ReviewID: formatUUID(r.ID), Decision: "reject"},
	}, actor)
	if err != nil {
		t.Fatalf("Second SubmitReviews: %v", err)
	}
	if result.Results[0].Status != "error" {
		t.Errorf("expected error for already-resolved review, got %s", result.Results[0].Status)
	}
}

func TestSubmitReviews_CategoryOverrideAppliedToTransaction(t *testing.T) {
	svc, queries, pool := newService(t)
	ctx := context.Background()

	// Create a category
	cat, err := queries.InsertCategory(ctx, db.InsertCategoryParams{
		Slug:        "food_and_drink",
		DisplayName: "Food & Drink",
	})
	if err != nil {
		t.Fatalf("InsertCategory: %v", err)
	}

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_1", "Checking")
	txn := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_1", "Starbucks", 500, "2025-01-15")

	r, err := queries.EnqueueReview(ctx, db.EnqueueReviewParams{
		TransactionID: txn.ID,
		ReviewType:    "uncategorized",
	})
	if err != nil {
		t.Fatalf("EnqueueReview: %v", err)
	}

	actor := service.Actor{Type: "agent", ID: "test", Name: "Test"}
	slug := "food_and_drink"
	_, err = svc.SubmitReviews(ctx, []service.ReviewDecision{
		{ReviewID: formatUUID(r.ID), Decision: "reject", OverrideCategorySlug: &slug},
	}, actor)
	if err != nil {
		t.Fatalf("SubmitReviews: %v", err)
	}

	// Verify the transaction now has the category set AND category_override=true
	var txnCategoryID pgtype.UUID
	var txnCategoryOverride bool
	err = pool.QueryRow(ctx,
		"SELECT category_id, category_override FROM transactions WHERE id = $1", txn.ID,
	).Scan(&txnCategoryID, &txnCategoryOverride)
	if err != nil {
		t.Fatalf("query transaction: %v", err)
	}
	if txnCategoryID != cat.ID {
		t.Errorf("expected transaction category_id %v, got %v", cat.ID, txnCategoryID)
	}
	if !txnCategoryOverride {
		t.Error("expected category_override=true on transaction after review override")
	}
}

func TestSubmitReviews_EmptyArray(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	actor := service.Actor{Type: "agent", ID: "test", Name: "Test"}
	_, err := svc.SubmitReviews(ctx, []service.ReviewDecision{}, actor)
	if err == nil {
		t.Fatal("expected error for empty decisions array")
	}
	if !errors.Is(err, service.ErrInvalidParameter) {
		t.Errorf("expected ErrInvalidParameter, got: %v", err)
	}
}

func TestSubmitReviews_NonexistentCategorySlug(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_1", "Checking")
	txn := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_1", "Test", 100, "2025-01-15")

	r, err := queries.EnqueueReview(ctx, db.EnqueueReviewParams{
		TransactionID: txn.ID,
		ReviewType:    "uncategorized",
	})
	if err != nil {
		t.Fatalf("EnqueueReview: %v", err)
	}

	actor := service.Actor{Type: "agent", ID: "test", Name: "Test"}
	slug := "nonexistent_category"
	result, err := svc.SubmitReviews(ctx, []service.ReviewDecision{
		{ReviewID: formatUUID(r.ID), Decision: "reject", OverrideCategorySlug: &slug},
	}, actor)
	if err != nil {
		t.Fatalf("SubmitReviews: %v", err)
	}
	if result.Results[0].Status != "error" {
		t.Errorf("expected error for nonexistent slug, got %s", result.Results[0].Status)
	}
	if !strings.Contains(result.Results[0].Error, "not found") {
		t.Errorf("expected 'not found' error, got: %s", result.Results[0].Error)
	}
}

// --- SaveReviewInstructions edge cases ---

func TestSaveReviewInstructions_MaxLength(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// 20001 chars should be rejected
	longText := strings.Repeat("x", 20001)
	err := svc.SaveReviewInstructions(ctx, longText, "")
	if err == nil {
		t.Fatal("expected error for instructions exceeding max length")
	}
	if !errors.Is(err, service.ErrInvalidParameter) {
		t.Errorf("expected ErrInvalidParameter, got: %v", err)
	}
}

func TestGetReviewInstructions_DefaultWhenEmpty(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	expanded, err := svc.GetReviewInstructions(ctx)
	if err != nil {
		t.Fatalf("GetReviewInstructions: %v", err)
	}
	if expanded == "" {
		t.Error("expected non-empty default instructions")
	}
	if !strings.Contains(expanded, "Review each transaction") {
		t.Errorf("expected default instructions text, got: %s", expanded)
	}
}

func TestGetReviewInstructions_DateRangeExpansion(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_1", "Checking")

	txn1 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_1", "Early", 100, "2025-01-10")
	txn2 := testutil.MustCreateTransaction(t, queries, acct.ID, "txn_2", "Late", 200, "2025-01-20")

	for _, txn := range []db.Transaction{txn1, txn2} {
		if _, err := queries.EnqueueReview(ctx, db.EnqueueReviewParams{
			TransactionID: txn.ID,
			ReviewType:    "new_transaction",
		}); err != nil {
			t.Fatalf("EnqueueReview: %v", err)
		}
	}

	instructions := "Date range: {{date_range_start}} to {{date_range_end}}"
	if err := svc.SaveReviewInstructions(ctx, instructions, ""); err != nil {
		t.Fatalf("SaveReviewInstructions: %v", err)
	}

	expanded, err := svc.GetReviewInstructions(ctx)
	if err != nil {
		t.Fatalf("GetReviewInstructions: %v", err)
	}
	if strings.Contains(expanded, "{{date_range_start}}") {
		t.Error("expected {{date_range_start}} to be expanded")
	}
	if !strings.Contains(expanded, "2025-01-10") {
		t.Errorf("expected earliest date 2025-01-10 in: %s", expanded)
	}
	if !strings.Contains(expanded, "2025-01-20") {
		t.Errorf("expected latest date 2025-01-20 in: %s", expanded)
	}
}

// --- WebhookConfig edge cases ---

func TestSaveWebhookConfig_ClearURL(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// First set a URL
	_, err := svc.SaveWebhookConfig(ctx, service.WebhookConfig{
		URL:    "https://example.com/hook",
		Events: []string{"review_items_added"},
	})
	if err != nil {
		t.Fatalf("SaveWebhookConfig: %v", err)
	}

	// Clear URL
	result, err := svc.SaveWebhookConfig(ctx, service.WebhookConfig{
		URL:    "",
		Events: []string{"review_items_added"},
	})
	if err != nil {
		t.Fatalf("SaveWebhookConfig clear: %v", err)
	}
	if result.URL != "" {
		t.Errorf("expected empty URL after clear, got %s", result.URL)
	}

	// Verify via GetWebhookConfig
	cfg, err := svc.GetWebhookConfig(ctx)
	if err != nil {
		t.Fatalf("GetWebhookConfig: %v", err)
	}
	if cfg.URL != "" {
		t.Errorf("expected empty URL on re-read, got %s", cfg.URL)
	}
}

func TestSaveWebhookConfig_SecretMinLength(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	_, err := svc.SaveWebhookConfig(ctx, service.WebhookConfig{
		URL:    "https://example.com/hook",
		Secret: "too_short",
		Events: []string{"review_items_added"},
	})
	if err == nil {
		t.Fatal("expected error for short secret")
	}
	if !errors.Is(err, service.ErrInvalidParameter) {
		t.Errorf("expected ErrInvalidParameter, got: %v", err)
	}
}

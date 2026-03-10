//go:build integration

package service_test

import (
	"context"
	"crypto/rand"
	"strings"
	"testing"

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

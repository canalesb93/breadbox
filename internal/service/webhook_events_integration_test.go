//go:build integration

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/db"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestListWebhookEventsPaginated_Empty(t *testing.T) {
	svc, _, _ := newService(t)
	result, err := svc.ListWebhookEventsPaginated(context.Background(), service.WebhookEventListParams{
		Page:     1,
		PageSize: 25,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("expected 0 total, got %d", result.Total)
	}
	if len(result.Events) != 0 {
		t.Errorf("expected 0 events, got %d", len(result.Events))
	}
}

func TestListWebhookEventsPaginated_WithData(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_wh_1")

	// Create several webhook events.
	for i := 0; i < 3; i++ {
		_, err := queries.CreateWebhookEvent(ctx, db.CreateWebhookEventParams{
			Provider:       db.ProviderTypePlaid,
			EventType:      "sync_available",
			ConnectionID:   conn.ID,
			RawPayloadHash: "abc123",
			Status:         "processed",
		})
		if err != nil {
			t.Fatalf("create webhook event: %v", err)
		}
	}
	// Create an error event without connection.
	_, err := queries.CreateWebhookEvent(ctx, db.CreateWebhookEventParams{
		Provider:       db.ProviderTypeTeller,
		EventType:      "verification_failed",
		ConnectionID:   pgtype.UUID{},
		RawPayloadHash: "def456",
		Status:         "error",
		ErrorMessage:   pgtype.Text{String: "bad signature", Valid: true},
	})
	if err != nil {
		t.Fatalf("create error event: %v", err)
	}

	// List all.
	result, err := svc.ListWebhookEventsPaginated(ctx, service.WebhookEventListParams{
		Page:     1,
		PageSize: 25,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 4 {
		t.Errorf("expected 4 total, got %d", result.Total)
	}

	// Filter by provider.
	plaid := "plaid"
	result, err = svc.ListWebhookEventsPaginated(ctx, service.WebhookEventListParams{
		Page:     1,
		PageSize: 25,
		Provider: &plaid,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 3 {
		t.Errorf("expected 3 plaid events, got %d", result.Total)
	}

	// Filter by status.
	errorStatus := "error"
	result, err = svc.ListWebhookEventsPaginated(ctx, service.WebhookEventListParams{
		Page:     1,
		PageSize: 25,
		Status:   &errorStatus,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("expected 1 error event, got %d", result.Total)
	}
	if result.Events[0].ErrorMessage == nil || *result.Events[0].ErrorMessage != "bad signature" {
		t.Errorf("expected error message 'bad signature', got %v", result.Events[0].ErrorMessage)
	}
}

func TestWebhookEventCounts(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	// Empty state.
	stats, err := svc.WebhookEventCounts(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TotalEvents != 0 {
		t.Errorf("expected 0 total, got %d", stats.TotalEvents)
	}

	// Add events with different statuses.
	user := testutil.MustCreateUser(t, queries, "Bob")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_wh_2")

	for _, status := range []string{"processed", "processed", "received", "error"} {
		_, err := queries.CreateWebhookEvent(ctx, db.CreateWebhookEventParams{
			Provider:       db.ProviderTypePlaid,
			EventType:      "sync_available",
			ConnectionID:   conn.ID,
			RawPayloadHash: "hash",
			Status:         status,
		})
		if err != nil {
			t.Fatalf("create event: %v", err)
		}
	}

	stats, err = svc.WebhookEventCounts(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TotalEvents != 4 {
		t.Errorf("expected 4 total, got %d", stats.TotalEvents)
	}
	if stats.ProcessedCount != 2 {
		t.Errorf("expected 2 processed, got %d", stats.ProcessedCount)
	}
	if stats.ReceivedCount != 1 {
		t.Errorf("expected 1 received, got %d", stats.ReceivedCount)
	}
	if stats.ErrorCount != 1 {
		t.Errorf("expected 1 error, got %d", stats.ErrorCount)
	}
}

func TestListWebhookEventsPaginated_Pagination(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Charlie")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_wh_3")

	// Create 7 events.
	for i := 0; i < 7; i++ {
		_, err := queries.CreateWebhookEvent(ctx, db.CreateWebhookEventParams{
			Provider:       db.ProviderTypePlaid,
			EventType:      "sync_available",
			ConnectionID:   conn.ID,
			RawPayloadHash: "hash",
			Status:         "processed",
		})
		if err != nil {
			t.Fatalf("create event: %v", err)
		}
	}

	// Page 1 of 3 (page size 3).
	result, err := svc.ListWebhookEventsPaginated(ctx, service.WebhookEventListParams{
		Page:     1,
		PageSize: 3,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 7 {
		t.Errorf("expected 7 total, got %d", result.Total)
	}
	if result.TotalPages != 3 {
		t.Errorf("expected 3 total pages, got %d", result.TotalPages)
	}
	if len(result.Events) != 3 {
		t.Errorf("expected 3 events on page 1, got %d", len(result.Events))
	}

	// Page 3 should have 1 event.
	result, err = svc.ListWebhookEventsPaginated(ctx, service.WebhookEventListParams{
		Page:     3,
		PageSize: 3,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Events) != 1 {
		t.Errorf("expected 1 event on page 3, got %d", len(result.Events))
	}
}

func TestListWebhookEventsPaginated_JoinsInstitutionName(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, queries, "Diana")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_wh_4")

	_, err := queries.CreateWebhookEvent(ctx, db.CreateWebhookEventParams{
		Provider:       db.ProviderTypePlaid,
		EventType:      "connection_error",
		ConnectionID:   conn.ID,
		RawPayloadHash: "hash",
		Status:         "processed",
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	result, err := svc.ListWebhookEventsPaginated(ctx, service.WebhookEventListParams{
		Page:     1,
		PageSize: 25,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(result.Events))
	}
	evt := result.Events[0]
	if evt.ConnectionID == nil {
		t.Error("expected connection ID to be set")
	}
	// MustCreateConnection does not set institution_name, so it will be NULL.
	// This test verifies that the LEFT JOIN works without error and the field is nil.
	if evt.InstitutionName != nil {
		t.Errorf("expected institution name to be nil (not set in fixture), got %q", *evt.InstitutionName)
	}
	if evt.EventType != "connection_error" {
		t.Errorf("expected event_type 'connection_error', got %q", evt.EventType)
	}
	if evt.Provider != "plaid" {
		t.Errorf("expected provider 'plaid', got %q", evt.Provider)
	}
}

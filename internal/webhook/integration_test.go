//go:build integration

package webhook_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/testutil"
	"breadbox/internal/webhook"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestMain(m *testing.M) {
	testutil.RunWithDB(m)
}

func TestWebhookDispatcher_EnqueueAndDeliver(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	// Set up a test HTTP server that records requests
	var receivedBody []byte
	var receivedHeaders http.Header
	var callCount atomic.Int32

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		receivedHeaders = r.Header.Clone()
		body, _ := io.ReadAll(r.Body)
		receivedBody = body
		w.WriteHeader(200)
		w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()

	// Configure webhook secret in app_config
	secret := "test_secret_that_is_at_least_32_characters_long"
	queries.SetAppConfig(ctx, db.SetAppConfigParams{
		Key:   "review_webhook_secret",
		Value: pgtype.Text{String: secret, Valid: true},
	})

	d := webhook.NewDispatcher(queries, pool, slog.Default(), "test")
	// Override the HTTP client to use the test server's TLS client
	d.SetHTTPClient(server.Client())

	payload := map[string]any{
		"event": "review_items_added",
		"data":  map[string]any{"count": 5},
	}

	err := d.Enqueue(ctx, webhook.EventReviewItemsAdded, payload, server.URL)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Wait for async delivery
	deadline := time.After(5 * time.Second)
	for {
		if callCount.Load() > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for webhook delivery")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	// Verify the payload was delivered
	var got map[string]any
	if err := json.Unmarshal(receivedBody, &got); err != nil {
		t.Fatalf("unmarshal received body: %v", err)
	}
	if got["event"] != "review_items_added" {
		t.Errorf("expected event review_items_added, got %v", got["event"])
	}

	// Verify required headers
	if receivedHeaders.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", receivedHeaders.Get("Content-Type"))
	}
	if receivedHeaders.Get("X-Breadbox-Event") != "review_items_added" {
		t.Errorf("expected X-Breadbox-Event header, got %s", receivedHeaders.Get("X-Breadbox-Event"))
	}
	if receivedHeaders.Get("X-Breadbox-Delivery-ID") == "" {
		t.Error("expected X-Breadbox-Delivery-ID header")
	}
	if !strings.HasPrefix(receivedHeaders.Get("User-Agent"), "Breadbox/") {
		t.Errorf("expected User-Agent to start with Breadbox/, got %s", receivedHeaders.Get("User-Agent"))
	}

	// Verify HMAC signature
	sigHeader := receivedHeaders.Get("X-Breadbox-Signature")
	if sigHeader == "" {
		t.Fatal("expected X-Breadbox-Signature header")
	}
	verifyHMACSignature(t, sigHeader, receivedBody, secret)

	// Verify DB delivery record updated to success
	time.Sleep(100 * time.Millisecond) // allow DB update to complete
	deliveries, err := queries.ListRecentWebhookDeliveries(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecentWebhookDeliveries: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery record, got %d", len(deliveries))
	}
	if deliveries[0].Status != webhook.DeliveryStatusSuccess {
		t.Errorf("expected delivery status success, got %s", deliveries[0].Status)
	}
	if !deliveries[0].ResponseStatus.Valid || deliveries[0].ResponseStatus.Int32 != 200 {
		t.Errorf("expected response status 200, got %v", deliveries[0].ResponseStatus)
	}
}

func TestWebhookDispatcher_SkipsWhenNoURL(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	d := webhook.NewDispatcher(queries, pool, slog.Default(), "test")

	err := d.Enqueue(ctx, webhook.EventReviewItemsAdded, map[string]any{}, "")
	if err != nil {
		t.Fatalf("Enqueue with empty URL should not error: %v", err)
	}

	// No delivery records should exist
	deliveries, err := queries.ListRecentWebhookDeliveries(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecentWebhookDeliveries: %v", err)
	}
	if len(deliveries) != 0 {
		t.Errorf("expected 0 deliveries for empty URL, got %d", len(deliveries))
	}
}

func TestWebhookDispatcher_Non2xxSchedulesRetry(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	var callCount atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(500)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	queries.SetAppConfig(ctx, db.SetAppConfigParams{
		Key:   "review_webhook_secret",
		Value: pgtype.Text{String: strings.Repeat("a", 32), Valid: true},
	})

	d := webhook.NewDispatcher(queries, pool, slog.Default(), "test")
	d.SetHTTPClient(server.Client())

	err := d.Enqueue(ctx, webhook.EventReviewItemsAdded, map[string]any{"test": true}, server.URL)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Wait for first attempt
	deadline := time.After(5 * time.Second)
	for {
		if callCount.Load() > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for first attempt")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	time.Sleep(100 * time.Millisecond) // allow DB update

	// Delivery should still be pending (scheduled for retry)
	deliveries, err := queries.ListRecentWebhookDeliveries(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecentWebhookDeliveries: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}
	if deliveries[0].Status != webhook.DeliveryStatusPending {
		t.Errorf("expected status pending (retry scheduled), got %s", deliveries[0].Status)
	}
	if deliveries[0].Attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", deliveries[0].Attempts)
	}
	// next_retry_at should be in the future
	if !deliveries[0].NextRetryAt.Valid || deliveries[0].NextRetryAt.Time.Before(time.Now()) {
		t.Error("expected next_retry_at in the future after failed attempt")
	}
}

func TestWebhookDispatcher_Cleanup(t *testing.T) {
	pool, queries := testutil.ServicePool(t)
	ctx := context.Background()

	d := webhook.NewDispatcher(queries, pool, slog.Default(), "test")

	// Insert a delivery record with old created_at (> 7 days)
	_, err := pool.Exec(ctx, `
		INSERT INTO webhook_deliveries (event, url, payload, delivery_id, status, created_at)
		VALUES ('review_items_added', 'https://old.test/hook', '{}', gen_random_uuid(), 'success', NOW() - INTERVAL '8 days')
	`)
	if err != nil {
		t.Fatalf("insert old delivery: %v", err)
	}

	// Insert a recent delivery
	_, err = pool.Exec(ctx, `
		INSERT INTO webhook_deliveries (event, url, payload, delivery_id, status)
		VALUES ('review_items_added', 'https://new.test/hook', '{}', gen_random_uuid(), 'success')
	`)
	if err != nil {
		t.Fatalf("insert recent delivery: %v", err)
	}

	// Verify both exist
	all, err := queries.ListRecentWebhookDeliveries(ctx, 100)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 deliveries before cleanup, got %d", len(all))
	}

	// Run cleanup
	if err := d.Cleanup(ctx); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	// Only the recent one should remain
	remaining, err := queries.ListRecentWebhookDeliveries(ctx, 100)
	if err != nil {
		t.Fatalf("list after cleanup: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected 1 delivery after cleanup, got %d", len(remaining))
	}
	if remaining[0].Url != "https://new.test/hook" {
		t.Errorf("expected new delivery to survive cleanup, got %s", remaining[0].Url)
	}
}

func TestWebhookDispatcher_SendTestWebhook(t *testing.T) {
	pool, queries := testutil.ServicePool(t)

	var receivedEvent string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedEvent = r.Header.Get("X-Breadbox-Event")
		w.WriteHeader(200)
	}))
	defer server.Close()

	d := webhook.NewDispatcher(queries, pool, slog.Default(), "test")
	d.SetHTTPClient(server.Client())

	result, err := d.SendTestWebhook(context.Background(), server.URL, strings.Repeat("s", 32))
	if err != nil {
		t.Fatalf("SendTestWebhook: %v", err)
	}
	if !result.Success {
		t.Error("expected test webhook to succeed")
	}
	if result.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", result.StatusCode)
	}
	if result.ResponseTimeMs <= 0 {
		t.Errorf("expected positive response time, got %d", result.ResponseTimeMs)
	}
	if receivedEvent != "test" {
		t.Errorf("expected X-Breadbox-Event: test, got %s", receivedEvent)
	}
}

// verifyHMACSignature verifies the t=timestamp,v1=signature header format.
func verifyHMACSignature(t *testing.T, sigHeader string, body []byte, secret string) {
	t.Helper()

	// Parse "t=1234567890,v1=hex_signature"
	parts := strings.Split(sigHeader, ",")
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts in signature header, got %d: %s", len(parts), sigHeader)
	}

	var timestamp, signature string
	for _, part := range parts {
		if strings.HasPrefix(part, "t=") {
			timestamp = strings.TrimPrefix(part, "t=")
		} else if strings.HasPrefix(part, "v1=") {
			signature = strings.TrimPrefix(part, "v1=")
		}
	}

	if timestamp == "" || signature == "" {
		t.Fatalf("missing timestamp or signature in: %s", sigHeader)
	}

	// Recompute
	signedPayload := fmt.Sprintf("%s.%s", timestamp, string(body))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signedPayload))
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expected)) {
		t.Errorf("HMAC signature mismatch:\n  got:      %s\n  expected: %s", signature, expected)
	}
}

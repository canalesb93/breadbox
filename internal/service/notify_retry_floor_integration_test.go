//go:build integration && !lite

package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"breadbox/internal/service"
)

// TestNotifyRetriesTransientThenSucceeds verifies the send is retried on a
// 5xx and succeeds once the sink recovers.
func TestNotifyRetriesTransientThenSucceeds(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable) // 503 → retriable
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if _, err := svc.UpdateNotificationSettings(ctx, service.UpdateNotificationSettingsParams{WebhookURL: &srv.URL}); err != nil {
		t.Fatalf("set webhook: %v", err)
	}
	if err := svc.SendWorkflowNotification(ctx, service.NotificationPayload{Event: "test", Title: "retry", Body: "x", Priority: "info"}); err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if n := attempts.Load(); n != 3 {
		t.Errorf("attempts = %d, want 3 (two 503s then OK)", n)
	}
}

// TestNotifyFailsFastOn4xx verifies a permanent 4xx is not retried.
func TestNotifyFailsFastOn4xx(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest) // 400 → permanent
	}))
	defer srv.Close()

	if _, err := svc.UpdateNotificationSettings(ctx, service.UpdateNotificationSettingsParams{WebhookURL: &srv.URL}); err != nil {
		t.Fatalf("set webhook: %v", err)
	}
	if err := svc.SendWorkflowNotification(ctx, service.NotificationPayload{Event: "test", Title: "nope", Body: "x", Priority: "info"}); err == nil {
		t.Fatal("expected error on 400")
	}
	if n := attempts.Load(); n != 1 {
		t.Errorf("attempts = %d, want 1 (4xx must not retry)", n)
	}
}

// TestNotifyMinPriorityFloor verifies reports below the floor are dropped,
// reports at/above it are delivered, and a test notification always goes.
func TestNotifyMinPriorityFloor(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	floor := "warning"
	if _, err := svc.UpdateNotificationSettings(ctx, service.UpdateNotificationSettingsParams{
		WebhookURL:  &srv.URL,
		MinPriority: &floor,
	}); err != nil {
		t.Fatalf("set settings: %v", err)
	}

	// Below floor → dropped, no HTTP call.
	if err := svc.SendWorkflowNotification(ctx, service.NotificationPayload{Event: "report", Title: "info report", Body: "x", Priority: "info"}); err != nil {
		t.Fatalf("info send returned error: %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("info report was delivered despite warning floor (hits=%d)", hits.Load())
	}

	// At floor → delivered.
	if err := svc.SendWorkflowNotification(ctx, service.NotificationPayload{Event: "report", Title: "warn report", Body: "x", Priority: "warning"}); err != nil {
		t.Fatalf("warning send returned error: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("warning report not delivered (hits=%d)", hits.Load())
	}

	// Test notifications bypass the floor.
	if err := svc.SendWorkflowNotification(ctx, service.NotificationPayload{Event: "test", Title: "test", Body: "x", Priority: "info"}); err != nil {
		t.Fatalf("test send returned error: %v", err)
	}
	if hits.Load() != 2 {
		t.Fatalf("test notification did not bypass floor (hits=%d)", hits.Load())
	}
}

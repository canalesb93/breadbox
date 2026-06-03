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

// TestResolveNotifyBaseURL covers the derived deep-link origin: an
// auto-detected origin is used when no manual override is set, the override
// wins when present, and clearing the override falls back to detected.
func TestResolveNotifyBaseURL(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	if got := svc.ResolveNotifyBaseURL(ctx); got != "" {
		t.Fatalf("fresh state base URL = %q, want empty", got)
	}

	// Detected origin (with a trailing slash) is captured and normalized.
	if err := svc.SetDetectedNotifyBaseURL(ctx, "https://bb.detected.example/"); err != nil {
		t.Fatalf("set detected: %v", err)
	}
	if got := svc.ResolveNotifyBaseURL(ctx); got != "https://bb.detected.example" {
		t.Fatalf("after detect, base URL = %q, want https://bb.detected.example", got)
	}

	// An invalid origin is ignored (best-effort), leaving the prior value.
	if err := svc.SetDetectedNotifyBaseURL(ctx, "not a url"); err != nil {
		t.Fatalf("set detected (invalid): %v", err)
	}
	if got := svc.ResolveNotifyBaseURL(ctx); got != "https://bb.detected.example" {
		t.Fatalf("invalid detect should be a no-op, base URL = %q", got)
	}

	// A loopback origin (dev/tunnel visit) must NOT clobber the real detected
	// origin — it can never be a valid deep-link target for an external client.
	for _, loop := range []string{"http://localhost:8080", "http://127.0.0.1:9000", "http://[::1]:8080"} {
		if err := svc.SetDetectedNotifyBaseURL(ctx, loop); err != nil {
			t.Fatalf("set detected (loopback %s): %v", loop, err)
		}
		if got := svc.ResolveNotifyBaseURL(ctx); got != "https://bb.detected.example" {
			t.Fatalf("loopback %s should be a no-op, base URL = %q", loop, got)
		}
	}

	// Manual override wins over detected.
	override := "https://override.example.com"
	if _, err := svc.UpdateNotificationSettings(ctx, service.UpdateNotificationSettingsParams{PublicBaseURL: &override}); err != nil {
		t.Fatalf("set override: %v", err)
	}
	if got := svc.ResolveNotifyBaseURL(ctx); got != override {
		t.Fatalf("override should win, base URL = %q, want %q", got, override)
	}

	// Clearing the override falls back to the detected origin.
	empty := ""
	if _, err := svc.UpdateNotificationSettings(ctx, service.UpdateNotificationSettingsParams{PublicBaseURL: &empty}); err != nil {
		t.Fatalf("clear override: %v", err)
	}
	if got := svc.ResolveNotifyBaseURL(ctx); got != "https://bb.detected.example" {
		t.Fatalf("after clear, base URL = %q, want detected fallback", got)
	}
}

// TestDetectedBaseURLAbsolutizesDeepLink proves the derived origin is applied
// end-to-end: a relative report URL is absolutized against the detected origin
// when a notification fires (no manual override configured).
func TestDetectedBaseURLAbsolutizesDeepLink(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	var gotClick atomic.Value
	gotClick.Store("")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClick.Store(r.Header.Get("X-Click"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := svc.SetDetectedNotifyBaseURL(ctx, "https://bb.detected.example"); err != nil {
		t.Fatalf("set detected: %v", err)
	}
	if _, err := svc.AddNotificationChannel(ctx, service.AddNotificationChannelParams{Name: "push", URL: srv.URL, Format: "ntfy"}); err != nil {
		t.Fatalf("add channel: %v", err)
	}

	if err := svc.SendWorkflowNotification(ctx, service.NotificationPayload{
		Event:    "report",
		Title:    "deep link",
		Body:     "body",
		Priority: "info",
		URL:      "/reports/abc123",
	}); err != nil {
		t.Fatalf("send: %v", err)
	}

	if got := gotClick.Load().(string); got != "https://bb.detected.example/reports/abc123" {
		t.Fatalf("X-Click = %q, want absolutized against detected origin", got)
	}
}

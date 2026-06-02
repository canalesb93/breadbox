//go:build integration && !lite

package service_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"breadbox/internal/service"
)

// TestT16NotifyNoOpWhenUnconfigured verifies that SendWorkflowNotification
// returns nil without contacting any server when KeyNotifyWebhookURL is unset.
func TestT16NotifyNoOpWhenUnconfigured(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// No webhook URL has been written to app_config — expect a silent no-op.
	var called atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Deliberately do NOT set the URL in app_config.
	err := svc.SendWorkflowNotification(ctx, service.NotificationPayload{
		Event: "test",
		Title: "T16 no-op check",
	})
	if err != nil {
		t.Fatalf("T16: unconfigured SendWorkflowNotification should return nil, got %v", err)
	}
	if called.Load() {
		t.Fatal("T16: httptest server was called, but no URL was configured")
	}
}

// TestT16NotifyPostsJSONWhenConfigured verifies that SendWorkflowNotification
// makes a POST with the correct Content-Type and a well-formed JSON body
// when KeyNotifyWebhookURL is set.
func TestT16NotifyPostsJSONWhenConfigured(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	type captured struct {
		method      string
		contentType string
		userAgent   string
		body        []byte
	}
	var got captured

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.contentType = r.Header.Get("Content-Type")
		got.userAgent = r.Header.Get("User-Agent")
		got.body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Configure the webhook URL via UpdateNotificationSettings (the canonical write path).
	if _, err := svc.UpdateNotificationSettings(ctx, service.UpdateNotificationSettingsParams{
		WebhookURL: &srv.URL,
	}); err != nil {
		t.Fatalf("T16: set webhook URL: %v", err)
	}

	payload := service.NotificationPayload{
		Event:    "report",
		Title:    "T16 test charge",
		Body:     "A suspicious transaction was flagged.",
		Priority: "warning",
		Workflow: "routine-reviewer",
	}
	if err := svc.SendWorkflowNotification(ctx, payload); err != nil {
		t.Fatalf("T16: SendWorkflowNotification: %v", err)
	}

	// Verify HTTP method.
	if got.method != http.MethodPost {
		t.Errorf("T16: method = %q, want POST", got.method)
	}

	// Verify Content-Type header.
	if got.contentType != "application/json" {
		t.Errorf("T16: Content-Type = %q, want application/json", got.contentType)
	}

	// Verify User-Agent header.
	if !strings.Contains(got.userAgent, "breadbox") {
		t.Errorf("T16: User-Agent = %q, want it to contain 'breadbox'", got.userAgent)
	}

	// Verify body is valid JSON and contains the expected fields.
	var p map[string]any
	if err := json.Unmarshal(got.body, &p); err != nil {
		t.Fatalf("T16: response body is not valid JSON: %v (raw: %s)", err, got.body)
	}
	checks := map[string]any{
		"event":    "report",
		"title":    "T16 test charge",
		"priority": "warning",
		"workflow": "routine-reviewer",
	}
	for field, want := range checks {
		if p[field] != want {
			t.Errorf("T16: JSON field %q = %v, want %v", field, p[field], want)
		}
	}

	// sent_at must be a non-empty RFC3339-ish string.
	sentAt, ok := p["sent_at"].(string)
	if !ok || sentAt == "" {
		t.Errorf("T16: sent_at missing or not a string, got %v", p["sent_at"])
	}
}

// TestT16NotifyErrorOnNon2xx verifies that SendWorkflowNotification returns a
// non-nil error when the remote webhook responds with a non-2xx status code.
func TestT16NotifyErrorOnNon2xx(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	cases := []struct {
		name   string
		status int
	}{
		{"bad_request_400", http.StatusBadRequest},
		{"not_found_404", http.StatusNotFound},
		{"server_error_500", http.StatusInternalServerError},
		{"service_unavailable_503", http.StatusServiceUnavailable},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			// (Re-)configure the webhook URL for each sub-test.
			if _, err := svc.UpdateNotificationSettings(ctx, service.UpdateNotificationSettingsParams{
				WebhookURL: &srv.URL,
			}); err != nil {
				t.Fatalf("T16: set webhook URL: %v", err)
			}

			err := svc.SendWorkflowNotification(ctx, service.NotificationPayload{
				Event: "test",
				Title: "T16 error probe",
			})
			if err == nil {
				t.Fatalf("T16: expected error for HTTP %d, got nil", tc.status)
			}
			// The error message should mention the status code.
			if !strings.Contains(err.Error(), "HTTP") {
				t.Errorf("T16: error %v does not mention HTTP status", err)
			}
		})
	}
}

//go:build integration && !lite

package service_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"breadbox/internal/service"
)

// TestNotifyNtfyDispatch verifies the end-to-end ntfy publishing path:
// when the format is "ntfy" and a public base URL is set, the outbound
// request carries ntfy's metadata headers, an absolute X-Click, and a
// markdown body whose relative links have been absolutized.
func TestNotifyNtfyDispatch(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	type captured struct {
		method      string
		contentType string
		title       string
		priority    string
		markdown    string
		tags        string
		click       string
		body        []byte
	}
	var got captured

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.contentType = r.Header.Get("Content-Type")
		got.title = r.Header.Get("X-Title")
		got.priority = r.Header.Get("X-Priority")
		got.markdown = r.Header.Get("X-Markdown")
		got.tags = r.Header.Get("X-Tags")
		got.click = r.Header.Get("X-Click")
		got.body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ntfy := "ntfy"
	base := "https://bb.example.com"
	if _, err := svc.UpdateNotificationSettings(ctx, service.UpdateNotificationSettingsParams{
		WebhookURL:    &srv.URL,
		Format:        &ntfy,
		PublicBaseURL: &base,
	}); err != nil {
		t.Fatalf("set notification settings: %v", err)
	}

	err := svc.SendWorkflowNotification(ctx, service.NotificationPayload{
		Event:    "report",
		Title:    "Large charge detected",
		Body:     "A **$420** charge. See [tx](/transactions/abc).",
		Priority: "critical",
		URL:      "/reports/xyz",
	})
	if err != nil {
		t.Fatalf("SendWorkflowNotification: %v", err)
	}

	if got.method != http.MethodPost {
		t.Errorf("method = %q, want POST", got.method)
	}
	if got.title != "Large charge detected" {
		t.Errorf("X-Title = %q", got.title)
	}
	if got.priority != "5" {
		t.Errorf("X-Priority = %q, want 5 (critical)", got.priority)
	}
	if got.markdown != "yes" {
		t.Errorf("X-Markdown = %q, want yes", got.markdown)
	}
	if got.tags != "rotating_light" {
		t.Errorf("X-Tags = %q, want rotating_light", got.tags)
	}
	if got.click != "https://bb.example.com/reports/xyz" {
		t.Errorf("X-Click = %q, want absolute report URL", got.click)
	}
	wantBody := "A **$420** charge. See [tx](https://bb.example.com/transactions/abc)."
	if string(got.body) != wantBody {
		t.Errorf("body =\n  %q\nwant\n  %q", string(got.body), wantBody)
	}
}

//go:build integration && !lite

package service_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"breadbox/internal/service"
)

func TestSendWorkflowNotification(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// Unconfigured → no-op, no error.
	if err := svc.SendWorkflowNotification(ctx, service.NotificationPayload{Title: "x"}); err != nil {
		t.Fatalf("unconfigured send should be a no-op, got %v", err)
	}

	var gotBody []byte
	var gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if _, err := svc.UpdateNotificationSettings(ctx, service.UpdateNotificationSettingsParams{WebhookURL: &srv.URL}); err != nil {
		t.Fatalf("set webhook: %v", err)
	}
	if !svc.WorkflowNotificationConfigured(ctx) {
		t.Fatal("expected configured after setting URL")
	}
	if err := svc.SendWorkflowNotification(ctx, service.NotificationPayload{Event: "report", Title: "Flagged charge", Priority: "warning"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotCT != "application/json" {
		t.Errorf("content-type = %q, want application/json", gotCT)
	}
	var p map[string]any
	if err := json.Unmarshal(gotBody, &p); err != nil {
		t.Fatalf("payload not JSON: %v (%s)", err, gotBody)
	}
	if p["title"] != "Flagged charge" || p["event"] != "report" {
		t.Errorf("payload = %v, want title/event set", p)
	}
}

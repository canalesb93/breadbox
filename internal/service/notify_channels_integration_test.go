//go:build integration && !lite

package service_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"breadbox/internal/service"
)

// TestNotifyMultiChannelFanout configures two channels in different formats
// and verifies a single report reaches both, each in its own wire shape, with
// per-channel delivery status recorded.
func TestNotifyMultiChannelFanout(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	var ntfyHits, jsonHits atomic.Int32
	var sawNtfyTitle, sawJSONBody atomic.Bool

	ntfySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ntfyHits.Add(1)
		if r.Header.Get("X-Title") != "" {
			sawNtfyTitle.Store(true)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ntfySrv.Close()
	jsonSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jsonHits.Add(1)
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), `"event"`) {
			sawJSONBody.Store(true)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer jsonSrv.Close()

	ch1, err := svc.AddNotificationChannel(ctx, service.AddNotificationChannelParams{Name: "push", URL: ntfySrv.URL, Format: "ntfy"})
	if err != nil {
		t.Fatalf("add ntfy channel: %v", err)
	}
	if _, err := svc.AddNotificationChannel(ctx, service.AddNotificationChannelParams{Name: "bridge", URL: jsonSrv.URL, Format: "json"}); err != nil {
		t.Fatalf("add json channel: %v", err)
	}

	if err := svc.SendWorkflowNotification(ctx, service.NotificationPayload{Event: "report", Title: "fanout", Body: "x", Priority: "info"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if ntfyHits.Load() != 1 || jsonHits.Load() != 1 {
		t.Fatalf("fan-out hits: ntfy=%d json=%d, want 1/1", ntfyHits.Load(), jsonHits.Load())
	}
	if !sawNtfyTitle.Load() {
		t.Error("ntfy channel did not receive X-Title header")
	}
	if !sawJSONBody.Load() {
		t.Error("json channel did not receive JSON envelope")
	}

	// Per-channel status recorded + persisted.
	chans, _ := svc.GetNotificationChannels(ctx)
	if len(chans) != 2 {
		t.Fatalf("channels = %d, want 2", len(chans))
	}
	for _, c := range chans {
		if c.LastStatus == nil || !c.LastStatus.OK {
			t.Errorf("channel %q missing OK status: %+v", c.Name, c.LastStatus)
		}
	}

	// Disable the json channel → only ntfy is hit next time.
	if err := svc.SetNotificationChannelEnabled(ctx, ch1.ID, true); err != nil { // no-op enable, sanity
		t.Fatalf("enable: %v", err)
	}
	var jsonID string
	for _, c := range chans {
		if c.Name == "bridge" {
			jsonID = c.ID
		}
	}
	if err := svc.SetNotificationChannelEnabled(ctx, jsonID, false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if err := svc.SendWorkflowNotification(ctx, service.NotificationPayload{Event: "report", Title: "again", Body: "x", Priority: "info"}); err != nil {
		t.Fatalf("send 2: %v", err)
	}
	if ntfyHits.Load() != 2 || jsonHits.Load() != 1 {
		t.Fatalf("after disable: ntfy=%d json=%d, want 2/1", ntfyHits.Load(), jsonHits.Load())
	}

	// Delete the disabled channel.
	if err := svc.DeleteNotificationChannel(ctx, jsonID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	chans, _ = svc.GetNotificationChannels(ctx)
	if len(chans) != 1 {
		t.Fatalf("after delete channels = %d, want 1", len(chans))
	}
}

// TestNotifyChannelBackCompat verifies a legacy single-webhook config is
// surfaced as one synthesized channel.
func TestNotifyChannelBackCompat(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	url := "https://ntfy.sh/legacy-topic"
	if _, err := svc.UpdateNotificationSettings(ctx, service.UpdateNotificationSettingsParams{WebhookURL: &url}); err != nil {
		t.Fatalf("set legacy webhook: %v", err)
	}
	chans, _ := svc.GetNotificationChannels(ctx)
	if len(chans) != 1 {
		t.Fatalf("channels = %d, want 1 synthesized", len(chans))
	}
	if chans[0].URL != url || !chans[0].Enabled {
		t.Errorf("synth channel = %+v", chans[0])
	}
	if !svc.WorkflowNotificationConfigured(ctx) {
		t.Error("expected configured via legacy synth channel")
	}
}

// TestNotifyPerChannelFloor verifies each channel's own priority floor gates
// fan-out independently.
func TestNotifyPerChannelFloor(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if _, err := svc.AddNotificationChannel(ctx, service.AddNotificationChannelParams{Name: "crit", URL: srv.URL, Format: "json", MinPriority: "critical"}); err != nil {
		t.Fatalf("add channel: %v", err)
	}
	// Warning report is below the critical floor → skipped.
	if err := svc.SendWorkflowNotification(ctx, service.NotificationPayload{Event: "report", Title: "warn", Body: "x", Priority: "warning"}); err != nil {
		t.Fatalf("send warning: %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("warning delivered despite critical floor (hits=%d)", hits.Load())
	}
	// Critical report clears the floor.
	if err := svc.SendWorkflowNotification(ctx, service.NotificationPayload{Event: "report", Title: "crit", Body: "x", Priority: "critical"}); err != nil {
		t.Fatalf("send critical: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("critical not delivered (hits=%d)", hits.Load())
	}
}

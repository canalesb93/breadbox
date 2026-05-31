//go:build integration && !lite

package service_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"breadbox/internal/service"
)

// f5notifyRecorder captures the JSON bodies POSTed to a stub notification
// webhook. CreateAgentReport fires the notification asynchronously, so the
// test side blocks on `received` to observe the POST deterministically and
// inspects `count` to assert the no-fire cases.
type f5notifyRecorder struct {
	srv      *httptest.Server
	received chan map[string]any
	count    int32
}

func f5newNotifyRecorder(t *testing.T) *f5notifyRecorder {
	t.Helper()
	rec := &f5notifyRecorder{received: make(chan map[string]any, 4)}
	rec.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&rec.count, 1)
		body, _ := io.ReadAll(r.Body)
		var p map[string]any
		_ = json.Unmarshal(body, &p)
		// Stash the Content-Type so the assertion can confirm the JSON shape.
		p["_content_type"] = r.Header.Get("Content-Type")
		select {
		case rec.received <- p:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(rec.srv.Close)
	return rec
}

// await blocks until a webhook POST lands or the timeout elapses.
func (rec *f5notifyRecorder) await(t *testing.T, timeout time.Duration) map[string]any {
	t.Helper()
	select {
	case p := <-rec.received:
		return p
	case <-time.After(timeout):
		t.Fatal("timed out waiting for notification webhook POST")
		return nil
	}
}

// expectNoFire settles briefly and asserts no webhook POST was made.
func (rec *f5notifyRecorder) expectNoFire(t *testing.T, settle time.Duration) {
	t.Helper()
	select {
	case p := <-rec.received:
		t.Fatalf("expected no notification, but webhook fired with %v", p)
	case <-time.After(settle):
		if c := atomic.LoadInt32(&rec.count); c != 0 {
			t.Fatalf("expected no webhook POST, got %d", c)
		}
	}
}

// TestF5NotifyOnReport_WorkflowReportFires proves an agent-authored report
// POSTs a notification with the enriched payload shape.
func TestF5NotifyOnReport_WorkflowReportFires(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	rec := f5newNotifyRecorder(t)
	if _, err := svc.UpdateAgentSettings(ctx, service.UpdateAgentSettingsParams{NotifyWebhookURL: &rec.srv.URL}, devEncKey, ""); err != nil {
		t.Fatalf("set webhook: %v", err)
	}

	actor := service.Actor{Type: "agent", ID: "agent-f5", Name: "Subscriptions Watchdog"}
	report, err := svc.CreateAgentReport(ctx,
		"Recurring charge spike", "Netflix jumped from $15.49 to $22.99 this month.",
		actor, "warning", []string{"subscriptions"}, "", "", "")
	if err != nil {
		t.Fatalf("CreateAgentReport: %v", err)
	}

	p := rec.await(t, 5*time.Second)
	if p["_content_type"] != "application/json" {
		t.Errorf("content-type = %v, want application/json", p["_content_type"])
	}
	if p["event"] != "report" {
		t.Errorf("event = %v, want report", p["event"])
	}
	if p["title"] != "Recurring charge spike" {
		t.Errorf("title = %v, want report title", p["title"])
	}
	if p["priority"] != "warning" {
		t.Errorf("priority = %v, want warning", p["priority"])
	}
	if p["workflow"] != "Subscriptions Watchdog" {
		t.Errorf("workflow = %v, want actor name", p["workflow"])
	}
	if p["body"] != "Netflix jumped from $15.49 to $22.99 this month." {
		t.Errorf("body = %v, want report body", p["body"])
	}
	wantURL := "/reports/" + report.ShortID
	if p["url"] != wantURL {
		t.Errorf("url = %v, want %s", p["url"], wantURL)
	}
	if _, ok := p["sent_at"]; !ok {
		t.Error("sent_at missing from payload")
	}
}

// TestF5NotifyOnReport_NoWebhookNoOp proves that with no webhook configured,
// creating a workflow report is a silent no-op (no POST attempted).
func TestF5NotifyOnReport_NoWebhookNoOp(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// A recorder exists but is never wired into config — so nothing should
	// reach it.
	rec := f5newNotifyRecorder(t)

	actor := service.Actor{Type: "agent", ID: "agent-f5", Name: "Watchdog"}
	if _, err := svc.CreateAgentReport(ctx,
		"No sink", "This report has nowhere to go.",
		actor, "info", nil, "", "", ""); err != nil {
		t.Fatalf("CreateAgentReport: %v", err)
	}

	rec.expectNoFire(t, 750*time.Millisecond)
}

// TestF5NotifyOnReport_OperatorReportNoFire proves an operator-submitted
// report (actor.Type == "user") does NOT fire a notification even when a
// webhook is configured — only workflow/agent reports notify.
func TestF5NotifyOnReport_OperatorReportNoFire(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	rec := f5newNotifyRecorder(t)
	if _, err := svc.UpdateAgentSettings(ctx, service.UpdateAgentSettingsParams{NotifyWebhookURL: &rec.srv.URL}, devEncKey, ""); err != nil {
		t.Fatalf("set webhook: %v", err)
	}

	actor := service.Actor{Type: "user", ID: "operator-1", Name: "Ricardo"}
	if _, err := svc.CreateAgentReport(ctx,
		"Manual note", "Operator-authored report, should stay silent.",
		actor, "info", nil, "", "", ""); err != nil {
		t.Fatalf("CreateAgentReport: %v", err)
	}

	rec.expectNoFire(t, 750*time.Millisecond)
}

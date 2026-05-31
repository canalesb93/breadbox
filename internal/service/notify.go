//go:build !lite

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"breadbox/internal/appconfig"
)

// notifyHTTPClient is the shared client for outbound workflow notifications.
// A short timeout keeps a slow/hung webhook from blocking the caller.
var notifyHTTPClient = &http.Client{Timeout: 10 * time.Second}

// NotificationPayload is the JSON body POSTed to the configured notification
// webhook. Shaped to be readable by generic sinks (ntfy, Slack-compatible
// relays, email bridges) without per-provider formatting.
type NotificationPayload struct {
	Event    string `json:"event"`              // "test" | "report"
	Title    string `json:"title"`              // short headline
	Body     string `json:"body,omitempty"`     // longer text
	Priority string `json:"priority,omitempty"` // info | warning | critical
	Workflow string `json:"workflow,omitempty"` // originating workflow name
	URL      string `json:"url,omitempty"`      // deep link back into Breadbox
	SentAt   string `json:"sent_at"`            // RFC3339
}

// WorkflowNotificationConfigured reports whether an outbound webhook URL is set.
func (s *Service) WorkflowNotificationConfigured(ctx context.Context) bool {
	return strings.TrimSpace(appconfig.String(ctx, s.Queries, appconfig.KeyNotifyWebhookURL, "")) != ""
}

// SendWorkflowNotification POSTs the payload as JSON to the configured
// notification webhook. It is a no-op (returns nil) when no URL is
// configured, so callers can fire unconditionally. The URL must be http(s).
func (s *Service) SendWorkflowNotification(ctx context.Context, p NotificationPayload) error {
	raw := strings.TrimSpace(appconfig.String(ctx, s.Queries, appconfig.KeyNotifyWebhookURL, ""))
	if raw == "" {
		return nil // notifications disabled — no-op
	}
	if err := validateNotifyURL(raw); err != nil {
		return err
	}
	if p.SentAt == "" {
		p.SentAt = time.Now().UTC().Format(time.RFC3339)
	}
	body, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, raw, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build notification request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "breadbox-workflows")
	resp, err := notifyHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("notification webhook: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("notification webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// SendTestNotification fires a sample payload so an operator can verify their
// webhook wiring from Settings. Returns ErrInvalidParameter when no URL is set.
func (s *Service) SendTestNotification(ctx context.Context) error {
	if !s.WorkflowNotificationConfigured(ctx) {
		return fmt.Errorf("%w: no notification webhook URL configured", ErrInvalidParameter)
	}
	return s.SendWorkflowNotification(ctx, NotificationPayload{
		Event:    "test",
		Title:    "Breadbox workflow notification test",
		Body:     "If you can see this, your workflow notification webhook is wired up correctly.",
		Priority: "info",
	})
}

// validateNotifyURL rejects anything that isn't a well-formed http(s) URL.
func validateNotifyURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("%w: notification webhook URL must be a valid http(s) URL", ErrInvalidParameter)
	}
	return nil
}

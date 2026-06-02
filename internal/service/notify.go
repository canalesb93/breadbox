//go:build !lite

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"breadbox/internal/appconfig"
)

// notifyHTTPClient is the shared client for outbound workflow notifications.
// A short timeout keeps a slow/hung webhook from blocking the caller.
var notifyHTTPClient = &http.Client{Timeout: 10 * time.Second}

// notifyBodyMaxLen caps the report body carried in a notification payload.
// Sinks like ntfy / Slack / push relays truncate or choke on multi-KB
// bodies; the deep link in the payload carries the reader to the full text.
const notifyBodyMaxLen = 480

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

// reportNotificationPayload maps a freshly-created report into the outbound
// notification shape. Priority is normalized to the canonical info/warning/
// critical set; an unknown value falls back to "info" so a sink that keys
// behavior off priority never sees something unexpected. The body is
// truncated — the URL deep link carries the reader to the full report.
func reportNotificationPayload(r AgentReportResponse, workflow string) NotificationPayload {
	return NotificationPayload{
		Event:    "report",
		Title:    r.Title,
		Body:     truncateNotifyBody(r.Body),
		Priority: normalizeNotifyPriority(r.Priority),
		Workflow: workflow,
		URL:      "/reports/" + r.ShortID,
	}
}

// normalizeNotifyPriority clamps a report priority to the canonical
// info/warning/critical set, defaulting to "info".
func normalizeNotifyPriority(p string) string {
	switch p {
	case "warning", "critical":
		return p
	default:
		return "info"
	}
}

// truncateNotifyBody trims a report body to notifyBodyMaxLen runes, appending
// an ellipsis when it had to cut. Rune-aware so a multibyte body never splits
// mid-character.
func truncateNotifyBody(body string) string {
	r := []rune(body)
	if len(r) <= notifyBodyMaxLen {
		return body
	}
	return strings.TrimRight(string(r[:notifyBodyMaxLen]), " \t\n") + "…"
}

// WorkflowNotificationConfigured reports whether an outbound webhook URL is set.
func (s *Service) WorkflowNotificationConfigured(ctx context.Context) bool {
	return strings.TrimSpace(appconfig.String(ctx, s.Queries, appconfig.KeyNotifyWebhookURL, "")) != ""
}

// SendWorkflowNotification delivers the payload to the configured
// notification webhook. It is a no-op (returns nil) when no URL is
// configured, so callers can fire unconditionally. The URL must be http(s).
//
// The wire shape depends on the configured format (notify.format):
//   - ntfy   → a plain-text (markdown) body plus ntfy's publishing headers
//     (Title / Priority / Click / Markdown / Tags), so the push renders as
//     a real titled notification instead of a raw JSON blob.
//   - json   → the generic NotificationPayload envelope (Slack relays,
//     Discord bridges, custom consumers).
//   - auto   → ntfy when the URL looks like ntfy, else json.
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

	// Absolutize the deep link so both JSON consumers and ntfy tap-through
	// resolve to the real report rather than a bare path. No-op when the
	// operator hasn't set a public base URL.
	baseURL := normalizeBaseURL(appconfig.String(ctx, s.Queries, appconfig.KeyNotifyPublicBaseURL, ""))
	if baseURL != "" && strings.HasPrefix(p.URL, "/") {
		p.URL = baseURL + p.URL
	}

	format := resolveNotifyFormat(appconfig.String(ctx, s.Queries, appconfig.KeyNotifyFormat, appconfig.NotifyFormatAuto), raw)

	var req *http.Request
	var err error
	if format == appconfig.NotifyFormatNtfy {
		req, err = buildNtfyRequest(ctx, raw, p, baseURL)
	} else {
		req, err = buildJSONNotifyRequest(ctx, raw, p)
	}
	if err != nil {
		return err
	}

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

// resolveNotifyFormat maps the configured format to a concrete wire shape.
// "auto" (the default, and any unknown value) sniffs the URL for ntfy.
func resolveNotifyFormat(configured, rawURL string) string {
	switch configured {
	case appconfig.NotifyFormatNtfy:
		return appconfig.NotifyFormatNtfy
	case appconfig.NotifyFormatJSON:
		return appconfig.NotifyFormatJSON
	default:
		if looksLikeNtfy(rawURL) {
			return appconfig.NotifyFormatNtfy
		}
		return appconfig.NotifyFormatJSON
	}
}

// looksLikeNtfy reports whether a webhook URL points at an ntfy server.
// Matches the public host and any self-hosted host carrying "ntfy" in its
// name (push.ntfy.example.com, ntfy.lan, …).
func looksLikeNtfy(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "ntfy.sh" || strings.Contains(host, "ntfy")
}

// buildJSONNotifyRequest POSTs the generic JSON envelope. This is the
// original, provider-neutral shape consumed by Slack relays and bridges.
func buildJSONNotifyRequest(ctx context.Context, rawURL string, p NotificationPayload) (*http.Request, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal notification: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build notification request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "breadbox-workflows")
	return req, nil
}

// buildNtfyRequest POSTs to an ntfy topic using ntfy's native publishing
// protocol: the request body is the message text and metadata rides in
// headers. See https://docs.ntfy.sh/publish/.
func buildNtfyRequest(ctx context.Context, rawURL string, p NotificationPayload, baseURL string) (*http.Request, error) {
	message := p.Body
	if baseURL != "" {
		message = absolutizeMarkdownLinks(message, baseURL)
	}
	if strings.TrimSpace(message) == "" {
		message = p.Title // a body-less payload still needs a non-empty message
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(message))
	if err != nil {
		return nil, fmt.Errorf("build notification request: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	req.Header.Set("User-Agent", "breadbox-workflows")
	if title := encodeNtfyHeader(p.Title); title != "" {
		req.Header.Set("X-Title", title)
	}
	req.Header.Set("X-Priority", ntfyPriority(p.Priority))
	req.Header.Set("X-Markdown", "yes")
	if tags := ntfyTags(p.Priority); tags != "" {
		req.Header.Set("X-Tags", tags)
	}
	// ntfy needs an absolute URL for tap-through; a relative path is dropped.
	if strings.HasPrefix(p.URL, "http://") || strings.HasPrefix(p.URL, "https://") {
		req.Header.Set("X-Click", p.URL)
	}
	return req, nil
}

// ntfyPriority maps the canonical info/warning/critical scale to ntfy's
// numeric 1–5 priority (3 = default, 4 = high, 5 = max).
func ntfyPriority(priority string) string {
	switch priority {
	case "critical":
		return "5"
	case "warning":
		return "4"
	default:
		return "3"
	}
}

// ntfyTags maps a priority to an ntfy tag that renders as a leading emoji
// (ℹ️ / ⚠️ / 🚨) on the notification.
func ntfyTags(priority string) string {
	switch priority {
	case "critical":
		return "rotating_light"
	case "warning":
		return "warning"
	default:
		return "information_source"
	}
}

// encodeNtfyHeader makes a string safe to carry in an HTTP header value:
// header values are single-line and ASCII, so collapse newlines and
// RFC 2047-encode any non-ASCII content (ntfy decodes it back).
func encodeNtfyHeader(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if isASCII(s) {
		return s
	}
	return mime.QEncoding.Encode("utf-8", s)
}

// isASCII reports whether every rune in s is a 7-bit ASCII character.
func isASCII(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII {
			return false
		}
	}
	return true
}

// absolutizeMarkdownLinks rewrites root-relative markdown links (`](/foo)`)
// to absolute ones so an ntfy client (which has no notion of the Breadbox
// origin) can follow them. Only root-relative targets are touched.
func absolutizeMarkdownLinks(body, baseURL string) string {
	if baseURL == "" {
		return body
	}
	return strings.ReplaceAll(body, "](/", "]("+baseURL+"/")
}

// normalizeBaseURL trims surrounding whitespace and any trailing slash so
// callers can concatenate "/path" without doubling the separator.
func normalizeBaseURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
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

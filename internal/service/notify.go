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
	"regexp"
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

// WorkflowNotificationConfigured reports whether at least one enabled
// notification channel is configured.
func (s *Service) WorkflowNotificationConfigured(ctx context.Context) bool {
	for _, c := range s.loadNotificationChannels(ctx) {
		if c.Enabled {
			return true
		}
	}
	return false
}

// SendWorkflowNotification fans the payload out to every enabled notification
// channel. It is a no-op (returns nil) when no channel is configured, so
// callers can fire unconditionally.
//
// Each channel is rendered in its own format and gated by its own priority
// floor. The wire shape per channel:
//   - ntfy    → a plain-text (markdown) body plus ntfy's publishing headers
//     (Title / Priority / Click / Markdown / Tags / Actions), so the push
//     renders as a real titled notification instead of a raw JSON blob.
//   - slack   → a Slack incoming-webhook JSON ({"text": …}) with mrkdwn.
//   - discord → a Discord webhook JSON ({"content": …}) with markdown.
//   - json    → the generic NotificationPayload envelope (relays, bridges).
//   - auto    → the matching provider when the URL is recognizable, else json.
//
// Reports below a channel's priority floor are skipped for that channel.
// Per-channel delivery is retried with backoff on transient failures, and the
// outcome is recorded on the channel. Returns the first delivery error (if
// any) after attempting every channel.
func (s *Service) SendWorkflowNotification(ctx context.Context, p NotificationPayload) error {
	// A persisted channels array is the source of truth; an empty one means we
	// synthesize an ephemeral legacy channel. We must NOT persist statuses back
	// in the synth case — doing so would promote the legacy config into the
	// channels array and make further legacy-webhook edits silently ignored.
	persisted := strings.TrimSpace(appconfig.String(ctx, s.Queries, appconfig.KeyNotifyChannels, "")) != ""
	chans := s.loadNotificationChannels(ctx)
	if len(chans) == 0 {
		return nil // notifications disabled — no-op
	}

	if p.SentAt == "" {
		p.SentAt = nowRFC3339()
	}
	// Absolutize the deep link once so every channel (ntfy tap-through, JSON
	// consumers) resolves to the real report rather than a bare path.
	baseURL := normalizeBaseURL(appconfig.String(ctx, s.Queries, appconfig.KeyNotifyPublicBaseURL, ""))
	if baseURL != "" && strings.HasPrefix(p.URL, "/") {
		p.URL = baseURL + p.URL
	}

	var firstErr error
	attempted := false
	for i := range chans {
		c := &chans[i]
		if !c.Enabled {
			continue
		}
		// Per-channel priority floor; test sends always go through.
		if p.Event != "test" && priorityRank(p.Priority) < priorityRank(c.MinPriority) {
			continue
		}
		attempted = true
		if err := s.sendToChannel(ctx, c, p, baseURL); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if attempted && persisted {
		// Persist the recorded per-channel statuses (best-effort — a status
		// write failure must not mask a successful delivery). Skipped for the
		// synthesized legacy channel so legacy config stays editable.
		_ = s.persistNotificationChannels(ctx, chans)
	}
	return firstErr
}

// sendToChannel delivers p to a single channel, recording the outcome on
// c.LastStatus. The channel's URL is validated defensively (legacy/imported
// data) and its format + token drive the wire shape.
func (s *Service) sendToChannel(ctx context.Context, c *NotificationChannel, p NotificationPayload, baseURL string) error {
	if err := validateNotifyURL(c.URL); err != nil {
		c.LastStatus = &NotificationDeliveryStatus{OK: false, At: nowRFC3339(), Detail: "invalid URL"}
		return err
	}
	format := resolveNotifyFormat(c.Format, c.URL)
	// build constructs a fresh request per attempt — the body is a reader that
	// can't be rewound, so we can't reuse one *http.Request across retries.
	build := func() (*http.Request, error) {
		switch format {
		case appconfig.NotifyFormatNtfy:
			return buildNtfyRequest(ctx, c.URL, p, baseURL, c.NtfyToken)
		case appconfig.NotifyFormatSlack:
			return buildSlackRequest(ctx, c.URL, p)
		case appconfig.NotifyFormatDiscord:
			return buildDiscordRequest(ctx, c.URL, p)
		case appconfig.NotifyFormatGoogleChat:
			return buildGoogleChatRequest(ctx, c.URL, p)
		default:
			return buildJSONNotifyRequest(ctx, c.URL, p)
		}
	}
	err := sendNotifyWithRetry(ctx, build)
	status := NotificationDeliveryStatus{OK: err == nil, At: nowRFC3339(), Format: format, Detail: "delivered"}
	if err != nil {
		status.Detail = err.Error()
	}
	c.LastStatus = &status
	return err
}

// ResolveNotifyFormat is the exported view of format resolution — used by the
// admin UI to show what a channel's "auto" format actually resolved to.
func ResolveNotifyFormat(configured, rawURL string) string {
	return resolveNotifyFormat(configured, rawURL)
}

// resolveNotifyFormat maps the configured format to a concrete wire shape.
// "auto" (the default, and any unknown value) sniffs the URL for a known
// provider (ntfy / Slack / Discord / Google Chat), falling back to the
// generic JSON shape.
func resolveNotifyFormat(configured, rawURL string) string {
	switch configured {
	case appconfig.NotifyFormatNtfy, appconfig.NotifyFormatSlack,
		appconfig.NotifyFormatDiscord, appconfig.NotifyFormatGoogleChat,
		appconfig.NotifyFormatJSON:
		return configured
	default:
		switch {
		case looksLikeNtfy(rawURL):
			return appconfig.NotifyFormatNtfy
		case looksLikeSlack(rawURL):
			return appconfig.NotifyFormatSlack
		case looksLikeDiscord(rawURL):
			return appconfig.NotifyFormatDiscord
		case looksLikeGoogleChat(rawURL):
			return appconfig.NotifyFormatGoogleChat
		default:
			return appconfig.NotifyFormatJSON
		}
	}
}

// looksLikeGoogleChat reports whether a URL is a Google Chat incoming webhook.
func looksLikeGoogleChat(raw string) bool {
	return urlHost(raw) == "chat.googleapis.com"
}

// looksLikeNtfy reports whether a webhook URL points at an ntfy server.
// Matches the public host and any self-hosted host carrying "ntfy" in its
// name (push.ntfy.example.com, ntfy.lan, …).
func looksLikeNtfy(raw string) bool {
	host := urlHost(raw)
	return host == "ntfy.sh" || strings.Contains(host, "ntfy")
}

// looksLikeSlack reports whether a URL is a Slack incoming webhook.
func looksLikeSlack(raw string) bool {
	return urlHost(raw) == "hooks.slack.com"
}

// looksLikeDiscord reports whether a URL is a Discord webhook.
func looksLikeDiscord(raw string) bool {
	host := urlHost(raw)
	return (host == "discord.com" || host == "discordapp.com" || strings.HasSuffix(host, ".discord.com")) &&
		strings.Contains(raw, "/webhooks/")
}

// urlHost returns the lower-cased host of a URL, or "" if unparsable.
func urlHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// priorityRank orders the canonical priorities for floor comparison.
// Unknown values rank as "info" (the lowest) so they're never silently
// filtered by a stricter floor.
func priorityRank(priority string) int {
	switch priority {
	case "critical":
		return 2
	case "warning":
		return 1
	default:
		return 0
	}
}

// notifyMaxAttempts bounds delivery retries (1 initial + retries).
const notifyMaxAttempts = 3

// sendNotifyWithRetry delivers a freshly-built request, retrying transient
// failures (network errors, HTTP 429, and 5xx) with exponential backoff.
// A 4xx other than 429 fails fast — retrying a malformed payload or a bad
// topic won't help. Each attempt rebuilds the request because its body is a
// one-shot reader.
func sendNotifyWithRetry(ctx context.Context, build func() (*http.Request, error)) error {
	var lastErr error
	for attempt := 1; attempt <= notifyMaxAttempts; attempt++ {
		req, err := build()
		if err != nil {
			return err // build errors are deterministic — don't retry
		}
		resp, err := notifyHTTPClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("notification webhook: %w", err)
		} else {
			func() {
				defer resp.Body.Close()
				if resp.StatusCode < 300 {
					lastErr = nil
				} else {
					lastErr = fmt.Errorf("notification webhook returned HTTP %d", resp.StatusCode)
				}
			}()
			if lastErr == nil {
				return nil
			}
			if !notifyRetriableStatus(resp.StatusCode) {
				return lastErr // permanent (4xx ≠ 429) — stop now
			}
		}
		if attempt < notifyMaxAttempts {
			// Exponential backoff: 200ms, 400ms, … bounded and ctx-aware.
			backoff := time.Duration(200*(1<<(attempt-1))) * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}
	}
	return lastErr
}

// notifyRetriableStatus reports whether an HTTP status warrants a retry:
// 429 (rate limited) and any 5xx (server-side transient). 4xx otherwise is
// permanent.
func notifyRetriableStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= 500
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
// headers. See https://docs.ntfy.sh/publish/. token, when non-empty, is sent
// as a Bearer credential for protected topics on self-hosted servers.
func buildNtfyRequest(ctx context.Context, rawURL string, p NotificationPayload, baseURL, token string) (*http.Request, error) {
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
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if title := encodeNtfyHeader(p.Title); title != "" {
		req.Header.Set("X-Title", title)
	}
	req.Header.Set("X-Priority", ntfyPriority(p.Priority))
	req.Header.Set("X-Markdown", "yes")
	if tags := ntfyTags(p.Priority); tags != "" {
		req.Header.Set("X-Tags", tags)
	}
	// ntfy needs an absolute URL for tap-through; a relative path is dropped.
	if isAbsoluteURL(p.URL) {
		req.Header.Set("X-Click", p.URL)
		// A tappable action button in addition to the whole-notification tap.
		req.Header.Set("X-Actions", "view, View report, "+p.URL)
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

// notifyMarkdownLink matches a markdown inline link: [label](url).
var notifyMarkdownLink = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

// notifyBoldRun matches a **bold** run.
var notifyBoldRun = regexp.MustCompile(`\*\*([^*]+)\*\*`)

// notifyEmojiShortcode returns an emoji shortcode (rendered by Slack and
// Discord alike) for a priority — the same visual cue ntfy gets from tags.
func notifyEmojiShortcode(priority string) string {
	switch priority {
	case "critical":
		return ":rotating_light:"
	case "warning":
		return ":warning:"
	default:
		return ":information_source:"
	}
}

// buildSlackRequest POSTs a Slack incoming-webhook payload ({"text": …}).
// The body is converted from Markdown to Slack mrkdwn: **bold** → *bold*
// and [label](url) → <url|label>. A leading priority emoji + bold title set
// the headline; an absolute deep link is appended as a tappable link.
func buildSlackRequest(ctx context.Context, rawURL string, p NotificationPayload) (*http.Request, error) {
	var b strings.Builder
	b.WriteString(notifyEmojiShortcode(p.Priority))
	b.WriteString(" *")
	b.WriteString(slackEscape(collapseToLine(p.Title)))
	b.WriteString("*")
	if body := strings.TrimSpace(p.Body); body != "" {
		b.WriteString("\n")
		b.WriteString(toSlackMrkdwn(body))
	}
	if isAbsoluteURL(p.URL) {
		b.WriteString("\n<")
		b.WriteString(p.URL)
		b.WriteString("|View report →>")
	}
	return jsonNotifyRequest(ctx, rawURL, map[string]any{"text": b.String()})
}

// buildGoogleChatRequest POSTs a Google Chat incoming-webhook payload
// ({"text": …}). Google Chat uses the same single-asterisk bold + <url|label>
// link syntax as Slack mrkdwn, but renders unicode emoji rather than
// :shortcode: names — so the priority cue is a unicode glyph.
func buildGoogleChatRequest(ctx context.Context, rawURL string, p NotificationPayload) (*http.Request, error) {
	var b strings.Builder
	b.WriteString(notifyEmojiUnicode(p.Priority))
	b.WriteString(" *")
	b.WriteString(slackEscape(collapseToLine(p.Title)))
	b.WriteString("*")
	if body := strings.TrimSpace(p.Body); body != "" {
		b.WriteString("\n")
		b.WriteString(toSlackMrkdwn(body))
	}
	if isAbsoluteURL(p.URL) {
		b.WriteString("\n<")
		b.WriteString(p.URL)
		b.WriteString("|View report →>")
	}
	return jsonNotifyRequest(ctx, rawURL, map[string]any{"text": b.String()})
}

// notifyEmojiUnicode returns a unicode emoji for a priority — used where
// :shortcode: names aren't rendered (Google Chat).
func notifyEmojiUnicode(priority string) string {
	switch priority {
	case "critical":
		return "\U0001F6A8" // 🚨
	case "warning":
		return "⚠️" // ⚠️
	default:
		return "ℹ️" // ℹ️
	}
}

// buildDiscordRequest POSTs a Discord webhook payload ({"content": …}).
// Discord renders standard Markdown (bold, lists) in message content, so the
// body passes through; an absolute deep link is appended on its own line
// (Discord auto-links bare URLs). Content is capped at Discord's 2000-char
// limit.
func buildDiscordRequest(ctx context.Context, rawURL string, p NotificationPayload) (*http.Request, error) {
	var b strings.Builder
	b.WriteString(notifyEmojiShortcode(p.Priority))
	b.WriteString(" **")
	b.WriteString(collapseToLine(p.Title))
	b.WriteString("**")
	if body := strings.TrimSpace(p.Body); body != "" {
		b.WriteString("\n")
		b.WriteString(body)
	}
	if isAbsoluteURL(p.URL) {
		b.WriteString("\n")
		b.WriteString(p.URL)
	}
	content := b.String()
	if r := []rune(content); len(r) > 1990 {
		content = strings.TrimRight(string(r[:1990]), " \t\n") + "…"
	}
	return jsonNotifyRequest(ctx, rawURL, map[string]any{"content": content})
}

// jsonNotifyRequest marshals v and builds a JSON POST with the shared headers.
func jsonNotifyRequest(ctx context.Context, rawURL string, v any) (*http.Request, error) {
	body, err := json.Marshal(v)
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

// toSlackMrkdwn converts the subset of Markdown our report bodies use into
// Slack mrkdwn: [label](url) → <url|label>, then **bold** → *bold*.
// Links are rewritten first so the bold pass can't touch a URL.
func toSlackMrkdwn(s string) string {
	s = notifyMarkdownLink.ReplaceAllString(s, "<$2|$1>")
	s = notifyBoldRun.ReplaceAllString(s, "*$1*")
	return s
}

// slackEscape escapes the three characters Slack reserves in mrkdwn text so a
// stray <, >, or & in a title can't break link syntax. Applied to plain-text
// fragments only — never to a fragment that already contains <url|label>.
func slackEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// collapseToLine flattens newlines to spaces — used for titles that must
// render on a single line.
func collapseToLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

// isAbsoluteURL reports whether u is an http(s) absolute URL.
func isAbsoluteURL(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

// SendTestNotification fires a sample payload so an operator can verify their
// webhook wiring from Settings. Returns ErrInvalidParameter when no URL is set.
func (s *Service) SendTestNotification(ctx context.Context) error {
	if !s.WorkflowNotificationConfigured(ctx) {
		return fmt.Errorf("%w: no notification channel configured", ErrInvalidParameter)
	}
	return s.SendWorkflowNotification(ctx, testNotificationPayload())
}

// validateNotifyURL rejects anything that isn't a well-formed http(s) URL.
func validateNotifyURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("%w: notification webhook URL must be a valid http(s) URL", ErrInvalidParameter)
	}
	return nil
}

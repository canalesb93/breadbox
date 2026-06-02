//go:build !lite

package service

// Unit tests (no DB) for the Slack/Discord publishing paths, provider
// auto-detection, the priority floor, and the retry classifier.

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"breadbox/internal/appconfig"
)

func TestLooksLikeSlackAndDiscord(t *testing.T) {
	if !looksLikeSlack("https://hooks.slack.com/services/T0/B0/xyz") {
		t.Error("slack webhook not detected")
	}
	if looksLikeSlack("https://example.com/hook") {
		t.Error("non-slack detected as slack")
	}
	discord := []string{
		"https://discord.com/api/webhooks/123/abc",
		"https://discordapp.com/api/webhooks/123/abc",
		"https://ptb.discord.com/api/webhooks/123/abc",
	}
	for _, u := range discord {
		if !looksLikeDiscord(u) {
			t.Errorf("discord webhook not detected: %s", u)
		}
	}
	if looksLikeDiscord("https://discord.com/channels/123") {
		t.Error("non-webhook discord URL detected as webhook")
	}
}

func TestResolveNotifyFormat_Providers(t *testing.T) {
	cases := []struct {
		configured, url, want string
	}{
		{appconfig.NotifyFormatAuto, "https://hooks.slack.com/services/x", appconfig.NotifyFormatSlack},
		{appconfig.NotifyFormatAuto, "https://discord.com/api/webhooks/1/a", appconfig.NotifyFormatDiscord},
		{appconfig.NotifyFormatAuto, "https://chat.googleapis.com/v1/spaces/AAA/messages?key=k&token=t", appconfig.NotifyFormatGoogleChat},
		{appconfig.NotifyFormatAuto, "https://ntfy.sh/t", appconfig.NotifyFormatNtfy},
		{appconfig.NotifyFormatAuto, "https://example.com/h", appconfig.NotifyFormatJSON},
		{appconfig.NotifyFormatSlack, "https://ntfy.sh/t", appconfig.NotifyFormatSlack},     // explicit overrides sniff
		{appconfig.NotifyFormatDiscord, "https://example.com/h", appconfig.NotifyFormatDiscord},
	}
	for _, tc := range cases {
		if got := resolveNotifyFormat(tc.configured, tc.url); got != tc.want {
			t.Errorf("resolveNotifyFormat(%q,%q)=%q want %q", tc.configured, tc.url, got, tc.want)
		}
	}
}

func TestPriorityRank(t *testing.T) {
	if !(priorityRank("info") < priorityRank("warning") && priorityRank("warning") < priorityRank("critical")) {
		t.Error("priority ranks not ordered info<warning<critical")
	}
	if priorityRank("bogus") != priorityRank("info") {
		t.Error("unknown priority should rank as info")
	}
}

func TestToSlackMrkdwn(t *testing.T) {
	in := "A **bold** word and a [link](https://x.com/a)."
	got := toSlackMrkdwn(in)
	if !strings.Contains(got, "*bold*") {
		t.Errorf("bold not converted: %q", got)
	}
	if !strings.Contains(got, "<https://x.com/a|link>") {
		t.Errorf("link not converted: %q", got)
	}
	if strings.Contains(got, "**") || strings.Contains(got, "](") {
		t.Errorf("markdown left over: %q", got)
	}
}

// decodeJSONField reads the request body and returns one string field.
func decodeJSONField(t *testing.T, r *http.Request, field string) string {
	t.Helper()
	b, _ := io.ReadAll(r.Body)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("body not JSON: %v (%s)", err, string(b))
	}
	s, _ := m[field].(string)
	return s
}

func TestBuildSlackRequest(t *testing.T) {
	p := NotificationPayload{
		Title:    "Large charge",
		Body:     "A **$420** charge. See [tx](https://bb/tx/abc).",
		Priority: "critical",
		URL:      "https://bb/reports/xyz",
	}
	req, err := buildSlackRequest(t.Context(), "https://hooks.slack.com/services/x", p)
	if err != nil {
		t.Fatalf("buildSlackRequest: %v", err)
	}
	if ct := req.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q", ct)
	}
	text := decodeJSONField(t, req, "text")
	wants := []string{":rotating_light:", "*Large charge*", "*$420*", "<https://bb/tx/abc|tx>", "<https://bb/reports/xyz|View report →>"}
	for _, w := range wants {
		if !strings.Contains(text, w) {
			t.Errorf("slack text missing %q\ngot: %q", w, text)
		}
	}
}

func TestBuildGoogleChatRequest(t *testing.T) {
	p := NotificationPayload{
		Title:    "Sync watchdog",
		Body:     "Last sync **failed**. See [logs](https://bb/logs).",
		Priority: "critical",
		URL:      "https://bb/reports/xyz",
	}
	req, err := buildGoogleChatRequest(t.Context(), "https://chat.googleapis.com/v1/spaces/A/messages?key=k", p)
	if err != nil {
		t.Fatalf("buildGoogleChatRequest: %v", err)
	}
	if ct := req.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q", ct)
	}
	text := decodeJSONField(t, req, "text")
	// Single-asterisk bold title, mrkdwn link conversion, unicode emoji (not a
	// :shortcode:), and the view-report link.
	wants := []string{"\U0001F6A8", "*Sync watchdog*", "*failed*", "<https://bb/logs|logs>", "<https://bb/reports/xyz|View report →>"}
	for _, w := range wants {
		if !strings.Contains(text, w) {
			t.Errorf("googlechat text missing %q\ngot: %q", w, text)
		}
	}
	if strings.Contains(text, ":rotating_light:") {
		t.Errorf("googlechat should use unicode emoji, not a shortcode: %q", text)
	}
}

func TestLooksLikeGoogleChat(t *testing.T) {
	if !looksLikeGoogleChat("https://chat.googleapis.com/v1/spaces/A/messages?key=k&token=t") {
		t.Error("google chat webhook not detected")
	}
	if looksLikeGoogleChat("https://hooks.slack.com/services/x") {
		t.Error("slack detected as google chat")
	}
}

func TestBuildDiscordRequest(t *testing.T) {
	p := NotificationPayload{
		Title:    "Spending spike",
		Body:     "Dining up **3×** this week.",
		Priority: "warning",
		URL:      "https://bb/reports/xyz",
	}
	req, err := buildDiscordRequest(t.Context(), "https://discord.com/api/webhooks/1/a", p)
	if err != nil {
		t.Fatalf("buildDiscordRequest: %v", err)
	}
	content := decodeJSONField(t, req, "content")
	wants := []string{":warning:", "**Spending spike**", "**3×**", "https://bb/reports/xyz"}
	for _, w := range wants {
		if !strings.Contains(content, w) {
			t.Errorf("discord content missing %q\ngot: %q", w, content)
		}
	}
}

func TestBuildDiscordRequest_TruncatesAt2000(t *testing.T) {
	p := NotificationPayload{Title: "T", Body: strings.Repeat("x", 5000), Priority: "info"}
	req, err := buildDiscordRequest(t.Context(), "https://discord.com/api/webhooks/1/a", p)
	if err != nil {
		t.Fatalf("buildDiscordRequest: %v", err)
	}
	content := decodeJSONField(t, req, "content")
	if n := len([]rune(content)); n > 2000 {
		t.Errorf("discord content = %d runes, want ≤ 2000", n)
	}
	if !strings.HasSuffix(content, "…") {
		t.Error("truncated content should end with ellipsis")
	}
}

func TestNotifyRetriableStatus(t *testing.T) {
	retriable := []int{429, 500, 502, 503}
	permanent := []int{400, 401, 403, 404, 422}
	for _, c := range retriable {
		if !notifyRetriableStatus(c) {
			t.Errorf("status %d should be retriable", c)
		}
	}
	for _, c := range permanent {
		if notifyRetriableStatus(c) {
			t.Errorf("status %d should be permanent", c)
		}
	}
}

func TestValidNotifyMinPriority(t *testing.T) {
	for _, v := range []string{"info", "warning", "critical"} {
		if !validNotifyMinPriority(v) {
			t.Errorf("%q should be valid", v)
		}
	}
	for _, v := range []string{"", "low", "urgent"} {
		if validNotifyMinPriority(v) {
			t.Errorf("%q should be invalid", v)
		}
	}
	if notifyMinPriorityOrDefault("bogus") != appconfig.NotifyMinPriorityInfo {
		t.Error("bogus floor should default to info")
	}
}

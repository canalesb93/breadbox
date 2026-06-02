//go:build !lite

package service

// Unit tests for the ntfy publishing path in notify.go — format resolution,
// header encoding, priority/tag mapping, link absolutization, and the
// assembled ntfy request. All pure (no DB).

import (
	"io"
	"testing"

	"breadbox/internal/appconfig"
)

func TestResolveNotifyFormat(t *testing.T) {
	cases := []struct {
		name       string
		configured string
		url        string
		want       string
	}{
		{"explicit ntfy wins", appconfig.NotifyFormatNtfy, "https://example.com/hook", appconfig.NotifyFormatNtfy},
		{"explicit json wins", appconfig.NotifyFormatJSON, "https://ntfy.sh/topic", appconfig.NotifyFormatJSON},
		{"auto detects ntfy.sh", appconfig.NotifyFormatAuto, "https://ntfy.sh/breadbox", appconfig.NotifyFormatNtfy},
		{"auto detects self-hosted ntfy", appconfig.NotifyFormatAuto, "https://push.ntfy.example.com/t", appconfig.NotifyFormatNtfy},
		{"auto falls back to json", appconfig.NotifyFormatAuto, "https://example.com/generic-hook", appconfig.NotifyFormatJSON},
		{"empty configured behaves as auto", "", "https://ntfy.sh/t", appconfig.NotifyFormatNtfy},
		{"unknown configured behaves as auto", "garbage", "https://example.com/h", appconfig.NotifyFormatJSON},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveNotifyFormat(tc.configured, tc.url); got != tc.want {
				t.Errorf("resolveNotifyFormat(%q, %q) = %q, want %q", tc.configured, tc.url, got, tc.want)
			}
		})
	}
}

func TestLooksLikeNtfy(t *testing.T) {
	yes := []string{
		"https://ntfy.sh/breadbox",
		"http://ntfy.sh/t",
		"https://ntfy.example.com/topic",
		"https://push.ntfy.lan:8080/alerts",
	}
	no := []string{
		"https://hooks.slack.com/services/x",
		"https://example.com/webhook",
		"https://discord.com/api/webhooks/1/abc",
		"not a url",
	}
	for _, u := range yes {
		if !looksLikeNtfy(u) {
			t.Errorf("looksLikeNtfy(%q) = false, want true", u)
		}
	}
	for _, u := range no {
		if looksLikeNtfy(u) {
			t.Errorf("looksLikeNtfy(%q) = true, want false", u)
		}
	}
}

func TestNtfyPriority(t *testing.T) {
	cases := map[string]string{
		"info":     "3",
		"warning":  "4",
		"critical": "5",
		"":         "3", // unknown → default
		"weird":    "3",
	}
	for in, want := range cases {
		if got := ntfyPriority(in); got != want {
			t.Errorf("ntfyPriority(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNtfyTags(t *testing.T) {
	cases := map[string]string{
		"info":     "information_source",
		"warning":  "warning",
		"critical": "rotating_light",
		"":         "information_source",
	}
	for in, want := range cases {
		if got := ntfyTags(in); got != want {
			t.Errorf("ntfyTags(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEncodeNtfyHeader(t *testing.T) {
	// ASCII passes through unchanged (newlines collapsed to spaces).
	if got := encodeNtfyHeader("Large charge detected"); got != "Large charge detected" {
		t.Errorf("ascii encode = %q, want unchanged", got)
	}
	if got := encodeNtfyHeader("line1\nline2"); got != "line1 line2" {
		t.Errorf("newline collapse = %q, want %q", got, "line1 line2")
	}
	// Non-ASCII is RFC 2047 encoded (starts with the encoded-word marker).
	got := encodeNtfyHeader("Café charge ⚠️")
	if len(got) < 8 || got[:2] != "=?" {
		t.Errorf("non-ascii encode = %q, want RFC2047 encoded-word", got)
	}
	if encodeNtfyHeader("   ") != "" {
		t.Error("whitespace-only should encode to empty")
	}
}

func TestAbsolutizeMarkdownLinks(t *testing.T) {
	body := "See [tx](/transactions/abc) and [report](/reports/x)."
	want := "See [tx](https://bb.example.com/transactions/abc) and [report](https://bb.example.com/reports/x)."
	if got := absolutizeMarkdownLinks(body, "https://bb.example.com"); got != want {
		t.Errorf("absolutizeMarkdownLinks =\n  %q\nwant\n  %q", got, want)
	}
	// Absolute links and external links are left untouched.
	ext := "Visit [site](https://other.com/x)"
	if got := absolutizeMarkdownLinks(ext, "https://bb.example.com"); got != ext {
		t.Errorf("external link rewritten: %q", got)
	}
	// Empty base URL is a no-op.
	if got := absolutizeMarkdownLinks(body, ""); got != body {
		t.Errorf("empty base URL should no-op, got %q", got)
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	cases := map[string]string{
		"https://bb.example.com/":   "https://bb.example.com",
		"  https://bb.example.com ": "https://bb.example.com",
		"https://bb.example.com///": "https://bb.example.com",
		"":                          "",
	}
	for in, want := range cases {
		if got := normalizeBaseURL(in); got != want {
			t.Errorf("normalizeBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildNtfyRequest(t *testing.T) {
	p := NotificationPayload{
		Event:    "report",
		Title:    "Large charge detected",
		Body:     "A **$420** charge hit checking. See [tx](/transactions/abc).",
		Priority: "warning",
		URL:      "https://bb.example.com/reports/xyz",
	}
	req, err := buildNtfyRequest(t.Context(), "https://ntfy.sh/breadbox", p, "https://bb.example.com", "tk_secret")
	if err != nil {
		t.Fatalf("buildNtfyRequest: %v", err)
	}
	if req.Method != "POST" {
		t.Errorf("method = %q, want POST", req.Method)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer tk_secret" {
		t.Errorf("Authorization = %q, want Bearer tk_secret", got)
	}
	if got := req.Header.Get("X-Actions"); got != "view, View report, https://bb.example.com/reports/xyz" {
		t.Errorf("X-Actions = %q", got)
	}
	if got := req.Header.Get("X-Title"); got != "Large charge detected" {
		t.Errorf("X-Title = %q", got)
	}
	if got := req.Header.Get("X-Priority"); got != "4" {
		t.Errorf("X-Priority = %q, want 4", got)
	}
	if got := req.Header.Get("X-Markdown"); got != "yes" {
		t.Errorf("X-Markdown = %q, want yes", got)
	}
	if got := req.Header.Get("X-Tags"); got != "warning" {
		t.Errorf("X-Tags = %q, want warning", got)
	}
	if got := req.Header.Get("X-Click"); got != "https://bb.example.com/reports/xyz" {
		t.Errorf("X-Click = %q", got)
	}
	body, _ := io.ReadAll(req.Body)
	want := "A **$420** charge hit checking. See [tx](https://bb.example.com/transactions/abc)."
	if string(body) != want {
		t.Errorf("body =\n  %q\nwant\n  %q", string(body), want)
	}
}

func TestBuildNtfyRequest_RelativeClickDropped(t *testing.T) {
	// Without a public base URL the deep link stays relative; ntfy needs an
	// absolute URL for X-Click, so it must be omitted rather than sent broken.
	p := NotificationPayload{Title: "Test", Body: "hi", Priority: "info", URL: "/reports/xyz"}
	req, err := buildNtfyRequest(t.Context(), "https://ntfy.sh/t", p, "", "")
	if err != nil {
		t.Fatalf("buildNtfyRequest: %v", err)
	}
	if got := req.Header.Get("X-Click"); got != "" {
		t.Errorf("X-Click = %q, want empty for relative URL", got)
	}
	if got := req.Header.Get("X-Actions"); got != "" {
		t.Errorf("X-Actions = %q, want empty for relative URL", got)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization = %q, want empty when no token", got)
	}
}

func TestBuildNtfyRequest_EmptyBodyFallsBackToTitle(t *testing.T) {
	p := NotificationPayload{Title: "Sync watchdog", Body: "", Priority: "info"}
	req, err := buildNtfyRequest(t.Context(), "https://ntfy.sh/t", p, "", "")
	if err != nil {
		t.Fatalf("buildNtfyRequest: %v", err)
	}
	body, _ := io.ReadAll(req.Body)
	if string(body) != "Sync watchdog" {
		t.Errorf("body = %q, want title fallback", string(body))
	}
}

func TestValidNotifyFormat(t *testing.T) {
	for _, v := range []string{"auto", "ntfy", "slack", "discord", "json"} {
		if !validNotifyFormat(v) {
			t.Errorf("validNotifyFormat(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"", "JSON", "Slack", "Ntfy", "webhook"} {
		if validNotifyFormat(v) {
			t.Errorf("validNotifyFormat(%q) = true, want false", v)
		}
	}
}

func TestNotifyFormatOrDefault(t *testing.T) {
	if got := notifyFormatOrDefault("ntfy"); got != "ntfy" {
		t.Errorf("valid passthrough = %q", got)
	}
	if got := notifyFormatOrDefault(""); got != appconfig.NotifyFormatAuto {
		t.Errorf("empty → %q, want auto", got)
	}
	if got := notifyFormatOrDefault("bogus"); got != appconfig.NotifyFormatAuto {
		t.Errorf("bogus → %q, want auto", got)
	}
}

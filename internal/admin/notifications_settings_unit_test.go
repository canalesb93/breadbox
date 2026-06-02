//go:build !headless && !lite

package admin

import "testing"

func TestMaskNotifyURL(t *testing.T) {
	cases := map[string]string{
		"https://hooks.slack.com/services/T0/B0/secret": "https://hooks.slack.com/…cret",
		"https://ntfy.sh/my-topic":                      "https://ntfy.sh/…opic",
		"https://ntfy.sh":                               "https://ntfy.sh",
		"https://ntfy.sh/":                              "https://ntfy.sh",
	}
	for in, want := range cases {
		if got := maskNotifyURL(in); got != want {
			t.Errorf("maskNotifyURL(%q) = %q, want %q", in, got, want)
		}
	}
	// A secret webhook path must never appear in full.
	full := "https://discord.com/api/webhooks/123456/SUPER-SECRET-TOKEN"
	if got := maskNotifyURL(full); got == full {
		t.Error("maskNotifyURL leaked the full secret URL")
	}
}

func TestNotifyFormatLabel(t *testing.T) {
	cases := map[string]string{
		"ntfy":       "ntfy",
		"slack":      "Slack",
		"discord":    "Discord",
		"googlechat": "Google Chat",
		"json":       "Generic JSON",
		"auto":       "Auto-detect",
		"":           "Auto-detect",
		"weird":      "Auto-detect",
	}
	for in, want := range cases {
		if got := notifyFormatLabel(in); got != want {
			t.Errorf("notifyFormatLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestChannelFormatLabel(t *testing.T) {
	// Explicit format → plain label.
	if got := channelFormatLabel("slack", "https://hooks.slack.com/x"); got != "Slack" {
		t.Errorf("explicit slack = %q", got)
	}
	// Auto over a recognizable URL → "Auto · <provider>".
	if got := channelFormatLabel("auto", "https://ntfy.sh/t"); got != "Auto · ntfy" {
		t.Errorf("auto ntfy = %q, want 'Auto · ntfy'", got)
	}
	if got := channelFormatLabel("auto", "https://chat.googleapis.com/v1/spaces/A/messages"); got != "Auto · Google Chat" {
		t.Errorf("auto googlechat = %q", got)
	}
	// Auto over a generic URL → plain "Auto-detect".
	if got := channelFormatLabel("auto", "https://example.com/hook"); got != "Auto-detect" {
		t.Errorf("auto generic = %q, want 'Auto-detect'", got)
	}
}

func TestRelTimeRFC3339_BadInput(t *testing.T) {
	if got := relTimeRFC3339("not-a-time"); got != "" {
		t.Errorf("relTimeRFC3339(bad) = %q, want empty", got)
	}
}

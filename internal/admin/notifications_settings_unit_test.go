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
		"ntfy":    "ntfy",
		"slack":   "Slack",
		"discord": "Discord",
		"json":    "Generic JSON",
		"auto":    "Auto-detect",
		"":        "Auto-detect",
		"weird":   "Auto-detect",
	}
	for in, want := range cases {
		if got := notifyFormatLabel(in); got != want {
			t.Errorf("notifyFormatLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRelTimeRFC3339_BadInput(t *testing.T) {
	if got := relTimeRFC3339("not-a-time"); got != "" {
		t.Errorf("relTimeRFC3339(bad) = %q, want empty", got)
	}
}

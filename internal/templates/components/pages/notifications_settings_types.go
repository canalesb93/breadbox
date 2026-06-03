//go:build !headless && !lite

package pages

import "strings"

// NotificationsSettingsProps drives the Settings → Notifications tab. The
// page manages a list of outbound notification channels (the multi-sink
// model) plus the global deep-link origin. Nothing here is secret beyond the
// channel URLs/tokens, which are masked for display.
type NotificationsSettingsProps struct {
	Channels []NotificationChannelView
	// PublicBaseURL is the manual deep-link origin override (empty = use
	// the auto-detected origin).
	PublicBaseURL string
	// DetectedBaseURL is the origin Breadbox auto-captured from the admin's
	// current request — the effective deep-link origin when no override is
	// set.
	DetectedBaseURL string
	FieldErrors     map[string]string
	FormError       string
	FormSuccess     string
	CSRFToken       string
}

// UsingDetectedBaseURL reports whether deep links fall back to the
// auto-detected origin (no manual override set).
func (p NotificationsSettingsProps) UsingDetectedBaseURL() bool {
	return strings.TrimSpace(p.PublicBaseURL) == ""
}

// EffectiveBaseURL is the origin notifications actually prepend to report
// deep links: the override when set, otherwise the auto-detected origin.
func (p NotificationsSettingsProps) EffectiveBaseURL() string {
	if v := strings.TrimSpace(p.PublicBaseURL); v != "" {
		return v
	}
	return strings.TrimSpace(p.DetectedBaseURL)
}

// OverridePlaceholder is the example shown in the override input — the
// detected origin when known, else a generic example.
func (p NotificationsSettingsProps) OverridePlaceholder() string {
	if v := strings.TrimSpace(p.DetectedBaseURL); v != "" {
		return v
	}
	return "https://breadbox.example.com"
}

// NotificationChannelView is one channel rendered in the list. Display fields
// (masked URL, format label, status line) are precomputed by the handler so
// the template stays declarative.
type NotificationChannelView struct {
	ID          string
	Name        string
	Format      string // raw value ("auto" | "ntfy" | …)
	FormatLabel string // human label ("Auto-detect", "ntfy", …)
	URLMasked   string
	MinPriority string
	Enabled     bool
	// Action endpoints, precomputed so the template passes them to
	// templ.SafeURL as variables (not string literals — keeps the
	// route-drift guard from treating these POST endpoints as GET hrefs).
	ToggleURL string
	DeleteURL string
	// Status line: HasStatus=false → never delivered. Otherwise StatusOK
	// drives the tone and StatusText is the message ("Delivered · 14:02" /
	// "Failed: HTTP 500").
	HasStatus  bool
	StatusOK   bool
	StatusText string
}

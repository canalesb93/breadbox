//go:build !headless && !lite

package pages

// NotificationsSettingsProps drives the Settings → Notifications tab. The
// page manages a list of outbound notification channels (the multi-sink
// model) plus the global public base URL used for deep links. Nothing here is
// secret beyond the channel URLs/tokens, which are masked for display.
type NotificationsSettingsProps struct {
	Channels      []NotificationChannelView
	PublicBaseURL string
	FieldErrors   map[string]string
	FormError     string
	FormSuccess   string
	CSRFToken     string
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

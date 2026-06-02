//go:build !headless && !lite

package pages

// NotificationsSettingsProps drives the Settings → Notifications tab. The
// page owns the outbound notification sink (webhook URL + wire format +
// public base URL) that workflow runs use to push reports. Nothing here is
// secret — every value renders in plaintext.
type NotificationsSettingsProps struct {
	Form        NotificationsSettingsFormFields
	FieldErrors map[string]string
	FormError   string
	FormSuccess string
	CSRFToken   string
}

// NotificationsSettingsFormFields mirrors the writable settings exposed by
// service.UpdateNotificationSettings.
type NotificationsSettingsFormFields struct {
	WebhookURL    string // http(s) sink; empty = notifications off
	Format        string // "auto" | "ntfy" | "slack" | "discord" | "json"
	PublicBaseURL string // absolute origin for deep links; empty = relative
	MinPriority   string // "info" | "warning" | "critical" delivery floor
}

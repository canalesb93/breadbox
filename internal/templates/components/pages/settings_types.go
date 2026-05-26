//go:build !headless && !lite

package pages

import "breadbox/internal/avatar"

// avatarPreviewExpr builds the Alpine `:src` expression for an
// avatar-style preview tile. The seed is a fixed literal supplied by
// the templ (alice/bob/casey/drew); the `style` variable comes from
// the parent x-data block bound to the picker's <select>.
//
// Tiles route through the server's /avatars/preview proxy rather
// than hitting api.dicebear.com directly: the proxy honors the
// avatar package's configured API base URL (test/staging overrides
// don't bypass it), keeps the admin's browser IP off DiceBear's
// access logs, and warms the same cache /avatars/{id} uses.
func avatarPreviewExpr(seed string) string {
	return "'/avatars/preview/" + seed + "?style=' + encodeURIComponent(style)"
}

// SettingsProps mirrors the field set the old settings.html read off the layout
// data map. Kept flat so admin/settings.go can copy fields one-to-one.
type SettingsProps struct {
	CSRFToken            string
	SyncIntervalMinutes  int
	SyncLogRetentionDays int
	SyncLogCount         int64
	Version              string
	GoVersion            string
	PostgresVersion      string
	Uptime               string
	ProviderCount        int
	HasEncryptionKey     bool
	OnboardingDismissed  bool
	NextSyncTime         string
	// ConfigSources maps config keys to their source: "env", "db", or "default".
	ConfigSources map[string]string

	// Update-availability fields, folded into the General tab when the
	// sidebar footer's version badge moved here.
	UpdateAvailable bool
	LatestVersion   string
	LatestURL       string

	// Avatar style — DiceBear slug used for auto-generated user identicons.
	// AvatarStyles drives the dropdown options.
	AvatarStyle  string
	AvatarStyles []avatar.StyleOption
}

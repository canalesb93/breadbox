package pages

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
	// EncryptionKeyFingerprint is the first 8 hex chars of sha256(key).
	// Empty when no key is configured. Shown in the Security card so admins
	// can sanity-check a host migration or `.env` restore against the value
	// they stashed during install.
	EncryptionKeyFingerprint string
	OnboardingDismissed      bool
	NextSyncTime             string
	// ConfigSources maps config keys to their source: "env", "db", or "default".
	ConfigSources map[string]string
}

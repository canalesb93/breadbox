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
	OnboardingDismissed  bool
	NextSyncTime         string
	// ConfigSources maps config keys to their source: "env", "db", or "default".
	ConfigSources map[string]string

	// Update-availability fields, folded into the General tab when the
	// sidebar footer's version badge moved here.
	UpdateAvailable bool
	LatestVersion   string
	LatestURL       string
}

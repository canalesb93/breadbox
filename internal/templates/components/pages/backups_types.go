package pages

// BackupsProps mirrors the data map that the old backups.html read off the
// layout's data map. The handler pre-resolves every backup entry into a
// flat view-model so the templ side stays free of pgtype/time/byte
// formatting helpers.
type BackupsProps struct {
	// CSRFToken used by every POST form on the page.
	CSRFToken string

	// Error, when set, replaces the page body with a single error alert
	// (mirrors the old `{{if .Error}}` short-circuit). Used when the
	// backup service is not available (pg_dump missing, etc).
	Error string

	// HasEncryptionKey toggles the inline "no key set" warning under the
	// always-on encryption-key reminder alert.
	HasEncryptionKey bool

	// Stats (top row).
	BackupCount   int
	TotalSize     string // pre-formatted via service.FormatBytes
	Schedule      string // "", "daily_2am", "daily_3am", "daily_4am", "weekly"
	RetentionDays int

	// Schedule form helpers.
	BackupDir string

	// Preflight reflects BackupService.Preflight — when OK is false the
	// page disables Create-Backup actions and surfaces the message.
	PreflightOK      bool
	PreflightMessage string

	// Backups is the pre-formatted list rendered in the file table. The
	// handler builds this from service.BackupInfo so the templ doesn't
	// have to call relativeTime / formatBytes funcMap helpers.
	Backups []BackupRow
}

// BackupRow is the view-model for one row in the backup list table.
// All time/byte fields are pre-rendered as strings.
type BackupRow struct {
	Filename       string
	SizeFormatted  string // "1.2 MB"
	CreatedAtRel   string // "2 hours ago"
	Trigger        string // "manual" / "scheduled" / other
	DownloadHref   string // /-/backups/{filename}/download
	RestoreAction  string // POST target /-/backups/{filename}/restore
	DeleteAction   string // POST target /-/backups/{filename}/delete
}

//go:build !headless

package pages

// DeveloperSettingsProps backs the Settings → Developer tab — the
// configuration surface for Developer Mode (the always-on-top bug/task
// reporter). See .claude/rules/settings.md for the section/row vocabulary.
type DeveloperSettingsProps struct {
	Form        DeveloperSettingsFormFields
	FieldErrors map[string]string
	FormError   string
	FormSuccess string
	CSRFToken   string
	// Reports is the recent-report history (most recent first).
	Reports []DevReportRow
	// RepoConfigured / TokenConfigured drive the "needs setup" warning
	// shown when developer mode is enabled but filing isn't wired up.
	RepoConfigured  bool
	TokenConfigured bool
}

// DeveloperSettingsFormFields holds the editable developer settings. The
// GitHub token is represented only by a masked hint — the plaintext never
// reaches the browser.
type DeveloperSettingsFormFields struct {
	Enabled            bool
	GithubRepo         string
	GithubTokenDisplay string // masked hint, "" when unset
	IssueLabel         string
}

// DevReportRow is one entry of the filed-report history list.
type DevReportRow struct {
	ShortID     string
	Type        string // bug | task
	Title       string
	PagePath    string
	Status      string // pending | open | failed
	IssueNumber int
	IssueURL    string
	CreatedBy   string
	CreatedAt   string // preformatted for display
	Error       string
}

// devReportTypeTone maps a report type to a daisy badge tone.
func devReportTypeTone(t string) string {
	if t == "task" {
		return "info"
	}
	return "error"
}

// devReportTypeLabel is the human label for a report type.
func devReportTypeLabel(t string) string {
	if t == "task" {
		return "Task"
	}
	return "Bug"
}

// devReportStatusTone maps a report status to a daisy badge tone.
func devReportStatusTone(s string) string {
	switch s {
	case "open":
		return "success"
	case "failed":
		return "error"
	case "saved":
		return "info"
	default:
		return "warning"
	}
}

// devReportStatusLabel is the human label for a report status.
func devReportStatusLabel(s string) string {
	switch s {
	case "open":
		return "Filed"
	case "failed":
		return "Failed"
	case "saved":
		return "Saved"
	default:
		return "Pending"
	}
}

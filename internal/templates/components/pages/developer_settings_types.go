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
}

// DeveloperSettingsFormFields holds the editable developer settings.
// UploadTokenMasked is the masked display of the stored artifact upload token
// (empty when none is set); the input itself is always rendered blank so a
// resubmit never writes the mask back.
type DeveloperSettingsFormFields struct {
	Enabled           bool
	GithubRepo        string
	UploadTokenMasked string
}

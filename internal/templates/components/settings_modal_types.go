package components

// SettingsModalTab identifiers — the canonical /settings/<tab> ids.
// (Named "Modal" for historical reasons; Settings is now a full page, not
// a modal — see pages.SettingsPage.) Handlers reference these via the
// pages.SettingsTab* aliases; the page rail maps each to its route.
//
// Minus Profile (merged into Account) and Household (promoted to its own
// top-level page). Security folds into System; the legacy Sync tab became
// General (schedule, retention, avatars).
const (
	SettingsModalTabAccount       = "account"
	SettingsModalTabGeneral       = "general"
	SettingsModalTabSystem        = "system"
	SettingsModalTabProviders     = "providers"
	SettingsModalTabWorkflows     = "workflows"
	SettingsModalTabNotifications = "notifications"
	SettingsModalTabMCP           = "mcp"
	SettingsModalTabAccess        = "api-keys"
	SettingsModalTabBackups       = "backups"
	SettingsModalTabDeveloper     = "developer"
	SettingsModalTabHelp          = "help"
)

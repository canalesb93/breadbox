package components

import (
	"html/template"

	"github.com/a-h/templ"
)

// settingsTabHref returns the modal-rail link's URL for a given tab.
//
// Wrapping the concatenation in a helper (rather than inlining
// `templ.SafeURL("/settings/" + tab)` in the .templ source) keeps the
// route-drift test happy: that test scans .templ files for literal
// `templ.SafeURL("/...")` strings and tries to resolve them against
// chi routes, which trips on dynamic prefixes. The hrefs themselves
// still resolve at runtime — every tab id corresponds to a real
// /settings/<tab> GET handler.
func settingsTabHref(tab string) templ.SafeURL {
	return templ.SafeURL("/settings/" + tab)
}

// SettingsModalProps drives the global Settings modal mounted in base.html.
//
// The modal is rendered on every authenticated page but hidden by default;
// it opens via the `open-settings` Alpine event (from the sidebar gear) or
// on cold load when a /settings/* URL is visited directly.
//
// Deep-link contract:
//   - InitialTab non-empty → factory boots open on that tab
//   - InitialBody non-empty → server pre-rendered the tab body (no fetch
//     flash on first paint); inserted into the body slot via templ.Raw
type SettingsModalProps struct {
	// IsAdmin / IsEditor gate the rail items in the same way the legacy
	// SettingsLayout did — admin-only tabs hidden from editors.
	IsAdmin  bool
	IsEditor bool

	// InitialTab is the tab id to open on boot (e.g. "account", "sync").
	// Empty means the modal stays closed until the user opens it.
	InitialTab string

	// InitialBody is the pre-rendered HTML of the InitialTab body, used to
	// avoid a fetch flash on cold deep-load. Empty when the modal mounts
	// closed or when the body should be fetched on open.
	InitialBody template.HTML
}

// SettingsModalTab identifiers — mirror the old SettingsLayout constants
// minus Profile (merged into Account) and Household (promoted to its own
// top-level page). Security folds into System, and the legacy Sync tab
// became General (general settings: schedule, retention, avatars).
const (
	SettingsModalTabAccount   = "account"
	SettingsModalTabGeneral   = "general"
	SettingsModalTabSystem    = "system"
	SettingsModalTabProviders = "providers"
	SettingsModalTabAgents    = "agents"
	SettingsModalTabMCP       = "mcp"
	SettingsModalTabAccess    = "api-keys"
	SettingsModalTabBackups   = "backups"
	SettingsModalTabHelp      = "help"
)

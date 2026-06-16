//go:build !headless

package pages

import "breadbox/internal/templates/components"

// SettingsTab* identifiers — thin alias layer over the
// components.SettingsModalTab* canonical ids. Handlers continue to
// import these from `pages` while the rail itself is owned by the
// SettingsModal component.
//
// Notable removals vs the legacy SettingsLayout shell:
//   - SettingsTabProfile — merged into SettingsTabAccount (single
//     identity page covering avatar/name/email + password + danger).
//   - SettingsTabHousehold — promoted to its own top-level /household
//     page, hung off the sidebar's System section.
const (
	SettingsTabAccount       = components.SettingsModalTabAccount
	SettingsTabGeneral       = components.SettingsModalTabGeneral
	SettingsTabSystem        = components.SettingsModalTabSystem
	SettingsTabProviders     = components.SettingsModalTabProviders
	SettingsTabAgents        = components.SettingsModalTabAgents
	SettingsTabNotifications = components.SettingsModalTabNotifications
	SettingsTabMCP           = components.SettingsModalTabMCP
	SettingsTabConnectors    = components.SettingsModalTabConnectors
	SettingsTabAccess        = components.SettingsModalTabAccess
	SettingsTabBackups       = components.SettingsModalTabBackups
	SettingsTabDeveloper     = components.SettingsModalTabDeveloper
	SettingsTabHelp          = components.SettingsModalTabHelp
)

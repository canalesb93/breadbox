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
	SettingsTabAccount   = components.SettingsModalTabAccount
	SettingsTabSync      = components.SettingsModalTabSync
	SettingsTabSecurity  = components.SettingsModalTabSecurity
	SettingsTabSystem    = components.SettingsModalTabSystem
	SettingsTabProviders = components.SettingsModalTabProviders
	SettingsTabAgents    = components.SettingsModalTabAgents
	SettingsTabMCP       = components.SettingsModalTabMCP
	SettingsTabAccess    = components.SettingsModalTabAccess
	SettingsTabBackups   = components.SettingsModalTabBackups
	SettingsTabHelp      = components.SettingsModalTabHelp
)

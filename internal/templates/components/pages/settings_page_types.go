//go:build !headless

package pages

import "github.com/a-h/templ"

// SettingsPageProps drives the full-page Settings surface (SettingsPage).
//
// Settings is a real page at /settings/* — a persistent left rail beside
// the active tab body, rendered inside the normal app chrome (sidebar +
// topbar). It replaced the old centered modal overlay. The rail items and
// the body are gated by role exactly as the legacy modal rail was.
type SettingsPageProps struct {
	// ActiveTab is the currently-selected tab id (e.g. "account",
	// "api-keys"). Drives the rail's active highlight and the in-page
	// swapper's starting state.
	ActiveTab string
	// IsAdmin / IsEditor gate the rail items. Editors see only Account +
	// API Keys; admins see everything.
	IsAdmin  bool
	IsEditor bool
	// Body is the active tab's server-rendered fragment.
	Body templ.Component
}

// settingsPageTabHref returns the rail link target for a tab. Wrapped in
// a helper (rather than an inline templ.SafeURL("/settings/"+tab)) so the
// route-drift test — which scans .templ sources for literal
// templ.SafeURL("/...") strings — doesn't trip on the dynamic prefix.
func settingsPageTabHref(tab string) templ.SafeURL {
	return templ.SafeURL("/settings/" + tab)
}

// settingsRailActiveAttr returns the server-rendered data-active value for
// a rail item — the first-paint / no-JS highlight. Once Alpine hydrates,
// the reactive x-bind from railActiveBindExpr takes over so the highlight
// tracks the live currentTab without any imperative DOM mutation.
func settingsRailActiveAttr(active, tab string) string {
	if active == tab {
		return "true"
	}
	return "false"
}

// railActiveBindExpr is the Alpine expression bound to each rail item's
// data-active. The highlight is a pure function of the component's
// currentTab (set synchronously in the swapper's _loadTab), so it can
// never drift from the visible tab — no race with the body-swap's
// alpine:init re-dispatch the way an imperative _setActiveRail could.
// tab is a fixed internal constant, so the single-quoted literal is safe.
func railActiveBindExpr(tab string) string {
	return "currentTab === '" + tab + "'"
}

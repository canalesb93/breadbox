//go:build !headless && !lite

package layout

import "strings"

// NavItem is a single sidebar link. Icon is a key resolved by components.Icon.
type NavItem struct {
	Title string
	Href  string // absolute, under /app
	Icon  string
}

// NavGroup is a labeled cluster of sidebar links.
type NavGroup struct {
	Label string
	Items []NavItem
}

// Nav mirrors the v2 SPA information architecture (web/src/lib/nav.ts): same groups,
// same order, same labels. Settings is a real route here (not a modal) per the v3 decision.
var Nav = []NavGroup{
	{
		Label: "Money",
		Items: []NavItem{
			{Title: "Home", Href: "/app/", Icon: "home"},
			{Title: "Transactions", Href: "/app/transactions", Icon: "transactions"},
			{Title: "Reports", Href: "/app/reports", Icon: "reports"},
		},
	},
	{
		Label: "Library",
		Items: []NavItem{
			{Title: "Accounts", Href: "/app/accounts", Icon: "accounts"},
			{Title: "Connections", Href: "/app/connections", Icon: "connections"},
			{Title: "Providers", Href: "/app/providers", Icon: "providers"},
			{Title: "Categories", Href: "/app/categories", Icon: "categories"},
			{Title: "Tags", Href: "/app/tags", Icon: "tags"},
			{Title: "Rules", Href: "/app/rules", Icon: "rules"},
		},
	},
	{
		Label: "System",
		Items: []NavItem{
			{Title: "Agents", Href: "/app/agents", Icon: "agents"},
			{Title: "API keys", Href: "/app/api-keys", Icon: "apikeys"},
			{Title: "Settings", Href: "/app/settings", Icon: "settings"},
		},
	},
}

// IsActive reports whether item should render as the current page given the request path.
func (i NavItem) IsActive(currentPath string) bool {
	if i.Href == "/app/" {
		return currentPath == "/app/" || currentPath == "/app"
	}
	return currentPath == i.Href || strings.HasPrefix(currentPath, i.Href+"/")
}

// BaseData is the minimal data the <html>/<head> shell needs.
type BaseData struct {
	Title      string
	ThemeClass string // "" or "dark"
}

// ShellData is everything the authenticated app shell (sidebar + topbar + main) needs.
type ShellData struct {
	Title       string
	CurrentPath string
	UserName    string
	ThemeClass  string
}

// Base derives the head-only data from the shell data.
func (d ShellData) Base() BaseData {
	return BaseData{Title: d.Title, ThemeClass: d.ThemeClass}
}

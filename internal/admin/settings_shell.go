//go:build !headless && !lite

package admin

import (
	"net/http"
	"net/url"

	"breadbox/internal/templates/components/pages"

	"github.com/a-h/templ"
)

// settingsFragmentHeader marks a request as a Settings modal swap — the
// modal's Alpine factory sends it on every fetch (GET tab loads and
// POST form submits). Handlers branch on it to return just the tab body
// instead of the full host page; the modal then follows any 303 with
// the same header so the redirect target also lands as a fragment.
const settingsFragmentHeader = "X-Settings-Fragment"

// Response headers carrying a one-shot flash inside a fragment response.
// The modal reads these and renders the message as an in-dialog toast,
// instead of relying on the layout-level flash partial (which renders
// inside <main>, behind the open modal).
const (
	settingsFlashTypeHeader    = "X-BB-Flash-Type"
	settingsFlashMessageHeader = "X-BB-Flash-Message"
)

// renderSettingsTab wraps the given tab body in a settings-host page and
// renders it inside base.html with the global SettingsModal pre-opened on
// the requested tab.
//
// Two render paths:
//
//  1. HTMX-style fragment swap (X-Settings-Fragment: 1 set by the modal
//     JS) — returns just the tab body, no layout. Used when the modal is
//     already open and the user switches tabs or opens the modal via the
//     sidebar gear.
//
//  2. Full page GET (direct /settings/:tab navigation) — renders the
//     full Settings page (rail + the active tab body) inside the normal
//     app chrome via base.html.
//
// `tab` is one of the components.SettingsModalTab* identifiers — used to
// highlight the active rail item and seed the in-page swapper.
func renderSettingsTab(
	tr *TemplateRenderer,
	w http.ResponseWriter,
	r *http.Request,
	data map[string]any,
	tab string,
	body templ.Component,
) {
	if r.Header.Get(settingsFragmentHeader) == "1" {
		if f, ok := data["Flash"].(*Flash); ok && f != nil && f.Message != "" {
			w.Header().Set(settingsFlashTypeHeader, f.Type)
			w.Header().Set(settingsFlashMessageHeader, url.QueryEscape(truncateFlash(f.Message)))
		}
		tr.RenderTemplFragment(w, r, body)
		return
	}

	// Role flags drive the rail's per-tab gating. Handlers that build
	// their data via BaseTemplateData already set them; the
	// buildSettingsProps tabs (General/System/Help) don't — so resolve
	// them here from the session, mirroring the base-layout enrichment in
	// TemplateRenderer.Render (same single-admin default), BEFORE we build
	// the rail. The matching SessionRole key also makes Render's own role
	// block a no-op so the two stay consistent.
	if _, ok := data["SessionRole"]; !ok && tr.sm != nil {
		role := tr.sm.GetString(r.Context(), sessionKeyAccountRole)
		if role == "" {
			role = RoleAdmin
		}
		data["SessionRole"] = role
		data["IsAdmin"] = role == RoleAdmin
		data["IsEditor"] = role == RoleAdmin || role == RoleEditor
		data["RoleDisplay"] = RoleDisplayName(role)
	}

	// Full navigation — render the Settings page (rail + active tab body)
	// inside base.html. The in-page swapper (settings_page.js) takes over
	// subsequent tab switches and in-tab saves.
	isAdmin, _ := data["IsAdmin"].(bool)
	isEditor, _ := data["IsEditor"].(bool)
	tr.RenderWithTempl(w, r, data, pages.SettingsPage(pages.SettingsPageProps{
		ActiveTab: tab,
		IsAdmin:   isAdmin,
		IsEditor:  isEditor,
		Body:      body,
	}))
}

// truncateFlash bounds flash messages we ship via response headers.
// Provider error paths (Plaid, Teller) can wrap upstream HTTP bodies
// with arbitrary length — keeping the header line under ~1KB plays
// nicely with the default `proxy_buffer_size` in nginx/caddy and
// avoids leaking pages of provider detail into proxy access logs.
const settingsFlashMaxLen = 512

func truncateFlash(s string) string {
	if len(s) <= settingsFlashMaxLen {
		return s
	}
	return s[:settingsFlashMaxLen-1] + "…"
}

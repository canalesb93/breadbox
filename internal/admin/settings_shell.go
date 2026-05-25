//go:build !headless && !lite

package admin

import (
	"bytes"
	"html/template"
	"net/http"

	"breadbox/internal/templates/components/pages"

	"github.com/a-h/templ"
)

// settingsFragmentHeader marks a request as a Settings modal swap — the
// modal's Alpine factory sends it on every fetch. Handlers branch on it
// to return just the tab body instead of the full host page.
const settingsFragmentHeader = "X-Settings-Fragment"

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
//  2. Full page GET (cold deep-link to /settings/:tab) — pre-renders the
//     tab body, stuffs it into the SettingsModal via the data map, and
//     renders an empty host main. The modal opens on first paint.
//
// `tab` is one of the components.SettingsModalTab* identifiers — used to
// tell the modal which row to highlight and where to push history.
func renderSettingsTab(
	tr *TemplateRenderer,
	w http.ResponseWriter,
	r *http.Request,
	data map[string]any,
	tab string,
	body templ.Component,
) {
	if r.Header.Get(settingsFragmentHeader) == "1" {
		tr.RenderTemplFragment(w, r, body)
		return
	}

	// Cold load — pre-render the body, then host an empty page that opens
	// the modal on top.
	var buf bytes.Buffer
	if err := body.Render(r.Context(), &buf); err != nil {
		http.Error(w, "templ render error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	data["SettingsInitialTab"] = tab
	data["SettingsInitialBody"] = template.HTML(buf.String())
	tr.RenderWithTempl(w, r, data, pages.SettingsHost())
}

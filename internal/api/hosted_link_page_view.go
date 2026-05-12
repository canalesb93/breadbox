package api

// Standalone hosted-link page (GET /link/{token}).
//
// The page is plain HTML with inline CSS+JS — no admin shell, no auth, no
// shared base template. It's served at the root of the router so end-users
// can open the URL without an admin session.
//
// The token itself is NOT rendered into the HTML server-side; the JS reads
// it back from window.location.pathname. That keeps the credential out of
// HTML caches and out of any screenshots / share-this-page surfaces.

import (
	"embed"
	"html/template"
	"net/http"
)

//go:embed all:hosted_link_assets
var hostedLinkFS embed.FS

// hostedLinkTemplate is parsed once at startup; the page has no
// per-request server-side data so a cached *template.Template is safe.
var hostedLinkTemplate = func() *template.Template {
	t, err := template.ParseFS(hostedLinkFS, "hosted_link_assets/page.html.tmpl")
	if err != nil {
		// Compile failures here are programmer errors — fail loud at boot.
		panic("parse hosted-link page template: " + err.Error())
	}
	return t
}()

// HostedLinkPageHandler serves GET /link/{token}.
//
// No bearer middleware — the page is HTML; the JS authenticates against
// /_link/{token}/* on its own. We don't even need the token here (it's in
// the URL the browser already has).
func HostedLinkPageHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// Don't cache — the page bundles a credential resolver and may be
		// served as a one-shot URL.
		w.Header().Set("Cache-Control", "no-store")
		if err := hostedLinkTemplate.Execute(w, nil); err != nil {
			// Headers may already be flushed; logging would need the app
			// logger threading through. Best-effort: write a plain 500.
			http.Error(w, "failed to render hosted-link page", http.StatusInternalServerError)
			return
		}
	}
}

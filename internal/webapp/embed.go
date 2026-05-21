// Package webapp is the v3 browser-native, server-rendered admin UI mounted at /app.
//
// It is a true multi-page app: every URL is a real document, the browser owns
// navigation/history/scroll/bfcache, and smoothness comes from native platform APIs
// (cross-document View Transitions, Speculation Rules) rather than a client router.
// It renders with templ, styles with Tailwind v4 (standalone CLI, no Node), and reuses
// the shared internal/service layer — no internal HTTP round-trips.
//
// It is a clean start: it does NOT import or reuse v1 admin (internal/admin templates,
// internal/templates) markup or CSS. It coexists with v1 admin (/) and the retiring
// React SPA (/v2) until cutover.
package webapp

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:static
var staticFS embed.FS

// staticHandler serves the embedded built assets (Tailwind output, JS) under /app/static/.
func staticHandler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	fileServer := http.StripPrefix("/app/static/", http.FileServer(http.FS(sub)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// app.css/app.js are rebuilt (not fingerprinted yet) → moderate cache.
		// Fingerprinted island bundles (Phase 4) get immutable caching.
		trimmed := strings.TrimPrefix(r.URL.Path, "/app/static")
		if strings.Contains(path.Base(trimmed), ".") && looksFingerprinted(trimmed) {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		fileServer.ServeHTTP(w, r)
	})
}

// looksFingerprinted reports whether a path carries a content hash (…-a1b2c3d4.js).
func looksFingerprinted(p string) bool {
	base := path.Base(p)
	dot := strings.LastIndex(base, ".")
	if dot < 0 {
		return false
	}
	stem := base[:dot]
	dash := strings.LastIndex(stem, "-")
	if dash < 0 || len(stem)-dash < 9 {
		return false
	}
	for _, c := range stem[dash+1:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

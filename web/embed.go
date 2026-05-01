// Package webui embeds the v2 SPA build (web/dist) and serves it under /v2/*.
//
// When the bundle hasn't been built (only web/dist/.gitkeep present),
// Handler() returns a stub explaining how to build it instead of 404ing.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"breadbox/internal/admin"
	mw "breadbox/internal/middleware"

	"github.com/alexedwards/scs/v2"
)

//go:embed all:dist
var distFS embed.FS

const stubHTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <title>Breadbox v2 — build missing</title>
    <style>
      body { font-family: ui-sans-serif, system-ui, sans-serif; padding: 48px; max-width: 640px; margin: 0 auto; color: #1c1917; }
      code { background: #f5f5f4; padding: 2px 6px; border-radius: 4px; font-size: 0.9em; }
      h1 { font-size: 22px; margin-bottom: 8px; }
      p { color: #57534e; line-height: 1.6; }
    </style>
  </head>
  <body>
    <h1>v2 SPA bundle missing</h1>
    <p>The Go binary was built without a v2 bundle. Run <code>make build</code> (which runs <code>bun run build</code> first) to produce a binary that serves the real SPA at <code>/v2/</code>.</p>
    <p>For development, run <code>cd web && bun run dev</code> in a second terminal — the Vite dev server (5173) proxies API calls to this Go server and supports HMR.</p>
    <p><a href="/">← Back to classic admin UI</a></p>
  </body>
</html>`

// Handler returns the v2 SPA static handler with SPA fallback. Mount under
// /v2/. Extension-less paths are rewritten to / so the client router resolves
// them. If the bundle is absent, every request gets a stub HTML page.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}

	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(stubHTML))
		})
	}

	fileServer := http.StripPrefix("/v2", http.FileServer(http.FS(sub)))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trimmed := strings.TrimPrefix(r.URL.Path, "/v2")
		// Extension-less paths under /v2/* are client routes — rewrite to /
		// so the file server returns index.html.
		if trimmed != "" && trimmed != "/" && path.Ext(trimmed) == "" {
			r.URL.Path = "/v2/"
		}
		// Long-cache hashed assets (Vite content-hashes filenames).
		if strings.HasPrefix(trimmed, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fileServer.ServeHTTP(w, r)
	})
}

// MeResponse is the shape returned by GET /web/v1/me. Internal to the SPA;
// no stability promise.
type MeResponse struct {
	AccountID string `json:"account_id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
}

// MeHandler returns the current admin from the session. RequireSessionJSON
// must run before this.
func MeHandler(sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mw.WriteJSON(w, http.StatusOK, MeResponse{
			AccountID: admin.SessionAccountID(sm, r),
			Username:  admin.SessionUsername(sm, r),
			Role:      admin.SessionRole(sm, r),
		})
	}
}

// RequireSessionJSON gates /web/v1/* endpoints behind a session cookie.
// Returns JSON 401 instead of redirecting to /login (which would break a
// JSON fetch — the SPA handles redirect on its own).
func RequireSessionJSON(sm *scs.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if admin.SessionAccountID(sm, r) == "" {
				mw.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Session required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

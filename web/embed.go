// Package webui embeds the v2 SPA build (web/dist) and serves it under /v2/*.
//
// Real builds (make build, Dockerfile, CI) run `bun run build` to populate
// web/dist/ before compiling the Go binary. Only web/dist/.gitkeep is
// committed — real bundle files are gitignored. When the bundle is absent
// (e.g. someone ran `go build ./...` without first building the SPA), the
// handler serves an inline stub explaining how to build it.
package webui

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"breadbox/internal/admin"

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
// /v2/. Any request for an unknown path that lacks a file extension returns
// index.html so the client router can resolve it. If the bundle is absent
// (no dist/index.html), every request gets a stub HTML page instead.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// Build-time guarantee: dist/ always exists (.gitkeep is committed).
		panic(err)
	}

	bundleBuilt := false
	if _, err := fs.Stat(sub, "index.html"); err == nil {
		bundleBuilt = true
	}

	if !bundleBuilt {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(stubHTML))
		})
	}

	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip the /v2 prefix so the file server sees paths relative to dist/.
		trimmed := strings.TrimPrefix(r.URL.Path, "/v2")
		if trimmed == "" {
			trimmed = "/"
		}

		// SPA fallback: paths without an extension that aren't found should
		// serve index.html so client routing can resolve them.
		if path.Ext(trimmed) == "" && trimmed != "/" {
			if _, err := fs.Stat(sub, strings.TrimPrefix(trimmed, "/")); err != nil {
				r2 := r.Clone(r.Context())
				r2.URL.Path = "/"
				fileServer.ServeHTTP(w, r2)
				return
			}
		}

		// Long-cache hashed assets (Vite includes content hashes in filenames).
		if strings.HasPrefix(trimmed, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}

		r2 := r.Clone(r.Context())
		r2.URL.Path = trimmed
		fileServer.ServeHTTP(w, r2)
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
		resp := MeResponse{
			AccountID: admin.SessionAccountID(sm, r),
			Username:  admin.SessionUsername(sm, r),
			Role:      admin.SessionRole(sm, r),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// RequireSessionJSON is chi middleware that gates /web/v1/* endpoints behind
// a session cookie. Unlike admin.RequireAuth, it returns a JSON 401 instead
// of redirecting to /login — the SPA handles redirect on its own, and an
// HTML redirect would break a JSON fetch.
func RequireSessionJSON(sm *scs.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if admin.SessionAccountID(sm, r) == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":{"code":"UNAUTHORIZED","message":"Session required"}}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

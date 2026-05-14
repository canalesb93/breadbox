//go:build headless && !lite

// Headless-build stub for the v2 SPA package. The real implementation
// (web/embed.go) is excluded by `//go:build !headless` because the SPA
// bundle (and its session-cookie surface) is part of the dashboard
// surface area we strip in headless builds. api/router.go still references
// the three exported entry points unconditionally during compilation, so
// the stubs below keep the package importable.

package webui

import (
	"net/http"

	"breadbox/internal/db"

	"github.com/alexedwards/scs/v2"
)

// Handler returns a 410 Gone for every request. The real Handler embeds the
// SPA bundle and serves /v2/*.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "v2 SPA disabled in this build", http.StatusGone)
	})
}

// MeHandler is the SPA-only /web/v1/me endpoint. Stub returns 410.
func MeHandler(_ *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "v2 SPA disabled in this build", http.StatusGone)
	}
}

// RequireSessionJSON wraps /web/v1/* handlers. Stub always 410s.
func RequireSessionJSON(_ *scs.SessionManager) func(http.Handler) http.Handler {
	return func(_ http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "v2 SPA disabled in this build", http.StatusGone)
		})
	}
}

// RequireSameOrigin is the CSRF middleware for /web/v1/* writes. Stub is a
// no-op pass-through (the routes it would guard are themselves stubbed to
// 410 elsewhere, so this never lets a real request through).
func RequireSameOrigin() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler { return next }
}

// LoginHandler authenticates the v2 SPA's session. Stub returns 410.
func LoginHandler(_ *scs.SessionManager, _ *db.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "v2 SPA disabled in this build", http.StatusGone)
	}
}

// LogoutHandler destroys the v2 SPA's session. Stub returns 410.
func LogoutHandler(_ *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "v2 SPA disabled in this build", http.StatusGone)
	}
}

// ChangePasswordHandler updates the session account password. Stub returns 410.
func ChangePasswordHandler(_ *scs.SessionManager, _ *db.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "v2 SPA disabled in this build", http.StatusGone)
	}
}

//go:build !headless && !lite

package webapp

import (
	"net/http"
	"net/url"
	"strings"

	"breadbox/internal/admin"
	mw "breadbox/internal/middleware"
)

// requireAuth gates authenticated pages. Unauthenticated requests get a real
// server-side 303 redirect to /app/login, preserving the intended destination in
// ?next= so login can bounce the user back. No JS, no 401 trap.
func (h *Handler) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if admin.SessionAccountID(h.sm, r) == "" {
			dest := r.URL.Path
			if r.URL.RawQuery != "" {
				dest += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, "/app/login?next="+url.QueryEscape(dest), http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireSameOrigin is the CSRF guard for unsafe methods: SameSite=Lax cookie plus an
// Origin/Referer host check — the same strategy /web/v1 uses. Native same-origin form
// POSTs pass by construction.
func (h *Handler) requireSameOrigin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if mw.IsUnsafeMethod(r.Method) && !mw.SameOrigin(r) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}
}

// safeNext returns dest if it is a same-site /app path, else /app/. Prevents open-redirect.
func safeNext(dest string) string {
	if dest == "" {
		return "/app/"
	}
	if !strings.HasPrefix(dest, "/app") || strings.HasPrefix(dest, "//") {
		return "/app/"
	}
	return dest
}

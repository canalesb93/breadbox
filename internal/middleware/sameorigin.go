//go:build !lite

package middleware

import (
	"net/http"
	"net/url"
)

// IsUnsafeMethod reports whether an HTTP method can change server state and
// therefore needs CSRF protection. GET/HEAD/OPTIONS are safe.
func IsUnsafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

// SameOrigin reports whether the request's Origin (or Referer fallback) host
// matches the request host — the SameSite=Lax CSRF check shared by the
// /web/v1/* routes and session-authed /api/v1/* writes.
//
// A request with neither header is treated as cross-origin: modern browsers
// always send one on a state-changing request, so a caller that sends neither
// should be using API-key auth, not a cookie.
func SameOrigin(r *http.Request) bool {
	got := r.Header.Get("Origin")
	if got == "" {
		got = r.Header.Get("Referer")
	}
	if got == "" {
		return false
	}
	u, err := url.Parse(got)
	if err != nil {
		return false
	}
	return u.Host == r.Host
}

//go:build !lite

package middleware

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"strings"
)

// SecureCookie policy modes (values of Config.SecureCookies).
const (
	secureCookieAlways = "always"
	secureCookieNever  = "never"
	// anything else (including "auto" and "") follows the request scheme.
)

var errNotHijacker = errors.New("securecookie: underlying ResponseWriter is not an http.Hijacker")

// RequestIsHTTPS reports whether the browser-facing connection is HTTPS —
// either a direct TLS connection or a request forwarded by a trusted reverse
// proxy that set X-Forwarded-Proto=https (Caddy and Cloudflare do this by
// default). Mirrors the scheme detection already used in apikey.go and the
// admin requestBaseURL helpers.
func RequestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// SecureSessionCookie returns middleware that stamps the Secure attribute onto
// the Set-Cookie header for the cookie named cookieName, according to mode:
//
//   - "always" — always mark Secure.
//   - "never"  — never mark Secure.
//   - anything else ("auto", the default) — mark Secure only when the request
//     is actually HTTPS (see RequestIsHTTPS).
//
// Why a header rewrite instead of scs's own Cookie.Secure: that flag is
// process-global (set once on the SessionManager), so toggling it per request
// would race across concurrent requests. Instead we leave scs's cookie
// non-Secure and append "; Secure" here, per request. The payoff: a plain-HTTP
// LAN/localhost install can store the session cookie and log in, while any
// HTTPS (or HTTPS-proxied) deployment keeps the Secure hardening automatically.
//
// Wire this OUTSIDE scs's LoadAndSave (add it with an earlier r.Use) so the
// wrapped ResponseWriter is the one scs writes its Set-Cookie into.
func SecureSessionCookie(mode, cookieName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var secure bool
			switch mode {
			case secureCookieAlways:
				secure = true
			case secureCookieNever:
				secure = false
			default:
				secure = RequestIsHTTPS(r)
			}
			// When we won't add anything, stay fully transparent — no wrapper,
			// so streaming/hijacking paths are byte-for-byte unchanged.
			if !secure {
				next.ServeHTTP(w, r)
				return
			}
			sw := &secureCookieWriter{ResponseWriter: w, cookieName: cookieName}
			next.ServeHTTP(sw, r)
			// Handle responses that finish without ever writing a header or
			// body (Set-Cookie set, empty 200): the header map is still
			// mutable here, so the fixup still lands.
			sw.fixup()
		})
	}
}

// secureCookieWriter appends "; Secure" to the session Set-Cookie header the
// first time the response is committed (WriteHeader / Write / Flush), and
// preserves the Flusher and Hijacker interfaces so SSE streams and websocket
// upgrades keep working.
type secureCookieWriter struct {
	http.ResponseWriter
	cookieName string
	fixed      bool
}

func (w *secureCookieWriter) fixup() {
	if w.fixed {
		return
	}
	w.fixed = true
	appendSecureAttr(w.Header(), w.cookieName)
}

func (w *secureCookieWriter) WriteHeader(code int) {
	w.fixup()
	w.ResponseWriter.WriteHeader(code)
}

func (w *secureCookieWriter) Write(b []byte) (int, error) {
	w.fixup()
	return w.ResponseWriter.Write(b)
}

func (w *secureCookieWriter) Flush() {
	w.fixup()
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *secureCookieWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, errNotHijacker
}

// Unwrap lets http.ResponseController reach the underlying writer (Go 1.20+).
func (w *secureCookieWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// appendSecureAttr appends "; Secure" to the Set-Cookie entry for cookieName
// when it isn't already present. Other cookies are left untouched.
func appendSecureAttr(h http.Header, cookieName string) {
	cookies := h["Set-Cookie"]
	prefix := cookieName + "="
	for i, c := range cookies {
		if !strings.HasPrefix(c, prefix) || hasSecureAttr(c) {
			continue
		}
		cookies[i] = c + "; Secure"
	}
}

func hasSecureAttr(cookie string) bool {
	for _, part := range strings.Split(cookie, ";") {
		if strings.EqualFold(strings.TrimSpace(part), "Secure") {
			return true
		}
	}
	return false
}

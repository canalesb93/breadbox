//go:build !lite

package middleware

import (
	"bufio"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sessionCookieHandler sets a cookie named `name` then commits the response,
// mimicking what scs does on login.
func sessionCookieHandler(name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: name, Value: "tok", Path: "/", HttpOnly: true})
		w.WriteHeader(http.StatusSeeOther)
	})
}

func runAndGetSetCookie(mode, cookieName string, r *http.Request) string {
	rec := httptest.NewRecorder()
	h := SecureSessionCookie(mode, cookieName)(sessionCookieHandler(cookieName))
	h.ServeHTTP(rec, r)
	return rec.Header().Get("Set-Cookie")
}

func httpsReq() *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/login", nil)
	r.TLS = &tls.ConnectionState{}
	return r
}

func fwdHTTPSReq() *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/login", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	return r
}

func plainReq() *http.Request {
	return httptest.NewRequest(http.MethodPost, "/login", nil)
}

func TestSecureSessionCookie_AutoFollowsScheme(t *testing.T) {
	cases := []struct {
		name    string
		req     *http.Request
		wantSec bool
	}{
		{"plain http → not secure", plainReq(), false},
		{"direct TLS → secure", httpsReq(), true},
		{"x-forwarded-proto https → secure", fwdHTTPSReq(), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runAndGetSetCookie("auto", "session", tc.req)
			if has := hasSecureAttr(got); has != tc.wantSec {
				t.Fatalf("Set-Cookie=%q: Secure=%v, want %v", got, has, tc.wantSec)
			}
		})
	}
}

func TestSecureSessionCookie_AlwaysAndNever(t *testing.T) {
	if got := runAndGetSetCookie("always", "session", plainReq()); !hasSecureAttr(got) {
		t.Fatalf("mode=always over http: want Secure, got %q", got)
	}
	if got := runAndGetSetCookie("never", "session", httpsReq()); hasSecureAttr(got) {
		t.Fatalf("mode=never over https: want no Secure, got %q", got)
	}
}

func TestSecureSessionCookie_OnlyTargetsNamedCookie(t *testing.T) {
	r := httpsReq()
	rec := httptest.NewRecorder()
	h := SecureSessionCookie("auto", "session")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "other", Value: "x", Path: "/"})
		w.WriteHeader(http.StatusOK)
	}))
	h.ServeHTTP(rec, r)
	for _, c := range rec.Header()["Set-Cookie"] {
		if strings.HasPrefix(c, "other=") && hasSecureAttr(c) {
			t.Fatalf("unrelated cookie should be untouched: %q", c)
		}
	}
}

func TestSecureSessionCookie_NoDoubleSecure(t *testing.T) {
	r := httpsReq()
	rec := httptest.NewRecorder()
	h := SecureSessionCookie("always", "session")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Cookie already Secure (e.g. scs configured Secure=true).
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "tok", Path: "/", Secure: true})
		w.WriteHeader(http.StatusOK)
	}))
	h.ServeHTTP(rec, r)
	got := rec.Header().Get("Set-Cookie")
	if n := strings.Count(strings.ToLower(got), "secure"); n != 1 {
		t.Fatalf("expected exactly one Secure attr, got %d in %q", n, got)
	}
}

// flushHijackRW is an underlying writer that records Flush/Hijack delegation.
type flushHijackRW struct {
	http.ResponseWriter
	flushed  bool
	hijacked bool
}

func (f *flushHijackRW) Flush() { f.flushed = true }
func (f *flushHijackRW) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	f.hijacked = true
	return nil, nil, nil
}

func TestSecureSessionCookie_PreservesFlusherAndHijacker(t *testing.T) {
	base := &flushHijackRW{ResponseWriter: httptest.NewRecorder()}
	var seen http.ResponseWriter
	h := SecureSessionCookie("always", "session")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		seen = w
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		if hj, ok := w.(http.Hijacker); ok {
			_, _, _ = hj.Hijack()
		}
	}))
	h.ServeHTTP(base, httptest.NewRequest(http.MethodGet, "/", nil))

	if _, ok := seen.(http.Flusher); !ok {
		t.Fatal("wrapped writer should expose http.Flusher")
	}
	if _, ok := seen.(http.Hijacker); !ok {
		t.Fatal("wrapped writer should expose http.Hijacker")
	}
	if !base.flushed {
		t.Fatal("Flush was not delegated to the underlying writer")
	}
	if !base.hijacked {
		t.Fatal("Hijack was not delegated to the underlying writer")
	}
}

func TestSecureSessionCookie_NeverModeIsTransparent(t *testing.T) {
	// In "never" mode the middleware must not wrap the writer at all, so a
	// plain ResponseRecorder (no Flusher/Hijacker addition) flows through.
	base := httptest.NewRecorder()
	var seen http.ResponseWriter
	h := SecureSessionCookie("never", "session")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		seen = w
		w.WriteHeader(http.StatusOK)
	}))
	h.ServeHTTP(base, plainReq())
	if _, wrapped := seen.(*secureCookieWriter); wrapped {
		t.Fatal("never mode should not wrap the ResponseWriter")
	}
}

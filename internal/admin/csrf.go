package admin

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"

	"github.com/alexedwards/scs/v2"
)

const csrfSessionKey = "csrf_token"

// GenerateCSRFToken creates a new CSRF token and stores it in the session.
func GenerateCSRFToken(ctx context.Context, sm *scs.SessionManager) string {
	token := sm.GetString(ctx, csrfSessionKey)
	if token != "" {
		return token
	}
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token = base64.RawURLEncoding.EncodeToString(b)
	sm.Put(ctx, csrfSessionKey, token)
	return token
}

// GetCSRFToken returns the CSRF token from the request context (set by CSRFMiddleware).
func GetCSRFToken(r *http.Request) string {
	if v, ok := r.Context().Value(csrfContextKey).(string); ok {
		return v
	}
	return ""
}

type contextKey string

const csrfContextKey contextKey = "csrf_token"

// CSRFMiddleware validates CSRF tokens on POST requests and injects the token
// into the request context for use by handlers and templates.
func CSRFMiddleware(sm *scs.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Generate or retrieve token and add to context.
			token := GenerateCSRFToken(r.Context(), sm)
			ctx := context.WithValue(r.Context(), csrfContextKey, token)
			r = r.WithContext(ctx)

			// On state-changing methods, validate the submitted token.
			if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete || r.Method == http.MethodPatch {
				submitted := r.FormValue("_csrf")
				if submitted == "" {
					submitted = r.Header.Get("X-CSRF-Token")
				}

				if subtle.ConstantTimeCompare([]byte(submitted), []byte(token)) != 1 {
					http.Error(w, "Invalid CSRF token", http.StatusForbidden)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

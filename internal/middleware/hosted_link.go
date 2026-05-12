package middleware

import (
	"context"
	"errors"
	"net/http"
	"time"

	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// hostedLinkCtxKey scopes the session value stored on the request context
// by HostedLinkBearer so handlers can pull it back out via HostedLinkToken.
type hostedLinkCtxKey struct{}

// HostedLinkToken returns the session resolved by the bearer middleware,
// or (zero, false) if no session is attached. Handlers behind the bearer
// middleware should always see ok=true; the boolean exists for defense
// against accidental mount mistakes.
func HostedLinkToken(r *http.Request) (service.HostedLinkSession, bool) {
	s, ok := r.Context().Value(hostedLinkCtxKey{}).(service.HostedLinkSession)
	return s, ok
}

// withHostedLinkSession attaches a session to a context. Exposed only to
// this package; handlers read it via HostedLinkToken.
func withHostedLinkSession(ctx context.Context, s service.HostedLinkSession) context.Context {
	return context.WithValue(ctx, hostedLinkCtxKey{}, s)
}

// HostedLinkBearer returns middleware that resolves the `{token}` chi URL
// parameter against the hosted_link_sessions table and gates the request
// based on the session's freshness:
//
//   - Unknown token → 401 INVALID_TOKEN
//   - Expired (status=="expired" or past expires_at) → 410 EXPIRED
//   - Completed → 410 CONSUMED
//   - Failed / any other terminal state → 410 GONE
//   - Pending / active and not expired → attach to context, call next
//
// The token in the URL path is the credential — no API key, no admin
// session, no rate limiter run by this middleware. Mount it on the
// internal `/_link/{token}/*` subtree only.
//
// Scope-pinning (provider / action matching) lives in each handler so the
// middleware can stay generic across the five page-internal endpoints.
func HostedLinkBearer(svc *service.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := chi.URLParam(r, "token")
			if token == "" {
				WriteError(w, http.StatusUnauthorized, "INVALID_TOKEN", "Hosted-link token is required")
				return
			}

			session, err := svc.ResolveHostedLinkSessionByToken(r.Context(), token)
			if err != nil {
				if errors.Is(err, service.ErrNotFound) {
					WriteError(w, http.StatusUnauthorized, "INVALID_TOKEN", "Hosted-link token is invalid")
					return
				}
				WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to resolve hosted-link token")
				return
			}

			// Status==expired comes from the service's in-memory expiry
			// override (GetHostedLinkSession / ResolveHostedLinkSessionByToken
			// both apply it). Belt-and-suspenders the live expires_at check
			// too in case a future caller bypasses that override.
			switch session.Status {
			case service.HostedLinkStatusPending, service.HostedLinkStatusActive:
				if !session.ExpiresAt.IsZero() && session.ExpiresAt.Before(time.Now()) {
					WriteError(w, http.StatusGone, "EXPIRED", "Hosted-link session has expired")
					return
				}
			case service.HostedLinkStatusExpired:
				WriteError(w, http.StatusGone, "EXPIRED", "Hosted-link session has expired")
				return
			case service.HostedLinkStatusCompleted:
				WriteError(w, http.StatusGone, "CONSUMED", "Hosted-link session has already been used")
				return
			default:
				// failed and any future terminal status fall through here.
				WriteError(w, http.StatusGone, "GONE", "Hosted-link session is no longer usable")
				return
			}

			ctx := withHostedLinkSession(r.Context(), session)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

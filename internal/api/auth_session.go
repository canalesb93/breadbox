//go:build !lite

package api

import (
	"net/http"

	"breadbox/internal/admin"
	"breadbox/internal/db"
	mw "breadbox/internal/middleware"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgtype"
)

// sessionOrAPIKeyAuth gates /api/v1/* behind either a v2 dashboard session
// cookie OR the existing API-key / OAuth-Bearer credentials.
//
// A valid session is translated into a synthetic db.ApiKey so every
// downstream scope check (RequireWriteScope) and actor-attribution path keeps
// working unchanged — there is no second auth code path to maintain. The
// synthetic key's scope is derived from the account's role: admin/editor map
// to full_access, viewer maps to read_only.
//
// sm may be nil (headless builds, or --no-dashboard) — the session path is
// then skipped entirely and this behaves exactly like middleware.APIKeyAuth.
func sessionOrAPIKeyAuth(
	svc *service.Service,
	sm *scs.SessionManager,
) func(http.Handler) http.Handler {
	apiKeyAuth := mw.APIKeyAuth(svc)
	return func(next http.Handler) http.Handler {
		// The non-session path: the existing API-key/Bearer chain wrapping next.
		apiKeyChain := apiKeyAuth(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if sm != nil {
				if accountID := admin.SessionAccountID(sm, r); accountID != "" {
					// /api/v1/* historically had no cookie auth, so a
					// session-authed unsafe method introduces CSRF surface —
					// guard it with the same SameSite=Lax + Origin check the
					// /web/v1/* routes use. Pure API-key clients send no cookie
					// and never reach this branch.
					if mw.IsUnsafeMethod(r.Method) && !mw.SameOrigin(r) {
						mw.WriteError(
							w,
							http.StatusForbidden,
							"ORIGIN_MISMATCH",
							"Cross-origin request rejected",
						)
						return
					}
					key := syntheticSessionKey(sm, r, accountID)
					ctx := service.ContextWithAPIKey(r.Context(), key)
					ctx = mw.SetAPIKey(ctx, key)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
			apiKeyChain.ServeHTTP(w, r)
		})
	}
}

// syntheticSessionKey builds the in-memory db.ApiKey that represents a
// cookie-authenticated v2 dashboard user.
func syntheticSessionKey(
	sm *scs.SessionManager,
	r *http.Request,
	accountID string,
) *db.ApiKey {
	name := admin.SessionUsername(sm, r)
	// Prefer the linked household-member users.id for attribution, matching
	// admin.ActorFromSession; fall back to the auth_accounts.id.
	actorID := admin.SessionUserID(sm, r)
	if actorID == "" {
		actorID = accountID
	}
	var id pgtype.UUID
	_ = id.Scan(actorID)
	return &db.ApiKey{
		ID:        id,
		Name:      name,
		Scope:     roleToScope(admin.SessionRole(sm, r)),
		ActorType: "user",
		ActorName: pgconv.Text(name),
	}
}

// roleToScope maps an auth_accounts.role to an API-key scope. admin and
// editor can write (full_access); viewer is read-only. Mirrors the
// admin.IsEditor semantics.
func roleToScope(role string) string {
	if role == admin.RoleAdmin || role == admin.RoleEditor {
		return "full_access"
	}
	return "read_only"
}

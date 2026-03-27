package admin

import (
	"context"
	"log/slog"
	"net/http"

	"breadbox/internal/db"

	"github.com/alexedwards/scs/v2"
)

const navBadgesKey contextKey = "navBadges"

// NavBadges holds notification counts displayed in the sidebar navigation.
type NavBadges struct {
	PendingReviews       int64
	ConnectionsAttention int64
	UnreadReports        int64
}

// NavBadgesMiddleware fetches sidebar notification badge counts and stores them
// in the request context. The Render method auto-injects these into template data.
func NavBadgesMiddleware(queries *db.Queries, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			badges := NavBadges{}

			if pending, err := queries.CountPendingReviews(ctx); err == nil {
				badges.PendingReviews = pending
			} else {
				logger.Debug("nav badges: count pending reviews", "error", err)
			}

			if attn, err := queries.CountConnectionsNeedingAttention(ctx); err == nil {
				badges.ConnectionsAttention = attn
			} else {
				logger.Debug("nav badges: count connections needing attention", "error", err)
			}

			if unread, err := queries.CountUnreadAgentReports(ctx); err == nil {
				badges.UnreadReports = unread
			} else {
				logger.Debug("nav badges: count unread agent reports", "error", err)
			}

			ctx = context.WithValue(ctx, navBadgesKey, badges)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// getNavBadges retrieves NavBadges from the request context. Returns zero-value if absent.
func getNavBadges(ctx context.Context) NavBadges {
	if badges, ok := ctx.Value(navBadgesKey).(NavBadges); ok {
		return badges
	}
	return NavBadges{}
}

// RequireAuth is chi middleware that checks for an authenticated admin session.
// If the session does not contain an admin_id, it redirects to /login.
func RequireAuth(sm *scs.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			adminID := sm.GetString(r.Context(), sessionKeyAdminID)
			if adminID == "" {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SetupDetection is chi middleware that redirects to the admin creation page
// if no admin accounts exist in the database.
func SetupDetection(queries *db.Queries) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count, err := queries.CountAdminAccounts(r.Context())
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			if count == 0 {
				// Allow access to setup routes.
				if isSetupRoute(r.URL.Path) {
					next.ServeHTTP(w, r)
					return
				}
				http.Redirect(w, r, "/setup", http.StatusSeeOther)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// isSetupRoute checks whether the path is a setup or setup API route.
func isSetupRoute(path string) bool {
	return path == "/setup" || path == "/-/setup"
}

package admin

import (
	"context"
	"log/slog"
	"net/http"

	"breadbox/internal/appconfig"
	"breadbox/internal/db"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgtype"
)

const navBadgesKey contextKey = "navBadges"

// NavBadges holds notification counts displayed in the sidebar navigation.
type NavBadges struct {
	// PendingReviews is the count of transactions currently tagged
	// "needs-review". Displayed next to the Tags nav link.
	PendingReviews       int64
	ConnectionsAttention int64
	UnreadReports        int64
	ShowGettingStarted   bool
}

// NavBadgesMiddleware fetches sidebar notification badge counts and stores them
// in the request context. The Render method auto-injects these into template data.
func NavBadgesMiddleware(queries *db.Queries, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			badges := NavBadges{}

			// Count transactions currently tagged "needs-review" — this is
			// the review backlog surfaced in the Tags nav badge.
			if pending, err := countPendingReviewsFromTags(ctx, queries); err == nil {
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

			// Check if getting started guide should show in nav.
			badges.ShowGettingStarted = !appconfig.Bool(ctx, queries, "onboarding_dismissed", false)

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

// countPendingReviewsFromTags returns the number of transactions currently
// tagged "needs-review" (excluding matched dependent transactions).
func countPendingReviewsFromTags(ctx context.Context, queries *db.Queries) (int64, error) {
	return queries.CountTransactionsWithTagSlug(ctx, "needs-review")
}

// pendingReviewsCount returns the count of transactions currently tagged
// "needs-review". Shared across agent_wizard / agents_page / mcp_guide.
func pendingReviewsCount(ctx context.Context, svc pendingReviewCountSource) (int64, error) {
	return svc.CountTransactionsTag(ctx, "needs-review")
}

// pendingReviewCountSource is satisfied by *service.Service via its
// CountTransactionsTag helper. Narrow interface so middleware.go doesn't pull
// service into the admin package indirectly.
type pendingReviewCountSource interface {
	CountTransactionsTag(ctx context.Context, slug string) (int64, error)
}

// RequireAuth is chi middleware that checks for an authenticated session.
// If the session does not contain an account_id, it redirects to /login.
// It also refreshes the session role from the database on every request,
// so that role changes made by an admin take effect immediately without
// requiring the user to log out and back in.
func RequireAuth(sm *scs.SessionManager, args ...interface{}) func(http.Handler) http.Handler {
	// Extract optional *db.Queries from variadic args for backward compatibility.
	var queries *db.Queries
	for _, arg := range args {
		if q, ok := arg.(*db.Queries); ok {
			queries = q
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			accountID := sm.GetString(ctx, sessionKeyAccountID)
			if accountID == "" {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}

			// Refresh role from DB to pick up admin-initiated role changes.
			if queries != nil {
				var uuid pgtype.UUID
				if err := uuid.Scan(accountID); err == nil {
					if account, err := queries.GetAuthAccountByID(ctx, uuid); err == nil {
						sessionRole := sm.GetString(ctx, sessionKeyAccountRole)
						if sessionRole != account.Role {
							sm.Put(ctx, sessionKeyAccountRole, account.Role)
						}
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAdmin is chi middleware that blocks non-admin users.
// Must be used after RequireAuth. Returns 403 for non-admins.
func RequireAdmin(sm *scs.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !IsAdmin(sm, r) {
				http.Error(w, "Admin access required", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireEditor is chi middleware that blocks viewers from edit operations.
// Must be used after RequireAuth. Allows admin and editor, blocks viewer.
func RequireEditor(sm *scs.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !IsEditor(sm, r) {
				http.Error(w, "Editor access required", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SetupDetection is chi middleware that redirects to the admin creation page
// if no auth accounts exist in the database.
func SetupDetection(queries *db.Queries) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count, err := queries.CountAuthAccounts(r.Context())
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

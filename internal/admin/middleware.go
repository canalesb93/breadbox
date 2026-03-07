package admin

import (
	"net/http"

	"breadbox/internal/db"

	"github.com/alexedwards/scs/v2"
)

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
				http.Redirect(w, r, "/admin/setup", http.StatusSeeOther)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// isSetupRoute checks whether the path is a setup or setup API route.
func isSetupRoute(path string) bool {
	return path == "/admin/setup" || path == "/admin/api/setup"
}

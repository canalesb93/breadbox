package admin

import (
	"net/http"

	"breadbox/internal/db"

	"github.com/alexedwards/scs/v2"
	"golang.org/x/crypto/bcrypt"
)

const sessionKeyAdminID = "admin_id"

// LoginHandler returns an http.HandlerFunc that handles GET and POST /login.
func LoginHandler(sm *scs.SessionManager, queries *db.Queries, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			data := map[string]any{
				"PageTitle": "Sign In",
				"CSRFToken": GenerateCSRFToken(r.Context(), sm),
				"Flash":     GetFlash(r.Context(), sm),
				"Username":  "",
				"Error":     "",
			}
			tr.Render(w, r, "login.html", data)
			return
		}

		// POST: validate credentials.
		username := r.FormValue("username")
		password := r.FormValue("password")

		if username == "" || password == "" {
			data := map[string]any{
				"PageTitle": "Sign In",
				"CSRFToken": GenerateCSRFToken(r.Context(), sm),
				"Username":  username,
				"Error":     "Invalid username or password",
			}
			tr.Render(w, r, "login.html", data)
			return
		}

		admin, err := queries.GetAdminAccountByUsername(r.Context(), username)
		if err != nil {
			data := map[string]any{
				"PageTitle": "Sign In",
				"CSRFToken": GenerateCSRFToken(r.Context(), sm),
				"Username":  username,
				"Error":     "Invalid username or password",
			}
			tr.Render(w, r, "login.html", data)
			return
		}

		if err := bcrypt.CompareHashAndPassword(admin.HashedPassword, []byte(password)); err != nil {
			data := map[string]any{
				"PageTitle": "Sign In",
				"CSRFToken": GenerateCSRFToken(r.Context(), sm),
				"Username":  username,
				"Error":     "Invalid username or password",
			}
			tr.Render(w, r, "login.html", data)
			return
		}

		// Renew session token to prevent session fixation.
		if err := sm.RenewToken(r.Context()); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		sm.Put(r.Context(), sessionKeyAdminID, formatUUID(admin.ID))
		http.Redirect(w, r, "/admin/", http.StatusSeeOther)
	}
}

// LogoutHandler returns an http.HandlerFunc that handles POST /logout.
func LogoutHandler(sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		if err := sm.Destroy(r.Context()); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

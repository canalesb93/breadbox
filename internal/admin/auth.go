package admin

import (
	"net/http"

	"breadbox/internal/db"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionKeyAccountID       = "account_id"       // auth_accounts.id
	sessionKeyAccountUsername = "account_username"  // auth_accounts.username
	sessionKeyAccountRole     = "account_role"      // "admin", "editor", or "viewer"
	sessionKeyUserID          = "user_id"           // linked family member UUID (NULL-linked admins have "")
)

// Role constants.
const (
	RoleAdmin  = "admin"
	RoleEditor = "editor"
	RoleViewer = "viewer"
)

// ValidRoles is the set of valid role values.
var ValidRoles = map[string]bool{
	RoleAdmin:  true,
	RoleEditor: true,
	RoleViewer: true,
}

// dummyHash is a pre-computed bcrypt hash used for constant-time login responses.
// When a username is not found, we still run bcrypt.CompareHashAndPassword against
// this dummy hash so that the response time is indistinguishable from a valid-user
// wrong-password attempt. This prevents username enumeration via timing side channels.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), 12)

// LoginHandler returns an http.HandlerFunc that handles GET and POST /login.
// Single table lookup against auth_accounts.
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

		renderLoginError := func() {
			data := map[string]any{
				"PageTitle": "Sign In",
				"CSRFToken": GenerateCSRFToken(r.Context(), sm),
				"Username":  username,
				"Error":     "Invalid username or password",
			}
			tr.Render(w, r, "login.html", data)
		}

		// Single table lookup.
		account, err := queries.GetAuthAccountByUsername(r.Context(), username)
		if err != nil {
			// Not found — run dummy bcrypt to prevent timing enumeration.
			bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
			renderLoginError()
			return
		}

		// Account exists but no password set yet — redirect to setup.
		if account.HashedPassword == nil {
			if err := sm.RenewToken(r.Context()); err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			sm.Put(r.Context(), sessionKeyAccountID, formatUUID(account.ID))
			http.Redirect(w, r, "/member-setup", http.StatusSeeOther)
			return
		}

		if err := bcrypt.CompareHashAndPassword(account.HashedPassword, []byte(password)); err != nil {
			renderLoginError()
			return
		}

		if err := sm.RenewToken(r.Context()); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		sm.Put(r.Context(), sessionKeyAccountID, formatUUID(account.ID))
		sm.Put(r.Context(), sessionKeyAccountUsername, account.Username)
		sm.Put(r.Context(), sessionKeyAccountRole, account.Role)
		if account.UserID.Valid {
			sm.Put(r.Context(), sessionKeyUserID, formatUUID(account.UserID))
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

// ActorFromSession builds a service.Actor from the session.
func ActorFromSession(sm *scs.SessionManager, r *http.Request) service.Actor {
	id := sm.GetString(r.Context(), sessionKeyAccountID)
	name := sm.GetString(r.Context(), sessionKeyAccountUsername)
	if name == "" {
		name = "admin"
	}
	return service.Actor{Type: "user", ID: id, Name: name}
}

// SessionRole returns the role of the logged-in user ("admin", "editor", or "viewer").
// Returns "admin" for legacy sessions that lack the role key.
func SessionRole(sm *scs.SessionManager, r *http.Request) string {
	role := sm.GetString(r.Context(), sessionKeyAccountRole)
	if role == "" {
		// Legacy admin sessions don't have the role key.
		return RoleAdmin
	}
	return role
}

// SessionUserID returns the family member user_id for accounts linked to a user.
// Returns empty string for accounts not linked to a family member (initial admin).
func SessionUserID(sm *scs.SessionManager, r *http.Request) string {
	return sm.GetString(r.Context(), sessionKeyUserID)
}

// SessionAccountID returns the auth_accounts.id for the current session.
func SessionAccountID(sm *scs.SessionManager, r *http.Request) string {
	return sm.GetString(r.Context(), sessionKeyAccountID)
}

// IsAdmin returns true if the current session has admin role.
func IsAdmin(sm *scs.SessionManager, r *http.Request) bool {
	return SessionRole(sm, r) == RoleAdmin
}

// IsEditor returns true if the current session has admin or editor role.
func IsEditor(sm *scs.SessionManager, r *http.Request) bool {
	role := SessionRole(sm, r)
	return role == RoleAdmin || role == RoleEditor
}

// IsViewer returns true for any authenticated user (always true if session exists).
func IsViewer(sm *scs.SessionManager, r *http.Request) bool {
	return sm.GetString(r.Context(), sessionKeyAccountID) != ""
}

// RoleDisplayName returns a human-readable display name for the role.
func RoleDisplayName(role string) string {
	switch role {
	case RoleAdmin:
		return "Administrator"
	case RoleEditor:
		return "Editor"
	case RoleViewer:
		return "Viewer"
	default:
		return role
	}
}

// MemberSetupHandler handles GET/POST /member-setup — first-time password setup for members.
func MemberSetupHandler(sm *scs.SessionManager, queries *db.Queries, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountIDStr := sm.GetString(r.Context(), sessionKeyAccountID)
		if accountIDStr == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		var accountID pgtype.UUID
		if err := accountID.Scan(accountIDStr); err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		account, err := queries.GetAuthAccountByID(r.Context(), accountID)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// If password is already set, redirect to login.
		if account.HashedPassword != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		if r.Method == http.MethodGet {
			data := map[string]any{
				"PageTitle": "Set Your Password",
				"CSRFToken": GenerateCSRFToken(r.Context(), sm),
				"Username":  account.Username,
				"Error":     "",
				"Errors":    map[string]string{},
			}
			tr.Render(w, r, "member_setup.html", data)
			return
		}

		// POST: set password.
		password := r.FormValue("password")
		confirmPassword := r.FormValue("confirm_password")

		errors := map[string]string{}
		if len(password) < 8 {
			errors["Password"] = "Password must be at least 8 characters"
		}
		if password != confirmPassword {
			errors["ConfirmPassword"] = "Passwords do not match"
		}

		if len(errors) > 0 {
			data := map[string]any{
				"PageTitle": "Set Your Password",
				"CSRFToken": GenerateCSRFToken(r.Context(), sm),
				"Username":  account.Username,
				"Error":     "",
				"Errors":    errors,
			}
			tr.Render(w, r, "member_setup.html", data)
			return
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), 12)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if err := queries.UpdateAuthAccountPassword(r.Context(), db.UpdateAuthAccountPasswordParams{
			ID:             accountID,
			HashedPassword: hashedPassword,
		}); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Renew session token (clear setup state) and set flash for login page.
		if err := sm.RenewToken(r.Context()); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		// Remove setup-only session keys but keep the session alive for flash.
		sm.Remove(r.Context(), sessionKeyAccountID)
		SetFlash(r.Context(), sm, "success", "Password set successfully. Please sign in.")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
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

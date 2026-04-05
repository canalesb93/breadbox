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
	sessionKeyAdminID       = "admin_id"
	sessionKeyAdminUsername = "admin_username"
	sessionKeyAccountRole   = "account_role"   // "admin" or "member"
	sessionKeyUserID        = "user_id"         // linked family member UUID (member accounts only)
	sessionKeyAccountType   = "account_type"    // "admin_account" or "member_account"
	sessionKeyMemberID      = "member_id"       // member_accounts.id (member accounts only)
)

// Session role constants.
const (
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// dummyHash is a pre-computed bcrypt hash used for constant-time login responses.
// When a username is not found, we still run bcrypt.CompareHashAndPassword against
// this dummy hash so that the response time is indistinguishable from a valid-user
// wrong-password attempt. This prevents username enumeration via timing side channels.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), 12)

// LoginHandler returns an http.HandlerFunc that handles GET and POST /login.
// It checks both admin_accounts and member_accounts tables for authentication.
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

		// Try admin_accounts first.
		admin, adminErr := queries.GetAdminAccountByUsername(r.Context(), username)
		if adminErr == nil {
			if err := bcrypt.CompareHashAndPassword(admin.HashedPassword, []byte(password)); err != nil {
				renderLoginError()
				return
			}

			if err := sm.RenewToken(r.Context()); err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			sm.Put(r.Context(), sessionKeyAdminID, formatUUID(admin.ID))
			sm.Put(r.Context(), sessionKeyAdminUsername, admin.Username)
			sm.Put(r.Context(), sessionKeyAccountRole, RoleAdmin)
			sm.Put(r.Context(), sessionKeyAccountType, "admin_account")
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		// Try member_accounts.
		member, memberErr := queries.GetMemberAccountByUsername(r.Context(), username)
		if memberErr == nil {
			// Member must have a password set (non-nil).
			if member.HashedPassword == nil {
				// Account exists but no password set yet — redirect to setup.
				if err := sm.RenewToken(r.Context()); err != nil {
					http.Error(w, "Internal server error", http.StatusInternalServerError)
					return
				}
				sm.Put(r.Context(), sessionKeyMemberID, formatUUID(member.ID))
				http.Redirect(w, r, "/member-setup", http.StatusSeeOther)
				return
			}

			if err := bcrypt.CompareHashAndPassword(member.HashedPassword, []byte(password)); err != nil {
				renderLoginError()
				return
			}

			if err := sm.RenewToken(r.Context()); err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			sm.Put(r.Context(), sessionKeyAdminID, formatUUID(member.ID))
			sm.Put(r.Context(), sessionKeyAdminUsername, member.Username)
			sm.Put(r.Context(), sessionKeyAccountRole, member.Role)
			sm.Put(r.Context(), sessionKeyAccountType, "member_account")
			sm.Put(r.Context(), sessionKeyUserID, formatUUID(member.UserID))
			sm.Put(r.Context(), sessionKeyMemberID, formatUUID(member.ID))
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		// Neither found — run dummy bcrypt to prevent timing enumeration.
		bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		renderLoginError()
	}
}

// ActorFromSession builds a service.Actor from the admin session.
func ActorFromSession(sm *scs.SessionManager, r *http.Request) service.Actor {
	id := sm.GetString(r.Context(), sessionKeyAdminID)
	name := sm.GetString(r.Context(), sessionKeyAdminUsername)
	if name == "" {
		name = "admin"
	}
	return service.Actor{Type: "user", ID: id, Name: name}
}

// SessionRole returns the role of the logged-in user ("admin" or "member").
// Returns "admin" for legacy admin_account sessions that lack the role key.
func SessionRole(sm *scs.SessionManager, r *http.Request) string {
	role := sm.GetString(r.Context(), sessionKeyAccountRole)
	if role == "" {
		// Legacy admin sessions don't have the role key.
		return RoleAdmin
	}
	return role
}

// SessionUserID returns the family member user_id for member accounts.
// Returns empty string for admin accounts (which aren't linked to a user).
func SessionUserID(sm *scs.SessionManager, r *http.Request) string {
	return sm.GetString(r.Context(), sessionKeyUserID)
}

// SessionMemberID returns the member_accounts.id for member account sessions.
// Returns empty string for legacy admin accounts.
func SessionMemberID(sm *scs.SessionManager, r *http.Request) string {
	return sm.GetString(r.Context(), sessionKeyMemberID)
}

// IsAdmin returns true if the current session is an admin (either admin_account or member with admin role).
func IsAdmin(sm *scs.SessionManager, r *http.Request) bool {
	return SessionRole(sm, r) == RoleAdmin
}

// MemberSetupHandler handles GET/POST /member-setup — first-time password setup for members.
func MemberSetupHandler(sm *scs.SessionManager, queries *db.Queries, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		memberIDStr := sm.GetString(r.Context(), sessionKeyMemberID)
		if memberIDStr == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		var memberID pgtype.UUID
		if err := memberID.Scan(memberIDStr); err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		member, err := queries.GetMemberAccountByID(r.Context(), memberID)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// If password is already set, redirect to login.
		if member.HashedPassword != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		if r.Method == http.MethodGet {
			data := map[string]any{
				"PageTitle": "Set Your Password",
				"CSRFToken": GenerateCSRFToken(r.Context(), sm),
				"Username":  member.Username,
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
				"Username":  member.Username,
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

		if err := queries.UpdateMemberAccountPassword(r.Context(), db.UpdateMemberAccountPasswordParams{
			ID:             memberID,
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
		sm.Remove(r.Context(), sessionKeyMemberID)
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

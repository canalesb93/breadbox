package admin

import (
	"net/http"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
)

// renderLogin renders the login page via the templ component, so the
// handler and the template stay decoupled from the html/template
// renderer. Called by every branch of LoginHandler that needs to show
// the form (GET + error paths).
func renderLogin(w http.ResponseWriter, r *http.Request, sm *scs.SessionManager, username, errMsg string) {
	props := pages.LoginProps{
		PageTitle: "Sign In",
		CSRFToken: GenerateCSRFToken(r.Context(), sm),
		Username:  username,
		Error:     errMsg,
	}
	if f := GetFlash(r.Context(), sm); f != nil {
		props.FlashType = f.Type
		props.FlashMsg = f.Message
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.Login(props).Render(r.Context(), w); err != nil {
		http.Error(w, "template render error: "+err.Error(), http.StatusInternalServerError)
	}
}

const (
	sessionKeyAccountID       = "account_id"       // auth_accounts.id
	sessionKeyAccountUsername = "account_username"  // auth_accounts.username
	sessionKeyAccountRole     = "account_role"      // "admin", "editor", or "viewer"
	sessionKeyUserID          = "user_id"           // linked family member UUID (NULL-linked admins have "")
	sessionKeyUserName        = "user_name"         // linked family member display name; surfaced in sidebar footer
	sessionKeyAvatarVersion   = "avatar_v"          // bumped on avatar change for cache busting
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
func LoginHandler(sm *scs.SessionManager, queries *db.Queries, _ *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if sm.GetString(r.Context(), sessionKeyAccountID) != "" {
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
			renderLogin(w, r, sm, "", "")
			return
		}

		// POST: validate credentials.
		username := r.FormValue("username")
		password := r.FormValue("password")

		if username == "" || password == "" {
			renderLogin(w, r, sm, username, "Invalid email or password")
			return
		}

		renderLoginError := func() {
			renderLogin(w, r, sm, username, "Invalid email or password")
		}

		// Single table lookup.
		account, err := queries.GetAuthAccountByUsername(r.Context(), username)
		if err != nil {
			// Not found — run dummy bcrypt to prevent timing enumeration.
			bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
			renderLoginError()
			return
		}

		// Account exists but no password set yet — tell user to use setup link.
		if account.HashedPassword == nil {
			bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
			renderLogin(w, r, sm, username, "Your account hasn't been set up yet. Ask your administrator for a setup link.")
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

		sm.RememberMe(r.Context(), r.FormValue("remember_me") != "")

		sm.Put(r.Context(), sessionKeyAccountID, pgconv.FormatUUID(account.ID))
		sm.Put(r.Context(), sessionKeyAccountUsername, account.Username)
		sm.Put(r.Context(), sessionKeyAccountRole, account.Role)
		if account.UserID.Valid {
			sm.Put(r.Context(), sessionKeyUserID, pgconv.FormatUUID(account.UserID))
			// Cache the linked household-member display name so the
			// sidebar footer can show "Ricardo" instead of falling back
			// to the auth_accounts.username (often an email). Updated
			// on profile edits via /settings/account/profile.
			if u, err := queries.GetUser(r.Context(), account.UserID); err == nil {
				sm.Put(r.Context(), sessionKeyUserName, u.Name)
			} else {
				sm.Remove(r.Context(), sessionKeyUserName)
			}
		} else {
			sm.Remove(r.Context(), sessionKeyUserID)
			sm.Remove(r.Context(), sessionKeyUserName)
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

// ActorFromSession builds a service.Actor from the session.
//
// Prefers the linked users.id over the auth_accounts.id so that actor_id on
// annotations matches the avatar handler's lookup key (/avatars/{users.id}).
// Falls back to auth_accounts.id only for unlinked initial admin accounts.
func ActorFromSession(sm *scs.SessionManager, r *http.Request) service.Actor {
	id := sm.GetString(r.Context(), sessionKeyUserID)
	if id == "" {
		id = sm.GetString(r.Context(), sessionKeyAccountID)
	}
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

// SetupAccountHandler handles GET/POST /setup-account/{token} — token-based password setup.
// This is an unauthenticated route. Members receive a setup URL from their admin.
func SetupAccountHandler(sm *scs.SessionManager, queries *db.Queries, _ *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "token")
		if token == "" {
			renderSetupError(w, r, sm, "This setup link is invalid.")
			return
		}

		account, err := queries.GetAuthAccountBySetupToken(r.Context(), pgconv.Text(token))
		if err != nil {
			renderSetupError(w, r, sm, "This setup link is invalid or has expired.")
			return
		}

		// Check expiry.
		if account.SetupTokenExpiresAt.Valid && account.SetupTokenExpiresAt.Time.Before(time.Now()) {
			renderSetupError(w, r, sm, "This setup link has expired. Ask your administrator for a new one.")
			return
		}

		// If password is already set, redirect to login.
		if account.HashedPassword != nil {
			FlashRedirect(w, r, sm, "info", "Your account is already set up. Please sign in.", "/login")
			return
		}

		if r.Method == http.MethodGet {
			renderSetupAccount(w, r, sm, account.Username, token, "", nil)
			return
		}

		// POST: set password.
		password := r.FormValue("password")
		confirmPassword := r.FormValue("confirm_password")

		fieldErrors := map[string]string{}
		if len(password) < 8 {
			fieldErrors["Password"] = "Password must be at least 8 characters"
		}
		if password != confirmPassword {
			fieldErrors["ConfirmPassword"] = "Passwords do not match"
		}

		if len(fieldErrors) > 0 {
			renderSetupAccount(w, r, sm, account.Username, token, "", fieldErrors)
			return
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), 12)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if err := queries.UpdateAuthAccountPassword(r.Context(), db.UpdateAuthAccountPasswordParams{
			ID:             account.ID,
			HashedPassword: hashedPassword,
		}); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Clear the setup token.
		_ = queries.ClearAuthAccountSetupToken(r.Context(), account.ID)

		SetFlash(r.Context(), sm, "success", "Password set successfully. Please sign in.")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

// renderSetupAccount renders the token-based setup-account page via the
// templ component. Mirrors renderLogin: handler is decoupled from the
// html/template renderer. When fieldErrors is nil an empty map is used.
func renderSetupAccount(w http.ResponseWriter, r *http.Request, sm *scs.SessionManager, username, token, errMsg string, fieldErrors map[string]string) {
	if fieldErrors == nil {
		fieldErrors = map[string]string{}
	}
	props := pages.SetupAccountProps{
		PageTitle:   "Set Your Password",
		CSRFToken:   GenerateCSRFToken(r.Context(), sm),
		Username:    username,
		Token:       token,
		Error:       errMsg,
		FieldErrors: fieldErrors,
	}
	if f := GetFlash(r.Context(), sm); f != nil {
		props.FlashType = f.Type
		props.FlashMsg = f.Message
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SetupAccount(props).Render(r.Context(), w); err != nil {
		http.Error(w, "template render error: "+err.Error(), http.StatusInternalServerError)
	}
}

// renderSetupError renders the setup account page with an error message (no form).
func renderSetupError(w http.ResponseWriter, r *http.Request, sm *scs.SessionManager, message string) {
	props := pages.SetupAccountProps{
		PageTitle:  "Account Setup",
		SetupError: message,
	}
	if f := GetFlash(r.Context(), sm); f != nil {
		props.FlashType = f.Type
		props.FlashMsg = f.Message
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SetupAccount(props).Render(r.Context(), w); err != nil {
		http.Error(w, "template render error: "+err.Error(), http.StatusInternalServerError)
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

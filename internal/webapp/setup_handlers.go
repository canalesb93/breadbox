//go:build !headless && !lite

package webapp

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"breadbox/internal/admin"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/webapp/pages"
)

// setupAccountPage validates a setup token and renders the set-password form, or an
// invalid-token message. Pre-auth: the token is the credential. Unknown, expired, and
// already-used tokens collapse to a single message so they can't be distinguished.
func (h *Handler) setupAccountPage(w http.ResponseWriter, r *http.Request) {
	account, invalid := h.lookupSetupAccount(r)
	if invalid != "" {
		render(w, r, http.StatusOK, pages.SetupAccount(themeClass(r), pages.SetupAccountData{Invalid: invalid}))
		return
	}
	render(w, r, http.StatusOK, pages.SetupAccount(themeClass(r), pages.SetupAccountData{
		Token:    chi.URLParam(r, "token"),
		Username: account.Username,
		Errors:   map[string]string{},
	}))
}

// setupAccountSubmit consumes the token, sets the password via the same query path the
// SPA uses (UpdateAuthAccountPassword + ClearAuthAccountSetupToken), establishes the
// shared session via admin.SetLoginSessionKeys, and 303s into the app. Validation failures
// re-render the form at HTTP 422. Passwords are never logged or echoed back.
func (h *Handler) setupAccountSubmit(w http.ResponseWriter, r *http.Request) {
	account, invalid := h.lookupSetupAccount(r)
	if invalid != "" {
		render(w, r, http.StatusGone, pages.SetupAccount(themeClass(r), pages.SetupAccountData{Invalid: invalid}))
		return
	}

	token := chi.URLParam(r, "token")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	fieldErrs := map[string]string{}
	if len(password) < 8 {
		fieldErrs["password"] = "Password must be at least 8 characters."
	}
	if password != confirmPassword {
		fieldErrs["confirm_password"] = "Passwords do not match."
	}
	if len(fieldErrs) > 0 {
		render(w, r, http.StatusUnprocessableEntity, pages.SetupAccount(themeClass(r), pages.SetupAccountData{
			Token:    token,
			Username: account.Username,
			Errors:   fieldErrs,
		}))
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	if err := h.app.Queries.UpdateAuthAccountPassword(r.Context(), db.UpdateAuthAccountPasswordParams{
		ID:             account.ID,
		HashedPassword: hashed,
	}); err != nil {
		h.serverError(w, r, err)
		return
	}
	_ = h.app.Queries.ClearAuthAccountSetupToken(r.Context(), account.ID)

	// Re-read so the session keys reflect the freshly-set password and no stale token
	// state. Mirrors the login path, then open a session so the invitee lands in /app/.
	fresh, err := h.app.Queries.GetAuthAccountByID(r.Context(), account.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	if err := h.sm.RenewToken(r.Context()); err != nil {
		h.serverError(w, r, err)
		return
	}
	admin.SetLoginSessionKeys(r.Context(), h.sm, fresh, h.app.Queries)
	http.Redirect(w, r, "/app/", http.StatusSeeOther)
}

// lookupSetupAccount resolves the URL token to an account. It returns a non-empty
// invalid string for an empty/unknown/expired token, or one whose password is already
// set — collapsed to one message so callers can't tell the cases apart.
func (h *Handler) lookupSetupAccount(r *http.Request) (db.AuthAccount, string) {
	token := chi.URLParam(r, "token")
	if token == "" {
		return db.AuthAccount{}, "This setup link is invalid."
	}
	account, err := h.app.Queries.GetAuthAccountBySetupToken(r.Context(), pgconv.Text(token))
	if err != nil {
		return db.AuthAccount{}, "This setup link is invalid or has expired."
	}
	if account.SetupTokenExpiresAt.Valid && account.SetupTokenExpiresAt.Time.Before(time.Now()) {
		return db.AuthAccount{}, "This setup link has expired. Ask your administrator for a new one."
	}
	if account.HashedPassword != nil {
		return db.AuthAccount{}, "This account is already set up. Please sign in."
	}
	return account, ""
}

// registerSetup wires the public (pre-auth) token-invite set-password routes. The
// orchestrator mounts this in the public group — no requireAuth.
func (h *Handler) registerSetup(r chi.Router) {
	r.Get("/setup-account/{token}", h.setupAccountPage)
	r.Post("/setup-account/{token}", h.requireSameOrigin(h.setupAccountSubmit))
}

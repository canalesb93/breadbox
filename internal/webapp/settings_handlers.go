//go:build !headless && !lite

package webapp

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"

	"breadbox/internal/admin"
	"breadbox/internal/db"
	"breadbox/internal/webapp/pages"
)

// settingsAccount renders the account settings page: read-only identity + password form.
// ?saved=1 (set by the post-redirect-get on a successful password change) shows a banner.
func (h *Handler) settingsAccount(w http.ResponseWriter, r *http.Request) {
	render(w, r, http.StatusOK, pages.SettingsAccount(h.shellData(r, "Settings"), pages.SettingsAccountData{
		Username: admin.SessionUsername(h.sm, r),
		RoleName: admin.RoleDisplayName(admin.SessionRole(h.sm, r)),
		Errors:   map[string]string{},
		Saved:    r.URL.Query().Get("saved") == "1",
	}))
}

// changePassword validates the current password via bcrypt and stores a new one through
// the same query path the SPA uses (UpdateAuthAccountPassword). On any validation failure
// it re-renders the account page with field errors at HTTP 422; on success it 303s to
// /app/settings/account?saved=1 (post-redirect-get). Passwords are never logged or echoed.
func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	accountID, ok := h.sessionAccountUUID(r)
	if !ok {
		http.Redirect(w, r, "/app/login", http.StatusSeeOther)
		return
	}

	account, err := h.app.Queries.GetAuthAccountByID(r.Context(), accountID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	// An account with no password set (token-invited, never set up) can't verify a
	// current password — route them through the setup flow instead.
	if account.HashedPassword == nil {
		h.rerenderSettingsAccount(w, r, map[string]string{
			"form": "No password is set on this account. Use your setup link to set one.",
		})
		return
	}

	fieldErrs := map[string]string{}
	if bcrypt.CompareHashAndPassword(account.HashedPassword, []byte(currentPassword)) != nil {
		fieldErrs["current_password"] = "Current password is incorrect."
	}
	if len(newPassword) < 8 {
		fieldErrs["new_password"] = "New password must be at least 8 characters."
	}
	if newPassword != confirmPassword {
		fieldErrs["confirm_password"] = "New passwords do not match."
	}
	if len(fieldErrs) > 0 {
		h.rerenderSettingsAccount(w, r, fieldErrs)
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), 12)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	if err := h.app.Queries.UpdateAuthAccountPassword(r.Context(), db.UpdateAuthAccountPasswordParams{
		ID:             accountID,
		HashedPassword: hashed,
	}); err != nil {
		h.serverError(w, r, err)
		return
	}

	http.Redirect(w, r, "/app/settings/account?saved=1", http.StatusSeeOther)
}

// rerenderSettingsAccount re-renders the account page with errors at HTTP 422.
func (h *Handler) rerenderSettingsAccount(w http.ResponseWriter, r *http.Request, fieldErrs map[string]string) {
	render(w, r, http.StatusUnprocessableEntity, pages.SettingsAccount(h.shellData(r, "Settings"), pages.SettingsAccountData{
		Username: admin.SessionUsername(h.sm, r),
		RoleName: admin.RoleDisplayName(admin.SessionRole(h.sm, r)),
		Errors:   fieldErrs,
	}))
}

// sessionAccountUUID parses the session's auth_accounts.id into a pgtype.UUID.
func (h *Handler) sessionAccountUUID(r *http.Request) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(admin.SessionAccountID(h.sm, r)); err != nil {
		return pgtype.UUID{}, false
	}
	return id, true
}

// registerSettings wires the account settings read + password-change routes onto an
// authenticated subrouter. /settings and /settings/account both render the account page.
func (h *Handler) registerSettings(r chi.Router) {
	r.Get("/settings", h.settingsAccount)
	r.Get("/settings/account", h.settingsAccount)
	r.Post("/settings/password", h.requireSameOrigin(h.changePassword))
}

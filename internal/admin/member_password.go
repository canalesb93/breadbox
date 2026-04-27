package admin

import (
	"net/http"

	"breadbox/internal/app"
	"breadbox/internal/db"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
)

// changePasswordForAccount handles password change for any auth account.
// Used by both /my-account/password and /settings/password.
func changePasswordForAccount(a *app.App, sm *scs.SessionManager, w http.ResponseWriter, r *http.Request, accountIDStr, redirectPath string) {
	ctx := r.Context()

	var accountID pgtype.UUID
	if err := accountID.Scan(accountIDStr); err != nil {
		FlashRedirect(w, r, sm, "error", "Invalid session.", redirectPath)
		return
	}

	account, err := a.Queries.GetAuthAccountByID(ctx, accountID)
	if err != nil {
		FlashRedirect(w, r, sm, "error", "Account not found.", redirectPath)
		return
	}

	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	if account.HashedPassword == nil {
		FlashRedirect(w, r, sm, "error", "No password set. Please contact your administrator.", redirectPath)
		return
	}
	if err := bcrypt.CompareHashAndPassword(account.HashedPassword, []byte(currentPassword)); err != nil {
		FlashRedirect(w, r, sm, "error", "Current password is incorrect.", redirectPath)
		return
	}

	if len(newPassword) < 8 {
		FlashRedirect(w, r, sm, "error", "New password must be at least 8 characters.", redirectPath)
		return
	}

	if newPassword != confirmPassword {
		FlashRedirect(w, r, sm, "error", "New passwords do not match.", redirectPath)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), 12)
	if err != nil {
		FlashRedirect(w, r, sm, "error", "Failed to hash password.", redirectPath)
		return
	}

	if err := a.Queries.UpdateAuthAccountPassword(ctx, db.UpdateAuthAccountPasswordParams{
		ID:             accountID,
		HashedPassword: hashedPassword,
	}); err != nil {
		a.Logger.Error("update account password", "error", err)
		FlashRedirect(w, r, sm, "error", "Failed to update password.", redirectPath)
		return
	}

	SetFlash(ctx, sm, "success", "Password updated successfully.")
	http.Redirect(w, r, redirectPath, http.StatusSeeOther)
}

package admin

import (
	"net/http"

	"breadbox/internal/app"
	"breadbox/internal/db"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
)

// changePasswordFromMember handles password change for a member account.
// Used by both /my-account/password and /settings/password (when logged in as member).
func changePasswordFromMember(a *app.App, sm *scs.SessionManager, w http.ResponseWriter, r *http.Request, memberIDStr, redirectPath string) {
	ctx := r.Context()

	var memberID pgtype.UUID
	if err := memberID.Scan(memberIDStr); err != nil {
		SetFlash(ctx, sm, "error", "Invalid session.")
		http.Redirect(w, r, redirectPath, http.StatusSeeOther)
		return
	}

	member, err := a.Queries.GetMemberAccountByID(ctx, memberID)
	if err != nil {
		SetFlash(ctx, sm, "error", "Account not found.")
		http.Redirect(w, r, redirectPath, http.StatusSeeOther)
		return
	}

	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	if member.HashedPassword != nil {
		if err := bcrypt.CompareHashAndPassword(member.HashedPassword, []byte(currentPassword)); err != nil {
			SetFlash(ctx, sm, "error", "Current password is incorrect.")
			http.Redirect(w, r, redirectPath, http.StatusSeeOther)
			return
		}
	}

	if len(newPassword) < 8 {
		SetFlash(ctx, sm, "error", "New password must be at least 8 characters.")
		http.Redirect(w, r, redirectPath, http.StatusSeeOther)
		return
	}

	if newPassword != confirmPassword {
		SetFlash(ctx, sm, "error", "New passwords do not match.")
		http.Redirect(w, r, redirectPath, http.StatusSeeOther)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), 12)
	if err != nil {
		SetFlash(ctx, sm, "error", "Failed to hash password.")
		http.Redirect(w, r, redirectPath, http.StatusSeeOther)
		return
	}

	if err := a.Queries.UpdateMemberAccountPassword(ctx, db.UpdateMemberAccountPasswordParams{
		ID:             memberID,
		HashedPassword: hashedPassword,
	}); err != nil {
		a.Logger.Error("update member password", "error", err)
		SetFlash(ctx, sm, "error", "Failed to update password.")
		http.Redirect(w, r, redirectPath, http.StatusSeeOther)
		return
	}

	SetFlash(ctx, sm, "success", "Password updated successfully.")
	http.Redirect(w, r, redirectPath, http.StatusSeeOther)
}

package admin

import (
	"net/http"
	"net/mail"
	"strings"

	"breadbox/internal/app"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// createLoginAccountRequest is the JSON body for POST /-/members.
type createLoginAccountRequest struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// CreateLoginAccountHandler serves POST /-/members -- create a login account for a family member.
func CreateLoginAccountHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createLoginAccountRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		req.Username = strings.TrimSpace(req.Username)
		if req.Username == "" {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Email is required")
			return
		}

		// Validate email format since username = email.
		if _, err := mail.ParseAddress(req.Username); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Please enter a valid email address")
			return
		}

		if len(req.Username) > 64 {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Email must be 64 characters or fewer")
			return
		}

		if req.UserID == "" {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Family member (user_id) is required")
			return
		}

		if req.Role == "" {
			req.Role = "viewer"
		}

		member, err := svc.CreateLoginAccount(r.Context(), service.CreateLoginAccountParams{
			UserID:   req.UserID,
			Username: req.Username,
			Role:     req.Role,
		})
		if err != nil {
			if strings.Contains(err.Error(), "username already taken") {
				writeError(w, http.StatusConflict, "EMAIL_TAKEN", "This email is already in use as a login")
				return
			}
			if strings.Contains(err.Error(), "already has a login account") {
				writeError(w, http.StatusConflict, "DUPLICATE_MEMBER", "This family member already has a login account")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create login account")
			return
		}

		writeJSON(w, http.StatusCreated, member)
	}
}

// ListLoginAccountsHandler serves GET /-/members -- list all login accounts.
func ListLoginAccountsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		members, err := svc.ListLoginAccounts(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list login accounts")
			return
		}
		writeJSON(w, http.StatusOK, members)
	}
}

// UpdateLoginAccountRoleHandler serves PUT /-/members/{id}/role.
func UpdateLoginAccountRoleHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var req struct {
			Role string `json:"role"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		if err := svc.UpdateLoginAccountRole(r.Context(), id, req.Role); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// DeleteLoginAccountHandler serves DELETE /-/members/{id}.
func DeleteLoginAccountHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		if err := svc.DeleteLoginAccount(r.Context(), id); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete login account")
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// RegenerateSetupTokenHandler serves POST /-/members/{id}/setup-token -- regenerate setup token.
func RegenerateSetupTokenHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		token, err := svc.RegenerateSetupToken(r.Context(), id)
		if err != nil {
			if strings.Contains(err.Error(), "already has a password") {
				writeError(w, http.StatusConflict, "PASSWORD_ALREADY_SET", "This account already has a password set")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to regenerate setup token")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"setup_token": token,
		})
	}
}

// WipeUserDataHandler serves POST /-/users/{id}/wipe -- delete all connections and transactions for a user.
func WipeUserDataHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		txnCount, err := a.Service.WipeUserData(r.Context(), id)
		if err != nil {
			a.Logger.Error("wipe user data", "error", err, "user_id", id)
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to wipe user data")
			return
		}

		SetFlash(r.Context(), sm, "success", "User data wiped successfully.")
		writeJSON(w, http.StatusOK, map[string]any{
			"status":               "ok",
			"transactions_deleted": txnCount,
		})
	}
}

// MyAccountHandler serves GET /my-account -- member's own account page.
func MyAccountHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountIDStr := SessionAccountID(sm, r)
		userIDStr := SessionUserID(sm, r)

		data := BaseTemplateData(r, sm, "my-account", "My Account")
		data["AccountID"] = accountIDStr
		data["UserID"] = userIDStr

		// Check if this admin is unlinked (no household member).
		isUnlinked := userIDStr == ""
		data["IsUnlinked"] = isUnlinked

		if isUnlinked {
			// Fetch users that don't have a login account for the "link existing" option.
			unlinked, err := a.Queries.ListUsersWithoutAuthAccount(r.Context())
			if err == nil {
				data["UnlinkedUsers"] = unlinked
			}
		}

		// Load the user's connections and accounts for display.
		if userIDStr != "" {
			conns, err := svc.ListConnections(r.Context(), &userIDStr)
			if err == nil {
				data["Connections"] = conns
			}
		}

		tr.Render(w, r, "my_account.html", data)
	}
}

// LinkAdminToUserHandler serves POST /my-account/link-user -- link an unlinked admin to a household member.
func LinkAdminToUserHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Guard: must be an unlinked account.
		if SessionUserID(sm, r) != "" {
			SetFlash(ctx, sm, "error", "Your account is already linked to a household member.")
			http.Redirect(w, r, "/my-account", http.StatusSeeOther)
			return
		}

		accountIDStr := SessionAccountID(sm, r)
		accountID, err := parseUUID(accountIDStr)
		if err != nil {
			SetFlash(ctx, sm, "error", "Invalid session.")
			http.Redirect(w, r, "/my-account", http.StatusSeeOther)
			return
		}

		mode := r.FormValue("mode")
		var userID pgtype.UUID

		switch mode {
		case "create":
			name := strings.TrimSpace(r.FormValue("name"))
			if name == "" {
				SetFlash(ctx, sm, "error", "Name is required.")
				http.Redirect(w, r, "/my-account", http.StatusSeeOther)
				return
			}
			user, err := a.Queries.CreateUser(ctx, db.CreateUserParams{Name: name})
			if err != nil {
				SetFlash(ctx, sm, "error", "Failed to create household member.")
				http.Redirect(w, r, "/my-account", http.StatusSeeOther)
				return
			}
			userID = user.ID

		case "existing":
			uid := r.FormValue("user_id")
			if uid == "" {
				SetFlash(ctx, sm, "error", "Please select a household member.")
				http.Redirect(w, r, "/my-account", http.StatusSeeOther)
				return
			}
			parsed, err := parseUUID(uid)
			if err != nil {
				SetFlash(ctx, sm, "error", "Invalid user selected.")
				http.Redirect(w, r, "/my-account", http.StatusSeeOther)
				return
			}
			// Verify user exists and isn't already linked.
			if _, err := a.Queries.GetUser(ctx, parsed); err != nil {
				SetFlash(ctx, sm, "error", "Household member not found.")
				http.Redirect(w, r, "/my-account", http.StatusSeeOther)
				return
			}
			if _, err := a.Queries.GetAuthAccountByUserID(ctx, parsed); err == nil {
				SetFlash(ctx, sm, "error", "That household member already has a login account.")
				http.Redirect(w, r, "/my-account", http.StatusSeeOther)
				return
			}
			userID = parsed

		default:
			SetFlash(ctx, sm, "error", "Invalid request.")
			http.Redirect(w, r, "/my-account", http.StatusSeeOther)
			return
		}

		// Link the auth account to the user.
		if err := a.Queries.UpdateAuthAccountUserID(ctx, db.UpdateAuthAccountUserIDParams{
			ID:     accountID,
			UserID: userID,
		}); err != nil {
			SetFlash(ctx, sm, "error", "Failed to link account.")
			http.Redirect(w, r, "/my-account", http.StatusSeeOther)
			return
		}

		// Update session so the change takes effect immediately.
		sm.Put(ctx, sessionKeyUserID, pgconv.FormatUUID(userID))

		SetFlash(ctx, sm, "success", "Account linked to household member.")
		http.Redirect(w, r, "/my-account", http.StatusSeeOther)
	}
}

// MyAccountChangePasswordHandler serves POST /my-account/password -- member changes their own password.
func MyAccountChangePasswordHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		accountIDStr := SessionAccountID(sm, r)
		if accountIDStr == "" {
			SetFlash(ctx, sm, "error", "Invalid session.")
			http.Redirect(w, r, "/my-account", http.StatusSeeOther)
			return
		}

		changePasswordForAccount(a, sm, w, r, accountIDStr, "/my-account")
	}
}

// MyAccountWipeDataHandler serves POST /my-account/wipe-data -- member wipes their own data.
func MyAccountWipeDataHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		userIDStr := SessionUserID(sm, r)
		if userIDStr == "" {
			SetFlash(ctx, sm, "error", "Invalid session.")
			http.Redirect(w, r, "/my-account", http.StatusSeeOther)
			return
		}

		txnCount, err := a.Service.WipeUserData(ctx, userIDStr)
		if err != nil {
			a.Logger.Error("member wipe own data", "error", err, "user_id", userIDStr)
			SetFlash(ctx, sm, "error", "Failed to wipe data.")
			http.Redirect(w, r, "/my-account", http.StatusSeeOther)
			return
		}

		a.Logger.Info("member wiped own data", "user_id", userIDStr, "transactions_deleted", txnCount)
		SetFlash(ctx, sm, "success", "Your data has been wiped successfully.")
		http.Redirect(w, r, "/my-account", http.StatusSeeOther)
	}
}

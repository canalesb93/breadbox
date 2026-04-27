package admin

import (
	"net/http"
	"net/mail"
	"strings"

	"breadbox/internal/app"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"

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

// buildMyAccountProps assembles the typed props shared by the Account
// (password + danger zone) and Profile (avatar + name + email) tabs.
// Both pages live under /settings/* and need the same identity + linked-
// user data, so the fetch is centralised here.
func buildMyAccountProps(a *app.App, sm *scs.SessionManager, r *http.Request) (pages.MyAccountProps, map[string]any) {
	accountIDStr := SessionAccountID(sm, r)
	userIDStr := SessionUserID(sm, r)
	isUnlinked := userIDStr == ""
	role := SessionRole(sm, r)

	data := BaseTemplateData(r, sm, "account", "My Account")
	data["AccountID"] = accountIDStr
	data["UserID"] = userIDStr
	data["IsUnlinked"] = isUnlinked

	props := pages.MyAccountProps{
		UserID:               userIDStr,
		IsUnlinked:           isUnlinked,
		CSRFToken:            GetCSRFToken(r),
		AdminUsername:        sm.GetString(r.Context(), sessionKeyAccountUsername),
		RoleDisplay:          RoleDisplayName(role),
		SessionUserID:        userIDStr,
		SessionAvatarVersion: sm.GetString(r.Context(), sessionKeyAvatarVersion),
	}

	if userIDStr != "" {
		var userID pgtype.UUID
		if err := userID.Scan(userIDStr); err == nil {
			if u, err := a.Queries.GetUser(r.Context(), userID); err == nil {
				props.UserName = u.Name
				props.UserEmail = pgconv.TextOr(u.Email, "")
				props.HasCustomAvatar = len(u.AvatarData) > 0
			}
		}
	}

	return props, data
}

// MyAccountHandler serves GET /settings/account -- the Account tab
// (password + sign-out + danger zone).
func MyAccountHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		props, data := buildMyAccountProps(a, sm, r)
		data["PageTitle"] = "Account"
		data["CurrentPage"] = "account"
		renderSettingsTab(tr, w, r, tr.sm, data, pages.SettingsTabAccount, pages.MyAccount(props))
	}
}

// MyProfileHandler serves GET /settings/profile -- the Profile tab
// (avatar + name + email editor). Shares MyAccountProps with the Account
// tab so both pages render the same identity context.
func MyProfileHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		props, data := buildMyAccountProps(a, sm, r)
		data["PageTitle"] = "Profile"
		data["CurrentPage"] = "profile"
		renderSettingsTab(tr, w, r, tr.sm, data, pages.SettingsTabProfile, pages.MyProfile(props))
	}
}

// LinkAdminToUserHandler serves POST /settings/account/link-user -- link an unlinked admin to a household member.
func LinkAdminToUserHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Guard: must be an unlinked account.
		if SessionUserID(sm, r) != "" {
			FlashRedirect(w, r, sm, "error", "Your account is already linked to a household member.", "/settings/account")
			return
		}

		accountIDStr := SessionAccountID(sm, r)
		accountID, err := pgconv.ParseUUID(accountIDStr)
		if err != nil {
			FlashRedirect(w, r, sm, "error", "Invalid session.", "/settings/account")
			return
		}

		mode := r.FormValue("mode")
		var userID pgtype.UUID

		switch mode {
		case "create":
			name := strings.TrimSpace(r.FormValue("name"))
			if name == "" {
				FlashRedirect(w, r, sm, "error", "Name is required.", "/settings/account")
				return
			}
			user, err := a.Queries.CreateUser(ctx, db.CreateUserParams{Name: name})
			if err != nil {
				FlashRedirect(w, r, sm, "error", "Failed to create household member.", "/settings/account")
				return
			}
			userID = user.ID

		case "existing":
			uid := r.FormValue("user_id")
			if uid == "" {
				FlashRedirect(w, r, sm, "error", "Please select a household member.", "/settings/account")
				return
			}
			parsed, err := pgconv.ParseUUID(uid)
			if err != nil {
				FlashRedirect(w, r, sm, "error", "Invalid user selected.", "/settings/account")
				return
			}
			// Verify user exists and isn't already linked.
			if _, err := a.Queries.GetUser(ctx, parsed); err != nil {
				FlashRedirect(w, r, sm, "error", "Household member not found.", "/settings/account")
				return
			}
			if _, err := a.Queries.GetAuthAccountByUserID(ctx, parsed); err == nil {
				FlashRedirect(w, r, sm, "error", "That household member already has a login account.", "/settings/account")
				return
			}
			userID = parsed

		default:
			FlashRedirect(w, r, sm, "error", "Invalid request.", "/settings/account")
			return
		}

		// Link the auth account to the user.
		if err := a.Queries.UpdateAuthAccountUserID(ctx, db.UpdateAuthAccountUserIDParams{
			ID:     accountID,
			UserID: userID,
		}); err != nil {
			FlashRedirect(w, r, sm, "error", "Failed to link account.", "/settings/account")
			return
		}

		// Update session so the change takes effect immediately.
		sm.Put(ctx, sessionKeyUserID, pgconv.FormatUUID(userID))

		SetFlash(ctx, sm, "success", "Account linked to household member.")
		http.Redirect(w, r, "/settings/account", http.StatusSeeOther)
	}
}

// MyAccountChangePasswordHandler serves POST /settings/account/password -- member changes their own password.
func MyAccountChangePasswordHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountIDStr := SessionAccountID(sm, r)
		if accountIDStr == "" {
			FlashRedirect(w, r, sm, "error", "Invalid session.", "/settings/account")
			return
		}

		changePasswordForAccount(a, sm, w, r, accountIDStr, "/settings/account")
	}
}

// MyAccountUpdateProfileHandler serves PUT /settings/account/profile -- member
// updates their own household profile (display name + email). Requires the
// account to be linked to a user. JSON-shaped to match the existing
// /-/users/{id} PUT endpoint that the shared avatarEditor factory drives.
func MyAccountUpdateProfileHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := SessionUserID(sm, r)
		if userIDStr == "" {
			writeError(w, http.StatusForbidden, "ACCOUNT_NOT_LINKED", "Your account isn't linked to a household member yet.")
			return
		}

		var userID pgtype.UUID
		if err := userID.Scan(userIDStr); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid user ID")
			return
		}

		var req updateUserRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		existing, err := a.Queries.GetUser(r.Context(), userID)
		if err != nil {
			a.Logger.Error("get user for self-update", "error", err)
			writeError(w, http.StatusNotFound, "NOT_FOUND", "User not found")
			return
		}

		name := existing.Name
		if req.Name != nil {
			trimmed := strings.TrimSpace(*req.Name)
			if trimmed == "" {
				writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Name must not be empty")
				return
			}
			name = trimmed
		}

		email := existing.Email
		if req.Email != nil {
			if *req.Email == "" {
				email = pgtype.Text{}
			} else {
				if _, err := mail.ParseAddress(*req.Email); err != nil {
					writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid email format")
					return
				}
				email = pgconv.Text(*req.Email)
			}
		}

		user, err := a.Queries.UpdateUser(r.Context(), db.UpdateUserParams{
			ID:    userID,
			Name:  name,
			Email: email,
		})
		if err != nil {
			a.Logger.Error("update self profile", "error", err)
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update profile")
			return
		}

		// Refresh the cached display name so the sidebar footer reflects
		// the edit on the next page render without a re-login.
		sm.Put(r.Context(), sessionKeyUserName, user.Name)

		writeJSON(w, http.StatusOK, map[string]any{
			"id":    pgconv.FormatUUID(user.ID),
			"name":  user.Name,
			"email": pgconv.TextPtr(user.Email),
		})
	}
}

// MyAccountWipeDataHandler serves POST /settings/account/wipe-data -- member wipes their own data.
func MyAccountWipeDataHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		userIDStr := SessionUserID(sm, r)
		if userIDStr == "" {
			FlashRedirect(w, r, sm, "error", "Invalid session.", "/settings/account")
			return
		}

		txnCount, err := a.Service.WipeUserData(ctx, userIDStr)
		if err != nil {
			a.Logger.Error("member wipe own data", "error", err, "user_id", userIDStr)
			FlashRedirect(w, r, sm, "error", "Failed to wipe data.", "/settings/account")
			return
		}

		a.Logger.Info("member wiped own data", "user_id", userIDStr, "transactions_deleted", txnCount)
		SetFlash(ctx, sm, "success", "Your data has been wiped successfully.")
		http.Redirect(w, r, "/settings/account", http.StatusSeeOther)
	}
}

package admin

import (
	"encoding/json"
	"net/http"
	"strings"

	"breadbox/internal/app"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// MembersPageHandler serves GET /users (updated to include member accounts tab).
// This is kept as the enhanced users page — member account management is shown alongside
// existing family member management.

// createMemberAccountRequest is the JSON body for POST /-/members.
type createMemberAccountRequest struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// CreateMemberAccountHandler serves POST /-/members — create a member account.
func CreateMemberAccountHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createMemberAccountRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		req.Username = strings.TrimSpace(req.Username)
		if req.Username == "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error": map[string]string{"code": "VALIDATION_ERROR", "message": "Username is required"},
			})
			return
		}

		if len(req.Username) > 64 {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error": map[string]string{"code": "VALIDATION_ERROR", "message": "Username must be 64 characters or fewer"},
			})
			return
		}

		if req.UserID == "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error": map[string]string{"code": "VALIDATION_ERROR", "message": "Family member (user_id) is required"},
			})
			return
		}

		if req.Role == "" {
			req.Role = "member"
		}

		member, err := svc.CreateMemberAccount(r.Context(), service.CreateMemberAccountParams{
			UserID:   req.UserID,
			Username: req.Username,
			Role:     req.Role,
		})
		if err != nil {
			if strings.Contains(err.Error(), "username already taken") {
				writeJSON(w, http.StatusConflict, map[string]any{
					"error": map[string]string{"code": "USERNAME_TAKEN", "message": "This username is already in use"},
				})
				return
			}
			if strings.Contains(err.Error(), "already has a member account") {
				writeJSON(w, http.StatusConflict, map[string]any{
					"error": map[string]string{"code": "DUPLICATE_MEMBER", "message": "This family member already has a login account"},
				})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create member account"})
			return
		}

		writeJSON(w, http.StatusCreated, member)
	}
}

// ListMemberAccountsHandler serves GET /-/members — list all member accounts.
func ListMemberAccountsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		members, err := svc.ListMemberAccounts(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list member accounts"})
			return
		}
		writeJSON(w, http.StatusOK, members)
	}
}

// UpdateMemberRoleHandler serves PUT /-/members/{id}/role.
func UpdateMemberRoleHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var req struct {
			Role string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		if err := svc.UpdateMemberRole(r.Context(), id, req.Role); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error": map[string]string{"code": "VALIDATION_ERROR", "message": err.Error()},
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// DeleteMemberAccountHandler serves DELETE /-/members/{id}.
func DeleteMemberAccountHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		if err := svc.DeleteMemberAccount(r.Context(), id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete member account"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// WipeUserDataHandler serves POST /-/users/{id}/wipe — delete all connections and transactions for a user.
func WipeUserDataHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		txnCount, err := a.Service.WipeUserData(r.Context(), id)
		if err != nil {
			a.Logger.Error("wipe user data", "error", err, "user_id", id)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to wipe user data"})
			return
		}

		SetFlash(r.Context(), sm, "success", "User data wiped successfully.")
		writeJSON(w, http.StatusOK, map[string]any{
			"status":              "ok",
			"transactions_deleted": txnCount,
		})
	}
}

// MyAccountHandler serves GET /my-account — member's own account page.
func MyAccountHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		memberIDStr := SessionMemberID(sm, r)
		userIDStr := SessionUserID(sm, r)

		data := BaseTemplateData(r, sm, "my-account", "My Account")
		data["MemberID"] = memberIDStr
		data["UserID"] = userIDStr

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

// MyAccountChangePasswordHandler serves POST /my-account/password — member changes their own password.
func MyAccountChangePasswordHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		memberIDStr := SessionMemberID(sm, r)
		if memberIDStr == "" {
			SetFlash(ctx, sm, "error", "Invalid session.")
			http.Redirect(w, r, "/my-account", http.StatusSeeOther)
			return
		}

		changePasswordFromMember(a, sm, w, r, memberIDStr, "/my-account")
	}
}

// MyAccountWipeDataHandler serves POST /my-account/wipe-data — member wipes their own data.
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

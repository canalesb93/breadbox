package api

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"strings"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// Login-account CRUD endpoints. A "login account" is the auth identity that
// can sign into the admin UI; it's linked to a household member (user). All
// endpoints in this file are write-scope — listing login accounts lets a
// leaked key enumerate sign-in identities, so it's intentionally not exposed
// to read-only keys.
//
// The setup_token returned on create + regenerate is the one-time secret the
// member uses to set their initial password. It is never echoed by the list
// endpoint and cannot be retrieved later — if lost, regenerate.

// createLoginRequest is the JSON body for POST /users/{user_id}/login.
type createLoginRequest struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

// updateLoginRequest is the JSON body for PATCH /users/{user_id}/login/{login_id}.
type updateLoginRequest struct {
	Role string `json:"role"`
}

// ListUserLoginsHandler returns the login account(s) attached to a user.
// GET /api/v1/users/{user_id}/login
//
// Returns a bare JSON array. setup_token is never included — it only appears
// at create + regenerate.
func ListUserLoginsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDParam := chi.URLParam(r, "user_id")

		// Resolve and verify the user exists; this gives us the canonical
		// UUID to filter by.
		user, err := svc.GetUser(r.Context(), userIDParam)
		if err != nil {
			writeServiceError(w, err, "User not found", "Failed to look up user")
			return
		}

		all, err := svc.ListLoginAccounts(r.Context())
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list login accounts")
			return
		}

		filtered := make([]service.LoginAccountResponse, 0, 1)
		for _, acct := range all {
			if acct.UserID != user.ID {
				continue
			}
			// Belt-and-suspenders: setup_token is *not* part of the
			// list contract. Strip it even if the underlying query
			// happens to surface it.
			acct.SetupToken = ""
			acct.SetupTokenExpiresAt = nil
			filtered = append(filtered, acct)
		}

		writeData(w, filtered)
	}
}

// CreateUserLoginHandler creates a new login account for an existing user.
// POST /api/v1/users/{user_id}/login
//
// Response includes the plaintext setup_token — this is the only time it is
// exposed. The member redeems it via the /setup-password flow.
func CreateUserLoginHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDParam := chi.URLParam(r, "user_id")

		var req createLoginRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		req.Username = strings.TrimSpace(req.Username)
		if req.Username == "" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "username is required")
			return
		}
		if len(req.Username) > 64 {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "username must be 64 characters or fewer")
			return
		}
		// The admin UI uses email-as-username; mirror that validation here so
		// the headless contract stays consistent with the canonical flow.
		if _, err := mail.ParseAddress(req.Username); err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "username must be a valid email address")
			return
		}

		if req.Role == "" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "role is required (admin, editor, or viewer)")
			return
		}

		// Verify the user exists up front so we return a clean 404 instead
		// of a 400 wrapped around the service-layer "invalid user_id" error.
		user, err := svc.GetUser(r.Context(), userIDParam)
		if err != nil {
			writeServiceError(w, err, "User not found", "Failed to look up user")
			return
		}

		account, err := svc.CreateLoginAccount(r.Context(), service.CreateLoginAccountParams{
			UserID:   user.ID,
			Username: req.Username,
			Role:     req.Role,
		})
		if err != nil {
			writeLoginCreateError(w, err)
			return
		}

		writeJSON(w, http.StatusCreated, account)
	}
}

// UpdateUserLoginHandler updates the role on an existing login account.
// PATCH /api/v1/users/{user_id}/login/{login_id}
func UpdateUserLoginHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDParam := chi.URLParam(r, "user_id")
		loginID := chi.URLParam(r, "login_id")

		var req updateLoginRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		// Confirm the login exists and belongs to this user so we return
		// 404 on a typoed user_id rather than silently mutating an
		// unrelated login.
		acct, err := findLoginForUser(r.Context(), svc, userIDParam, loginID)
		if err != nil {
			writeServiceError(w, err, "Login account not found", "Failed to look up login account")
			return
		}

		if err := svc.UpdateLoginAccountRole(r.Context(), acct.ID, req.Role); err != nil {
			if strings.Contains(err.Error(), "invalid role") {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update login account")
			return
		}

		// Re-read so the response reflects the updated role.
		updated, err := findLoginForUser(r.Context(), svc, userIDParam, loginID)
		if err != nil {
			writeServiceError(w, err, "Login account not found", "Failed to re-read login account")
			return
		}
		updated.SetupToken = ""
		updated.SetupTokenExpiresAt = nil
		writeData(w, updated)
	}
}

// DeleteUserLoginHandler removes a login account.
// DELETE /api/v1/users/{user_id}/login/{login_id}
//
// Does not delete the linked household member.
func DeleteUserLoginHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDParam := chi.URLParam(r, "user_id")
		loginID := chi.URLParam(r, "login_id")

		acct, err := findLoginForUser(r.Context(), svc, userIDParam, loginID)
		if err != nil {
			writeServiceError(w, err, "Login account not found", "Failed to look up login account")
			return
		}

		if err := svc.DeleteLoginAccount(r.Context(), acct.ID); err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete login account")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// RegenerateLoginTokenHandler issues a new setup token for an existing login
// that has not yet had a password set. The previous token is invalidated.
// POST /api/v1/users/{user_id}/login/{login_id}/regenerate-token
//
// Like create, the plaintext token is returned exactly once.
func RegenerateLoginTokenHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDParam := chi.URLParam(r, "user_id")
		loginID := chi.URLParam(r, "login_id")

		acct, err := findLoginForUser(r.Context(), svc, userIDParam, loginID)
		if err != nil {
			writeServiceError(w, err, "Login account not found", "Failed to look up login account")
			return
		}

		token, err := svc.RegenerateSetupToken(r.Context(), acct.ID)
		if err != nil {
			if strings.Contains(err.Error(), "already has a password") {
				mw.WriteError(w, http.StatusConflict, "PASSWORD_ALREADY_SET", "This login already has a password set; cannot regenerate setup token")
				return
			}
			if strings.Contains(err.Error(), "account not found") {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Login account not found")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to regenerate setup token")
			return
		}

		writeData(w, map[string]any{
			"setup_token": token,
		})
	}
}

// findLoginForUser returns the login account identified by loginID, but only
// if it belongs to userID. Returns service.ErrNotFound when either side
// doesn't match so callers can lean on the canonical 404 path.
func findLoginForUser(ctx context.Context, svc *service.Service, userIDParam, loginID string) (*service.LoginAccountResponse, error) {
	user, err := svc.GetUser(ctx, userIDParam)
	if err != nil {
		return nil, err
	}

	all, err := svc.ListLoginAccounts(ctx)
	if err != nil {
		return nil, err
	}
	for _, acct := range all {
		if acct.UserID != user.ID {
			continue
		}
		if acct.ID != loginID {
			continue
		}
		out := acct
		return &out, nil
	}
	return nil, service.ErrNotFound
}

// writeLoginCreateError maps service-layer create errors to the canonical
// error envelope. The service surfaces conflicts and validation failures via
// fmt.Errorf strings (no sentinels), so we string-match the same way the
// admin handler does.
func writeLoginCreateError(w http.ResponseWriter, err error) {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "username already taken"):
		mw.WriteError(w, http.StatusConflict, "USERNAME_TAKEN", "A login with that username already exists")
	case strings.Contains(msg, "already has a login account"):
		mw.WriteError(w, http.StatusConflict, "LOGIN_EXISTS", "This user already has a login account")
	case strings.Contains(msg, "invalid role"):
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", msg)
	case strings.Contains(msg, "invalid user_id"), errors.Is(err, service.ErrNotFound):
		mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "User not found")
	default:
		mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create login account")
	}
}

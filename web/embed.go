//go:build !headless && !lite

// Package webui embeds the v2 SPA build (web/dist) and serves it under /v2/*.
//
// When the bundle hasn't been built (only web/dist/.gitkeep present),
// Handler() returns a stub explaining how to build it instead of 404ing.
package webui

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"breadbox/internal/admin"
	"breadbox/internal/db"
	mw "breadbox/internal/middleware"
	"breadbox/internal/pgconv"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
)

//go:embed all:dist
var distFS embed.FS

const stubHTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <title>Breadbox v2 — build missing</title>
    <style>
      body { font-family: ui-sans-serif, system-ui, sans-serif; padding: 48px; max-width: 640px; margin: 0 auto; color: #1c1917; }
      code { background: #f5f5f4; padding: 2px 6px; border-radius: 4px; font-size: 0.9em; }
      h1 { font-size: 22px; margin-bottom: 8px; }
      p { color: #57534e; line-height: 1.6; }
    </style>
  </head>
  <body>
    <h1>v2 SPA bundle missing</h1>
    <p>The Go binary was built without a v2 bundle. Run <code>make build</code> (which runs <code>bun run build</code> first) to produce a binary that serves the real SPA at <code>/v2/</code>.</p>
    <p>For development, run <code>cd web && bun run dev</code> in a second terminal — the Vite dev server (5173) proxies API calls to this Go server and supports HMR.</p>
    <p><a href="/">← Back to classic admin UI</a></p>
  </body>
</html>`

// Handler returns the v2 SPA static handler with SPA fallback. Mount under
// /v2/. Extension-less paths are rewritten to / so the client router resolves
// them. If the bundle is absent, every request gets a stub HTML page.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}

	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(stubHTML))
		})
	}

	fileServer := http.StripPrefix("/v2", http.FileServer(http.FS(sub)))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trimmed := strings.TrimPrefix(r.URL.Path, "/v2")
		// Extension-less paths under /v2/* are client routes — rewrite to /
		// so the file server returns index.html.
		if trimmed != "" && trimmed != "/" && path.Ext(trimmed) == "" {
			r.URL.Path = "/v2/"
		}
		// Long-cache hashed assets (Vite content-hashes filenames).
		if strings.HasPrefix(trimmed, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fileServer.ServeHTTP(w, r)
	})
}

// MeResponse is the shape returned by GET /web/v1/me. Internal to the SPA;
// no stability promise.
type MeResponse struct {
	AccountID string `json:"account_id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
}

// MeHandler returns the current admin from the session. RequireSessionJSON
// must run before this.
func MeHandler(sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mw.WriteJSON(w, http.StatusOK, MeResponse{
			AccountID: admin.SessionAccountID(sm, r),
			Username:  admin.SessionUsername(sm, r),
			Role:      admin.SessionRole(sm, r),
		})
	}
}

// RequireSessionJSON gates /web/v1/* endpoints behind a session cookie.
// Returns JSON 401 instead of redirecting to /login (which would break a
// JSON fetch — the SPA handles redirect on its own).
func RequireSessionJSON(sm *scs.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if admin.SessionAccountID(sm, r) == "" {
				mw.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Session required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireSameOrigin is the CSRF strategy for /web/v1/* writes: an
// unsafe-method request must pass the shared SameSite=Lax + Origin check
// (mw.SameOrigin). Same-origin SPA + Lax cookies make this sufficient — no
// double-submit token needed.
func RequireSameOrigin() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if mw.IsUnsafeMethod(r.Method) && !mw.SameOrigin(r) {
				mw.WriteError(w, http.StatusForbidden, "ORIGIN_MISMATCH", "Cross-origin request rejected")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// LoginRequest is the POST body for /web/v1/login.
type LoginRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	RememberMe bool   `json:"remember_me"`
}

// LoginHandler authenticates against auth_accounts and sets the session
// keys the rest of the dashboard expects. JSON twin of admin.LoginHandler.
func LoginHandler(sm *scs.SessionManager, queries *db.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Request body must be valid JSON")
			return
		}
		if req.Username == "" || req.Password == "" {
			bcrypt.CompareHashAndPassword(loginDummyHash, []byte(req.Password))
			mw.WriteError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password")
			return
		}

		account, err := queries.GetAuthAccountByUsername(r.Context(), req.Username)
		if err != nil {
			bcrypt.CompareHashAndPassword(loginDummyHash, []byte(req.Password))
			mw.WriteError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password")
			return
		}
		if account.HashedPassword == nil {
			bcrypt.CompareHashAndPassword(loginDummyHash, []byte(req.Password))
			mw.WriteError(w, http.StatusUnauthorized, "ACCOUNT_NOT_SETUP", "Your account hasn't been set up yet. Ask your administrator for a setup link.")
			return
		}
		if err := bcrypt.CompareHashAndPassword(account.HashedPassword, []byte(req.Password)); err != nil {
			mw.WriteError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password")
			return
		}

		if err := sm.RenewToken(r.Context()); err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "SESSION_ERROR", "Failed to renew session")
			return
		}
		sm.RememberMe(r.Context(), req.RememberMe)
		admin.SetLoginSessionKeys(r.Context(), sm, account, queries)

		mw.WriteJSON(w, http.StatusOK, MeResponse{
			AccountID: pgconv.FormatUUID(account.ID),
			Username:  account.Username,
			Role:      account.Role,
		})
	}
}

// LogoutHandler destroys the current session.
func LogoutHandler(sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := sm.Destroy(r.Context()); err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "SESSION_ERROR", "Failed to destroy session")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ChangePasswordRequest is the POST body for /web/v1/account/password.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
	ConfirmPassword string `json:"confirm_password"`
}

// ChangePasswordHandler updates the current session's password. Mirrors
// admin.changePasswordForAccount validation rules: current password must
// verify, new must be 8+ chars and match confirm.
func ChangePasswordHandler(sm *scs.SessionManager, queries *db.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountIDStr := admin.SessionAccountID(sm, r)
		if accountIDStr == "" {
			mw.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Session required")
			return
		}
		var accountID pgtype.UUID
		if err := accountID.Scan(accountIDStr); err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_SESSION", "Invalid session")
			return
		}

		var req ChangePasswordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Request body must be valid JSON")
			return
		}

		account, err := queries.GetAuthAccountByID(r.Context(), accountID)
		if err != nil {
			mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Account not found")
			return
		}
		if account.HashedPassword == nil {
			mw.WriteError(w, http.StatusUnprocessableEntity, "ACCOUNT_NOT_SETUP", "No password set. Contact your administrator.")
			return
		}
		if err := bcrypt.CompareHashAndPassword(account.HashedPassword, []byte(req.CurrentPassword)); err != nil {
			mw.WriteError(w, http.StatusUnauthorized, "INVALID_CURRENT_PASSWORD", "Current password is incorrect")
			return
		}
		if len(req.NewPassword) < 8 {
			mw.WriteError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "New password must be at least 8 characters")
			return
		}
		if req.NewPassword != req.ConfirmPassword {
			mw.WriteError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "New passwords do not match")
			return
		}

		hashed, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), 12)
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "HASH_ERROR", "Failed to hash password")
			return
		}
		if err := queries.UpdateAuthAccountPassword(r.Context(), db.UpdateAuthAccountPasswordParams{
			ID:             accountID,
			HashedPassword: hashed,
		}); err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "UPDATE_FAILED", "Failed to update password")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// loginDummyHash mirrors admin.dummyHash — used for constant-time login
// responses against unknown usernames so timing can't be used for
// enumeration.
var loginDummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), 12)

// SetupAccountInfoResponse is the GET shape for /web/v1/setup-account/{token}:
// just enough for the SPA to greet the new member with their email.
type SetupAccountInfoResponse struct {
	Username string `json:"username"`
}

// SetupAccountInfoHandler validates a setup token and returns the username
// the SPA should display. Pre-auth (no session required) — the token *is*
// the credential. Returns 404 for unknown/expired tokens; 410 GONE when the
// password has already been set so the SPA can route the visitor to /login.
func SetupAccountInfoHandler(queries *db.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "token")
		account, ok := lookupSetupToken(w, r, queries, token)
		if !ok {
			return
		}
		mw.WriteJSON(w, http.StatusOK, SetupAccountInfoResponse{Username: account.Username})
	}
}

// SetupAccountRequest is the POST body for /web/v1/setup-account/{token}.
type SetupAccountRequest struct {
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirm_password"`
}

// SetupAccountHandler consumes a setup token, hashes + stores the password,
// clears the token, and opens a session so the SPA can route the visitor
// straight into /v2/ without a second login round-trip. Pre-auth.
func SetupAccountHandler(sm *scs.SessionManager, queries *db.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "token")
		account, ok := lookupSetupToken(w, r, queries, token)
		if !ok {
			return
		}

		var req SetupAccountRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Request body must be valid JSON")
			return
		}
		if len(req.Password) < 8 {
			mw.WriteError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Password must be at least 8 characters")
			return
		}
		if req.Password != req.ConfirmPassword {
			mw.WriteError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Passwords do not match")
			return
		}

		hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "HASH_ERROR", "Failed to hash password")
			return
		}
		if err := queries.UpdateAuthAccountPassword(r.Context(), db.UpdateAuthAccountPasswordParams{
			ID:             account.ID,
			HashedPassword: hashed,
		}); err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "UPDATE_FAILED", "Failed to set password")
			return
		}
		_ = queries.ClearAuthAccountSetupToken(r.Context(), account.ID)

		// Re-read so the session keys reflect the freshly-set password and the
		// row has no stale token state. Mirrors the LoginHandler path.
		fresh, err := queries.GetAuthAccountByID(r.Context(), account.ID)
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Account refresh failed")
			return
		}
		if err := sm.RenewToken(r.Context()); err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "SESSION_ERROR", "Failed to start session")
			return
		}
		admin.SetLoginSessionKeys(r.Context(), sm, fresh, queries)

		mw.WriteJSON(w, http.StatusOK, MeResponse{
			AccountID: pgconv.FormatUUID(fresh.ID),
			Username:  fresh.Username,
			Role:      fresh.Role,
		})
	}
}

// lookupSetupToken resolves the URL `token` parameter to an `auth_accounts`
// row, writing the canonical error envelope (and HTTP status) if anything is
// off so callers can early-return.
//
// Returns ok=false in three cases the SPA should distinguish:
//   - 400 BAD_REQUEST: empty token (almost always a SPA bug).
//   - 410 GONE / ALREADY_SETUP: token row exists but password is already
//     stored — the SPA reads this and bounces to /login instead of showing
//     the form a second time.
//   - 404 NOT_FOUND / SETUP_TOKEN_INVALID: unknown token, or expired. We
//     collapse these to one code on purpose so a malicious caller can't tell
//     "expired" from "never existed".
func lookupSetupToken(w http.ResponseWriter, r *http.Request, queries *db.Queries, token string) (db.AuthAccount, bool) {
	if token == "" {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "Missing setup token")
		return db.AuthAccount{}, false
	}
	account, err := queries.GetAuthAccountBySetupToken(r.Context(), pgconv.Text(token))
	if err != nil {
		mw.WriteError(w, http.StatusNotFound, "SETUP_TOKEN_INVALID", "This setup link is invalid or has expired.")
		return db.AuthAccount{}, false
	}
	if account.SetupTokenExpiresAt.Valid && account.SetupTokenExpiresAt.Time.Before(time.Now()) {
		mw.WriteError(w, http.StatusNotFound, "SETUP_TOKEN_INVALID", "This setup link is invalid or has expired.")
		return db.AuthAccount{}, false
	}
	if account.HashedPassword != nil {
		mw.WriteError(w, http.StatusGone, "ALREADY_SETUP", "This account is already set up. Please sign in.")
		return db.AuthAccount{}, false
	}
	return account, true
}

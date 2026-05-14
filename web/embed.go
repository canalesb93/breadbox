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
	"net/url"
	"path"
	"strings"

	"breadbox/internal/admin"
	"breadbox/internal/db"
	mw "breadbox/internal/middleware"
	"breadbox/internal/pgconv"

	"github.com/alexedwards/scs/v2"
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

// RequireSameOrigin is the CSRF strategy for /web/v1/* writes. On any
// unsafe-method request (POST/PUT/PATCH/DELETE), the Origin (or Referer
// fallback) host must match the request host. SameSite=Lax cookies +
// same-origin SPA make this sufficient — no double-submit token needed.
//
// Browsers send Origin on every cross-origin and same-origin POST, so the
// only legitimate case where Origin is absent is older clients or non-CORS
// fetches; we accept Referer as a fallback there.
func RequireSameOrigin() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}
			if !sameOrigin(r) {
				mw.WriteError(w, http.StatusForbidden, "ORIGIN_MISMATCH", "Cross-origin request rejected")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func sameOrigin(r *http.Request) bool {
	got := r.Header.Get("Origin")
	if got == "" {
		got = r.Header.Get("Referer")
	}
	if got == "" {
		// No Origin and no Referer — refuse rather than guess. Modern browsers
		// always send one on a POST; missing both indicates a non-browser
		// caller that should use the public /api/v1 + API key surface instead.
		return false
	}
	u, err := url.Parse(got)
	if err != nil {
		return false
	}
	return u.Host == r.Host
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

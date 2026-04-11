package admin

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/avatar"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const maxAvatarUploadSize = 5 << 20 // 5 MB

// AvatarHandler serves GET /avatars/{id} — returns uploaded image or generated SVG.
// Looks up the user by UUID. If not found, generates a pattern from the raw ID string
// (covers unlinked admin accounts whose session falls back to account UUID).
func AvatarHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")

		uid, err := resolveUUID(idStr)
		if err != nil {
			serveGeneratedAvatar(w, r, idStr)
			return
		}

		row, err := a.Queries.GetUserAvatar(r.Context(), uid)
		if err != nil {
			// Not a user — generate a stable pattern from the raw ID.
			serveGeneratedAvatar(w, r, idStr)
			return
		}

		if row.AvatarData != nil && row.AvatarContentType.Valid {
			serveUploadedAvatar(w, r, row.AvatarData, row.AvatarContentType.String)
			return
		}

		seed := pgconv.FormatUUID(uid)
		if row.AvatarSeed.Valid && row.AvatarSeed.String != "" {
			seed = row.AvatarSeed.String
		}
		serveGeneratedAvatar(w, r, seed)
	}
}

// AvatarPreviewHandler serves GET /avatars/preview/{seed} — generates a pattern preview.
// Used by the create member form to show a live preview before the user exists.
func AvatarPreviewHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seed := chi.URLParam(r, "seed")
		if seed == "" {
			seed = "default"
		}
		serveGeneratedAvatar(w, r, seed)
	}
}

func serveUploadedAvatar(w http.ResponseWriter, r *http.Request, data []byte, contentType string) {
	etag := fmt.Sprintf(`"%x"`, sha256.Sum256(data))
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("ETag", etag)
	w.Write(data)
}

func serveGeneratedAvatar(w http.ResponseWriter, r *http.Request, seed string) {
	svg := avatar.GenerateSVG(seed, 256)
	etag := fmt.Sprintf(`"%x"`, sha256.Sum256(svg))
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("ETag", etag)
	w.Write(svg)
}

func resolveUUID(idStr string) (pgtype.UUID, error) {
	var uid pgtype.UUID
	if err := uid.Scan(idStr); err != nil {
		return pgtype.UUID{}, err
	}
	return uid, nil
}

// --- Admin avatar management (for any user) ---

// UploadUserAvatarHandler serves POST /-/users/{id}/avatar.
func UploadUserAvatarHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := parseUserID(w, r)
		if !ok {
			return
		}
		processAndStoreAvatar(a, w, r, userID)
	}
}

// DeleteUserAvatarHandler serves DELETE /-/users/{id}/avatar.
func DeleteUserAvatarHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := parseUserID(w, r)
		if !ok {
			return
		}
		if err := a.Queries.ClearUserAvatar(r.Context(), userID); err != nil {
			a.Logger.Error("clear avatar", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to remove avatar"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// RegenerateUserAvatarHandler serves POST /-/users/{id}/avatar/regenerate.
func RegenerateUserAvatarHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := parseUserID(w, r)
		if !ok {
			return
		}
		seed := randomSeed()
		if err := a.Queries.ClearUserAvatar(r.Context(), userID); err != nil {
			a.Logger.Error("clear avatar for regenerate", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to regenerate"})
			return
		}
		if err := a.Queries.SetUserAvatarSeed(r.Context(), db.SetUserAvatarSeedParams{
			ID: userID, AvatarSeed: pgtype.Text{String: seed, Valid: true},
		}); err != nil {
			a.Logger.Error("set avatar seed", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to regenerate"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "seed": seed})
	}
}

// --- Self-service avatar (requires linked user) ---

// UploadMyAvatarHandler serves POST /my-account/avatar.
func UploadMyAvatarHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := requireSessionUser(w, sm, r)
		if !ok {
			return
		}
		processAndStoreAvatar(a, w, r, userID)
		bumpAvatarVersion(sm, r)
	}
}

// DeleteMyAvatarHandler serves DELETE /my-account/avatar.
func DeleteMyAvatarHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := requireSessionUser(w, sm, r)
		if !ok {
			return
		}
		if err := a.Queries.ClearUserAvatar(r.Context(), userID); err != nil {
			a.Logger.Error("clear avatar", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to remove avatar"})
			return
		}
		bumpAvatarVersion(sm, r)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// RegenerateMyAvatarHandler serves POST /my-account/avatar/regenerate.
func RegenerateMyAvatarHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := requireSessionUser(w, sm, r)
		if !ok {
			return
		}
		seed := randomSeed()
		if err := a.Queries.ClearUserAvatar(r.Context(), userID); err != nil {
			a.Logger.Error("clear avatar for regenerate", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to regenerate"})
			return
		}
		if err := a.Queries.SetUserAvatarSeed(r.Context(), db.SetUserAvatarSeedParams{
			ID: userID, AvatarSeed: pgtype.Text{String: seed, Valid: true},
		}); err != nil {
			a.Logger.Error("set avatar seed", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to regenerate"})
			return
		}
		bumpAvatarVersion(sm, r)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "seed": seed})
	}
}

// --- Helpers ---

func parseUserID(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "id")); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid user ID"})
		return pgtype.UUID{}, false
	}
	return userID, true
}

func requireSessionUser(w http.ResponseWriter, sm *scs.SessionManager, r *http.Request) (pgtype.UUID, bool) {
	uid := SessionUserID(sm, r)
	if uid == "" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Account must be linked to a household member to manage avatars"})
		return pgtype.UUID{}, false
	}
	var userID pgtype.UUID
	if err := userID.Scan(uid); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Invalid session"})
		return pgtype.UUID{}, false
	}
	return userID, true
}

// bumpAvatarVersion updates the session's avatar version so the sidebar
// avatar URL fingerprint changes, busting the browser cache.
func bumpAvatarVersion(sm *scs.SessionManager, r *http.Request) {
	sm.Put(r.Context(), sessionKeyAvatarVersion, strconv.FormatInt(time.Now().Unix(), 10))
}

func processAndStoreAvatar(a *app.App, w http.ResponseWriter, r *http.Request, userID pgtype.UUID) {
	processed, ct, ok := parseAvatarUpload(a, w, r)
	if !ok {
		return
	}
	if err := a.Queries.SetUserAvatar(r.Context(), db.SetUserAvatarParams{
		ID:                userID,
		AvatarData:        processed,
		AvatarContentType: pgtype.Text{String: ct, Valid: true},
	}); err != nil {
		a.Logger.Error("store avatar", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to save avatar"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "avatar_url": "/avatars/" + pgconv.FormatUUID(userID)})
}

func parseAvatarUpload(a *app.App, w http.ResponseWriter, r *http.Request) ([]byte, string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAvatarUploadSize)
	if err := r.ParseMultipartForm(maxAvatarUploadSize); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "File too large (max 5MB)"})
		return nil, "", false
	}
	file, header, err := r.FormFile("avatar")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "No file uploaded"})
		return nil, "", false
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if contentType != "image/png" && contentType != "image/jpeg" && contentType != "image/gif" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Unsupported image type. Use PNG, JPEG, or GIF."})
		return nil, "", false
	}
	data, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Failed to read file"})
		return nil, "", false
	}
	processed, ct, err := avatar.ProcessUpload(data, contentType)
	if err != nil {
		a.Logger.Error("process avatar", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Failed to process image"})
		return nil, "", false
	}
	return processed, ct, true
}

func randomSeed() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

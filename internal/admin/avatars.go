//go:build !headless && !lite

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
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const maxAvatarUploadSize = 5 << 20 // 5 MB

// AvatarHandler serves GET /avatars/{id} — returns uploaded image or generated SVG.
// Lookup order: users.id → api_keys.id → raw-seed fallback. The api_keys
// branch lets agent identities (HTTP MCP keys, future per-client stdio
// keys) render with the agent-style DiceBear even when the URL was
// built from just an actor_id (notification deep-links, embedded
// avatars) and doesn't carry an explicit ?type=agent param.
//
// Query params:
//
//	?type=user|agent — picks which configured DiceBear style to use.
//	                   Default "user". Agent identicons use a separate
//	                   style (default "bottts-neutral") so agent activity reads
//	                   as obviously non-human. When the id resolves to
//	                   an api_keys row with actor_type='agent', the
//	                   handler upgrades the style to "agent" regardless
//	                   of what ?type= said.
//	?size=N          — requested pixel size; clamped to [32, 512].
//	                   Passed through to DiceBear so big tiles fetch
//	                   crisper SVGs and small tiles fetch lighter ones.
func AvatarHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := clampSeed(chi.URLParam(r, "id"))
		actor := parseActorType(r)
		size := parseAvatarSize(r)

		uid, err := resolveUUID(idStr)
		if err != nil {
			serveGeneratedAvatarForActor(w, r, idStr, actor, size)
			return
		}

		row, err := a.Queries.GetUserAvatar(r.Context(), uid)
		if err == nil {
			if row.AvatarData != nil && row.AvatarContentType.Valid {
				serveUploadedAvatar(w, r, row.AvatarData, row.AvatarContentType.String)
				return
			}
			seed := pgconv.FormatUUID(uid)
			if row.AvatarSeed.Valid && row.AvatarSeed.String != "" {
				seed = row.AvatarSeed.String
			}
			serveGeneratedAvatarForActor(w, r, seed, actor, size)
			return
		}

		// Not a user — try api_keys next. Annotations written by
		// agents store the api_keys row UUID in actor_id and avatar
		// URLs are built from it. When the row is an agent key,
		// upgrade the actor type so the agent DiceBear style wins
		// regardless of the URL's ?type= param.
		//
		// Every run mints a fresh per-run key (name "agent:<slug>:<runID>"),
		// so seeding on the key UUID would give the same agent a different
		// robot on every run. Instead we recover the agent's stable slug
		// from the key name and seed on that — so an agent's activity rows
		// share one avatar that matches its /agents/<slug> profile. Keys
		// whose name isn't in that shape (HTTP MCP keys, the stdio
		// singleton) fall back to the key UUID seed.
		if key, kerr := a.Queries.GetApiKey(r.Context(), uid); kerr == nil {
			seed := idStr
			if key.ActorType == "agent" {
				actor = avatar.ActorAgent
				if slug, ok := service.ParseAgentKeySlug(key.Name); ok {
					seed = slug
				}
			}
			serveGeneratedAvatarForActor(w, r, seed, actor, size)
			return
		}

		// Unknown id — generate a stable pattern from the raw string.
		serveGeneratedAvatarForActor(w, r, idStr, actor, size)
	}
}

// parseActorType reads the `?type=` query param and normalises it
// into an avatar.ActorType. Anything other than "agent" falls back
// to ActorUser — the safer default because the previous single-style
// behavior was user-side.
func parseActorType(r *http.Request) avatar.ActorType {
	if r.URL.Query().Get("type") == string(avatar.ActorAgent) {
		return avatar.ActorAgent
	}
	return avatar.ActorUser
}

// parseAvatarSize reads the `?size=` query param and clamps it into
// the supported range. Returns 256 (the legacy default) when the
// param is missing or invalid. Bounded to [32, 512] so an attacker
// can't fan out cache entries by spamming arbitrary sizes — the
// component-side enum only emits 16, 20, 24, 32, 40, 48, 56, 256.
func parseAvatarSize(r *http.Request) int {
	raw := r.URL.Query().Get("size")
	if raw == "" {
		return 256
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 256
	}
	if n < 32 {
		return 32
	}
	if n > 512 {
		return 512
	}
	return n
}

// clampSeed bounds an attacker-supplied path segment to MaxSeedLength
// so unauthenticated `/avatars/<long-string>` requests can't blow up
// the upstream DiceBear URL or the cache key. We keep the relaxed
// charset for back-compat with embed contexts that use arbitrary
// IDs as identicon seeds; cache capping handles memory pressure.
func clampSeed(s string) string {
	if len(s) > avatar.MaxSeedLength {
		return s[:avatar.MaxSeedLength]
	}
	return s
}

// AvatarPreviewHandler serves GET /avatars/preview/{seed} — generates
// a pattern preview. Used by the create-member form and by the
// Settings → System style picker (via ?style=<dicebear-id>).
//
// The route is unauthenticated so the preview tiles render before
// the user is logged in. Seeds and style overrides are validated to
// bound the upstream URL + cache key; arbitrary input → 400.
func AvatarPreviewHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seed := chi.URLParam(r, "seed")
		if seed == "" {
			seed = "default"
		}
		if !avatar.IsValidSeed(seed) {
			http.Error(w, "invalid seed", http.StatusBadRequest)
			return
		}
		styleOverride := r.URL.Query().Get("style")
		if styleOverride != "" && !avatar.IsValidStyle(styleOverride) {
			http.Error(w, "invalid style", http.StatusBadRequest)
			return
		}
		serveGeneratedAvatar(w, r, seed, styleOverride, parseAvatarSize(r))
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

// serveGeneratedAvatar renders a DiceBear SVG with an explicit style
// override + size. Used by the settings preview tiles to render each
// style on demand; empty styleOverride falls back to the configured
// user style.
//
// Cache-Control is intentionally shorter than the uploaded-avatar
// path: generated SVGs flip when the operator changes the style and
// can fall back to the placeholder during a transient DiceBear
// outage. 1h fresh + must-revalidate keeps the bytes hot for normal
// traffic but lets a recovery propagate within the hour instead of
// pinning the placeholder for a full day.
func serveGeneratedAvatar(w http.ResponseWriter, r *http.Request, seed, styleOverride string, size int) {
	svg := avatar.GenerateSVGStyled(seed, size, styleOverride)
	writeAvatarSVG(w, r, svg)
}

// serveGeneratedAvatarForActor is the actor-typed variant — the URL
// carried `?type=user|agent` so we look up the right configured
// style rather than passing a literal slug. Threaded through from
// /avatars/{id} where the styleOverride knob isn't exposed.
func serveGeneratedAvatarForActor(w http.ResponseWriter, r *http.Request, seed string, actor avatar.ActorType, size int) {
	svg := avatar.GenerateSVGForActor(seed, size, actor)
	writeAvatarSVG(w, r, svg)
}

// writeAvatarSVG owns the ETag + Cache-Control headers shared by both
// serve paths. Lives separately so adding a new render path (e.g. a
// signed-URL variant in the future) doesn't accidentally diverge the
// cache contract.
func writeAvatarSVG(w http.ResponseWriter, r *http.Request, svg []byte) {
	etag := fmt.Sprintf(`"%x"`, sha256.Sum256(svg))
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=3600, must-revalidate")
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
		userID, ok := parseURLUUIDOrInvalid(w, r, "id", "Invalid user ID")
		if !ok {
			return
		}
		processAndStoreAvatar(a, w, r, userID)
	}
}

// DeleteUserAvatarHandler serves DELETE /-/users/{id}/avatar.
func DeleteUserAvatarHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := parseURLUUIDOrInvalid(w, r, "id", "Invalid user ID")
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
		userID, ok := parseURLUUIDOrInvalid(w, r, "id", "Invalid user ID")
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
			ID: userID, AvatarSeed: pgconv.Text(seed),
		}); err != nil {
			a.Logger.Error("set avatar seed", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to regenerate"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "seed": seed})
	}
}

// --- Self-service avatar (requires linked user) ---

// UploadMyAvatarHandler serves POST /settings/account/avatar.
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

// DeleteMyAvatarHandler serves DELETE /settings/account/avatar.
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

// RegenerateMyAvatarHandler serves POST /settings/account/avatar/regenerate.
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
			ID: userID, AvatarSeed: pgconv.Text(seed),
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
		AvatarContentType: pgconv.Text(ct),
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

//go:build !lite

package api

import (
	"net/http"
	"strings"

	"breadbox/internal/app"
	"breadbox/internal/db"
	mw "breadbox/internal/middleware"
	"breadbox/internal/pgconv"

	"github.com/go-chi/chi/v5"
)

// App-config REST endpoints. These mirror the admin-side surface for editing
// rows in the `app_config` table, but live under `/api/v1/config` so the
// headless CLI (`breadbox config ...`) can read/write them with an API key.
//
// Secret-sensitive keys (anything matching secretKeyPattern) are still
// returned in the listing, but their `value` is replaced with a masked
// representation. Clients that genuinely need the raw value pass
// `?reveal=true` (denied for keys in the alwaysDenyKeys set).
//
// The source field on each row reports where the *effective* value comes
// from at runtime — env (process environment), db (app_config table), or
// default (compiled-in fallback). This mirrors the badge the admin UI
// renders next to each setting.

// configEntry is the wire shape of one row in the listing.
type configEntry struct {
	Key    string  `json:"key"`
	Value  *string `json:"value,omitempty"`
	Masked bool    `json:"masked,omitempty"`
	Source string  `json:"source"`
	Secret bool    `json:"secret,omitempty"`
}

// secretKeySubstrings — keys containing any of these substrings get their
// value masked on GET. Match is case-insensitive against the upper-cased key.
var secretKeySubstrings = []string{"SECRET", "TOKEN", "PASSWORD", "PRIVATE_KEY"}

// secretKeySuffixes catches "*_KEY" without flagging the literal key column.
var secretKeySuffixes = []string{"_KEY"}

// alwaysDenyKeys may not be revealed via the API even with reveal=true.
// These are the keys whose disclosure would defeat at-rest encryption.
var alwaysDenyKeys = map[string]struct{}{
	"ENCRYPTION_KEY":  {},
	"teller_cert_pem": {},
	"teller_key_pem":  {},
}

// isSecretKey reports whether a key's value should be masked by default.
func isSecretKey(key string) bool {
	upper := strings.ToUpper(key)
	for _, s := range secretKeySubstrings {
		if strings.Contains(upper, s) {
			return true
		}
	}
	for _, suf := range secretKeySuffixes {
		if strings.HasSuffix(upper, suf) {
			return true
		}
	}
	return false
}

// isAlwaysDenied reports whether a key may never be revealed.
func isAlwaysDenied(key string) bool {
	if _, ok := alwaysDenyKeys[key]; ok {
		return true
	}
	return strings.EqualFold(key, "ENCRYPTION_KEY")
}

// maskValue produces a redacted form preserving rough length info. Empty
// values stay empty so callers can tell "set but blank" apart from "unset".
func maskValue(v string) string {
	if v == "" {
		return ""
	}
	if len(v) <= 4 {
		return "****"
	}
	if len(v) <= 11 {
		return strings.Repeat("*", len(v)-2) + v[len(v)-2:]
	}
	return v[:4] + strings.Repeat("*", 8) + "..."
}

// sourceFor returns the effective source ("env" / "db" / "default") for a key.
// Falls back to the in-memory ConfigSources map for known keys; for everything
// else, presence in app_config implies "db", absence implies "default".
func sourceFor(a *app.App, key string, presentInDB bool) string {
	if a.Config != nil && a.Config.ConfigSources != nil {
		if src, ok := a.Config.ConfigSources[key]; ok && src != "" {
			return src
		}
	}
	if presentInDB {
		return "db"
	}
	return "default"
}

// ListConfigHandler serves GET /api/v1/config.
func ListConfigHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := a.Queries.ListAppConfig(r.Context())
		if err != nil {
			a.Logger.Error("list app_config", "error", err)
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR",
				"Failed to load configuration")
			return
		}

		reveal := strings.EqualFold(r.URL.Query().Get("reveal"), "true")

		out := make([]configEntry, 0, len(rows))
		for _, row := range rows {
			entry := configEntry{
				Key:    row.Key,
				Source: sourceFor(a, row.Key, true),
				Secret: isSecretKey(row.Key),
			}
			val := ""
			if row.Value.Valid {
				val = row.Value.String
			}
			switch {
			case isAlwaysDenied(row.Key):
				masked := maskValue(val)
				entry.Value = &masked
				entry.Masked = true
			case isSecretKey(row.Key) && !reveal:
				masked := maskValue(val)
				entry.Value = &masked
				entry.Masked = true
			default:
				v := val
				entry.Value = &v
			}
			out = append(out, entry)
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// GetConfigHandler serves GET /api/v1/config/{key}.
func GetConfigHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(chi.URLParam(r, "key"))
		if key == "" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "key is required")
			return
		}
		reveal := strings.EqualFold(r.URL.Query().Get("reveal"), "true")

		row, err := a.Queries.GetAppConfig(r.Context(), key)
		if err != nil {
			// Treat all read errors as not-found here; the service consumer
			// only cares whether the key has a stored value.
			entry := configEntry{
				Key:    key,
				Source: sourceFor(a, key, false),
				Secret: isSecretKey(key),
			}
			writeJSON(w, http.StatusOK, entry)
			return
		}

		val := ""
		if row.Value.Valid {
			val = row.Value.String
		}
		entry := configEntry{
			Key:    key,
			Source: sourceFor(a, key, true),
			Secret: isSecretKey(key),
		}
		switch {
		case isAlwaysDenied(key):
			masked := maskValue(val)
			entry.Value = &masked
			entry.Masked = true
		case isSecretKey(key) && !reveal:
			masked := maskValue(val)
			entry.Value = &masked
			entry.Masked = true
		default:
			v := val
			entry.Value = &v
		}
		writeJSON(w, http.StatusOK, entry)
	}
}

// setConfigRequest is the JSON body for PUT /config/{key}.
type setConfigRequest struct {
	Value string `json:"value"`
}

// SetConfigHandler serves PUT /api/v1/config/{key}.
func SetConfigHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(chi.URLParam(r, "key"))
		if key == "" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "key is required")
			return
		}
		if isAlwaysDenied(key) {
			mw.WriteError(w, http.StatusForbidden, "FORBIDDEN",
				"This key cannot be set via the API; manage it via env vars or admin UI")
			return
		}
		var req setConfigRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if err := a.Queries.SetAppConfig(r.Context(), db.SetAppConfigParams{
			Key:   key,
			Value: pgconv.Text(req.Value),
		}); err != nil {
			a.Logger.Error("set app_config", "error", err, "key", key)
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR",
				"Failed to save configuration")
			return
		}
		if a.Config.ConfigSources != nil {
			a.Config.ConfigSources[key] = "db"
		}
		writeJSON(w, http.StatusOK, configEntry{
			Key:    key,
			Value:  ptrStr(req.Value),
			Source: "db",
			Secret: isSecretKey(key),
		})
	}
}

// DeleteConfigHandler serves DELETE /api/v1/config/{key}.
func DeleteConfigHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(chi.URLParam(r, "key"))
		if key == "" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "key is required")
			return
		}
		if isAlwaysDenied(key) {
			mw.WriteError(w, http.StatusForbidden, "FORBIDDEN",
				"This key cannot be deleted via the API")
			return
		}
		if err := a.Queries.DeleteAppConfig(r.Context(), key); err != nil {
			a.Logger.Error("delete app_config", "error", err, "key", key)
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR",
				"Failed to remove configuration")
			return
		}
		if a.Config.ConfigSources != nil {
			delete(a.Config.ConfigSources, key)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ptrStr is a tiny helper so handlers can return *string without local vars.
func ptrStr(s string) *string { return &s }

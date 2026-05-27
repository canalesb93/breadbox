//go:build !lite

package api

import (
	"net/http"
	"strings"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// ListAPIKeysHandler handles GET /api/v1/api-keys.
//
// Note: this endpoint is gated by RequireWriteScope on the router. Listing
// API keys (even with the plaintext suppressed) reveals the names, prefixes,
// and last-used timestamps of every credential — sensitive enumeration that
// shouldn't be available to read-only keys.
func ListAPIKeysHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		keys, err := svc.ListAPIKeys(r.Context())
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list API keys")
			return
		}
		writeData(w, keys)
	}
}

// CreateAPIKeyHandler handles POST /api/v1/api-keys.
//
// The response is the only time the plaintext key is returned — it is
// surfaced as plaintext_key on the create response and never on list/get.
// Callers MUST persist the value here.
func CreateAPIKeyHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name      string `json:"name"`
			Scope     string `json:"scope"`
			ActorType string `json:"actor_type"`
			ActorName string `json:"actor_name"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name is required")
			return
		}
		if req.Scope == "" {
			req.Scope = "full_access"
		}
		if req.Scope != "full_access" && req.Scope != "read_only" {
			mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "scope must be full_access or read_only")
			return
		}
		if req.ActorType == "" {
			req.ActorType = "user"
		}
		if req.ActorType != "user" && req.ActorType != "agent" && req.ActorType != "system" {
			mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "actor_type must be user, agent, or system")
			return
		}
		result, err := svc.CreateAPIKey(r.Context(), service.CreateAPIKeyParams{
			Name:      req.Name,
			Scope:     req.Scope,
			ActorType: req.ActorType,
			ActorName: strings.TrimSpace(req.ActorName),
		})
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create API key")
			return
		}
		writeJSON(w, http.StatusCreated, result)
	}
}

// WhoamiResponse is the shape returned by GET /api/v1/keys/me — small
// purpose-built endpoint the CLI uses for `breadbox auth whoami`. Mirrors
// APIKeyResponse but omits sensitive fields and never returns plaintext.
type WhoamiResponse struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	KeyPrefix string  `json:"key_prefix"`
	Scope     string  `json:"scope"`
	ActorType string  `json:"actor_type"`
	ActorName *string `json:"actor_name,omitempty"`
	CreatedAt string  `json:"created_at"`
}

// WhoamiHandler returns the API key record corresponding to the caller's
// credential — the row middleware already loaded. Mounted at
// GET /api/v1/keys/me; readable with any scope (it only reveals data the
// caller already presented).
func WhoamiHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := mw.GetAPIKey(r.Context())
		if key == nil {
			mw.WriteError(w, http.StatusUnauthorized, "MISSING_CREDENTIALS", "API key context missing")
			return
		}
		resp := WhoamiResponse{
			ID:        key.ID.String(),
			Name:      key.Name,
			KeyPrefix: key.KeyPrefix,
			Scope:     key.Scope,
			ActorType: key.ActorType,
		}
		if key.ActorName.Valid {
			s := key.ActorName.String
			resp.ActorName = &s
		}
		if key.CreatedAt.Valid {
			resp.CreatedAt = key.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z")
		}
		writeData(w, resp)
	}
}

// RevokeAPIKeyHandler handles DELETE /api/v1/api-keys/{id}.
//
// Soft-revoke (sets revoked_at). The auth middleware checks revoked_at on
// every request, so the next call using the revoked key will be rejected
// with REVOKED_API_KEY.
func RevokeAPIKeyHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.RevokeAPIKey(r.Context(), id); err != nil {
			writeServiceError(w, err, "API key not found", "Failed to revoke API key")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

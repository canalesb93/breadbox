//go:build !headless && !lite

package admin

import (
	"net/http"
	"strings"
	"time"

	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"
	"breadbox/internal/timefmt"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// --- JSON API handlers (programmatic access) ---

// CreateAPIKeyHandler handles POST /admin/api/api-keys.
func CreateAPIKeyHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name  string `json:"name"`
			Scope string `json:"scope"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Name is required")
			return
		}
		if req.Scope == "" {
			req.Scope = "full_access"
		}
		result, err := svc.CreateAPIKeyLegacy(r.Context(), req.Name, req.Scope)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create API key")
			return
		}
		writeJSON(w, http.StatusCreated, result)
	}
}

// ListAPIKeysHandler handles GET /admin/api/api-keys.
func ListAPIKeysHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		keys, err := svc.ListAPIKeys(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list API keys")
			return
		}
		writeJSON(w, http.StatusOK, keys)
	}
}

// RevokeAPIKeyHandler handles DELETE /admin/api/api-keys/{id}.
func RevokeAPIKeyHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.RevokeAPIKey(r.Context(), id); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to revoke API key")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- HTML page handlers (admin dashboard) ---

// AccessPageHandler serves GET /admin/settings/api-keys — combined API Keys + OAuth Clients page.
func AccessPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// On a cold/no-JS load the reveal arrives via session flash
		// (set by a create that 303-redirected here). The JS create path
		// never lands here — it renders the fragment directly.
		renderAccessTab(svc, sm, tr, w, r, popAPIKeyReveal(r, sm), popOAuthClientReveal(r, sm))
	}
}

// renderAccessTab lists keys + clients and renders the Access tab,
// optionally with a one-time reveal block for a just-created key or
// client. Shared by AccessPageHandler (GET) and the create handlers so a
// fragment create can paint the reveal inline without a redirect.
func renderAccessTab(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer, w http.ResponseWriter, r *http.Request, keyReveal, clientReveal *pages.AccessReveal) {
	keys, err := svc.ListAPIKeys(r.Context())
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	clients, err := svc.ListOAuthClients(r.Context())
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	// Absolute creation dates render in the viewer's timezone (bb_tz cookie).
	loc := UserLocation(r)
	var activeKeys, revokedKeys []pages.AccessKeyRow
	for _, k := range keys {
		row := buildAccessKeyRow(k, loc)
		if k.RevokedAt != nil {
			revokedKeys = append(revokedKeys, row)
		} else {
			activeKeys = append(activeKeys, row)
		}
	}
	var activeClients, revokedClients []pages.AccessClientRow
	for _, c := range clients {
		row := buildAccessClientRow(c, loc)
		if c.RevokedAt != nil {
			revokedClients = append(revokedClients, row)
		} else {
			activeClients = append(activeClients, row)
		}
	}
	data := BaseTemplateData(r, sm, "api-keys", "Access")
	props := pages.AccessProps{
		IsAdmin:           IsAdmin(sm, r),
		CSRFToken:         GetCSRFToken(r),
		ActiveKeys:        activeKeys,
		RevokedKeys:       revokedKeys,
		HasAnyKeys:        len(keys) > 0,
		ActiveClients:     activeClients,
		RevokedClients:    revokedClients,
		HasAnyClients:     len(clients) > 0,
		JustCreatedKey:    keyReveal,
		JustCreatedClient: clientReveal,
	}
	renderSettingsTab(tr, w, r, data, pages.SettingsTabAccess, pages.Access(props))
}

// popAPIKeyReveal consumes the one-time plaintext flash an API-key create
// left behind (no-JS path only), so the Access tab can render the copy-now
// block inline on the redirect-back render. Returns nil when no key was
// just created.
func popAPIKeyReveal(r *http.Request, sm *scs.SessionManager) *pages.AccessReveal {
	secret := sm.PopString(r.Context(), "created_api_key")
	if secret == "" {
		return nil
	}
	return &pages.AccessReveal{
		Name:   sm.PopString(r.Context(), "created_api_key_name"),
		Secret: secret,
		Scope:  sm.PopString(r.Context(), "created_api_key_scope"),
	}
}

// buildAccessKeyRow flattens a service.APIKeyResponse into the templ-side
// view-model, pre-rendering the date helpers (`formatDateShort`,
// `relativeTime`) the old html/template called via funcMap. loc is the
// viewer's timezone (admin.UserLocation) so the absolute creation date
// renders in their wall clock, not the server's.
func buildAccessKeyRow(k service.APIKeyResponse, loc *time.Location) pages.AccessKeyRow {
	return pages.AccessKeyRow{
		ID:               k.ID,
		Name:             k.Name,
		KeyPrefix:        k.KeyPrefix,
		Scope:            k.Scope,
		CreatedAtShort:   timefmt.FormatRFC3339In(k.CreatedAt, loc, timefmt.LayoutDateShort),
		LastUsedRelative: timefmt.RelativeRFC3339Ptr(k.LastUsedAt),
	}
}

// buildAccessClientRow flattens a service.OAuthClientResponse into the
// templ-side view-model, pre-rendering the creation date. loc is the viewer's
// timezone (admin.UserLocation).
func buildAccessClientRow(c service.OAuthClientResponse, loc *time.Location) pages.AccessClientRow {
	return pages.AccessClientRow{
		ID:             c.ID,
		Name:           c.Name,
		ClientIDPrefix: c.ClientIDPrefix,
		Scope:          c.Scope,
		CreatedAtShort: timefmt.FormatRFC3339In(c.CreatedAt, loc, timefmt.LayoutDateShort),
	}
}

// APIKeyCreatePageHandler serves POST /settings/api-keys/new.
//
// One-click create: the name is optional (defaults to "API key") and the
// scope defaults to full access, so the bare button mints immediately. The
// plaintext is stashed in a one-shot session flash and we 303 back to the
// Access tab, where popAPIKeyReveal renders it inline — no /created
// subpage. The in-page swapper follows the redirect as a fragment so the
// reveal lands without a full navigation.
func APIKeyCreatePageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			name = "API key"
		}
		scope := r.FormValue("scope")
		if scope != "read_only" {
			scope = "full_access"
		}
		result, err := svc.CreateAPIKeyLegacy(r.Context(), name, scope)
		if err != nil {
			FlashRedirect(w, r, sm, "error", "Failed to create API key", "/settings/api-keys")
			return
		}
		reveal := &pages.AccessReveal{Name: result.Name, Secret: result.PlaintextKey, Scope: scope}
		// JS path: the in-page swapper POSTs with the fragment header.
		// Render the Access tab fragment directly with the reveal inline —
		// no redirect, no one-shot flash to race against.
		if r.Header.Get(settingsFragmentHeader) == "1" {
			renderAccessTab(svc, sm, tr, w, r, reveal, nil)
			return
		}
		// No-JS fallback: stash the reveal in a flash and 303 back so a
		// form post doesn't re-submit on refresh.
		sm.Put(r.Context(), "created_api_key", result.PlaintextKey)
		sm.Put(r.Context(), "created_api_key_name", result.Name)
		sm.Put(r.Context(), "created_api_key_scope", scope)
		http.Redirect(w, r, "/settings/api-keys", http.StatusSeeOther)
	}
}

// APIKeyRevokePageHandler serves POST /admin/api-keys/{id}/revoke.
func APIKeyRevokePageHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.RevokeAPIKey(r.Context(), id); err != nil {
			SetFlash(r.Context(), sm, "error", "Failed to revoke API key")
		} else {
			SetFlash(r.Context(), sm, "success", "API key revoked successfully")
		}
		http.Redirect(w, r, "/settings/api-keys", http.StatusSeeOther)
	}
}

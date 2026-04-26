package admin

import (
	"net/http"
	"strings"

	"breadbox/internal/service"
	"breadbox/internal/templates/components"
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
		result, err := svc.CreateAPIKey(r.Context(), req.Name, req.Scope)
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

// AccessPageHandler serves GET /admin/access — combined API Keys + OAuth Clients page.
func AccessPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		// Split keys into active and revoked for cleaner display.
		var activeKeys, revokedKeys []pages.AccessKeyRow
		for _, k := range keys {
			row := buildAccessKeyRow(k)
			if k.RevokedAt != nil {
				revokedKeys = append(revokedKeys, row)
			} else {
				activeKeys = append(activeKeys, row)
			}
		}
		var activeClients, revokedClients []pages.AccessClientRow
		for _, c := range clients {
			row := buildAccessClientRow(c)
			if c.RevokedAt != nil {
				revokedClients = append(revokedClients, row)
			} else {
				activeClients = append(activeClients, row)
			}
		}
		data := BaseTemplateData(r, sm, "access", "Access")
		props := pages.AccessProps{
			IsAdmin:        IsAdmin(sm, r),
			CSRFToken:      GetCSRFToken(r),
			ActiveKeys:     activeKeys,
			RevokedKeys:    revokedKeys,
			HasAnyKeys:     len(keys) > 0,
			ActiveClients:  activeClients,
			RevokedClients: revokedClients,
			HasAnyClients:  len(clients) > 0,
		}
		renderAccess(w, r, tr, data, props)
	}
}

// renderAccess mirrors the renderLogs / renderRules pattern: it hands the
// typed AccessProps to the templ component and uses RenderWithTempl to host
// it inside base.html.
func renderAccess(w http.ResponseWriter, r *http.Request, tr *TemplateRenderer, data map[string]any, props pages.AccessProps) {
	tr.RenderWithTempl(w, r, data, pages.Access(props))
}

// buildAccessKeyRow flattens a service.APIKeyResponse into the templ-side
// view-model, pre-rendering the date helpers (`formatDateShort`,
// `relativeTime`) the old html/template called via funcMap.
func buildAccessKeyRow(k service.APIKeyResponse) pages.AccessKeyRow {
	return pages.AccessKeyRow{
		ID:               k.ID,
		Name:             k.Name,
		KeyPrefix:        k.KeyPrefix,
		Scope:            k.Scope,
		CreatedAtShort:   timefmt.FormatRFC3339(k.CreatedAt, timefmt.LayoutDateShort),
		LastUsedRelative: timefmt.RelativeRFC3339Ptr(k.LastUsedAt),
	}
}

// buildAccessClientRow flattens a service.OAuthClientResponse into the
// templ-side view-model, pre-rendering the creation date.
func buildAccessClientRow(c service.OAuthClientResponse) pages.AccessClientRow {
	return pages.AccessClientRow{
		ID:             c.ID,
		Name:           c.Name,
		ClientIDPrefix: c.ClientIDPrefix,
		Scope:          c.Scope,
		CreatedAtShort: timefmt.FormatRFC3339(c.CreatedAt, timefmt.LayoutDateShort),
	}
}

// APIKeyNewPageHandler serves GET /admin/api-keys/new.
func APIKeyNewPageHandler(sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := BaseTemplateData(r, sm, "access", "Create API Key")
		renderAPIKeyNew(w, r, tr, data, pages.APIKeyNewProps{
			CSRFToken: GetCSRFToken(r),
			Breadcrumbs: []components.Breadcrumb{
				{Label: "Access", Href: "/access"},
				{Label: "Create API Key"},
			},
		})
	}
}

// renderAPIKeyNew mirrors the renderCSVImport / renderCategoryForm pattern:
// hands the typed APIKeyNewProps to the templ component and uses
// RenderWithTempl to host it inside base.html.
func renderAPIKeyNew(w http.ResponseWriter, r *http.Request, tr *TemplateRenderer, data map[string]any, props pages.APIKeyNewProps) {
	tr.RenderWithTempl(w, r, data, pages.APIKeyNew(props))
}

// APIKeyCreatePageHandler serves POST /admin/api-keys/new.
// Creates the key and redirects to the "created" page that shows the plaintext key once.
func APIKeyCreatePageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			SetFlash(r.Context(), sm, "error", "Name is required")
			http.Redirect(w, r, "/api-keys/new", http.StatusSeeOther)
			return
		}
		scope := r.FormValue("scope")
		if scope == "" {
			scope = "full_access"
		}
		result, err := svc.CreateAPIKey(r.Context(), name, scope)
		if err != nil {
			SetFlash(r.Context(), sm, "error", "Failed to create API key")
			http.Redirect(w, r, "/api-keys/new", http.StatusSeeOther)
			return
		}
		// Store the plaintext key in the session so the "created" page can display it once.
		sm.Put(r.Context(), "created_api_key", result.PlaintextKey)
		sm.Put(r.Context(), "created_api_key_name", result.Name)
		http.Redirect(w, r, "/api-keys/"+result.ID+"/created", http.StatusSeeOther)
	}
}

// APIKeyCreatedPageHandler serves GET /admin/api-keys/{id}/created.
// Shows the plaintext key ONE TIME (from session flash).
func APIKeyCreatedPageHandler(sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := sm.PopString(r.Context(), "created_api_key")
		name := sm.PopString(r.Context(), "created_api_key_name")
		if key == "" {
			// Key already shown or session expired — redirect to list.
			http.Redirect(w, r, "/access", http.StatusSeeOther)
			return
		}
		data := BaseTemplateData(r, sm, "access", "API Key Created")
		renderAPIKeyCreated(w, r, tr, data, pages.APIKeyCreatedProps{
			KeyName:      name,
			PlaintextKey: key,
			Breadcrumbs: []components.Breadcrumb{
				{Label: "Access", Href: "/access"},
				{Label: "Key Created"},
			},
		})
	}
}

// renderAPIKeyCreated mirrors renderAPIKeyNew: hands the typed
// APIKeyCreatedProps to the templ component and uses RenderWithTempl
// to host it inside base.html.
func renderAPIKeyCreated(w http.ResponseWriter, r *http.Request, tr *TemplateRenderer, data map[string]any, props pages.APIKeyCreatedProps) {
	tr.RenderWithTempl(w, r, data, pages.APIKeyCreated(props))
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
		http.Redirect(w, r, "/access", http.StatusSeeOther)
	}
}

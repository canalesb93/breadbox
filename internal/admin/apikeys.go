package admin

import (
	"net/http"
	"strings"

	"breadbox/internal/service"

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
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error": map[string]string{"code": "VALIDATION_ERROR", "message": "Name is required"},
			})
			return
		}
		if req.Scope == "" {
			req.Scope = "full_access"
		}
		result, err := svc.CreateAPIKey(r.Context(), req.Name, req.Scope)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create API key"})
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
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list API keys"})
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
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to revoke API key"})
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
		// Split keys into active and revoked for cleaner display
		var activeKeys, revokedKeys []service.APIKeyResponse
		for _, k := range keys {
			if k.RevokedAt != nil {
				revokedKeys = append(revokedKeys, k)
			} else {
				activeKeys = append(activeKeys, k)
			}
		}
		var activeClients, revokedClients []service.OAuthClientResponse
		for _, c := range clients {
			if c.RevokedAt != nil {
				revokedClients = append(revokedClients, c)
			} else {
				activeClients = append(activeClients, c)
			}
		}
		data := map[string]any{
			"PageTitle":      "Access",
			"CurrentPage":    "access",
			"Keys":           keys,
			"ActiveKeys":     activeKeys,
			"RevokedKeys":    revokedKeys,
			"Clients":        clients,
			"ActiveClients":  activeClients,
			"RevokedClients": revokedClients,
			"Flash":          GetFlash(r.Context(), sm),
			"CSRFToken":      GetCSRFToken(r),
		}
		tr.Render(w, r, "access.html", data)
	}
}

// APIKeyNewPageHandler serves GET /admin/api-keys/new.
func APIKeyNewPageHandler(tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := map[string]any{
			"PageTitle":   "Create API Key",
			"CurrentPage": "access",
			"CSRFToken":   GetCSRFToken(r),
			"Breadcrumbs": []Breadcrumb{
				{Label: "Access", Href: "/access"},
				{Label: "Create API Key"},
			},
		}
		tr.Render(w, r, "api_key_new.html", data)
	}
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
		data := map[string]any{
			"PageTitle":    "API Key Created",
			"CurrentPage":  "access",
			"PlaintextKey": key,
			"KeyName":      name,
			"CSRFToken":    GetCSRFToken(r),
			"Breadcrumbs": []Breadcrumb{
				{Label: "Access", Href: "/access"},
				{Label: "Key Created"},
			},
		}
		tr.Render(w, r, "api_key_created.html", data)
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
		http.Redirect(w, r, "/access", http.StatusSeeOther)
	}
}

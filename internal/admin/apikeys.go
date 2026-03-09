package admin

import (
	"encoding/json"
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
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
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

// APIKeysListPageHandler serves GET /admin/api-keys.
func APIKeysListPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		keys, err := svc.ListAPIKeys(r.Context())
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		data := map[string]any{
			"PageTitle":   "API Keys",
			"CurrentPage": "api-keys",
			"Keys":        keys,
			"Flash":       GetFlash(r.Context(), sm),
			"CSRFToken":   GetCSRFToken(r),
		}
		tr.Render(w, r, "api_keys.html", data)
	}
}

// APIKeyNewPageHandler serves GET /admin/api-keys/new.
func APIKeyNewPageHandler(tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := map[string]any{
			"PageTitle":   "Create API Key",
			"CurrentPage": "api-keys",
			"CSRFToken":   GetCSRFToken(r),
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
			http.Redirect(w, r, "/admin/api-keys/new", http.StatusSeeOther)
			return
		}
		scope := r.FormValue("scope")
		if scope == "" {
			scope = "full_access"
		}
		result, err := svc.CreateAPIKey(r.Context(), name, scope)
		if err != nil {
			SetFlash(r.Context(), sm, "error", "Failed to create API key")
			http.Redirect(w, r, "/admin/api-keys/new", http.StatusSeeOther)
			return
		}
		// Store the plaintext key in the session so the "created" page can display it once.
		sm.Put(r.Context(), "created_api_key", result.PlaintextKey)
		sm.Put(r.Context(), "created_api_key_name", result.Name)
		http.Redirect(w, r, "/admin/api-keys/"+result.ID+"/created", http.StatusSeeOther)
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
			http.Redirect(w, r, "/admin/api-keys", http.StatusSeeOther)
			return
		}
		data := map[string]any{
			"PageTitle":    "API Key Created",
			"CurrentPage":  "api-keys",
			"PlaintextKey": key,
			"KeyName":      name,
			"CSRFToken":    GetCSRFToken(r),
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
		http.Redirect(w, r, "/admin/api-keys", http.StatusSeeOther)
	}
}

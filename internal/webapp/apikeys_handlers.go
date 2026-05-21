//go:build !headless && !lite

package webapp

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"breadbox/internal/service"
	"breadbox/internal/webapp/pages"
)

// registerAPIKeys wires the API-keys read + write routes onto the authenticated subrouter.
// "/api-keys/new" is registered before any "/api-keys/{id}/..." so it isn't shadowed.
func (h *Handler) registerAPIKeys(r chi.Router) {
	r.Get("/api-keys", h.apiKeysList)
	r.Post("/api-keys", h.requireSameOrigin(h.createAPIKey))
	r.Get("/api-keys/new", h.newAPIKey)
	r.Post("/api-keys/{id}/revoke", h.requireSameOrigin(h.revokeAPIKey))
}

// apiKeysList renders every API key, masked — plaintext is never re-rendered.
func (h *Handler) apiKeysList(w http.ResponseWriter, r *http.Request) {
	keys, err := h.app.Service.ListAPIKeys(r.Context())
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	render(w, r, http.StatusOK, pages.APIKeysList(h.shellData(r, "API keys"), keys))
}

// newAPIKey renders the empty create form (scope defaults to full_access).
func (h *Handler) newAPIKey(w http.ResponseWriter, r *http.Request) {
	render(w, r, http.StatusOK, pages.APIKeyForm(h.shellData(r, "New API key"), pages.APIKeyFormData{
		ActionURL: "/app/api-keys",
		CancelURL: "/app/api-keys",
		Values:    pages.APIKeyFormValues{Scope: "full_access"},
		Errors:    map[string]string{},
	}))
}

// createAPIKey validates and mints a new key. On validation failure it re-renders the
// form with field errors at HTTP 422. On success it renders the one-time plaintext page
// directly (no redirect) — the plaintext exists only in this response and is never stored
// in cleartext or re-rendered on any later page.
func (h *Handler) createAPIKey(w http.ResponseWriter, r *http.Request) {
	vals := pages.APIKeyFormValues{
		Name:  strings.TrimSpace(r.FormValue("name")),
		Scope: strings.TrimSpace(r.FormValue("scope")),
	}

	fieldErrs := validateAPIKey(vals.Name, vals.Scope)
	if len(fieldErrs) > 0 {
		h.rerenderAPIKeyForm(w, r, vals, fieldErrs)
		return
	}

	result, err := h.app.Service.CreateAPIKey(r.Context(), service.CreateAPIKeyParams{
		Name:      vals.Name,
		Scope:     vals.Scope,
		ActorType: "user",
	})
	if err != nil {
		h.rerenderAPIKeyForm(w, r, vals, map[string]string{"form": "Could not create the key. Check the name and scope and try again."})
		return
	}
	// Show the plaintext exactly once.
	render(w, r, http.StatusOK, pages.APIKeyCreated(h.shellData(r, "API key created"), result.PlaintextKey, result.APIKeyResponse))
}

// revokeAPIKey revokes a key by UUID, then 303s back to the list.
func (h *Handler) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := h.app.Service.RevokeAPIKey(r.Context(), id)
	if errors.Is(err, service.ErrNotFound) {
		h.notFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/app/api-keys", http.StatusSeeOther)
}

// rerenderAPIKeyForm re-renders the create form with errors at HTTP 422.
func (h *Handler) rerenderAPIKeyForm(w http.ResponseWriter, r *http.Request, vals pages.APIKeyFormValues, fieldErrs map[string]string) {
	render(w, r, http.StatusUnprocessableEntity, pages.APIKeyForm(h.shellData(r, "New API key"), pages.APIKeyFormData{
		ActionURL: "/app/api-keys",
		CancelURL: "/app/api-keys",
		Values:    vals,
		Errors:    fieldErrs,
	}))
}

// validateAPIKey runs server-side rules: name required; scope must be one of the two
// valid scopes.
func validateAPIKey(name, scope string) map[string]string {
	errs := map[string]string{}
	if name == "" {
		errs["name"] = "Name is required."
	}
	if scope != "full_access" && scope != "read_only" {
		errs["scope"] = "Choose a valid scope."
	}
	return errs
}

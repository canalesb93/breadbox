package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// RulesPageHandler serves GET /admin/rules.
func RulesPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		params := service.TransactionRuleListParams{
			Limit: 50,
		}

		if v := r.URL.Query().Get("search"); v != "" {
			params.Search = &v
		}
		if v := r.URL.Query().Get("category_slug"); v != "" {
			params.CategorySlug = &v
		}
		if v := r.URL.Query().Get("enabled"); v != "" {
			b, err := strconv.ParseBool(v)
			if err == nil {
				params.Enabled = &b
			}
		}
		if v := r.URL.Query().Get("cursor"); v != "" {
			params.Cursor = v
		}
		if v := r.URL.Query().Get("limit"); v != "" {
			if l, err := strconv.Atoi(v); err == nil && l > 0 {
				params.Limit = l
			}
		}

		result, err := svc.ListTransactionRules(ctx, params)
		if err != nil {
			tr.Render(w, r, "500.html", map[string]any{"PageTitle": "Error", "CurrentPage": "rules"})
			return
		}

		// Load category tree for the category picker.
		categories, _ := svc.ListCategoryTree(ctx)

		data := BaseTemplateData(r, sm, "rules", "Transaction Rules")
		data["Rules"] = result.Rules
		data["HasMore"] = result.HasMore
		data["NextCursor"] = result.NextCursor
		data["Total"] = result.Total
		data["SearchFilter"] = r.URL.Query().Get("search")
		data["CategoryFilter"] = r.URL.Query().Get("category_slug")
		data["EnabledFilter"] = r.URL.Query().Get("enabled")
		data["Categories"] = categories
		data["FlatCategories"] = flattenCategories(categories)
		data["Version"] = version

		tr.Render(w, r, "rules.html", data)
	}
}

// CreateRuleAdminHandler handles POST /admin/api/rules.
func CreateRuleAdminHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor := ActorFromSession(sm, r)

		var body struct {
			Name         string            `json:"name"`
			Conditions   service.Condition `json:"conditions"`
			CategorySlug string            `json:"category_slug"`
			Priority     int               `json:"priority"`
			ExpiresIn    string            `json:"expires_in"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}

		if body.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name is required"})
			return
		}
		if body.CategorySlug == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "category_slug is required"})
			return
		}

		rule, err := svc.CreateTransactionRule(r.Context(), service.CreateTransactionRuleParams{
			Name:         body.Name,
			Conditions:   body.Conditions,
			CategorySlug: body.CategorySlug,
			Priority:     body.Priority,
			ExpiresIn:    body.ExpiresIn,
			Actor:        actor,
		})
		if err != nil {
			if errors.Is(err, service.ErrInvalidParameter) {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to create rule"})
			return
		}

		writeJSON(w, http.StatusCreated, rule)
	}
}

// UpdateRuleAdminHandler handles PUT /admin/api/rules/{id}.
func UpdateRuleAdminHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var body struct {
			Name         *string            `json:"name,omitempty"`
			Conditions   *service.Condition `json:"conditions,omitempty"`
			CategorySlug *string            `json:"category_slug,omitempty"`
			Priority     *int               `json:"priority,omitempty"`
			Enabled      *bool              `json:"enabled,omitempty"`
			ExpiresAt    *string            `json:"expires_at,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}

		rule, err := svc.UpdateTransactionRule(r.Context(), id, service.UpdateTransactionRuleParams{
			Name:         body.Name,
			Conditions:   body.Conditions,
			CategorySlug: body.CategorySlug,
			Priority:     body.Priority,
			Enabled:      body.Enabled,
			ExpiresAt:    body.ExpiresAt,
		})
		if err != nil {
			switch {
			case errors.Is(err, service.ErrNotFound):
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "rule not found"})
			case errors.Is(err, service.ErrInvalidParameter):
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to update rule"})
			}
			return
		}

		writeJSON(w, http.StatusOK, rule)
	}
}

// DeleteRuleAdminHandler handles DELETE /admin/api/rules/{id}.
func DeleteRuleAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		if err := svc.DeleteTransactionRule(r.Context(), id); err != nil {
			if errors.Is(err, service.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "rule not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to delete rule"})
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// ToggleRuleAdminHandler handles POST /admin/api/rules/{id}/toggle.
func ToggleRuleAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		// Get current state
		existing, err := svc.GetTransactionRule(r.Context(), id)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "rule not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to get rule"})
			return
		}

		newEnabled := !existing.Enabled
		rule, err := svc.UpdateTransactionRule(r.Context(), id, service.UpdateTransactionRuleParams{
			Enabled: &newEnabled,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to toggle rule"})
			return
		}

		writeJSON(w, http.StatusOK, rule)
	}
}

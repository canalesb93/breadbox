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

		// Compute summary stats from the returned rules.
		var activeCount, disabledCount, totalHits, agentCreated int
		for _, rule := range result.Rules {
			if rule.Enabled {
				activeCount++
			} else {
				disabledCount++
			}
			totalHits += rule.HitCount
			if rule.CreatedByType == "agent" {
				agentCreated++
			}
		}

		data := BaseTemplateData(r, sm, "rules", "Transaction Rules")
		data["Rules"] = result.Rules
		data["HasMore"] = result.HasMore
		data["NextCursor"] = result.NextCursor
		data["Total"] = result.Total
		data["ActiveCount"] = activeCount
		data["DisabledCount"] = disabledCount
		data["TotalHits"] = totalHits
		data["AgentCreated"] = agentCreated
		data["SearchFilter"] = r.URL.Query().Get("search")
		data["CategoryFilter"] = r.URL.Query().Get("category_slug")
		data["EnabledFilter"] = r.URL.Query().Get("enabled")
		data["Categories"] = categories
		data["FlatCategories"] = flattenCategories(categories)
		data["Version"] = version

		tr.Render(w, r, "rules.html", data)
	}
}

// RuleFormPageHandler serves GET /admin/rules/new and /admin/rules/{id}/edit.
func RuleFormPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		categories, _ := svc.ListCategoryTree(ctx)

		data := BaseTemplateData(r, sm, "rules", "New Rule")
		data["FlatCategories"] = flattenCategories(categories)
		data["IsEdit"] = false
		data["Breadcrumbs"] = []Breadcrumb{
			{Label: "Rules", Href: "/rules"},
			{Label: "New Rule"},
		}

		// Edit mode: load existing rule
		if id := chi.URLParam(r, "id"); id != "" {
			rule, err := svc.GetTransactionRule(ctx, id)
			if err != nil {
				if errors.Is(err, service.ErrNotFound) {
					tr.Render(w, r, "404.html", map[string]any{"PageTitle": "Not Found", "CurrentPage": "rules"})
					return
				}
				tr.Render(w, r, "500.html", map[string]any{"PageTitle": "Error", "CurrentPage": "rules"})
				return
			}
			data["Rule"] = rule
			data["IsEdit"] = true
			data["PageTitle"] = "Edit Rule"
			data["Breadcrumbs"] = []Breadcrumb{
				{Label: "Rules", Href: "/rules"},
				{Label: rule.Name},
			}
		}

		tr.Render(w, r, "rule_form.html", data)
	}
}

// RuleDetailPageHandler serves GET /admin/rules/{id}.
func RuleDetailPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id := chi.URLParam(r, "id")

		rule, err := svc.GetTransactionRule(ctx, id)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				tr.Render(w, r, "404.html", map[string]any{"PageTitle": "Not Found", "CurrentPage": "rules"})
				return
			}
			tr.Render(w, r, "500.html", map[string]any{"PageTitle": "Error", "CurrentPage": "rules"})
			return
		}

		// Preview: transactions that currently match this rule's conditions
		preview, _ := svc.PreviewRule(ctx, rule.Conditions, 10)

		// Application stats from junction table
		stats, _ := svc.GetRuleStats(ctx, id)

		// Recent applications
		applications, hasMoreApps, _ := svc.ListRuleApplications(ctx, id, 10, "")

		// Sync history where this rule matched
		syncHistory, _ := svc.GetRuleSyncHistory(ctx, id, 10)

		data := BaseTemplateData(r, sm, "rules", rule.Name)
		data["Rule"] = rule
		data["Preview"] = preview
		data["Stats"] = stats
		data["Applications"] = applications
		data["HasMoreApplications"] = hasMoreApps
		data["SyncHistory"] = syncHistory
		data["ConditionSummary"] = service.ConditionSummary(rule.Conditions)
		data["Breadcrumbs"] = []Breadcrumb{
			{Label: "Rules", Href: "/rules"},
			{Label: rule.Name},
		}

		tr.Render(w, r, "rule_detail.html", data)
	}
}

// CreateRuleAdminHandler handles POST /admin/api/rules.
func CreateRuleAdminHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor := ActorFromSession(sm, r)

		var body struct {
			Name         string               `json:"name"`
			Conditions   service.Condition     `json:"conditions"`
			Actions      []service.RuleAction  `json:"actions"`
			CategorySlug string                `json:"category_slug"`
			Priority     int                   `json:"priority"`
			ExpiresIn    string                `json:"expires_in"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}

		if body.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name is required"})
			return
		}
		if len(body.Actions) == 0 && body.CategorySlug == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "either actions or category_slug is required"})
			return
		}

		rule, err := svc.CreateTransactionRule(r.Context(), service.CreateTransactionRuleParams{
			Name:         body.Name,
			Conditions:   body.Conditions,
			Actions:      body.Actions,
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
			Name         *string               `json:"name,omitempty"`
			Conditions   *service.Condition     `json:"conditions,omitempty"`
			Actions      *[]service.RuleAction  `json:"actions,omitempty"`
			CategorySlug *string                `json:"category_slug,omitempty"`
			Priority     *int                   `json:"priority,omitempty"`
			Enabled      *bool                  `json:"enabled,omitempty"`
			ExpiresAt    *string                `json:"expires_at,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}

		rule, err := svc.UpdateTransactionRule(r.Context(), id, service.UpdateTransactionRuleParams{
			Name:         body.Name,
			Conditions:   body.Conditions,
			Actions:      body.Actions,
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

// ApplyRuleAdminHandler handles POST /-/rules/{id}/apply.
func ApplyRuleAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		count, err := svc.ApplyRuleRetroactively(r.Context(), id)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "rule not found"})
				return
			}
			if errors.Is(err, service.ErrInvalidParameter) {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to apply rule"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"rule_id":        id,
			"affected_count": count,
		})
	}
}

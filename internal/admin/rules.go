package admin

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// RulesPageHandler serves GET /admin/rules.
func RulesPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		q := r.URL.Query()
		params := service.TransactionRuleListParams{
			Page:         parsePage(r),
			PageSize:     parsePerPage(r, 50, 25, 50, 100),
			Search:       optStrQuery(q, "search"),
			CategorySlug: optStrQuery(q, "category_slug"),
		}

		if v := q.Get("enabled"); v != "" {
			if b, err := strconv.ParseBool(v); err == nil {
				params.Enabled = &b
			}
		}

		// Sort key — whitelisted in ruleOrderByClause so unknown values fall
		// back to created_at DESC instead of injecting arbitrary SQL.
		sortBy := r.URL.Query().Get("sort_by")
		params.SortBy = sortBy

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

		// Build pagination base URL (all params except page).
		paginationBase := buildRulesPaginationBase(r)

		data := BaseTemplateData(r, sm, "rules", "Rules")
		data["Rules"] = result.Rules
		data["Total"] = result.Total
		data["Page"] = result.Page
		data["PageSize"] = result.PageSize
		data["TotalPages"] = result.TotalPages
		data["PaginationBase"] = paginationBase
		data["ShowingStart"] = (result.Page-1)*result.PageSize + 1
		data["ShowingEnd"] = min(int64(result.Page*result.PageSize), result.Total)
		data["ActiveCount"] = activeCount
		data["DisabledCount"] = disabledCount
		data["TotalHits"] = totalHits
		data["AgentCreated"] = agentCreated
		data["SearchFilter"] = r.URL.Query().Get("search")
		data["CategoryFilter"] = r.URL.Query().Get("category_slug")
		data["EnabledFilter"] = r.URL.Query().Get("enabled")
		data["SortBy"] = sortBy
		data["FlatCategories"] = flattenCategories(categories)
		data["Version"] = version

		tr.Render(w, r, "rules.html", data)
	}
}

// buildRulesPaginationBase returns the pagination base URL for the rules page.
func buildRulesPaginationBase(r *http.Request) string {
	params := []string{"search", "category_slug", "enabled", "per_page", "sort_by"}
	q := r.URL.Query()
	var qs []string
	for _, key := range params {
		if v := q.Get(key); v != "" {
			qs = append(qs, key+"="+url.QueryEscape(v))
		}
	}
	base := "/rules?page="
	if len(qs) > 0 {
		base = "/rules?" + strings.Join(qs, "&") + "&page="
	}
	return base
}

// RuleFormPageHandler serves GET /admin/rules/new and /admin/rules/{id}/edit.
func RuleFormPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		categories, _ := svc.ListCategoryTree(ctx)
		// Tags feed the add_tag autocomplete in the form. Best-effort — empty
		// list just means no datalist suggestions.
		tags, _ := svc.ListTags(ctx)

		data := BaseTemplateData(r, sm, "rules", "New Rule")
		data["FlatCategories"] = flattenCategories(categories)
		data["Tags"] = tags
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

		// Preview: transactions matching conditions but NOT already applied by this rule
		preview, _ := svc.PreviewRuleForDetail(ctx, id, rule.Conditions, 10)

		// Application stats from junction table
		stats, _ := svc.GetRuleStats(ctx, id)

		// Recent applications
		applications, hasMoreApps, _ := svc.ListRuleApplications(ctx, id, 10, "")

		// Category tree powers (a) the inline category picker on tx-row, and
		// (b) slug → display-name resolution for the action meta line shown
		// above each Recent Applications row.
		categories, _ := svc.ListCategoryTree(ctx)
		type catMeta struct {
			DisplayName string
			Color       *string
			Icon        *string
		}
		catBySlug := make(map[string]catMeta)
		for _, p := range categories {
			catBySlug[p.Slug] = catMeta{DisplayName: p.DisplayName, Color: p.Color, Icon: p.Icon}
			for _, c := range p.Children {
				catBySlug[c.Slug] = catMeta{DisplayName: c.DisplayName, Color: c.Color, Icon: c.Icon}
			}
		}

		// Hydrate application rows with AdminTransactionRow data so the shared
		// tx-row partial can render them (category avatar, account, user,
		// agent-reviewed flag, pending state). Preserves order. ApplicationMeta
		// carries the per-application action so the rule_detail template can
		// prefix each row with a prominent "what happened" pill.
		applicationTxns := make([]service.AdminTransactionRow, 0, len(applications))
		applicationMeta := make(map[string]struct {
			ActionField        string
			ActionValue        string
			ActionDisplay      string
			ActionCategoryColor *string
			ActionCategoryIcon  *string
			AppliedBy          string
		}, len(applications))
		if len(applications) > 0 {
			txnIDs := make([]string, 0, len(applications))
			for _, a := range applications {
				txnIDs = append(txnIDs, a.TransactionID)
				display := a.ActionValue
				var cColor, cIcon *string
				if a.ActionField == "category" {
					if m, ok := catBySlug[a.ActionValue]; ok {
						display = m.DisplayName
						cColor = m.Color
						cIcon = m.Icon
					}
				}
				applicationMeta[a.TransactionID] = struct {
					ActionField        string
					ActionValue        string
					ActionDisplay      string
					ActionCategoryColor *string
					ActionCategoryIcon  *string
					AppliedBy          string
				}{
					ActionField:        a.ActionField,
					ActionValue:        a.ActionValue,
					ActionDisplay:      display,
					ActionCategoryColor: cColor,
					ActionCategoryIcon:  cIcon,
					AppliedBy:          a.AppliedBy,
				}
			}
			if rows, err := svc.GetAdminTransactionRowsByIDs(ctx, txnIDs); err == nil {
				applicationTxns = rows
			}
		}

		// Hydrate preview matches the same way so the pending-matches table can
		// reuse the compact partial.
		var previewTxns []service.AdminTransactionRow
		if preview != nil && len(preview.SampleMatches) > 0 {
			txnIDs := make([]string, 0, len(preview.SampleMatches))
			for _, m := range preview.SampleMatches {
				txnIDs = append(txnIDs, m.TransactionID)
			}
			if rows, err := svc.GetAdminTransactionRowsByIDs(ctx, txnIDs); err == nil {
				previewTxns = rows
			}
		}

		// Sync history where this rule matched
		syncHistory, _ := svc.GetRuleSyncHistory(ctx, id, 10)

		// Resolve category display name for the action description.
		var actionCategoryName string
		for _, a := range rule.Actions {
			if a.Type == "set_category" && rule.CategoryName != nil {
				actionCategoryName = *rule.CategoryName
				break
			}
		}

		data := BaseTemplateData(r, sm, "rules", rule.Name)
		data["Rule"] = rule
		data["Preview"] = preview
		data["Stats"] = stats
		data["Applications"] = applications
		data["ApplicationTxns"] = applicationTxns
		data["ApplicationMeta"] = applicationMeta
		data["PreviewTxns"] = previewTxns
		data["HasMoreApplications"] = hasMoreApps
		data["SyncHistory"] = syncHistory
		data["ActionCategoryName"] = actionCategoryName
		// Categories feed window.__bbCategories for the inline category picker
		// on tx-row partials used in Recent Applications and Matching sections.
		data["Categories"] = categories

		// Parse last_hit_at for relative time display
		if rule.LastHitAt != nil {
			if t, err := time.Parse(time.RFC3339, *rule.LastHitAt); err == nil {
				data["LastActiveTime"] = t
			}
		}
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
			Conditions   *service.Condition    `json:"conditions"`
			Actions      []service.RuleAction  `json:"actions"`
			CategorySlug string                `json:"category_slug"`
			Trigger      string                `json:"trigger"`
			Priority     int                   `json:"priority"`
			ExpiresIn    string                `json:"expires_in"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}

		if body.Name == "" {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "name is required")
			return
		}
		if len(body.Actions) == 0 && body.CategorySlug == "" {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "either actions or category_slug is required")
			return
		}

		params := service.CreateTransactionRuleParams{
			Name:         body.Name,
			Actions:      body.Actions,
			CategorySlug: body.CategorySlug,
			Trigger:      body.Trigger,
			Priority:     body.Priority,
			ExpiresIn:    body.ExpiresIn,
			Actor:        actor,
		}
		if body.Conditions != nil {
			params.Conditions = *body.Conditions
		}

		rule, err := svc.CreateTransactionRule(r.Context(), params)
		if err != nil {
			handleRuleError(w, err, "Failed to create rule")
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
			Trigger      *string                `json:"trigger,omitempty"`
			Priority     *int                   `json:"priority,omitempty"`
			Enabled      *bool                  `json:"enabled,omitempty"`
			ExpiresAt    *string                `json:"expires_at,omitempty"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}

		rule, err := svc.UpdateTransactionRule(r.Context(), id, service.UpdateTransactionRuleParams{
			Name:         body.Name,
			Conditions:   body.Conditions,
			Actions:      body.Actions,
			CategorySlug: body.CategorySlug,
			Trigger:      body.Trigger,
			Priority:     body.Priority,
			Enabled:      body.Enabled,
			ExpiresAt:    body.ExpiresAt,
		})
		if err != nil {
			handleRuleError(w, err, "Failed to update rule")
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
			handleRuleError(w, err, "Failed to delete rule")
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
			handleRuleError(w, err, "Failed to get rule")
			return
		}

		newEnabled := !existing.Enabled
		rule, err := svc.UpdateTransactionRule(r.Context(), id, service.UpdateTransactionRuleParams{
			Enabled: &newEnabled,
		})
		if err != nil {
			handleRuleError(w, err, "Failed to toggle rule")
			return
		}

		writeJSON(w, http.StatusOK, rule)
	}
}

// PreviewRuleAdminHandler handles POST /-/rules/preview.
// Evaluates conditions against existing transactions without modifying anything.
// Used by the rule form's live preview — no API key required, admin session only.
func PreviewRuleAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Conditions service.Condition `json:"conditions"`
			SampleSize int               `json:"sample_size"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}

		result, err := svc.PreviewRule(r.Context(), input.Conditions, input.SampleSize)
		if err != nil {
			handleRuleError(w, err, "Failed to preview rule")
			return
		}

		writeJSON(w, http.StatusOK, result)
	}
}

// ApplyRuleAdminHandler handles POST /-/rules/{id}/apply.
func ApplyRuleAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		count, err := svc.ApplyRuleRetroactively(r.Context(), id)
		if err != nil {
			handleRuleError(w, err, "Failed to apply rule")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"rule_id":        id,
			"affected_count": count,
		})
	}
}

// handleRuleError maps service-layer errors returned by rule mutations to the
// canonical {"error": {"code", "message"}} envelope. `fallback` is the
// human-readable message used when the error isn't a recognized sentinel.
func handleRuleError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Rule not found")
	case errors.Is(err, service.ErrInvalidParameter):
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", fallback)
	}
}

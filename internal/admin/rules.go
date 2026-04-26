package admin

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"breadbox/internal/service"
	"breadbox/internal/templates/components"
	"breadbox/internal/templates/components/pages"

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
			tr.RenderError(w, r)
			return
		}

		renderRules(w, r, sm, tr, result, sortBy, version)
	}
}

// renderRules builds the typed templ props for the rules list page and
// hosts them inside the base layout via TemplateRenderer.RenderWithTempl.
// Replaces the old html/template render — the page is now defined in
// internal/templates/components/pages/rules.templ.
func renderRules(w http.ResponseWriter, r *http.Request, sm *scs.SessionManager, tr *TemplateRenderer, result *service.TransactionRuleListResult, sortBy, version string) {
	rows := make([]pages.RulesRow, 0, len(result.Rules))
	for _, rule := range result.Rules {
		row := pages.BuildRulesRow(rule)
		if rule.LastHitAt != nil && *rule.LastHitAt != "" {
			if t, err := time.Parse(time.RFC3339, *rule.LastHitAt); err == nil {
				row.LastHitAtRelative = relativeTime(t)
			}
		}
		if rule.ExpiresAt != nil && *rule.ExpiresAt != "" {
			if t, err := time.Parse(time.RFC3339, *rule.ExpiresAt); err == nil {
				row.Expired = t.Before(time.Now())
			}
		}
		rows = append(rows, row)
	}

	props := pages.RulesProps{
		Rules:          rows,
		Total:          result.Total,
		Page:           result.Page,
		PageSize:       result.PageSize,
		TotalPages:     result.TotalPages,
		ShowingStart:   (result.Page-1)*result.PageSize + 1,
		ShowingEnd:     min(int64(result.Page*result.PageSize), result.Total),
		PaginationBase: buildRulesPaginationBase(r),
		SortBy:         sortBy,
	}

	data := BaseTemplateData(r, sm, "rules", "Rules")
	data["Version"] = version
	tr.RenderWithTempl(w, r, data, pages.Rules(props))
}

// buildRulesPaginationBase returns the pagination base URL for the rules page.
func buildRulesPaginationBase(r *http.Request) string {
	return paginationBase("/rules", pickValues(r, []string{
		"search", "category_slug", "enabled", "per_page", "sort_by",
	}), "page")
}

// RuleFormPageHandler serves GET /admin/rules/new and /admin/rules/{id}/edit.
func RuleFormPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		categories, _ := svc.ListCategoryTree(ctx)
		// Tags feed the add_tag autocomplete in the form. Best-effort — empty
		// list just means no datalist suggestions.
		tags, _ := svc.ListTags(ctx)

		props := pages.RuleFormProps{
			IsEdit:         false,
			FlatCategories: flattenCategories(categories),
			Tags:           tags,
			Breadcrumbs: []components.Breadcrumb{
				{Label: "Rules", Href: "/rules"},
				{Label: "New Rule"},
			},
		}

		data := BaseTemplateData(r, sm, "rules", "New Rule")

		// Edit mode: load existing rule
		if id := chi.URLParam(r, "id"); id != "" {
			rule, err := svc.GetTransactionRule(ctx, id)
			if err != nil {
				if errors.Is(err, service.ErrNotFound) {
					tr.RenderNotFound(w, r)
					return
				}
				tr.RenderError(w, r)
				return
			}
			props.Rule = rule
			props.IsEdit = true
			props.Breadcrumbs = []components.Breadcrumb{
				{Label: "Rules", Href: "/rules"},
				{Label: rule.Name},
			}
			data["PageTitle"] = "Edit Rule"
		}

		tr.RenderWithTempl(w, r, data, pages.RuleForm(props))
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
				tr.RenderNotFound(w, r)
				return
			}
			tr.RenderError(w, r)
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
		applicationMeta := make(map[string]pages.RuleApplicationMeta, len(applications))
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
				applicationMeta[a.TransactionID] = pages.RuleApplicationMeta{
					ActionField:         a.ActionField,
					ActionValue:         a.ActionValue,
					ActionDisplay:       display,
					ActionCategoryColor: cColor,
					ActionCategoryIcon:  cIcon,
					AppliedBy:           a.AppliedBy,
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
		renderRuleDetail(w, r, tr, data, rule, preview, stats, applications, applicationTxns, applicationMeta, previewTxns, hasMoreApps, syncHistory, actionCategoryName, categories)
	}
}

// renderRuleDetail builds the typed templ props from the loose handler-side
// values and forwards through tr.RenderWithTempl. Condition rows are
// pre-formatted using the ruleFieldLabel / ruleOpLabel / ruleValueFormat
// helpers in templates.go.
func renderRuleDetail(
	w http.ResponseWriter,
	r *http.Request,
	tr *TemplateRenderer,
	data map[string]any,
	rule *service.TransactionRuleResponse,
	preview *service.RulePreviewResult,
	stats *service.RuleStats,
	applications []service.RuleApplicationRow,
	applicationTxns []service.AdminTransactionRow,
	applicationMeta map[string]pages.RuleApplicationMeta,
	previewTxns []service.AdminTransactionRow,
	hasMoreApps bool,
	syncHistory []map[string]any,
	actionCategoryName string,
	categories []service.CategoryResponse,
) {
	props := pages.RuleDetailProps{
		Rule:                rule,
		Preview:             preview,
		Stats:               stats,
		Applications:        applications,
		ApplicationTxns:     applicationTxns,
		ApplicationMeta:     applicationMeta,
		PreviewTxns:         previewTxns,
		HasMoreApplications: hasMoreApps,
		SyncHistory:         syncHistory,
		ActionCategoryName:  actionCategoryName,
		Categories:          categories,
		ConditionSummary:    service.ConditionSummary(rule.Conditions),
		TriggerLabel:        service.TriggerLabel(rule.Trigger),
		ConditionRows:       buildRuleDetailConditionRows(rule.Conditions),
		Breadcrumbs: []components.Breadcrumb{
			{Label: "Rules", Href: "/rules"},
			{Label: rule.Name},
		},
	}

	// Parse last_hit_at for relative time display.
	if rule.LastHitAt != nil {
		if t, err := time.Parse(time.RFC3339, *rule.LastHitAt); err == nil {
			props.LastActiveTime = t
			props.HasLastActive = true
		}
	}

	tr.RenderWithTempl(w, r, data, pages.RuleDetail(props))
}

// buildRuleDetailConditionRows pre-formats the visible condition rows for
// the rule_detail page. Returns an empty slice when the condition is
// match-all, a leaf, or a degenerate empty tree — the templ branches on
// len(...) plus the ConditionSummary fallback to render those cases.
func buildRuleDetailConditionRows(c service.Condition) []components.ConditionRowProps {
	row := func(cond service.Condition, idx int, conj string) components.ConditionRowProps {
		return components.ConditionRowProps{
			IsFirst:     idx == 0,
			Conj:        conj,
			FieldLabel:  ruleFieldLabel(cond.Field),
			OpLabel:     ruleOpLabel(cond.Op, cond.Field),
			ValueFormat: ruleValueFormat(cond.Value),
		}
	}
	switch {
	case len(c.And) > 0:
		out := make([]components.ConditionRowProps, len(c.And))
		for i, sub := range c.And {
			out[i] = row(sub, i, "AND")
		}
		return out
	case len(c.Or) > 0:
		out := make([]components.ConditionRowProps, len(c.Or))
		for i, sub := range c.Or {
			out[i] = row(sub, i, "OR")
		}
		return out
	case c.Field != "":
		return []components.ConditionRowProps{row(c, 0, "")}
	default:
		return nil
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

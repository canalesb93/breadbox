//go:build !headless && !lite

package webapp

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"breadbox/internal/admin"
	"breadbox/internal/service"
	"breadbox/internal/webapp/components"
	"breadbox/internal/webapp/pages"
)

// registerRules wires the rules read + write routes onto the authenticated subrouter.
// "/rules/new" is registered before "/rules/{id}" so it isn't captured as an id.
func (h *Handler) registerRules(r chi.Router) {
	r.Get("/rules", h.rulesList)
	r.Post("/rules", h.requireSameOrigin(h.createRule))
	r.Get("/rules/new", h.newRule)
	r.Get("/rules/{id}", h.ruleDetail)
	r.Get("/rules/{id}/edit", h.editRule)
	r.Post("/rules/{id}", h.requireSameOrigin(h.updateRule))
}

// rulesList renders every transaction rule as a table.
func (h *Handler) rulesList(w http.ResponseWriter, r *http.Request) {
	result, err := h.app.Service.ListTransactionRules(r.Context(), service.TransactionRuleListParams{
		Page:     1,
		PageSize: 200,
		SortBy:   "priority",
	})
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	render(w, r, http.StatusOK, pages.RulesList(h.shellData(r, "Rules"), result.Rules))
}

// ruleDetail renders one rule with its conditions and actions.
func (h *Handler) ruleDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rule, err := h.app.Service.GetTransactionRule(r.Context(), id)
	if errors.Is(err, service.ErrNotFound) {
		h.notFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	render(w, r, http.StatusOK, pages.RuleDetail(h.shellData(r, rule.Name), rule))
}

// newRule renders the empty create form.
func (h *Handler) newRule(w http.ResponseWriter, r *http.Request) {
	opts, err := h.ruleCategoryOptions(r)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	render(w, r, http.StatusOK, pages.RuleForm(h.shellData(r, "New rule"), pages.RuleFormData{
		Mode:      "create",
		ActionURL: "/app/rules",
		CancelURL: "/app/rules",
		Values: pages.RuleFormValues{
			Enabled: true,
			Trigger: "on_create",
		},
		Errors:          map[string]string{},
		CategoryOptions: opts,
	}))
}

// createRule validates and creates a rule. On validation failure it re-renders the
// form with field errors and HTTP 422; on success it 303s to the new detail page.
func (h *Handler) createRule(w http.ResponseWriter, r *http.Request) {
	vals := parseRuleForm(r)
	conds, actions, fieldErrs := buildRuleInputs(vals)
	if len(fieldErrs) > 0 {
		h.rerenderRuleForm(w, r, "create", "/app/rules", "/app/rules", vals, fieldErrs)
		return
	}

	params := service.CreateTransactionRuleParams{
		Name:       vals.Name,
		Conditions: conds,
		Actions:    actions,
		Trigger:    vals.Trigger,
		Priority:   parsePriority(vals.Priority),
		Actor:      admin.ActorFromSession(h.sm, r),
	}

	rule, err := h.app.Service.CreateTransactionRule(r.Context(), params)
	if err != nil {
		h.rerenderRuleForm(w, r, "create", "/app/rules", "/app/rules", vals,
			map[string]string{"form": ruleServiceErrMsg(err)})
		return
	}
	http.Redirect(w, r, "/app/rules/"+rule.ShortID, http.StatusSeeOther)
}

// editRule prefills the form for an existing rule. Conditions are flattened from
// the stored (possibly AND-of-leaves) tree into the form's flat row model.
func (h *Handler) editRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rule, err := h.app.Service.GetTransactionRule(r.Context(), id)
	if errors.Is(err, service.ErrNotFound) {
		h.notFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	opts, err := h.ruleCategoryOptions(r)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	render(w, r, http.StatusOK, pages.RuleForm(h.shellData(r, "Edit rule"), pages.RuleFormData{
		Mode:      "edit",
		ActionURL: "/app/rules/" + rule.ShortID,
		CancelURL: "/app/rules/" + rule.ShortID,
		Values: pages.RuleFormValues{
			Name:       rule.Name,
			Enabled:    rule.Enabled,
			Trigger:    rule.Trigger,
			Priority:   strconv.Itoa(rule.Priority),
			Conditions: flattenConditionRows(rule.Conditions),
			Actions:    actionRowsFromService(rule.Actions),
		},
		Errors:          map[string]string{},
		CategoryOptions: opts,
	}))
}

// updateRule validates and updates a rule. Re-renders with errors + 422 on
// validation failure; 303s to the detail page on success.
func (h *Handler) updateRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rule, err := h.app.Service.GetTransactionRule(r.Context(), id)
	if errors.Is(err, service.ErrNotFound) {
		h.notFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	vals := parseRuleForm(r)
	conds, actions, fieldErrs := buildRuleInputs(vals)
	if len(fieldErrs) > 0 {
		h.rerenderRuleForm(w, r, "edit", "/app/rules/"+rule.ShortID, "/app/rules/"+rule.ShortID, vals, fieldErrs)
		return
	}

	name := vals.Name
	trigger := vals.Trigger
	priority := parsePriority(vals.Priority)
	enabled := vals.Enabled
	_, err = h.app.Service.UpdateTransactionRule(r.Context(), id, service.UpdateTransactionRuleParams{
		Name:       &name,
		Conditions: &conds,
		Actions:    &actions,
		Trigger:    &trigger,
		Priority:   &priority,
		Enabled:    &enabled,
	})
	if errors.Is(err, service.ErrNotFound) {
		h.notFound(w, r)
		return
	}
	if err != nil {
		h.rerenderRuleForm(w, r, "edit", "/app/rules/"+rule.ShortID, "/app/rules/"+rule.ShortID, vals,
			map[string]string{"form": ruleServiceErrMsg(err)})
		return
	}
	http.Redirect(w, r, "/app/rules/"+rule.ShortID, http.StatusSeeOther)
}

// rerenderRuleForm re-renders the form with errors at HTTP 422, preserving submitted values.
func (h *Handler) rerenderRuleForm(w http.ResponseWriter, r *http.Request, mode, action, cancel string, vals pages.RuleFormValues, fieldErrs map[string]string) {
	opts, err := h.ruleCategoryOptions(r)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	title := "New rule"
	if mode == "edit" {
		title = "Edit rule"
	}
	render(w, r, http.StatusUnprocessableEntity, pages.RuleForm(h.shellData(r, title), pages.RuleFormData{
		Mode:            mode,
		ActionURL:       action,
		CancelURL:       cancel,
		Values:          vals,
		Errors:          fieldErrs,
		CategoryOptions: opts,
	}))
}

// parseRuleForm reads the flat scalar fields plus the array-style condition/action
// rows into the form value struct. Array inputs (condition_field[], condition_op[],
// condition_value[], action_type[], action_value[]) are zipped positionally.
func parseRuleForm(r *http.Request) pages.RuleFormValues {
	_ = r.ParseForm()

	fields := r.Form["condition_field[]"]
	ops := r.Form["condition_op[]"]
	condVals := r.Form["condition_value[]"]
	conds := make([]pages.RuleFormRow, 0, len(fields))
	for i := range fields {
		conds = append(conds, pages.RuleFormRow{
			Field: strings.TrimSpace(fields[i]),
			Op:    strings.TrimSpace(at(ops, i)),
			Value: strings.TrimSpace(at(condVals, i)),
		})
	}

	types := r.Form["action_type[]"]
	targets := r.Form["action_value[]"]
	acts := make([]pages.RuleFormAction, 0, len(types))
	for i := range types {
		acts = append(acts, pages.RuleFormAction{
			Type:   strings.TrimSpace(types[i]),
			Target: strings.TrimSpace(at(targets, i)),
		})
	}

	return pages.RuleFormValues{
		Name:       strings.TrimSpace(r.FormValue("name")),
		Enabled:    r.FormValue("enabled") != "",
		Trigger:    strings.TrimSpace(r.FormValue("trigger")),
		Priority:   strings.TrimSpace(r.FormValue("priority")),
		Conditions: conds,
		Actions:    acts,
	}
}

// buildRuleInputs validates the form and converts it into the service condition
// tree + action slice. Returns field errors keyed by "name"/"priority"/"actions".
// Blank condition rows (no field) and blank action rows (no type) are skipped.
func buildRuleInputs(vals pages.RuleFormValues) (service.Condition, []service.RuleAction, map[string]string) {
	errs := map[string]string{}

	if vals.Name == "" {
		errs["name"] = "Name is required."
	}
	if vals.Priority != "" {
		if _, err := strconv.Atoi(vals.Priority); err != nil {
			errs["priority"] = "Priority must be a whole number."
		}
	}

	// Conditions: skip blank rows, require op + value on populated rows.
	leaves := make([]service.Condition, 0, len(vals.Conditions))
	for _, row := range vals.Conditions {
		if row.Field == "" {
			continue
		}
		if row.Op == "" || row.Value == "" {
			errs["conditions"] = "Each condition needs a field, operator, and value."
			continue
		}
		leaves = append(leaves, service.Condition{
			Field: row.Field,
			Op:    row.Op,
			Value: ruleConditionValue(row.Field, row.Op, row.Value),
		})
	}

	// Actions: skip blank rows, require a target on populated rows.
	actions := make([]service.RuleAction, 0, len(vals.Actions))
	for _, act := range vals.Actions {
		if act.Type == "" {
			continue
		}
		if act.Target == "" {
			errs["actions"] = "Each action needs a target value."
			continue
		}
		actions = append(actions, ruleActionFromForm(act))
	}
	if len(actions) == 0 && errs["actions"] == "" {
		errs["actions"] = "At least one action is required."
	}

	return composeConditions(leaves), actions, errs
}

// composeConditions wraps the leaf rows in the service Condition shape: a
// zero-value Condition (match-all) for none, the bare leaf for one, an AND for many.
func composeConditions(leaves []service.Condition) service.Condition {
	switch len(leaves) {
	case 0:
		return service.Condition{}
	case 1:
		return leaves[0]
	default:
		return service.Condition{And: leaves}
	}
}

// ruleConditionValue coerces a string form value into the type the DSL expects:
// numeric fields → float64, pending → bool, "in" operator → []string. Everything
// else stays a string (the service rejects type-mismatched values at write time).
func ruleConditionValue(field, op, raw string) interface{} {
	if op == "in" {
		parts := strings.Split(raw, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	switch field {
	case "amount":
		if f, err := strconv.ParseFloat(raw, 64); err == nil {
			return f
		}
	case "pending":
		return strings.EqualFold(raw, "true") || raw == "1"
	}
	return raw
}

// ruleActionFromForm maps a form action row to the service RuleAction, routing the
// single target field to the right typed slot.
func ruleActionFromForm(a pages.RuleFormAction) service.RuleAction {
	switch a.Type {
	case "set_category":
		return service.RuleAction{Type: a.Type, CategorySlug: a.Target}
	case "add_tag", "remove_tag":
		return service.RuleAction{Type: a.Type, TagSlug: a.Target}
	case "add_comment":
		return service.RuleAction{Type: a.Type, Content: a.Target}
	default:
		return service.RuleAction{Type: a.Type}
	}
}

// flattenConditionRows turns a stored Condition tree into flat form rows for edit
// prefill. It handles the shapes this form authors (single leaf, AND of leaves);
// any nested OR/NOT structure (Phase 4 builder territory) is dropped so it can't be
// silently mangled — the user re-authors conditions in that rare case.
func flattenConditionRows(c service.Condition) []pages.RuleFormRow {
	var rows []pages.RuleFormRow
	switch {
	case c.Field != "":
		rows = append(rows, conditionRowFromLeaf(c))
	case len(c.And) > 0:
		for _, sub := range c.And {
			if sub.Field != "" {
				rows = append(rows, conditionRowFromLeaf(sub))
			}
		}
	}
	return rows
}

func conditionRowFromLeaf(c service.Condition) pages.RuleFormRow {
	return pages.RuleFormRow{Field: c.Field, Op: c.Op, Value: conditionValueString(c.Value)}
}

// conditionValueString renders a stored condition value back into the form's text
// box: slices join with ", "; everything else uses its default string form.
func conditionValueString(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []string:
		return strings.Join(t, ", ")
	case []interface{}:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			parts = append(parts, conditionValueString(e))
		}
		return strings.Join(parts, ", ")
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		return ""
	}
}

// actionRowsFromService maps stored actions back into the form's flat action rows.
func actionRowsFromService(actions []service.RuleAction) []pages.RuleFormAction {
	rows := make([]pages.RuleFormAction, 0, len(actions))
	for _, a := range actions {
		rows = append(rows, pages.RuleFormAction{Type: a.Type, Target: ruleActionFormTarget(a)})
	}
	return rows
}

func ruleActionFormTarget(a service.RuleAction) string {
	switch a.Type {
	case "set_category":
		return a.CategorySlug
	case "add_tag", "remove_tag":
		return a.TagSlug
	case "add_comment":
		return a.Content
	default:
		return ""
	}
}

// ruleCategoryOptions lists category slugs to surface as a reference in the form
// (the per-row target is free text, but the slug list is a helpful crib).
func (h *Handler) ruleCategoryOptions(r *http.Request) ([]components.Option, error) {
	cats, err := h.app.Service.ListCategories(r.Context())
	if err != nil {
		return nil, err
	}
	opts := make([]components.Option, 0, len(cats))
	for _, c := range cats {
		opts = append(opts, components.Option{Value: c.Slug, Label: c.DisplayName})
	}
	return opts, nil
}

// parsePriority returns the int priority, defaulting to 0 on blank/invalid input
// (validation catches invalid input upstream; this is a safe fallback).
func parsePriority(s string) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return 0
}

// ruleServiceErrMsg maps a service-layer error to a form-banner message. Input
// validation errors (wrapping ErrInvalidParameter) carry a human-readable cause,
// so surface their text; everything else gets a generic message.
func ruleServiceErrMsg(err error) string {
	if errors.Is(err, service.ErrInvalidParameter) {
		msg := strings.TrimPrefix(err.Error(), service.ErrInvalidParameter.Error()+": ")
		if msg == "" {
			return "Invalid rule input."
		}
		return strings.ToUpper(msg[:1]) + msg[1:]
	}
	return "Could not save the rule. Please try again."
}

// at returns the i-th element of s or "" when out of range — guards the positional
// zip of parallel form-array inputs that may have mismatched lengths.
func at(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return ""
}

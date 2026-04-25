package pages

import (
	"encoding/json"

	"breadbox/internal/service"
	"breadbox/internal/templates/components"
)

// RuleFormProps mirrors the data map the old rule_form.html read off the
// layout's data map. Kept flat so admin/rules.go can copy fields one-to-one.
type RuleFormProps struct {
	IsEdit          bool
	Rule            *service.TransactionRuleResponse
	FlatCategories  []service.CategoryResponse
	Tags            []service.TagResponse
	Breadcrumbs     []components.Breadcrumb
}

// ruleJSON marshals the existing rule (or null) for the inline Alpine
// bootstrap. Returns the literal "null" string when no rule is set so the
// JS reads `const existingRule = null;` cleanly in create mode.
func ruleJSON(r *service.TransactionRuleResponse) string {
	if r == nil {
		return "null"
	}
	b, err := json.Marshal(r)
	if err != nil {
		return "null"
	}
	return string(b)
}

// flatCategoryParentLabel returns the parent display name with the trailing
// separator already attached, or empty string when the category has no
// parent. Mirrors the old html/template `{{if .ParentDisplayName}}{{.ParentDisplayName}} > {{end}}`.
func flatCategoryParentLabel(c service.CategoryResponse) string {
	if c.ParentDisplayName == nil || *c.ParentDisplayName == "" {
		return ""
	}
	return *c.ParentDisplayName + " > "
}

package pages

import (
	"breadbox/internal/service"
	"breadbox/internal/templates/components"
)

// RuleFormProps mirrors the data map the old rule_form.html read off the
// layout's data map. Kept flat so admin/rules.go can copy fields one-to-one.
type RuleFormProps struct {
	IsEdit         bool
	Rule           *service.TransactionRuleResponse
	FlatCategories []service.CategoryResponse
	Tags           []service.TagResponse
	Breadcrumbs    []components.Breadcrumb
}

// ruleFormIsEditAttr returns the literal string "true" or "false" for the
// `data-is-edit` attribute on the x-data root. The factory reads it via
// `this.$el.dataset.isEdit === 'true'` in init().
func ruleFormIsEditAttr(b bool) string {
	if b {
		return "true"
	}
	return "false"
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

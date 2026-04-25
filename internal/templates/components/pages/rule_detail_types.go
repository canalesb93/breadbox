package pages

import (
	"fmt"
	"time"

	"breadbox/internal/service"
	"breadbox/internal/templates/components"
)

// RuleDetailProps mirrors the data the old rule_detail.html read off the
// layout's data map. Kept flat so admin/rules.go can copy fields one-to-one.
type RuleDetailProps struct {
	Rule                *service.TransactionRuleResponse
	Preview             *service.RulePreviewResult
	Stats               *service.RuleStats
	Applications        []service.RuleApplicationRow
	ApplicationTxns     []service.AdminTransactionRow
	ApplicationMeta     map[string]RuleApplicationMeta
	PreviewTxns         []service.AdminTransactionRow
	HasMoreApplications bool
	SyncHistory         []map[string]any
	ActionCategoryName  string
	Categories          []service.CategoryResponse
	// LastActiveTime is the parsed last_hit_at; zero value means "never".
	LastActiveTime time.Time
	HasLastActive  bool
	ConditionSummary string
	Breadcrumbs      []components.Breadcrumb
	// ConditionRows is the pre-formatted list of ConditionRowProps the handler
	// computes for non-match-all rules. Empty when the rule matches everything
	// or when no conditions are stored. Order mirrors the source tree (And, Or,
	// or single-field root).
	ConditionRows []components.ConditionRowProps
	// TriggerLabel is the human label for Rule.Trigger (service.TriggerLabel).
	TriggerLabel string
}

// RuleApplicationMeta carries the per-application action info shown above
// each Recent Applications row. Mirrors the anonymous struct the old
// handler stored on the data map.
type RuleApplicationMeta struct {
	ActionField         string
	ActionValue         string
	ActionDisplay       string
	ActionCategoryColor *string
	ActionCategoryIcon  *string
	AppliedBy           string
}

// ruleID safely returns the rule's ID, or empty string when the rule
// pointer is nil. Used to build the data-apply-url / data-toggle-url
// attributes without crashing the template render on a transient nil.
func ruleID(rule *service.TransactionRuleResponse) string {
	if rule == nil {
		return ""
	}
	return rule.ID
}

// ruleConditionConjLabel returns the AND/OR conjunction string used by the
// "When" header label, or empty when there's no top-level conjunction.
func ruleConditionConjLabel(c service.Condition) (label, classes string) {
	if len(c.And) > 0 {
		return "AND", "text-base-content/30"
	}
	if len(c.Or) > 0 {
		return "OR", "text-warning"
	}
	return "", "text-base-content/30"
}

// ruleConditionMatchAll mirrors the funcMap helper of the same name —
// returns true when the rule has no field, no AND, no OR, no NOT.
func ruleConditionMatchAll(c service.Condition) bool {
	return c.Field == "" && len(c.And) == 0 && len(c.Or) == 0 && c.Not == nil
}

// ruleHasRetroactiveAction mirrors the funcMap helper — true when the rule
// has at least one set_category / add_tag / remove_tag action.
func ruleHasRetroactiveAction(actions []service.RuleAction) bool {
	for _, a := range actions {
		switch a.Type {
		case "set_category", "add_tag", "remove_tag":
			return true
		}
	}
	return false
}

// ruleHeaderTileClass returns the icon-tile background class for the page
// header. Mirrors the html/template ternary: disabled → bg-base-200,
// system → bg-info/10, otherwise bg-success/10.
func ruleHeaderTileClass(enabled, isSystem bool) string {
	if !enabled {
		return "bg-base-200"
	}
	if isSystem {
		return "bg-info/10"
	}
	return "bg-success/10"
}

// ruleCreatedByLabel returns the small "Created automatically/Built-in/..."
// caption beside the trigger badge.
func ruleCreatedByLabel(createdByType string) string {
	switch createdByType {
	case "agent":
		return "Created automatically"
	case "system":
		return "Built-in"
	default:
		return "Created manually"
	}
}

// ruleCategoryTileBgStyle returns the inline `style` attribute string for
// the set_category action's icon tile. Mirrors the html/template branch:
// when the rule has a category color, use that with 1a (10%) alpha;
// otherwise fall back to the indigo-tinted default.
func ruleCategoryTileBgStyle(rule *service.TransactionRuleResponse) string {
	if rule != nil && rule.CategoryColor != nil && *rule.CategoryColor != "" {
		return fmt.Sprintf("background-color: %s1a", *rule.CategoryColor)
	}
	return "background-color: rgba(99, 102, 241, 0.1)"
}

// ruleCategoryIconColorStyle returns the inline `style` attribute string
// for the category icon (foreground). Empty when the rule has no
// category color, in which case the icon picks up its parent class color.
func ruleCategoryIconColorStyle(rule *service.TransactionRuleResponse) string {
	if rule != nil && rule.CategoryColor != nil && *rule.CategoryColor != "" {
		return fmt.Sprintf("color: %s", *rule.CategoryColor)
	}
	return ""
}

// rulePendingTileClass returns the bg class for the third stat tile
// (warning when pending matches exist, success otherwise).
func rulePendingTileClass(pendingActive bool) string {
	if pendingActive {
		return "bg-warning/10"
	}
	return "bg-success/10"
}

// ruleStatsUnique returns the UniqueTransactions count, defensively
// handling a nil RuleStats pointer.
func ruleStatsUnique(s *service.RuleStats) int64 {
	if s == nil {
		return 0
	}
	return s.UniqueTransactions
}

// ruleRelativeTime renders a time.Time as "just now / N minutes ago / ..."
// — mirrors admin.relativeTime so the LastActiveTime stat reads identically
// to the original page.
func ruleRelativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

// ruleApplyModalCountSentence renders the body sentence in the apply
// modal. Returns a `template.HTML`-safe string with a `<strong>` wrapping
// the count, matching the html/template version. The bold count is emitted
// via @templ.Raw because templ's text-interpolation escapes the tags.
func ruleApplyModalCountSentence(count int64) string {
	noun := "transactions"
	if count == 1 {
		noun = "transaction"
	}
	return fmt.Sprintf(
		"This will run the rule's actions against <strong>%s</strong> matching %s.",
		commaIntStr(count),
		noun,
	)
}

// commaIntStr is a thin wrapper so the helper above doesn't depend on the
// `components` package (avoiding a back-edge in this scripts file).
func commaIntStr(n int64) string {
	return components.CommaInt(n)
}

// syncStatusBgClass returns the avatar background class for a sync history
// row based on its status string.
func syncStatusBgClass(status string) string {
	if status == "success" {
		return "bg-success/10"
	}
	return "bg-error/10"
}

// syncHistoryHitCount extracts the hit_count value from a sync_logs row map.
// Tolerates both int and int64 (the service builds these as plain int via
// JSON-decoded map[string]int lookups; a future change might switch types).
func syncHistoryHitCount(h map[string]any) int {
	switch v := h["hit_count"].(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	}
	return 0
}

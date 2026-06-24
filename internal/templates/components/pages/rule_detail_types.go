//go:build !headless && !lite

package pages

import (
	"fmt"
	"strings"
	"time"

	"breadbox/internal/service"
	"breadbox/internal/templates/components"
	"breadbox/internal/timefmt"
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
	LastActiveTime   time.Time
	HasLastActive    bool
	ConditionSummary string
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

// ruleAppliedByTitle returns the tooltip text for the inline meta badge that
// marks how a Recent-Applications row was touched. Retroactive is the common
// case; any other non-sync origin falls back to a generic phrasing.
func ruleAppliedByTitle(appliedBy string) string {
	if appliedBy == "retroactive" {
		return "Applied retroactively from this rule's detail page"
	}
	return "Applied via " + appliedBy
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

// ruleHasRetroactiveAction reports whether a rule has at least one action that
// the engine actually materializes against already-synced transactions. This
// MUST stay aligned with Service.ApplyRuleRetroactively (internal/service/rules.go):
// that path materializes set_category, add_tag, remove_tag, assign_series,
// assign_counterparty, set_metadata, remove_metadata, flag, and unflag. Only
// add_comment is sync-only (an explicit no-op retroactively). Treating a
// membership/metadata/flag rule as non-retroactive here would wrongly hide the
// "Apply now" affordance even though the engine would back-fill it.
func ruleHasRetroactiveAction(actions []service.RuleAction) bool {
	for _, a := range actions {
		switch a.Type {
		case "set_category", "add_tag", "remove_tag",
			"assign_series", "assign_counterparty",
			"set_metadata", "remove_metadata",
			"flag", "unflag":
			return true
		}
	}
	return false
}

// ruleHeaderIconTone maps rule (enabled, system) to the EntityHeader
// tile tone: disabled → neutral, system → info, otherwise success.
func ruleHeaderIconTone(enabled, isSystem bool) components.IconTone {
	if !enabled {
		return components.IconToneNeutral
	}
	if isSystem {
		return components.IconToneInfo
	}
	return components.IconToneSuccess
}

// ruleHeaderIconName mirrors ruleHeaderIconTone — the lucide glyph for
// the tile: disabled → pause, system → shield, otherwise zap.
func ruleHeaderIconName(enabled, isSystem bool) string {
	if !enabled {
		return "pause"
	}
	if isSystem {
		return "shield"
	}
	return "zap"
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

// rulePendingTone selects the StatTile tone for the pending-matches
// tile — warning when matches are waiting, success when caught up.
func rulePendingTone(p RuleDetailProps) components.StatTileTone {
	if p.Preview != nil && p.Preview.MatchCount > 0 {
		return components.StatToneWarning
	}
	return components.StatToneSuccess
}

// rulePendingIcon mirrors rulePendingTone: alert when there's
// untouched-match work, check-circle otherwise.
func rulePendingIcon(p RuleDetailProps) string {
	if p.Preview != nil && p.Preview.MatchCount > 0 {
		return "alert-circle"
	}
	return "check-circle"
}

// rulePendingValue is the numeric body of the pending-matches tile.
// Falls back to "0" when no preview has been computed yet.
func rulePendingValue(p RuleDetailProps) string {
	if p.Preview == nil {
		return "0"
	}
	return components.CommaInt(p.Preview.MatchCount)
}

// ruleLastActiveValue renders the last-active timestamp; the dash em
// keeps the tile non-empty when the rule has never matched.
func ruleLastActiveValue(p RuleDetailProps) string {
	if !p.HasLastActive {
		return "—"
	}
	return ruleRelativeTime(p.LastActiveTime)
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
// for the LastActiveTime stat. Thin alias over timefmt.Relative so the templ
// caller can stay package-local without importing timefmt.
func ruleRelativeTime(t time.Time) string {
	return timefmt.Relative(t)
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

// ruleActionEntityName returns the human display name for an assign_series /
// assign_counterparty action — the by-name value when the rule mints/binds by
// name, otherwise the short ID it targets, otherwise "". Used by the rule
// detail "Then" card so surrogate-first actions read legibly.
func ruleActionEntityName(a service.RuleAction) string {
	switch a.Type {
	case "assign_series":
		if n := strings.TrimSpace(a.SeriesName); n != "" {
			return n
		}
		return strings.TrimSpace(a.SeriesShortID)
	case "assign_counterparty":
		if n := strings.TrimSpace(a.CounterpartyName); n != "" {
			return n
		}
		return strings.TrimSpace(a.CounterpartyShortID)
	}
	return ""
}

// ruleMetadataValueText renders a set_metadata action's value for display.
// Returns "" when there is no value so the caller can omit the "= value" tail.
func ruleMetadataValueText(a service.RuleAction) string {
	if a.MetadataValue == nil {
		return ""
	}
	return fmt.Sprintf("%v", a.MetadataValue)
}

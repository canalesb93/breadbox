//go:build !lite

package service

// Field projection for recurring series and transaction rules, mirroring the
// transaction projection in fields.go. A series is now a thin, rule-maintained
// entity — id, short_id, name, type, tags, timestamps — so its projection is
// correspondingly small; a rule's conditions/actions trees stay excluded from
// the lean default alias.

// --- Recurring series ---

var seriesValidFields = map[string]bool{
	"id":         true,
	"short_id":   true,
	"name":       true,
	"type":       true,
	"tags":       true,
	"created_at": true,
	"updated_at": true,
}

var seriesFieldAliases = map[string][]string{
	// minimal: just enough to recognize a series in a list.
	"minimal": {"name", "type"},
	// overview: identity + type + tags — the useful default for list_series.
	"overview": {"name", "type", "tags"},
}

// DefaultSeriesFields is the lean projection list_series returns when the caller
// omits fields.
const DefaultSeriesFields = "overview"

// ParseSeriesFields parses the fields selector for recurring series.
func ParseSeriesFields(raw string) (map[string]bool, error) {
	return parseFieldsWith(raw, seriesValidFields, seriesFieldAliases)
}

// FilterSeriesFields projects a SeriesResponse down to the requested fields.
// Returns nil when fields is nil (caller uses the full struct).
func FilterSeriesFields(s SeriesResponse, fields map[string]bool) map[string]any {
	if fields == nil {
		return nil
	}
	m := make(map[string]any, len(fields))
	for key, want := range fields {
		if !want {
			continue
		}
		switch key {
		case "id":
			m["id"] = s.ID
		case "short_id":
			m["short_id"] = s.ShortID
		case "name":
			m["name"] = s.Name
		case "type":
			m["type"] = s.Type
		case "tags":
			m["tags"] = s.Tags
		case "created_at":
			m["created_at"] = s.CreatedAt
		case "updated_at":
			m["updated_at"] = s.UpdatedAt
		}
	}
	return m
}

// --- Transaction rules ---

var ruleValidFields = map[string]bool{
	"id":                    true,
	"short_id":              true,
	"name":                  true,
	"conditions":            true,
	"actions":               true,
	"trigger":               true,
	"category_slug":         true,
	"category_display_name": true,
	"category_icon":         true,
	"category_color":        true,
	"priority":              true,
	"enabled":               true,
	"expires_at":            true,
	"created_by_type":       true,
	"created_by_id":         true,
	"created_by_name":       true,
	"hit_count":             true,
	"last_hit_at":           true,
	"created_at":            true,
	"updated_at":            true,
}

var ruleFieldAliases = map[string][]string{
	// summary: roster view — what a rule is, whether it's on, how it's firing —
	// without the conditions and actions trees (the heavy, deeply-nested part
	// only needed when inspecting or editing a specific rule).
	"summary": {
		"name", "enabled", "priority", "trigger",
		"category_slug", "category_display_name",
		"created_by_type", "hit_count", "last_hit_at",
	},
}

// DefaultRuleFields is the lean projection list_transaction_rules returns when
// the caller omits fields.
const DefaultRuleFields = "summary"

// ParseRuleFields parses the fields selector for transaction rules.
func ParseRuleFields(raw string) (map[string]bool, error) {
	return parseFieldsWith(raw, ruleValidFields, ruleFieldAliases)
}

// FilterRuleFields projects a TransactionRuleResponse down to the requested
// fields. Returns nil when fields is nil (caller uses the full struct).
func FilterRuleFields(r TransactionRuleResponse, fields map[string]bool) map[string]any {
	if fields == nil {
		return nil
	}
	m := make(map[string]any, len(fields))
	for key, want := range fields {
		if !want {
			continue
		}
		switch key {
		case "id":
			m["id"] = r.ID
		case "short_id":
			m["short_id"] = r.ShortID
		case "name":
			m["name"] = r.Name
		case "conditions":
			m["conditions"] = r.Conditions
		case "actions":
			m["actions"] = r.Actions
		case "trigger":
			m["trigger"] = r.Trigger
		case "category_slug":
			m["category_slug"] = r.CategorySlug
		case "category_display_name":
			m["category_display_name"] = r.CategoryName
		case "category_icon":
			m["category_icon"] = r.CategoryIcon
		case "category_color":
			m["category_color"] = r.CategoryColor
		case "priority":
			m["priority"] = r.Priority
		case "enabled":
			m["enabled"] = r.Enabled
		case "expires_at":
			m["expires_at"] = r.ExpiresAt
		case "created_by_type":
			m["created_by_type"] = r.CreatedByType
		case "created_by_id":
			m["created_by_id"] = r.CreatedByID
		case "created_by_name":
			m["created_by_name"] = r.CreatedByName
		case "hit_count":
			m["hit_count"] = r.HitCount
		case "last_hit_at":
			m["last_hit_at"] = r.LastHitAt
		case "created_at":
			m["created_at"] = r.CreatedAt
		case "updated_at":
			m["updated_at"] = r.UpdatedAt
		}
	}
	return m
}

//go:build !lite

package service

// Field projection for recurring series and transaction rules, mirroring the
// transaction projection in fields.go. The heavy, rarely-needed payloads —
// a series' detection_signals JSON blob, a rule's conditions/actions trees —
// are excluded from each entity's lean default alias and only returned when
// the caller asks (fields=all, or names them explicitly).

// --- Recurring series ---

var seriesValidFields = map[string]bool{
	"id":                 true,
	"short_id":           true,
	"user_id":            true,
	"name":               true,
	"merchant_key":       true,
	"cadence":            true,
	"expected_day":       true,
	"expected_amount":    true,
	"amount_tolerance":   true,
	"iso_currency_code":  true,
	"category_id":        true,
	"status":             true,
	"type":               true,
	"detection_source":   true,
	"confidence":         true,
	"confirmed_by_type":  true,
	"last_amount":        true,
	"last_seen_date":     true,
	"next_expected_date": true,
	"renewal_health":     true,
	"days_until_renewal": true,
	"occurrence_count":   true,
	"detection_signals":  true,
	"tags":               true,
	"created_at":         true,
	"updated_at":         true,
}

var seriesFieldAliases = map[string][]string{
	// minimal: just enough to recognize a series in a list.
	"minimal": {"name", "status", "type", "cadence"},
	// overview: identity + lifecycle + renewal prediction — the useful default
	// for list_series. Deliberately omits detection_signals (verbose, only
	// needed for the get_series detail view), merchant_key/category_id internals,
	// detection_source, confirmed_by_type, tolerances, and timestamps.
	"overview": {
		"name", "status", "type", "cadence", "confidence",
		"expected_amount", "iso_currency_code", "expected_day",
		"last_amount", "last_seen_date", "next_expected_date",
		"renewal_health", "days_until_renewal", "occurrence_count", "tags",
	},
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
		case "user_id":
			m["user_id"] = s.UserID
		case "name":
			m["name"] = s.Name
		case "merchant_key":
			m["merchant_key"] = s.MerchantKey
		case "cadence":
			m["cadence"] = s.Cadence
		case "expected_day":
			m["expected_day"] = s.ExpectedDay
		case "expected_amount":
			m["expected_amount"] = s.ExpectedAmount
		case "amount_tolerance":
			m["amount_tolerance"] = s.AmountTolerance
		case "iso_currency_code":
			m["iso_currency_code"] = s.IsoCurrencyCode
		case "category_id":
			m["category_id"] = s.CategoryID
		case "status":
			m["status"] = s.Status
		case "type":
			m["type"] = s.Type
		case "detection_source":
			m["detection_source"] = s.DetectionSource
		case "confidence":
			m["confidence"] = s.Confidence
		case "confirmed_by_type":
			m["confirmed_by_type"] = s.ConfirmedByType
		case "last_amount":
			m["last_amount"] = s.LastAmount
		case "last_seen_date":
			m["last_seen_date"] = s.LastSeenDate
		case "next_expected_date":
			m["next_expected_date"] = s.NextExpectedDate
		case "renewal_health":
			m["renewal_health"] = s.RenewalHealth
		case "days_until_renewal":
			m["days_until_renewal"] = s.DaysUntilRenewal
		case "occurrence_count":
			m["occurrence_count"] = s.OccurrenceCount
		case "detection_signals":
			m["detection_signals"] = s.DetectionSignals
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

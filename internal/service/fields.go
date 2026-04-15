package service

import (
	"fmt"
	"sort"
	"strings"
)

// validFields is the set of JSON field names that can be selected on TransactionResponse.
var validFields = map[string]bool{
	"id":                   true,
	"short_id":             true,
	"account_id":           true,
	"account_name":         true,
	"user_name":            true,
	"amount":               true,
	"iso_currency_code":    true,
	"date":                 true,
	"authorized_date":      true,
	"datetime":             true,
	"authorized_datetime":  true,
	"name":                 true,
	"merchant_name":        true,
	"category":             true,
	"category_override":    true,
	"category_primary_raw": true,
	"category_detailed_raw": true,
	"category_confidence":  true,
	"payment_channel":       true,
	"pending":               true,
	"created_at":            true,
	"updated_at":            true,
}

// fieldAliases expand shorthand names to groups of fields.
var fieldAliases = map[string][]string{
	"minimal":    {"name", "amount", "date"},
	"core":       {"id", "date", "amount", "name", "iso_currency_code"},
	"category":   {"category", "category_primary_raw", "category_detailed_raw"},
	"timestamps": {"created_at", "updated_at", "datetime", "authorized_datetime"},
}

// ParseFields parses and validates the fields query parameter.
// Returns nil if no field selection (return all fields).
func ParseFields(raw string) (map[string]bool, error) {
	if raw == "" {
		return nil, nil
	}
	fields := make(map[string]bool)
	var unknown []string
	for _, f := range strings.Split(raw, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if expanded, ok := fieldAliases[f]; ok {
			for _, ef := range expanded {
				fields[ef] = true
			}
			continue
		}
		if !validFields[f] {
			unknown = append(unknown, f)
			continue
		}
		fields[f] = true
	}
	if len(unknown) > 0 {
		validList := make([]string, 0, len(validFields)+len(fieldAliases))
		for k := range validFields {
			validList = append(validList, k)
		}
		for k := range fieldAliases {
			validList = append(validList, k)
		}
		sort.Strings(validList)
		return nil, fmt.Errorf("unknown field(s): %s. Valid fields: %s", strings.Join(unknown, ", "), strings.Join(validList, ", "))
	}
	fields["id"] = true       // always include
	fields["short_id"] = true // always include
	return fields, nil
}

// Phase 3 retired ParseReviewFields / FilterReviewFields along with the
// review_queue table. Review-field selection on the deprecated MCP shims is
// dropped — the shims return tag-filtered transactions in their entirety.

// NormalizeTransactionAttribution merges attributed_user into user_name and
// clears the attributed_user_* fields. This gives MCP consumers a single
// "user_name" that always reflects the effective user (attributed or owner).
func NormalizeTransactionAttribution(t *TransactionResponse) {
	if t.AttributedUserName != nil {
		t.UserName = t.AttributedUserName
	}
	t.AttributedUserID = nil
	t.AttributedUserName = nil
}

// FilterTransactionFields returns a map with only the requested fields.
// If fields is nil, returns nil to signal the caller should use the full struct.
//
// This iterates the requested field set once (O(requested)) rather than
// performing a fixed O(22) map lookup per call. For small field sets
// (e.g. "core" = 5 fields) this is several times faster per transaction,
// and the total work is proportional to what the caller asked for.
func FilterTransactionFields(t TransactionResponse, fields map[string]bool) map[string]any {
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
			m["id"] = t.ID
		case "short_id":
			m["short_id"] = t.ShortID
		case "account_id":
			m["account_id"] = t.AccountID
			if t.AccountShortID != nil {
				m["account_short_id"] = t.AccountShortID
			}
		case "account_name":
			m["account_name"] = t.AccountName
		case "user_name":
			// Use attributed user when set (account linking), otherwise connection owner.
			if t.AttributedUserName != nil {
				m["user_name"] = t.AttributedUserName
			} else {
				m["user_name"] = t.UserName
			}
		case "amount":
			m["amount"] = t.Amount
		case "iso_currency_code":
			m["iso_currency_code"] = t.IsoCurrencyCode
		case "date":
			m["date"] = t.Date
		case "authorized_date":
			m["authorized_date"] = t.AuthorizedDate
		case "datetime":
			m["datetime"] = t.Datetime
		case "authorized_datetime":
			m["authorized_datetime"] = t.AuthorizedDatetime
		case "name":
			m["name"] = t.Name
		case "merchant_name":
			m["merchant_name"] = t.MerchantName
		case "category":
			m["category"] = t.Category
		case "category_override":
			m["category_override"] = t.CategoryOverride
		case "category_primary_raw":
			m["category_primary_raw"] = t.CategoryPrimaryRaw
		case "category_detailed_raw":
			m["category_detailed_raw"] = t.CategoryDetailedRaw
		case "category_confidence":
			m["category_confidence"] = t.CategoryConfidence
		case "payment_channel":
			m["payment_channel"] = t.PaymentChannel
		case "pending":
			m["pending"] = t.Pending
		case "created_at":
			m["created_at"] = t.CreatedAt
		case "updated_at":
			m["updated_at"] = t.UpdatedAt
		}
		// Keys outside the switch are silently ignored. ParseFields already
		// rejects unknown names; review delegation strips "transaction." before
		// calling this function, so callers always pass valid field names.
	}
	return m
}

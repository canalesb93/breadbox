package service

import (
	"fmt"
	"strings"
)

// validFields is the set of JSON field names that can be selected on TransactionResponse.
var validFields = map[string]bool{
	"id":                   true,
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
	"payment_channel":      true,
	"pending":              true,
	"created_at":           true,
	"updated_at":           true,
}

// fieldAliases expand shorthand names to groups of fields.
var fieldAliases = map[string][]string{
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
		return nil, fmt.Errorf("unknown field(s): %s. Valid fields: %s", strings.Join(unknown, ", "), strings.Join(validList, ", "))
	}
	fields["id"] = true // always include
	return fields, nil
}

// FilterTransactionFields returns a map with only the requested fields.
// If fields is nil, returns nil to signal the caller should use the full struct.
func FilterTransactionFields(t TransactionResponse, fields map[string]bool) map[string]any {
	if fields == nil {
		return nil
	}
	m := make(map[string]any, len(fields))
	if fields["id"] {
		m["id"] = t.ID
	}
	if fields["account_id"] {
		m["account_id"] = t.AccountID
	}
	if fields["account_name"] {
		m["account_name"] = t.AccountName
	}
	if fields["user_name"] {
		m["user_name"] = t.UserName
	}
	if fields["amount"] {
		m["amount"] = t.Amount
	}
	if fields["iso_currency_code"] {
		m["iso_currency_code"] = t.IsoCurrencyCode
	}
	if fields["date"] {
		m["date"] = t.Date
	}
	if fields["authorized_date"] {
		m["authorized_date"] = t.AuthorizedDate
	}
	if fields["datetime"] {
		m["datetime"] = t.Datetime
	}
	if fields["authorized_datetime"] {
		m["authorized_datetime"] = t.AuthorizedDatetime
	}
	if fields["name"] {
		m["name"] = t.Name
	}
	if fields["merchant_name"] {
		m["merchant_name"] = t.MerchantName
	}
	if fields["category"] {
		m["category"] = t.Category
	}
	if fields["category_override"] {
		m["category_override"] = t.CategoryOverride
	}
	if fields["category_primary_raw"] {
		m["category_primary_raw"] = t.CategoryPrimaryRaw
	}
	if fields["category_detailed_raw"] {
		m["category_detailed_raw"] = t.CategoryDetailedRaw
	}
	if fields["category_confidence"] {
		m["category_confidence"] = t.CategoryConfidence
	}
	if fields["payment_channel"] {
		m["payment_channel"] = t.PaymentChannel
	}
	if fields["pending"] {
		m["pending"] = t.Pending
	}
	if fields["created_at"] {
		m["created_at"] = t.CreatedAt
	}
	if fields["updated_at"] {
		m["updated_at"] = t.UpdatedAt
	}
	return m
}

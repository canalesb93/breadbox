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

// --- Review field selection ---

// validReviewFields is the set of JSON field names that can be selected on ReviewResponse.
var validReviewFields = map[string]bool{
	"id":                      true,
	"short_id":                true,
	"transaction_id":          true,
	"review_type":             true,
	"status":                  true,
	"provider":                true,
	"suggested_category_slug": true,
	"confidence_score":        true,
	"reviewer_type":           true,
	"reviewer_id":             true,
	"reviewer_name":           true,
	"review_note":             true,
	"resolved_category_slug":  true,
	"created_at":              true,
	"reviewed_at":             true,
	// transaction.* fields are handled via delegation
	"transaction.id":                   true,
	"transaction.name":                 true,
	"transaction.amount":               true,
	"transaction.date":                 true,
	"transaction.account_name":         true,
	"transaction.user_name":            true,
	"transaction.merchant_name":        true,
	"transaction.category_primary_raw": true,
	"transaction.account_id":           true,
	"transaction.iso_currency_code":    true,
	"transaction.pending":              true,
	"transaction.category":             true,
}

// reviewFieldAliases expand shorthand names to groups of review fields.
var reviewFieldAliases = map[string][]string{
	"triage": {
		"id", "review_type", "status", "suggested_category_slug",
		"transaction.name", "transaction.amount", "transaction.date",
		"transaction.category_primary_raw", "transaction.account_name",
		"transaction.user_name", "transaction.merchant_name",
	},
	"review_core": {
		"id", "review_type", "status", "suggested_category_slug",
		"confidence_score", "created_at",
	},
	"transaction_core": {
		"transaction.id", "transaction.name", "transaction.amount",
		"transaction.date", "transaction.category_primary_raw",
		"transaction.account_name", "transaction.user_name",
	},
}

// ParseReviewFields parses and validates the fields query parameter for reviews.
// Returns nil if no field selection (return all fields).
func ParseReviewFields(raw string) (map[string]bool, error) {
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
		if expanded, ok := reviewFieldAliases[f]; ok {
			for _, ef := range expanded {
				fields[ef] = true
			}
			continue
		}
		if !validReviewFields[f] {
			unknown = append(unknown, f)
			continue
		}
		fields[f] = true
	}
	if len(unknown) > 0 {
		validList := make([]string, 0, len(validReviewFields)+len(reviewFieldAliases))
		for k := range validReviewFields {
			validList = append(validList, k)
		}
		for k := range reviewFieldAliases {
			validList = append(validList, k)
		}
		sort.Strings(validList)
		return nil, fmt.Errorf("unknown field(s): %s. Valid fields: %s", strings.Join(unknown, ", "), strings.Join(validList, ", "))
	}
	fields["id"] = true       // always include
	fields["short_id"] = true // always include
	return fields, nil
}

// FilterReviewFields returns a map with only the requested review fields.
// Transaction.* fields are delegated to FilterTransactionFields.
// If fields is nil, returns nil to signal the caller should use the full struct.
func FilterReviewFields(r ReviewResponse, fields map[string]bool) map[string]any {
	if fields == nil {
		return nil
	}
	m := make(map[string]any, len(fields))
	if fields["id"] {
		m["id"] = r.ID
	}
	if fields["short_id"] {
		m["short_id"] = r.ShortID
	}
	if fields["transaction_id"] {
		m["transaction_id"] = r.TransactionID
	}
	if fields["review_type"] {
		m["review_type"] = r.ReviewType
	}
	if fields["status"] {
		m["status"] = r.Status
	}
	if fields["provider"] {
		m["provider"] = r.Provider
	}
	if fields["suggested_category_slug"] {
		m["suggested_category_slug"] = r.SuggestedCategory
	}
	if fields["suggested_category_display_name"] {
		m["suggested_category_display_name"] = r.SuggestedCategoryDisplayName
	}
	if fields["confidence_score"] {
		m["confidence_score"] = r.ConfidenceScore
	}
	if fields["reviewer_type"] {
		m["reviewer_type"] = r.ReviewerType
	}
	if fields["reviewer_id"] {
		m["reviewer_id"] = r.ReviewerID
	}
	if fields["reviewer_name"] {
		m["reviewer_name"] = r.ReviewerName
	}
	if fields["review_note"] {
		m["review_note"] = r.ReviewNote
	}
	if fields["resolved_category_slug"] {
		m["resolved_category_slug"] = r.ResolvedCategory
	}
	if fields["created_at"] {
		m["created_at"] = r.CreatedAt
	}
	if fields["reviewed_at"] {
		m["reviewed_at"] = r.ReviewedAt
	}

	// Delegate transaction.* fields
	if r.Transaction != nil {
		txnFields := make(map[string]bool)
		for f := range fields {
			if strings.HasPrefix(f, "transaction.") {
				txnFields[strings.TrimPrefix(f, "transaction.")] = true
			}
		}
		if len(txnFields) > 0 {
			m["transaction"] = FilterTransactionFields(*r.Transaction, txnFields)
		}
	}

	return m
}

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
func FilterTransactionFields(t TransactionResponse, fields map[string]bool) map[string]any {
	if fields == nil {
		return nil
	}
	m := make(map[string]any, len(fields))
	if fields["id"] {
		m["id"] = t.ID
	}
	if fields["short_id"] {
		m["short_id"] = t.ShortID
	}
	if fields["account_id"] {
		m["account_id"] = t.AccountID
	}
	if fields["account_name"] {
		m["account_name"] = t.AccountName
	}
	if fields["user_name"] {
		// Use attributed user when set (account linking), otherwise connection owner.
		if t.AttributedUserName != nil {
			m["user_name"] = t.AttributedUserName
		} else {
			m["user_name"] = t.UserName
		}
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

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
//
// Review-level keys are dispatched via a switch over the requested field set
// (O(requested) rather than O(16) unconditional map lookups). If any
// transaction.* keys were requested, a second narrow-scoped pass builds the
// nested txnFields map and delegates. Keeping the txnFields allocation inside
// that inner block lets the compiler prove it does not escape, so it stays
// stack-allocated (matches the pre-optimization allocation profile).
func FilterReviewFields(r ReviewResponse, fields map[string]bool) map[string]any {
	if fields == nil {
		return nil
	}
	m := make(map[string]any, len(fields))
	hasTxnField := false
	for key, want := range fields {
		// Detect transaction.* keys eagerly (before the want check) to match
		// the original implementation's delegation semantics: in the
		// pre-optimization code, the second loop scanning for "transaction."
		// prefixes did not consult the bool value, so a present-but-false
		// transaction.* key still triggered the nested filter call. Practical
		// callers always set values to true (ParseReviewFields enforces this),
		// but we preserve the prior behavior exactly.
		if strings.HasPrefix(key, "transaction.") {
			hasTxnField = true
			continue
		}
		if !want {
			continue
		}
		switch key {
		case "id":
			m["id"] = r.ID
		case "short_id":
			m["short_id"] = r.ShortID
		case "transaction_id":
			m["transaction_id"] = r.TransactionID
			if r.TransactionShortID != nil {
				m["transaction_short_id"] = r.TransactionShortID
			}
		case "review_type":
			m["review_type"] = r.ReviewType
		case "status":
			m["status"] = r.Status
		case "provider":
			m["provider"] = r.Provider
		case "suggested_category_slug":
			m["suggested_category_slug"] = r.SuggestedCategory
		case "suggested_category_display_name":
			m["suggested_category_display_name"] = r.SuggestedCategoryDisplayName
		case "confidence_score":
			m["confidence_score"] = r.ConfidenceScore
		case "reviewer_type":
			m["reviewer_type"] = r.ReviewerType
		case "reviewer_id":
			m["reviewer_id"] = r.ReviewerID
		case "reviewer_name":
			m["reviewer_name"] = r.ReviewerName
		case "resolved_category_slug":
			m["resolved_category_slug"] = r.ResolvedCategory
		case "created_at":
			m["created_at"] = r.CreatedAt
		case "reviewed_at":
			m["reviewed_at"] = r.ReviewedAt
		}
	}

	// Delegate transaction.* fields. Building txnFields in this narrow scope
	// keeps it stack-allocated (the compiler can see it never outlives the
	// nested call).
	if hasTxnField && r.Transaction != nil {
		txnFields := make(map[string]bool)
		for f := range fields {
			if strings.HasPrefix(f, "transaction.") {
				txnFields[strings.TrimPrefix(f, "transaction.")] = true
			}
		}
		m["transaction"] = FilterTransactionFields(*r.Transaction, txnFields)
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

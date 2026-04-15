package service

import (
	"strings"
	"testing"
)

func TestParseFields_Empty(t *testing.T) {
	fields, err := ParseFields("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fields != nil {
		t.Fatal("expected nil for empty input")
	}
}

func TestParseFields_SingleField(t *testing.T) {
	fields, err := ParseFields("amount")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fields["amount"] {
		t.Error("expected amount to be selected")
	}
	if !fields["id"] {
		t.Error("id should always be included")
	}
}

func TestParseFields_MultipleFields(t *testing.T) {
	fields, err := ParseFields("amount,date,name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, f := range []string{"id", "amount", "date", "name"} {
		if !fields[f] {
			t.Errorf("expected %s to be selected", f)
		}
	}
	if fields["merchant_name"] {
		t.Error("merchant_name should not be selected")
	}
}

func TestParseFields_CoreAlias(t *testing.T) {
	fields, err := ParseFields("core")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, f := range []string{"id", "date", "amount", "name", "iso_currency_code"} {
		if !fields[f] {
			t.Errorf("core alias should include %s", f)
		}
	}
}

func TestParseFields_CategoryAlias(t *testing.T) {
	fields, err := ParseFields("category")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, f := range []string{"id", "category", "category_primary_raw", "category_detailed_raw"} {
		if !fields[f] {
			t.Errorf("category alias should include %s", f)
		}
	}
}

func TestParseFields_TimestampsAlias(t *testing.T) {
	fields, err := ParseFields("timestamps")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, f := range []string{"id", "created_at", "updated_at", "datetime", "authorized_datetime"} {
		if !fields[f] {
			t.Errorf("timestamps alias should include %s", f)
		}
	}
}

func TestParseFields_MixedAliasAndField(t *testing.T) {
	fields, err := ParseFields("core,merchant_name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fields["amount"] {
		t.Error("core alias should expand")
	}
	if !fields["merchant_name"] {
		t.Error("explicit field should be included")
	}
}

func TestParseFields_UnknownField(t *testing.T) {
	_, err := ParseFields("amount,bogus_field")
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	if !strings.Contains(err.Error(), "bogus_field") {
		t.Errorf("error should mention the unknown field: %v", err)
	}
}

func TestParseFields_UnknownFieldSortedList(t *testing.T) {
	_, err := ParseFields("zzz_bad")
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	// Find the "Valid fields:" portion and verify it's sorted
	idx := strings.Index(msg, "Valid fields: ")
	if idx == -1 {
		t.Fatal("expected 'Valid fields:' in error message")
	}
	fieldListStr := msg[idx+len("Valid fields: "):]
	fieldList := strings.Split(fieldListStr, ", ")
	for i := 1; i < len(fieldList); i++ {
		if fieldList[i-1] > fieldList[i] {
			t.Errorf("valid fields not sorted: %q > %q", fieldList[i-1], fieldList[i])
			break
		}
	}
}

func TestParseFields_WhitespaceHandling(t *testing.T) {
	fields, err := ParseFields(" amount , date , name ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, f := range []string{"amount", "date", "name"} {
		if !fields[f] {
			t.Errorf("expected %s to be selected after trimming", f)
		}
	}
}

func TestParseFields_EmptySegments(t *testing.T) {
	fields, err := ParseFields("amount,,date")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fields["amount"] || !fields["date"] {
		t.Error("should skip empty segments")
	}
}

func TestParseFields_IDAlwaysIncluded(t *testing.T) {
	fields, err := ParseFields("amount")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fields["id"] {
		t.Error("id should always be included even when not requested")
	}
}

func TestFilterTransactionFields_Nil(t *testing.T) {
	txn := TransactionResponse{ID: "abc"}
	result := FilterTransactionFields(txn, nil)
	if result != nil {
		t.Error("nil fields should return nil (signal to use full struct)")
	}
}

func TestFilterTransactionFields_SelectsCorrectly(t *testing.T) {
	txn := TransactionResponse{
		ID:     "txn-1",
		Amount: 42.50,
		Name:   "Test Store",
		Date:   "2024-01-15",
	}
	fields := map[string]bool{"id": true, "amount": true, "name": true}
	result := FilterTransactionFields(txn, fields)

	if result["id"] != "txn-1" {
		t.Error("expected id")
	}
	if result["amount"] != 42.50 {
		t.Error("expected amount")
	}
	if result["name"] != "Test Store" {
		t.Error("expected name")
	}
	if _, ok := result["date"]; ok {
		t.Error("date should not be in filtered result")
	}
}

// Phase 3 retired ParseReviewFields / FilterReviewFields along with the
// review_queue. Their tests have moved out with them.

func strPtr(s string) *string { return &s }

func TestFilterTransactionFields_AllFields(t *testing.T) {
	// Build a fieldSet with all valid fields
	fields, err := ParseFields("id,account_id,account_name,user_name,amount,iso_currency_code,date,authorized_date,datetime,authorized_datetime,name,merchant_name,category,category_override,category_primary_raw,category_detailed_raw,category_confidence,payment_channel,pending,created_at,updated_at")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	txn := TransactionResponse{ID: "txn-all"}
	result := FilterTransactionFields(txn, fields)

	// Should have all valid field keys
	if len(result) != len(validFields) {
		t.Errorf("expected %d fields, got %d", len(validFields), len(result))
	}
}

func TestParseFields_AttributedFieldsRejected(t *testing.T) {
	_, err := ParseFields("attributed_user_id")
	if err == nil {
		t.Fatal("expected error for removed attributed_user_id field")
	}
	_, err = ParseFields("attributed_user_name")
	if err == nil {
		t.Fatal("expected error for removed attributed_user_name field")
	}
}

func TestParseFields_MinimalAlias(t *testing.T) {
	fields, err := ParseFields("minimal")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, f := range []string{"id", "name", "amount", "date"} {
		if !fields[f] {
			t.Errorf("minimal alias should include %s", f)
		}
	}
	// Should NOT include other core fields
	for _, f := range []string{"iso_currency_code", "account_id", "merchant_name"} {
		if fields[f] {
			t.Errorf("minimal alias should not include %s", f)
		}
	}
}

func TestNormalizeTransactionAttribution(t *testing.T) {
	owner := "Alice"
	attributed := "Ricardo"

	// When attributed_user is set, user_name should be overridden.
	txn := TransactionResponse{
		ID:                 "txn-1",
		UserName:           &owner,
		AttributedUserID:   strPtr("user-123"),
		AttributedUserName: &attributed,
	}
	NormalizeTransactionAttribution(&txn)

	if txn.UserName == nil || *txn.UserName != "Ricardo" {
		t.Errorf("expected user_name to be 'Ricardo', got %v", txn.UserName)
	}
	if txn.AttributedUserID != nil {
		t.Error("expected attributed_user_id to be cleared")
	}
	if txn.AttributedUserName != nil {
		t.Error("expected attributed_user_name to be cleared")
	}

	// When no attribution, user_name stays as-is.
	txn2 := TransactionResponse{
		ID:       "txn-2",
		UserName: &owner,
	}
	NormalizeTransactionAttribution(&txn2)

	if txn2.UserName == nil || *txn2.UserName != "Alice" {
		t.Errorf("expected user_name to remain 'Alice', got %v", txn2.UserName)
	}
}

func TestFilterTransactionFields_UserNameUsesAttribution(t *testing.T) {
	attributed := "Ricardo"
	owner := "Alice"
	txn := TransactionResponse{
		ID:                 "txn-1",
		UserName:           &owner,
		AttributedUserName: &attributed,
	}
	// After normalization (as MCP handler would do):
	NormalizeTransactionAttribution(&txn)

	fields := map[string]bool{"id": true, "user_name": true}
	result := FilterTransactionFields(txn, fields)

	name, ok := result["user_name"].(*string)
	if !ok || *name != "Ricardo" {
		t.Errorf("expected user_name to be 'Ricardo' after normalization, got %v", result["user_name"])
	}
}

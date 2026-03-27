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

// --- ParseReviewFields tests ---

func TestParseReviewFields_Empty(t *testing.T) {
	fields, err := ParseReviewFields("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fields != nil {
		t.Fatal("expected nil for empty input")
	}
}

func TestParseReviewFields_TriageAlias(t *testing.T) {
	fields, err := ParseReviewFields("triage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []string{
		"id", "review_type", "status", "suggested_category_slug",
		"transaction.name", "transaction.amount", "transaction.date",
		"transaction.category_primary_raw", "transaction.account_name",
		"transaction.user_name", "transaction.merchant_name",
	}
	for _, f := range expected {
		if !fields[f] {
			t.Errorf("triage alias should include %s", f)
		}
	}
}

func TestParseReviewFields_ReviewCoreAlias(t *testing.T) {
	fields, err := ParseReviewFields("review_core")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []string{"id", "review_type", "status", "suggested_category_slug", "confidence_score", "created_at"}
	for _, f := range expected {
		if !fields[f] {
			t.Errorf("review_core alias should include %s", f)
		}
	}
}

func TestParseReviewFields_TransactionCoreAlias(t *testing.T) {
	fields, err := ParseReviewFields("transaction_core")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []string{
		"transaction.id", "transaction.name", "transaction.amount",
		"transaction.date", "transaction.category_primary_raw",
		"transaction.account_name", "transaction.user_name",
	}
	for _, f := range expected {
		if !fields[f] {
			t.Errorf("transaction_core alias should include %s", f)
		}
	}
	if !fields["id"] {
		t.Error("id should always be included")
	}
}

func TestParseReviewFields_MixedAliasAndField(t *testing.T) {
	fields, err := ParseReviewFields("review_core,transaction.name,provider")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// From review_core alias
	if !fields["confidence_score"] {
		t.Error("review_core alias should expand")
	}
	// Individual fields
	if !fields["transaction.name"] {
		t.Error("explicit transaction.name should be included")
	}
	if !fields["provider"] {
		t.Error("explicit provider should be included")
	}
}

func TestParseReviewFields_UnknownField(t *testing.T) {
	_, err := ParseReviewFields("status,bogus_field")
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	if !strings.Contains(err.Error(), "bogus_field") {
		t.Errorf("error should mention the unknown field: %v", err)
	}
}

func TestParseReviewFields_IDAlwaysIncluded(t *testing.T) {
	fields, err := ParseReviewFields("status")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fields["id"] {
		t.Error("id should always be included even when not requested")
	}
}

func TestParseReviewFields_TransactionPrefixedFields(t *testing.T) {
	fields, err := ParseReviewFields("transaction.name,transaction.amount")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fields["transaction.name"] {
		t.Error("transaction.name should be selected")
	}
	if !fields["transaction.amount"] {
		t.Error("transaction.amount should be selected")
	}
	if !fields["id"] {
		t.Error("id should always be included")
	}
}

// --- FilterReviewFields tests ---

func TestFilterReviewFields_Nil(t *testing.T) {
	r := ReviewResponse{ID: "rev-1"}
	result := FilterReviewFields(r, nil)
	if result != nil {
		t.Error("nil fields should return nil (signal to use full struct)")
	}
}

func TestFilterReviewFields_ReviewLevelFields(t *testing.T) {
	r := ReviewResponse{
		ID:         "rev-1",
		ReviewType: "uncategorized",
		Status:     "pending",
	}
	fields := map[string]bool{"id": true, "review_type": true, "status": true}
	result := FilterReviewFields(r, fields)

	if result["id"] != "rev-1" {
		t.Error("expected id")
	}
	if result["review_type"] != "uncategorized" {
		t.Error("expected review_type")
	}
	if result["status"] != "pending" {
		t.Error("expected status")
	}
	if _, ok := result["transaction_id"]; ok {
		t.Error("transaction_id should not be in filtered result")
	}
}

func TestFilterReviewFields_TransactionNested(t *testing.T) {
	txn := &TransactionResponse{
		ID:     "txn-1",
		Name:   "Coffee Shop",
		Amount: 5.50,
		Date:   "2024-01-15",
	}
	r := ReviewResponse{
		ID:          "rev-1",
		Transaction: txn,
	}
	fields := map[string]bool{
		"id":               true,
		"transaction.name": true,
		"transaction.amount": true,
	}
	result := FilterReviewFields(r, fields)

	if result["id"] != "rev-1" {
		t.Error("expected id")
	}
	txnMap, ok := result["transaction"].(map[string]any)
	if !ok {
		t.Fatal("expected transaction to be a map")
	}
	if txnMap["name"] != "Coffee Shop" {
		t.Error("expected transaction.name")
	}
	if txnMap["amount"] != 5.50 {
		t.Error("expected transaction.amount")
	}
	if _, ok := txnMap["date"]; ok {
		t.Error("transaction.date should not be in filtered result")
	}
}

func TestFilterReviewFields_TriageAlias(t *testing.T) {
	slug := "uncategorized"
	txn := &TransactionResponse{
		ID:                "txn-1",
		Name:              "Grocery Store",
		Amount:            85.20,
		Date:              "2024-02-01",
		AccountName:       strPtr("Checking"),
		UserName:          strPtr("Alice"),
		MerchantName:      strPtr("Whole Foods"),
		CategoryPrimaryRaw: strPtr("FOOD_AND_DRINK"),
	}
	r := ReviewResponse{
		ID:                "rev-1",
		ReviewType:        "uncategorized",
		Status:            "pending",
		SuggestedCategory: &slug,
		Transaction:       txn,
	}

	fields, err := ParseReviewFields("triage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	result := FilterReviewFields(r, fields)

	// Review-level fields
	if result["id"] != "rev-1" {
		t.Error("expected id")
	}
	if result["review_type"] != "uncategorized" {
		t.Error("expected review_type")
	}
	if result["status"] != "pending" {
		t.Error("expected status")
	}
	if result["suggested_category_slug"] != &slug {
		t.Error("expected suggested_category_slug")
	}

	// Transaction fields nested
	txnMap, ok := result["transaction"].(map[string]any)
	if !ok {
		t.Fatal("expected transaction to be a map")
	}
	if txnMap["name"] != "Grocery Store" {
		t.Error("expected transaction.name")
	}
	if txnMap["amount"] != 85.20 {
		t.Error("expected transaction.amount")
	}
	if v, ok := txnMap["account_name"].(*string); !ok || *v != "Checking" {
		t.Error("expected transaction.account_name")
	}
	if v, ok := txnMap["user_name"].(*string); !ok || *v != "Alice" {
		t.Error("expected transaction.user_name")
	}

	// Fields NOT in triage should be absent
	if _, ok := result["confidence_score"]; ok {
		t.Error("confidence_score should not be in triage result")
	}
	if _, ok := result["transaction_id"]; ok {
		t.Error("transaction_id should not be in triage result")
	}
}

func strPtr(s string) *string { return &s }

func TestFilterTransactionFields_AllFields(t *testing.T) {
	// Build a fieldSet with all valid fields
	fields, err := ParseFields("id,account_id,account_name,user_name,amount,iso_currency_code,date,authorized_date,datetime,authorized_datetime,name,merchant_name,category,category_override,category_primary_raw,category_detailed_raw,category_confidence,payment_channel,pending,created_at,updated_at,attributed_user_id,attributed_user_name")
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

func TestParseFields_AttributedFields(t *testing.T) {
	fields, err := ParseFields("attributed_user_id,attributed_user_name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fields["attributed_user_id"] {
		t.Error("expected attributed_user_id to be selected")
	}
	if !fields["attributed_user_name"] {
		t.Error("expected attributed_user_name to be selected")
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

func TestFilterTransactionFields_AttributedFields(t *testing.T) {
	uid := "user-123"
	uname := "Ricardo"
	txn := TransactionResponse{
		ID:                 "txn-1",
		AttributedUserID:   &uid,
		AttributedUserName: &uname,
	}
	fields := map[string]bool{"id": true, "attributed_user_id": true, "attributed_user_name": true}
	result := FilterTransactionFields(txn, fields)

	if result["attributed_user_id"] != &uid {
		t.Error("expected attributed_user_id")
	}
	if result["attributed_user_name"] != &uname {
		t.Error("expected attributed_user_name")
	}
}

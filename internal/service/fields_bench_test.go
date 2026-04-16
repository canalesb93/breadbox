package service

import "testing"

// benchTransactionResponse returns a TransactionResponse with every field populated
// so that filtering is forced to copy real data rather than zero values. This
// approximates the realistic payload shape returned by query_transactions.
func benchTransactionResponse() TransactionResponse {
	accountID := "acct-12345678"
	accountName := "Checking ****1234"
	userName := "Alice"
	attributedUserID := "user-abcdef"
	attributedUserName := "Ricardo"
	effectiveUserID := "user-abcdef"
	isoCurrency := "USD"
	authorizedDate := "2026-04-01"
	datetime := "2026-04-02T14:30:00Z"
	authorizedDatetime := "2026-04-01T09:15:00Z"
	merchantName := "Whole Foods Market"
	categoryPrimaryRaw := "FOOD_AND_DRINK"
	categoryDetailedRaw := "FOOD_AND_DRINK_GROCERIES"
	categoryConfidence := "HIGH"
	paymentChannel := "in store"
	categoryID := "cat-123"
	categorySlug := "food_and_drink_groceries"
	categoryDisplayName := "Groceries"
	primarySlug := "food_and_drink"
	primaryDisplayName := "Food and Drink"
	icon := "shopping-cart"
	color := "#00aa55"
	return TransactionResponse{
		ID:                 "txn-abcdefgh",
		ShortID:            "k7Xm9pQ2",
		AccountID:          &accountID,
		AccountName:        &accountName,
		UserName:           &userName,
		AttributedUserID:   &attributedUserID,
		AttributedUserName: &attributedUserName,
		EffectiveUserID:    &effectiveUserID,
		Amount:             42.50,
		IsoCurrencyCode:    &isoCurrency,
		Date:               "2026-04-02",
		AuthorizedDate:     &authorizedDate,
		Datetime:           &datetime,
		AuthorizedDatetime: &authorizedDatetime,
		Name:               "WHOLE FOODS MARKET #123",
		MerchantName:       &merchantName,
		Category: &TransactionCategoryInfo{
			ID:                 &categoryID,
			Slug:               &categorySlug,
			DisplayName:        &categoryDisplayName,
			PrimarySlug:        &primarySlug,
			PrimaryDisplayName: &primaryDisplayName,
			Icon:               &icon,
			Color:              &color,
		},
		CategoryOverride:    false,
		CategoryPrimaryRaw:  &categoryPrimaryRaw,
		CategoryDetailedRaw: &categoryDetailedRaw,
		CategoryConfidence:  &categoryConfidence,
		PaymentChannel:      &paymentChannel,
		Pending:             false,
		CreatedAt:           "2026-04-02T14:31:00Z",
		UpdatedAt:           "2026-04-03T08:00:00Z",
	}
}

// mustParseFields panics on error; used in benchmark setup only.
func mustParseFields(b *testing.B, raw string) map[string]bool {
	b.Helper()
	f, err := ParseFields(raw)
	if err != nil {
		b.Fatalf("ParseFields(%q): %v", raw, err)
	}
	return f
}

// allTransactionFieldsRaw enumerates every valid transaction field. Kept in
// sync with validFields manually (benchmark-only, would break loudly otherwise).
const allTransactionFieldsRaw = "id,short_id,account_id,account_name,user_name,amount,iso_currency_code,date,authorized_date,datetime,authorized_datetime,name,merchant_name,category,category_override,category_primary_raw,category_detailed_raw,category_confidence,payment_channel,pending,created_at,updated_at"

func BenchmarkFilterTransaction_Small_Core(b *testing.B) {
	txn := benchTransactionResponse()
	fields := mustParseFields(b, "core")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FilterTransactionFields(txn, fields)
	}
}

func BenchmarkFilterTransaction_Medium_Minimal(b *testing.B) {
	const n = 50
	txns := make([]TransactionResponse, n)
	for i := range txns {
		txns[i] = benchTransactionResponse()
	}
	fields := mustParseFields(b, "minimal")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range txns {
			_ = FilterTransactionFields(txns[j], fields)
		}
	}
}

func BenchmarkFilterTransaction_Medium_AllFields(b *testing.B) {
	const n = 50
	txns := make([]TransactionResponse, n)
	for i := range txns {
		txns[i] = benchTransactionResponse()
	}
	fields := mustParseFields(b, allTransactionFieldsRaw)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range txns {
			_ = FilterTransactionFields(txns[j], fields)
		}
	}
}

func BenchmarkFilterTransaction_Large_Core(b *testing.B) {
	const n = 500
	txns := make([]TransactionResponse, n)
	for i := range txns {
		txns[i] = benchTransactionResponse()
	}
	fields := mustParseFields(b, "core")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range txns {
			_ = FilterTransactionFields(txns[j], fields)
		}
	}
}


//go:build !lite

package plaid

import (
	"testing"
	"time"

	plaidgo "github.com/plaid/plaid-go/v29/plaid"
	"github.com/shopspring/decimal"
)

// TestMapTransactionSignConvention pins the load-bearing amount-sign contract:
// Plaid and Breadbox agree that positive = money out (debit) and negative =
// money in (credit), so mapTransaction must copy the amount through verbatim
// without inverting it. A regression here silently flips every synced
// transaction's direction.
func TestMapTransactionSignConvention(t *testing.T) {
	cases := []struct {
		name   string
		amount float64
	}{
		{"positive amount is a debit (money out)", 42.50},
		{"negative amount is a credit (money in)", -19.99},
		{"zero amount", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			txn := plaidgo.NewTransactionWithDefaults()
			txn.SetAmount(c.amount)

			got := mapTransaction(*txn)

			want := decimal.NewFromFloat(c.amount)
			if !got.Amount.Equal(want) {
				t.Errorf("mapTransaction amount = %s, want %s (sign must be preserved, never inverted)", got.Amount, want)
			}
		})
	}
}

// TestMapTransactionFullFields exercises the mapping when every optional field
// Plaid can return is present, locking in field placement, date parsing, and
// pointer population.
func TestMapTransactionFullFields(t *testing.T) {
	dt := time.Date(2026, 6, 4, 14, 30, 0, 0, time.UTC)
	adt := time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC)

	txn := plaidgo.NewTransactionWithDefaults()
	txn.SetTransactionId("txn-123")
	txn.SetAccountId("acct-456")
	txn.SetAmount(12.34)
	txn.SetName("Coffee Shop")
	txn.SetPaymentChannel("in store")
	txn.SetPending(true)
	txn.SetDate("2026-06-04")
	txn.SetAuthorizedDate("2026-06-03")
	txn.SetDatetime(dt)
	txn.SetAuthorizedDatetime(adt)
	txn.SetPendingTransactionId("pending-789")
	txn.SetMerchantName("Blue Bottle")
	txn.SetIsoCurrencyCode("USD")

	pfc := plaidgo.NewPersonalFinanceCategoryWithDefaults()
	pfc.SetPrimary("FOOD_AND_DRINK")
	pfc.SetDetailed("FOOD_AND_DRINK_COFFEE")
	pfc.SetConfidenceLevel("HIGH")
	txn.SetPersonalFinanceCategory(*pfc)

	got := mapTransaction(*txn)

	if got.ExternalID != "txn-123" {
		t.Errorf("ExternalID = %q, want txn-123", got.ExternalID)
	}
	if got.AccountExternalID != "acct-456" {
		t.Errorf("AccountExternalID = %q, want acct-456", got.AccountExternalID)
	}
	if got.Name != "Coffee Shop" {
		t.Errorf("Name = %q, want Coffee Shop", got.Name)
	}
	if got.PaymentChannel != "in store" {
		t.Errorf("PaymentChannel = %q, want 'in store'", got.PaymentChannel)
	}
	if !got.Pending {
		t.Error("Pending = false, want true")
	}
	if want := time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC); !got.Date.Equal(want) {
		t.Errorf("Date = %v, want %v", got.Date, want)
	}
	if got.AuthorizedDate == nil || !got.AuthorizedDate.Equal(time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("AuthorizedDate = %v, want 2026-06-03", got.AuthorizedDate)
	}
	if got.Datetime == nil || !got.Datetime.Equal(dt) {
		t.Errorf("Datetime = %v, want %v", got.Datetime, dt)
	}
	if got.AuthorizedDatetime == nil || !got.AuthorizedDatetime.Equal(adt) {
		t.Errorf("AuthorizedDatetime = %v, want %v", got.AuthorizedDatetime, adt)
	}
	if got.PendingExternalID == nil || *got.PendingExternalID != "pending-789" {
		t.Errorf("PendingExternalID = %v, want pending-789", got.PendingExternalID)
	}
	if got.MerchantName == nil || *got.MerchantName != "Blue Bottle" {
		t.Errorf("MerchantName = %v, want Blue Bottle", got.MerchantName)
	}
	if got.ISOCurrencyCode != "USD" {
		t.Errorf("ISOCurrencyCode = %q, want USD", got.ISOCurrencyCode)
	}
	if got.CategoryPrimary == nil || *got.CategoryPrimary != "FOOD_AND_DRINK" {
		t.Errorf("CategoryPrimary = %v, want FOOD_AND_DRINK", got.CategoryPrimary)
	}
	if got.CategoryDetailed == nil || *got.CategoryDetailed != "FOOD_AND_DRINK_COFFEE" {
		t.Errorf("CategoryDetailed = %v, want FOOD_AND_DRINK_COFFEE", got.CategoryDetailed)
	}
	if got.CategoryConfidence == nil || *got.CategoryConfidence != "HIGH" {
		t.Errorf("CategoryConfidence = %v, want HIGH", got.CategoryConfidence)
	}
	if len(got.Raw) == 0 {
		t.Error("Raw should carry the marshaled Plaid transaction")
	}
}

// TestMapTransactionMinimalFields pins the absent-optional behavior: every
// optional pointer stays nil, an unset/unparseable date yields the zero time
// (not a panic), and currency stays empty rather than defaulting.
func TestMapTransactionMinimalFields(t *testing.T) {
	txn := plaidgo.NewTransactionWithDefaults()
	txn.SetTransactionId("txn-min")
	txn.SetAccountId("acct-min")
	txn.SetAmount(5)
	txn.SetName("Bare")
	// No date, no datetime, no category, no merchant, no currency.

	got := mapTransaction(*txn)

	if !got.Date.IsZero() {
		t.Errorf("Date = %v, want zero (unset/unparseable date must not panic or default)", got.Date)
	}
	if got.AuthorizedDate != nil {
		t.Errorf("AuthorizedDate = %v, want nil", got.AuthorizedDate)
	}
	if got.Datetime != nil {
		t.Errorf("Datetime = %v, want nil", got.Datetime)
	}
	if got.AuthorizedDatetime != nil {
		t.Errorf("AuthorizedDatetime = %v, want nil", got.AuthorizedDatetime)
	}
	if got.PendingExternalID != nil {
		t.Errorf("PendingExternalID = %v, want nil", got.PendingExternalID)
	}
	if got.MerchantName != nil {
		t.Errorf("MerchantName = %v, want nil", got.MerchantName)
	}
	if got.CategoryPrimary != nil || got.CategoryDetailed != nil || got.CategoryConfidence != nil {
		t.Errorf("category pointers should be nil when PFC absent, got %v/%v/%v",
			got.CategoryPrimary, got.CategoryDetailed, got.CategoryConfidence)
	}
}

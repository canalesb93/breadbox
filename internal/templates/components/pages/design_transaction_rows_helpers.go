//go:build !headless && !lite

package pages

import (
	"time"

	"breadbox/internal/service"
)

// designTxRowsToday returns today's date in the YYYY-MM-DD form the
// transaction helpers expect, so the sandbox sample rows render
// "Today" in relative-date columns without freezing a stale value.
func designTxRowsToday() string {
	return time.Now().Format("2006-01-02")
}

// designTxRowsDaysAgo returns an absolute date string n days before
// today. Used to seed the compact and full row variants with
// recognisable spreads ("Yesterday", "5 days ago", etc.) without
// hand-typing dates that go stale.
func designTxRowsDaysAgo(n int) string {
	return time.Now().AddDate(0, 0, -n).Format("2006-01-02")
}

// designTxRowsPtr is a tiny string-pointer helper for fixture rows —
// AdminTransactionRow uses `*string` for optional fields and inline
// `&"..."[0]` doesn't compile, so this keeps the fixtures readable.
func designTxRowsPtr(s string) *string {
	return &s
}

// designTxRowsCategorized returns a sample categorised, settled row
// with merchant + tags + a couple of comments — the densest variant
// of TxRow.
func designTxRowsCategorized() service.AdminTransactionRow {
	return service.AdminTransactionRow{
		ID:                  "tx12abcd",
		AccountID:           "acct1234",
		AccountName:         "Sapphire Reserve",
		InstitutionName:     "Chase",
		UserName:            "Alex",
		EffectiveUserID:     designTxRowsPtr("usr01abc"),
		Date:                designTxRowsToday(),
		Name:                "Whole Foods Market",
		MerchantName:        designTxRowsPtr("whole foods"),
		Amount:              48.27,
		IsoCurrencyCode:     designTxRowsPtr("USD"),
		CategoryID:          designTxRowsPtr("cat0001"),
		CategoryDisplayName: designTxRowsPtr("Groceries"),
		CategorySlug:        designTxRowsPtr("groceries"),
		CategoryIcon:        designTxRowsPtr("shopping-cart"),
		CategoryColor:       designTxRowsPtr("#22c55e"),
		Pending:             false,
		CommentCount:        2,
		CreatedAt:           designTxRowsToday(),
		UpdatedAt:           designTxRowsToday(),
		Tags: []service.AdminTransactionTag{
			{Slug: "household", DisplayName: "household", Color: designTxRowsPtr("#0ea5e9")},
			{Slug: "weekly", DisplayName: "weekly", Color: designTxRowsPtr("#a855f7")},
		},
	}
}

// designTxRowsIncome returns a sample income (negative amount) row
// without category — exercises the income-tone + uncategorised
// avatar at once.
func designTxRowsIncome() service.AdminTransactionRow {
	return service.AdminTransactionRow{
		ID:              "tx34efgh",
		AccountID:       "acct5678",
		AccountName:     "Everyday Checking",
		InstitutionName: "Chase",
		UserName:        "Sam",
		Date:            designTxRowsDaysAgo(1),
		Name:            "Acme Payroll",
		Amount:          -2845.00,
		IsoCurrencyCode: designTxRowsPtr("USD"),
		Pending:         false,
		CreatedAt:       designTxRowsDaysAgo(1),
		UpdatedAt:       designTxRowsDaysAgo(1),
	}
}

// designTxRowsPending returns a pending-status row so the clock-icon
// + dim amount treatment is on display.
func designTxRowsPending() service.AdminTransactionRow {
	return service.AdminTransactionRow{
		ID:                  "tx56ijkl",
		AccountID:           "acct1234",
		AccountName:         "Sapphire Reserve",
		InstitutionName:     "Chase",
		UserName:            "Alex",
		EffectiveUserID:     designTxRowsPtr("usr01abc"),
		Date:                designTxRowsToday(),
		Name:                "Blue Bottle Coffee",
		MerchantName:        designTxRowsPtr("blue bottle"),
		Amount:              6.75,
		IsoCurrencyCode:     designTxRowsPtr("USD"),
		CategoryID:          designTxRowsPtr("cat0002"),
		CategoryDisplayName: designTxRowsPtr("Coffee & Cafés"),
		CategorySlug:        designTxRowsPtr("coffee"),
		CategoryIcon:        designTxRowsPtr("coffee"),
		CategoryColor:       designTxRowsPtr("#d97706"),
		Pending:             true,
		CreatedAt:           designTxRowsToday(),
		UpdatedAt:           designTxRowsToday(),
	}
}

// designTxRowsUncategorized returns a settled but uncategorised
// row — the row in the queue that's waiting for a category.
func designTxRowsUncategorized() service.AdminTransactionRow {
	return service.AdminTransactionRow{
		ID:              "tx78mnop",
		AccountID:       "acct5678",
		AccountName:     "Everyday Checking",
		InstitutionName: "Chase",
		UserName:        "Sam",
		Date:            designTxRowsDaysAgo(3),
		Name:            "USPS PO 12345",
		Amount:          14.95,
		IsoCurrencyCode: designTxRowsPtr("USD"),
		Pending:         false,
		CreatedAt:       designTxRowsDaysAgo(3),
		UpdatedAt:       designTxRowsDaysAgo(3),
	}
}

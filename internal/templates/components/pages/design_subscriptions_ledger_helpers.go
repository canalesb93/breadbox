//go:build !headless && !lite

package pages

// design_subscriptions_ledger_helpers.go holds the fixture rows for the
// SectionRecurringLedger sandbox entry — the /recurring (Recurring) ledger
// migrated from a table to grouped daisy list-rows. The fixtures exercise every
// axis of the row: the three status tiles (active/paused/ended), the renewal
// attention chip (due-soon / overdue / stale), the price-rising chip, a
// no-amount fallback, and the Active group's single-currency monthly subtotal.
// Routing the fixtures through the real GroupSubscriptionsByStatus keeps the
// sandbox honest about the grouping + subtotal IA.

func designSubscriptionLedgerGroups() []SubscriptionStatusGroup {
	rows := []SubscriptionRow{
		{
			ShortID: "sub_netflix", Name: "Netflix", Status: "active",
			Type: "subscription", TypeLabel: "Subscription",
			CadenceLabel: "Monthly", NextExpected: "Jun 28",
			HasAmount: true, Amount: 15.99, Currency: "USD", MonthlyEquiv: 15.99,
			RenewalLabel: "Renews in 3d", RenewalTone: "info",
			Search: "netflix",
		},
		{
			ShortID: "sub_icloud", Name: "iCloud+ Storage", Status: "active",
			Type: "subscription", TypeLabel: "Subscription",
			CadenceLabel: "Monthly", NextExpected: "Jun 14",
			HasAmount: true, Amount: 2.99, Currency: "USD", MonthlyEquiv: 2.99,
			RenewalLabel: "Due tomorrow", RenewalTone: "info",
			Search: "icloud storage",
		},
		{
			ShortID: "sub_internet", Name: "Comcast Internet", Status: "active",
			Type: "bill", TypeLabel: "Bill",
			CadenceLabel: "Monthly", NextExpected: "Jun 9",
			HasAmount: true, Amount: 79.00, Currency: "USD", MonthlyEquiv: 79.00,
			RenewalLabel: "4d overdue", RenewalTone: "warning",
			Search: "comcast internet",
		},
		{
			ShortID: "sub_nyt", Name: "NYT Digital", Status: "active",
			Type: "subscription", TypeLabel: "Subscription",
			CadenceLabel: "Annual", NextExpected: "Mar 2",
			HasAmount: true, Amount: 120.00, Currency: "USD", MonthlyEquiv: 10.00,
			PriceChanged: true,
			Search:       "nyt digital",
		},
		{
			ShortID: "sub_gym", Name: "Equinox", Status: "paused",
			Type: "bill", TypeLabel: "Bill",
			CadenceLabel: "Monthly",
			HasAmount:    true, Amount: 240.00, Currency: "USD", MonthlyEquiv: 240.00,
			Search: "equinox gym",
		},
		{
			ShortID: "sub_hulu", Name: "Hulu", Status: "cancelled",
			Type: "subscription", TypeLabel: "Subscription",
			CadenceLabel: "Monthly",
			HasAmount:    true, Amount: 17.99, Currency: "USD", MonthlyEquiv: 17.99,
			RenewalLabel: "Likely cancelled", RenewalTone: "error",
			Search: "hulu",
		},
		{
			ShortID: "sub_water", Name: "City Water", Status: "cancelled",
			Type: "bill", TypeLabel: "Bill",
			CadenceLabel: "Quarterly",
			HasAmount:    false,
			Search:       "city water",
		},
	}
	return GroupSubscriptionsByStatus(rows)
}

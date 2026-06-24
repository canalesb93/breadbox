//go:build !headless && !lite

package pages

// design_subscriptions_ledger_helpers.go holds the fixture rows for the
// SectionRecurringLedger sandbox entry — the /recurring (Recurring) ledger under
// the rules-as-substrate model. A thin series carries only a name, a type, and a
// linked-charge count, so the fixtures exercise the three type glyphs and a
// range of member counts (including an empty series).

func designSubscriptionLedgerRows() []SubscriptionRow {
	return []SubscriptionRow{
		{ShortID: "sub_netflix", Name: "Netflix", Type: "subscription", TypeLabel: "Subscription", MemberCount: 11, Search: "netflix subscription"},
		{ShortID: "sub_icloud", Name: "iCloud+ Storage", Type: "subscription", TypeLabel: "Subscription", MemberCount: 6, Search: "icloud storage subscription"},
		{ShortID: "sub_internet", Name: "Comcast Internet", Type: "bill", TypeLabel: "Bill", MemberCount: 8, Search: "comcast internet bill"},
		{ShortID: "sub_mortgage", Name: "Home Mortgage", Type: "loan", TypeLabel: "Loan", MemberCount: 24, Search: "home mortgage loan"},
		{ShortID: "sub_new", Name: "City Water", Type: "bill", TypeLabel: "Bill", MemberCount: 0, Search: "city water bill"},
	}
}

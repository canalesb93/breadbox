//go:build !headless && !lite

package pages

import "fmt"

// design_accounts_redesign_helpers.go holds the sample data + small class
// helpers for SectionAccountsRedesign — the Workflows-DNA-applied prototype
// of the /accounts page. Demo only; figures are illustrative (rendered
// Public so privacy mode leaves them visible in the sandbox).

// designAcctRow is one sample account in the redesign prototype.
type designAcctRow struct {
	Icon        string  // lucide type icon (wallet / credit-card / trending-up / landmark)
	Name        string  // display name
	Institution string  // bank
	Mask        string  // last-4, "" when none
	TypeLabel   string  // human type ("Checking", "Credit card", …)
	OwnerName   string  // "" when no linked member
	OwnerID     string  // avatar seed
	Balance     float64 // sign reflects asset(+)/liability(−)
	HasBalance  bool
	IsLiability bool
	Status      string // "" healthy | "pending_reauth" | "disconnected"
	Badge       string // "" | "Linked" | "Excluded"
}

// designAcctRows is the fixed sample set: a mix of types, owners, balances,
// and connection-health states so every row variant is visible.
func designAcctRows() []designAcctRow {
	return []designAcctRow{
		{Icon: "wallet", Name: "Everyday Checking", Institution: "Chase", Mask: "1234", TypeLabel: "Checking", OwnerName: "Alice", OwnerID: "alice", Balance: 12340.18, HasBalance: true},
		{Icon: "credit-card", Name: "Sapphire Card", Institution: "Chase", Mask: "8021", TypeLabel: "Credit card", OwnerName: "Bob", OwnerID: "bob", Balance: -1204.55, HasBalance: true, IsLiability: true, Status: "pending_reauth"},
		{Icon: "trending-up", Name: "Brokerage", Institution: "Fidelity", TypeLabel: "Investment", Balance: 34120.00, HasBalance: true},
		{Icon: "landmark", Name: "Auto Loan", Institution: "Ally", Mask: "5567", TypeLabel: "Loan", OwnerName: "Alice", OwnerID: "alice", Balance: -3362.00, HasBalance: true, IsLiability: true},
		{Icon: "wallet", Name: "High-Yield Savings", Institution: "Ally", Mask: "9930", TypeLabel: "Savings", OwnerName: "Alice", OwnerID: "alice", Balance: 6116.92, HasBalance: true, Badge: "Linked"},
		{Icon: "wallet", Name: "Old Savings", Institution: "Ally", Mask: "2231", TypeLabel: "Savings", HasBalance: false, Status: "disconnected"},
	}
}

// designAcctMeta is the healthy-state body line: "Institution · ••••mask · Type".
func designAcctMeta(d designAcctRow) string {
	s := d.Institution
	if d.Mask != "" {
		s += " · ••••" + d.Mask
	}
	if d.TypeLabel != "" {
		s += " · " + d.TypeLabel
	}
	return s
}

// designAcctDotClass maps connection health to the leading-tile corner dot.
func designAcctDotClass(status string) string {
	switch status {
	case "pending_reauth":
		return "bg-warning"
	case "disconnected", "error":
		return "bg-error"
	default:
		return "bg-base-300"
	}
}

// designAcctBadgeTone maps an inline row badge to its daisy soft tone.
func designAcctBadgeTone(badge string) string {
	switch badge {
	case "Excluded":
		return "badge-warning"
	default: // Linked
		return "badge-info"
	}
}

// designAcctAmountClass tints liabilities red; assets keep the default ink.
func designAcctAmountClass(isLiability bool) string {
	if isLiability {
		return "text-sm font-semibold text-error"
	}
	return "text-sm font-semibold"
}

// designAcctOwnerVersion is a stable cache-bust stub for sample avatars.
func designAcctOwnerVersion(d designAcctRow) string {
	return fmt.Sprintf("demo-%s", d.OwnerID)
}

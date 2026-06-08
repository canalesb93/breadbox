//go:build !headless && !lite

package pages

import (
	"fmt"
	"strings"
)

// design_accounts_redesign_helpers.go holds the sample data + small class
// helpers for SectionAccountsRedesign — the Workflows-DNA-applied prototype
// of the /accounts page. Demo only; figures are illustrative (rendered
// Public so privacy mode leaves them visible in the sandbox).

// designAcctRow is one sample account within a connection group.
type designAcctRow struct {
	Icon        string  // lucide type icon (wallet / credit-card / trending-up / landmark)
	Name        string  // display name
	Mask        string  // last-4, "" when none
	TypeLabel   string  // human type ("Checking", "Credit card", …)
	OwnerName   string  // "" when no linked member
	OwnerID     string  // avatar seed
	Balance     float64 // sign reflects asset(+)/liability(−)
	HasBalance  bool
	IsLiability bool
	Badge       string // "" | "Linked" | "Excluded"
}

// designAcctConn is one connection (institution) — the grouping unit.
// Connection health lives here (one status per connection), so individual
// rows don't repeat it.
type designAcctConn struct {
	Institution string
	Status      string // "active" | "pending_reauth" | "disconnected"
	Accounts    []designAcctRow
}

// designAcctConns is the fixed sample set, grouped by connection: a mix of
// sizes, owners, balances, and connection-health states.
func designAcctConns() []designAcctConn {
	return []designAcctConn{
		{Institution: "Chase", Status: "active", Accounts: []designAcctRow{
			{Icon: "wallet", Name: "Everyday Checking", Mask: "1234", TypeLabel: "Checking", OwnerName: "Alice", OwnerID: "alice", Balance: 12340.18, HasBalance: true},
			{Icon: "credit-card", Name: "Sapphire Card", Mask: "8021", TypeLabel: "Credit card", OwnerName: "Bob", OwnerID: "bob", Balance: -1204.55, HasBalance: true, IsLiability: true},
		}},
		{Institution: "American Express", Status: "pending_reauth", Accounts: []designAcctRow{
			{Icon: "credit-card", Name: "Platinum Card", Mask: "3007", TypeLabel: "Credit card", OwnerName: "Alice", OwnerID: "alice", Balance: -842.30, HasBalance: true, IsLiability: true},
		}},
		{Institution: "Fidelity", Status: "active", Accounts: []designAcctRow{
			{Icon: "trending-up", Name: "Brokerage", TypeLabel: "Investment", Balance: 34120.00, HasBalance: true},
		}},
		{Institution: "Ally", Status: "active", Accounts: []designAcctRow{
			{Icon: "wallet", Name: "High-Yield Savings", Mask: "9930", TypeLabel: "Savings", OwnerName: "Alice", OwnerID: "alice", Balance: 6116.92, HasBalance: true, Badge: "Linked"},
			{Icon: "landmark", Name: "Auto Loan", Mask: "5567", TypeLabel: "Loan", OwnerName: "Alice", OwnerID: "alice", Balance: -3362.00, HasBalance: true, IsLiability: true},
		}},
		{Institution: "Old National", Status: "disconnected", Accounts: []designAcctRow{
			{Icon: "wallet", Name: "Legacy Savings", Mask: "2231", TypeLabel: "Savings", HasBalance: false},
		}},
	}
}

// designAcctMeta is the per-row body line — "••••mask · Type". Institution
// is omitted: the connection group header already names it.
func designAcctMeta(d designAcctRow) string {
	parts := make([]string, 0, 2)
	if d.Mask != "" {
		parts = append(parts, "••••"+d.Mask)
	}
	if d.TypeLabel != "" {
		parts = append(parts, d.TypeLabel)
	}
	return strings.Join(parts, " · ")
}

// designAcctConnSubtotal sums the balances of a connection's accounts.
func designAcctConnSubtotal(c designAcctConn) float64 {
	var t float64
	for _, a := range c.Accounts {
		if a.HasBalance {
			t += a.Balance
		}
	}
	return t
}

// designAcctConnCount is the dimmed "N accounts" label under the institution.
func designAcctConnCount(c designAcctConn) string {
	if len(c.Accounts) == 1 {
		return "1 account"
	}
	return fmt.Sprintf("%d accounts", len(c.Accounts))
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
	return "demo-" + d.OwnerID
}

//go:build !headless && !lite

package pages

import (
	"fmt"
	"sort"
)

// AccountsListProps is the typed input for the /accounts admin page.
type AccountsListProps struct {
	CSRFToken string

	// Net totals across all displayed accounts (computed per-viewer, and
	// re-scoped server-side when a member filter is active).
	NetWorth         float64
	TotalAssets      float64
	TotalLiabilities float64
	HasAnyBalance    bool

	// Household-member filter. Options are only populated for editors;
	// ActiveUserID is the currently-selected member ("" = all).
	Users        []AccountsListUserFilter
	ActiveUserID string

	// Accounts grouped by connection (institution). TotalAccounts is the
	// flat count, used for the empty-state gate.
	Groups        []AccountsListConnGroup
	TotalAccounts int
}

// AccountsListUserFilter is one option in the "filter by household member"
// dropdown.
type AccountsListUserFilter struct {
	ID            string // short_id — the ?user= filter value
	Name          string
	First         string // first letter for the avatar circle fallback
	AvatarVersion string // user updated_at unix timestamp for cache-busting
}

// AccountsListConnGroup is one connection's accounts plus the header data:
// the institution name, the shared connection health, and a balance
// subtotal. Connection health lives here (one status per connection) so the
// individual rows don't repeat it.
type AccountsListConnGroup struct {
	ConnectionShortID string
	InstitutionName   string
	Status            string // "active" | "error" | "pending_reauth" | "disconnected" | ""
	Subtotal          float64
	HasSubtotal       bool
	Accounts          []AccountsListRow
}

// AccountsListRow is one account row. Fields are pre-formatted (display_name
// applied, balance sign adjusted for liabilities, currency carried alongside).
type AccountsListRow struct {
	ID           string // formatted UUID
	UserID       string // short_id — owner; drives the member filter + avatar URL
	DisplayName  string // display_name with fallback to name
	Type         string // canonical account type ("depository", "credit", ...)
	SubtypeValid bool
	Subtype      string
	MaskValid    bool
	Mask         string

	OwnerName          string // empty when no linked household member
	OwnerFirst         string // first letter for the avatar dot fallback
	OwnerAvatarVersion string // user updated_at unix timestamp for cache-busting

	InstitutionName string

	// Connection context — used to group rows and (on the group header)
	// surface reauth/disconnected state.
	ConnectionShortID string
	ConnectionStatus  string

	IsDependentLinked bool
	Excluded          bool

	HasBalance      bool
	BalanceFloat    float64 // sign-adjusted (negative for liabilities)
	IsoCurrencyCode string

	IsLiability bool
}

// GroupAccountsByConnection buckets pre-sorted account rows by connection,
// preserving row order within each group. Groups are ordered by subtotal
// (largest first); groups without any balance sink below those with one, and
// the orphan bucket (accounts whose connection was deleted — SET NULL)
// sinks last. Pure function so the grouping is unit-testable without a DB.
func GroupAccountsByConnection(rows []AccountsListRow) []AccountsListConnGroup {
	order := make([]string, 0)
	byKey := make(map[string]*AccountsListConnGroup)
	for _, r := range rows {
		key := r.ConnectionShortID
		g, ok := byKey[key]
		if !ok {
			g = &AccountsListConnGroup{
				ConnectionShortID: key,
				InstitutionName:   r.InstitutionName,
				Status:            r.ConnectionStatus,
			}
			byKey[key] = g
			order = append(order, key)
		}
		if g.InstitutionName == "" && r.InstitutionName != "" {
			g.InstitutionName = r.InstitutionName
		}
		if g.Status == "" && r.ConnectionStatus != "" {
			g.Status = r.ConnectionStatus
		}
		if r.HasBalance {
			g.Subtotal += r.BalanceFloat
			g.HasSubtotal = true
		}
		g.Accounts = append(g.Accounts, r)
	}
	groups := make([]AccountsListConnGroup, 0, len(order))
	for _, k := range order {
		groups = append(groups, *byKey[k])
	}
	sort.SliceStable(groups, func(i, j int) bool {
		a, b := groups[i], groups[j]
		// Orphan accounts (no connection) always sink last.
		ao, bo := a.ConnectionShortID == "", b.ConnectionShortID == ""
		if ao != bo {
			return !ao
		}
		// Groups carrying a balance rank above balance-less ones.
		if a.HasSubtotal != b.HasSubtotal {
			return a.HasSubtotal
		}
		return a.Subtotal > b.Subtotal
	})
	return groups
}

// accountsConnLabel is the group header's institution label, with a fallback
// for orphan accounts whose connection was removed.
func accountsConnLabel(g AccountsListConnGroup) string {
	if g.InstitutionName != "" {
		return g.InstitutionName
	}
	return "Unlinked accounts"
}

// accountsConnCountLabel renders the dimmed "N accounts" suffix.
func accountsConnCountLabel(g AccountsListConnGroup) string {
	if len(g.Accounts) == 1 {
		return "1 account"
	}
	return fmt.Sprintf("%d accounts", len(g.Accounts))
}

//go:build !headless && !lite

package pages

// AccountsListProps is the typed input for the /accounts admin page.
type AccountsListProps struct {
	CSRFToken string

	// Net totals across all displayed accounts (computed per-viewer).
	// HasAnyBalance is the gate the templ uses to render the summary row.
	NetWorth         float64
	TotalAssets      float64
	TotalLiabilities float64
	HasAnyBalance    bool

	// Household-member filter strip. Only rendered when len > 1.
	Users []AccountsListUserFilter

	// One row per account.
	Accounts []AccountsListRow
}

// AccountsListUserFilter mirrors ConnectionsUserFilter — one chip in the
// "filter by household member" strip.
type AccountsListUserFilter struct {
	ID    string // formatted UUID — drives the x-show filter value
	Name  string
	First string // first letter for the avatar circle
}

// AccountsListRow is one table row. Fields are pre-formatted (display_name
// applied, balance sign adjusted for liabilities, currency carried alongside).
type AccountsListRow struct {
	ID          string // formatted UUID
	UserID      string // formatted UUID — used by the x-show filter
	DisplayName string // display_name with fallback to name
	Type        string // canonical account type ("depository", "credit", ...)
	SubtypeValid bool
	Subtype     string
	MaskValid   bool
	Mask        string
	OwnerName   string // empty when no linked household member
	OwnerFirst  string // first letter for the avatar dot
	InstitutionName string

	// Connection context — lets the row pill out reauth/disconnected state
	// without fetching connection rows again.
	ConnectionShortID string
	ConnectionStatus  string // "active" | "error" | "pending_reauth" | "disconnected" | ""

	IsDependentLinked bool
	Excluded          bool

	HasBalance   bool
	BalanceFloat float64 // sign-adjusted (negative for liabilities)
	IsoCurrencyCode string

	IsLiability bool
}

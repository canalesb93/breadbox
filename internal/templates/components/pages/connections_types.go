package pages

// ConnectionsProps mirrors the data map the old connections.html read off
// the layout. Kept flat so admin/connections.go can copy fields one-to-one.
type ConnectionsProps struct {
	Tab       string // "connections" or "links"
	CSRFToken string

	// Financial summary (top of connections tab)
	NetWorth         float64
	TotalAssets      float64
	TotalLiabilities float64
	HasAnyBalance    bool

	// Family member filter buttons (only rendered when len(Users) > 1)
	Users []ConnectionsUserFilter

	// Connection cards
	Connections []ConnectionsRow

	// Account links tab
	Links        []ConnectionsLinkRow
	LinkAccounts []ConnectionsLinkAccount
}

// ConnectionsUserFilter is one button in the family-member filter strip.
type ConnectionsUserFilter struct {
	ID    string // formatted UUID — used both as filter value and href
	Name  string
	First string // first letter for the avatar circle
}

// ConnectionsRow is one bank-connection card on the page.
type ConnectionsRow struct {
	ID                   string // formatted UUID
	UserID               string // formatted UUID — used by the filter x-show
	Provider             string // "plaid" | "teller" | "csv"
	Status               string // canonical connection status enum
	InstitutionName      string
	UserName             string
	Paused               bool
	IsStale              bool
	NewAccountsAvailable bool

	// Last-sync state (for the header pill)
	LastSyncStatus       string // "success" | "error" | "in_progress" | "" (none)
	LastSyncErrorMessage string // empty when no message
	LastSyncedAtValid    bool
	LastSyncedAtRelative string

	// Connection-level error (e.g. reauth)
	ErrorCodeValid    bool
	ErrorCode         string
	ErrorMessageValid bool

	// Per-connection totals
	HasBalance     bool
	TotalBalance   float64
	AccountCount   int64

	// Accounts list under the header
	Accounts []ConnectionsAccountRow
}

// ConnectionsAccountRow is one account inside a connection card.
type ConnectionsAccountRow struct {
	ID                string // formatted UUID
	Name              string
	DisplayNameValid  bool
	DisplayName       string
	Type              string // "credit" | "loan" | "investment" | depository/etc.
	SubtypeValid      bool
	Subtype           string
	MaskValid         bool
	Mask              string
	IsDependentLinked bool
	Excluded          bool
	HasBalance        bool
	BalanceFloat      float64
}

// ConnectionsLinkRow is one item on the Account Links tab.
type ConnectionsLinkRow struct {
	ID                       string
	PrimaryAccountName       string
	PrimaryUserName          string
	DependentAccountName     string
	DependentUserName        string
	Enabled                  bool
	MatchCount               int64
	UnmatchedDependentCount  int64
	MatchStrategy            string
	MatchToleranceDays       int
}

// ConnectionsLinkAccount is one option in the create-link modal selects.
type ConnectionsLinkAccount struct {
	ID              string
	DisplayName     string
	Mask            string
	UserName        string
	InstitutionName string
}

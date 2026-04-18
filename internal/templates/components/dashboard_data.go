// Package components holds templ-rendered UI components for the admin dashboard.
// The dashboard page is the first consumer (GitHub issue #462 migration from
// html/template to github.com/a-h/templ).
package components

// DashboardData is the full input shape for DashboardPage. The admin handler
// builds this and calls components.DashboardPage(data).Render(ctx, w).
type DashboardData struct {
	PageTitle    string
	CurrentPage  string
	CSRFToken    string

	// Update banner state.
	ShowUpdateBanner      bool
	LatestVersion         string
	LatestURL             string
	CurrentVersion        string
	DockerSocketAvailable bool

	// Top-of-page attention banner (connections needing reauth/error).
	NeedsAttention int64

	// Net worth hero + accounts grid.
	Accounts         []DashboardAccount
	AllocationSlices []AllocationSlice
	TotalAssets      float64
	TotalLiabilities float64
	NetWorth         float64

	// Two-col middle section.
	AgentReports       []DashboardReport
	MoreReportsCount   int
	TotalUnreadReports int
	RecentTransactions []RecentTxRow

	// Two-col bottom section.
	AttentionCount     int
	HasAttentionItems  bool
	UncategorizedCount int64
	ReviewsEnabled     bool
	ReviewPending      int64
	ErrorCount         int
	ConnectionHealth   []ConnectionHealthRow
	SyncHealth         *SyncHealthView
}

// DashboardAccount is one row in the collapsible accounts grid. SparklineData
// is a JSON string of 30 daily amounts; empty means "no sparkline".
type DashboardAccount struct {
	ID               string
	Name             string
	InstitutionName  string
	Type             string
	Subtype          string
	Mask             string
	BalanceCurrent   float64
	IsoCurrencyCode  string
	IsLiability      bool
	SparklineData    string // JSON array (e.g. "[1.2, 0, 3.4]")
	SpendingTotal    float64
	ConnectionStatus string
}

// AllocationSlice is a segment of the allocation bar under the net worth hero.
type AllocationSlice struct {
	Label   string
	Amount  float64
	Percent float64
	Color   string // OKLCH color string
}

// DashboardReport is a compact agent report for the dashboard widget.
type DashboardReport struct {
	ID            string
	Title         string
	Body          string
	CreatedByName string
	Priority      string
	Tags          []string
	DisplayAuthor string
	CreatedAt     string // already formatted as relative time
}

// ConnectionHealthRow mirrors one connection on the dashboard health panel.
type ConnectionHealthRow struct {
	ID           string
	Name         string
	Provider     string
	Status       string
	ErrorMessage string
	LastSyncedAt string
	AccountCount int64
	Paused       bool
	IsStale      bool
}

// SyncHealthView surfaces just the dashboard-relevant subset of
// service.SyncHealthSummary so the template package doesn't depend on service.
type SyncHealthView struct {
	LastSyncTime      *string
	RecentSyncCount   int64
	RecentSuccessRate float64
	NextSyncTime      string
}

// RecentTxRow is a compact transaction row used in the dashboard "Recent
// Transactions" list. Mirrors the fields consumed by tx_row_compact.html.
type RecentTxRow struct {
	ID                  string
	Name                string
	AccountName         string
	UserName            string
	EffectiveUserID     *string
	Date                string
	Amount              float64
	Pending             bool
	AgentReviewed       bool
	CategoryIcon        *string
	CategoryColor       *string
	CategoryDisplayName *string
}

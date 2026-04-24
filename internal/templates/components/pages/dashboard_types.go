package pages

import "breadbox/internal/service"

// DashboardAccount mirrors the account row the dashboard renders for
// each connected bank account. SparklineData carries the 30-day daily
// spending series as a JSON array string (`[1.23, 4.56, …]`) so the
// existing `.bb-sparkline` script can parse it verbatim.
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
	SparklineData    string
	SpendingTotal    float64
	ConnectionStatus string
}

// AllocationSlice is one segment of the net-worth allocation bar.
type AllocationSlice struct {
	Label   string
	Amount  float64
	Percent float64
	Color   string // OKLCH string
}

// ConnectionHealthRow is one row in the Connections health panel.
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

// DashboardReport is one card in the Agent Reports panel.
type DashboardReport struct {
	ID            string
	DisplayAuthor string
	CreatedAt     string
	Title         string
	Priority      string // "critical", "warning", or normal
}

// DashboardProps is the full view model the dashboard page renders.
type DashboardProps struct {
	CSRFToken             string
	ShowUpdateBanner      bool
	LatestVersion         string
	LatestURL             string
	CurrentVersion        string
	DockerSocketAvailable bool
	NeedsAttention        int64
	Accounts              []DashboardAccount
	AllocationSlices      []AllocationSlice
	NetWorth              float64
	TotalAssets           float64
	TotalLiabilities      float64
	AgentReports          []DashboardReport
	TotalUnreadReports    int
	MoreReportsCount      int
	RecentTransactions    []service.AdminTransactionRow
	TotalTransactions     int64
	HasAttentionItems     bool
	AttentionCount        int
	UncategorizedCount    int64
	ReviewsEnabled        bool
	ReviewPending         int64
	ErrorCount            int
	ConnectionHealth      []ConnectionHealthRow
	SyncHealth            *service.SyncHealthSummary
}

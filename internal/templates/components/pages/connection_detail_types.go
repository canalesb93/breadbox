package pages

import "breadbox/internal/templates/components"

// ConnectionDetailProps mirrors the data map the old connection_detail.html
// read off the layout. Kept flat so admin/connections.go can copy fields
// one-to-one.
type ConnectionDetailProps struct {
	ConnID      string
	CSRFToken   string
	Breadcrumbs []components.Breadcrumb

	// Connection fields (flattened from db.GetBankConnectionRow)
	Provider                          string
	Status                            string
	InstitutionName                   string
	UserName                          string
	UserNameValid                     bool
	Paused                            bool
	ConsecutiveFailures               int32
	HasErrorCode                      bool
	ErrorCode                         string
	HasErrorMessage                   bool
	ErrorMessage                      string
	LastSyncedAtValid                 bool
	LastSyncedAtRelative              string
	CreatedAtValid                    bool
	CreatedAtFormatted                string
	LastErrorAtValid                  bool
	LastErrorAtRelative               string
	SyncIntervalOverrideMinutesValid  bool
	SyncIntervalOverrideMinutesValue  int32

	// Latest sync log status (for header badge — matches list page).
	LastSyncStatus              string
	LastSyncErrorMessageValid   bool
	LastSyncErrorMessageString  string

	// Sync health stats
	TotalSyncs           int
	SuccessSyncs         int
	ErrorSyncs           int
	SuccessRate          float64
	TotalAdded           int
	TotalModified        int
	TotalRemoved         int
	AvgDurationSec       float64
	LastSuccessTime      string
	LastSuccessRelative  string
	DaySyncs             []DaySyncRow

	// Account totals
	TotalBalance float64
	HasBalance   bool

	// Next sync schedule
	NextSync NextSyncInfo

	// Accounts
	Accounts []AccountRow

	// Sync history (last 10)
	SyncLogs []SyncLogRow
}

// DaySyncRow is one bar in the 14-day sync timeline.
type DaySyncRow struct {
	Date       string
	Label      string
	ShortLabel string
	Success    int
	Error      int
	Total      int
}

// NextSyncInfo carries computed next-sync schedule information for a
// connection. Mirrors admin.NextSyncInfo without the time.Time field
// (the templ side only renders the precomputed Label).
type NextSyncInfo struct {
	Label                    string
	IsOverdue                bool
	IsPaused                 bool
	IsDisconnected           bool
	EffectiveIntervalMinutes int
}

// AccountRow is one account card in the Accounts grid.
type AccountRow struct {
	ID                    string
	Name                  string
	Type                  string
	SubtypeValid          bool
	SubtypeString         string
	MaskValid             bool
	MaskString            string
	BalanceCurrentValid   bool
	BalanceCurrentText    string // pre-formatted via service.FormatCurrency(abs)
	BalanceAvailableValid bool
	BalanceAvailableText  string
	DisplayName           string
	Excluded              bool
}

// SyncLogRow is one row in the Sync History list.
type SyncLogRow struct {
	ShortID         string
	Status          string
	Trigger         string
	StartedAtValid  bool
	StartedAtRelative string
	DurationLabel   string // pre-formatted; empty when unavailable
	HasDuration     bool
	ErrorMessageValid  bool
	ErrorMessageString string
	ErrorMessageFriendly string // syncFriendlyError result; empty if no friendly fallback
	AddedCount      int32
	ModifiedCount   int32
	RemovedCount    int32
	UnchangedCount  int32
}

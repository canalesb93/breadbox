package pages

import "breadbox/internal/service"

// LogsProps mirrors the field set the old logs.html read off the layout
// data map. Kept flat so admin/logs.go can build the props once and let
// the templ render directly.
type LogsProps struct {
	// Active tab: "syncs" or "webhooks".
	Tab string

	// ===== Sync logs tab =====
	Stats              *service.SyncLogStats
	HasAdvancedFilters bool
	Connections        []LogsConnectionOption
	FilterConnID       string
	FilterStatus       string
	FilterTrigger      string
	FilterDateFrom     string
	FilterDateTo       string

	Logs           []LogsSyncRow
	Total          int64
	Page           int
	TotalPages     int
	ShowingStart   int
	ShowingEnd     int
	PaginationBase string
	SuccessCount   int64
	ErrorCount     int64
	InProgressCount int64

	// Status filter query strings for the pill tabs (encoded url.Values
	// strings without a leading ?). Pre-built so the template only emits
	// the strings.
	StatusQueryAll        string
	StatusQuerySuccess    string
	StatusQueryError      string
	StatusQueryInProgress string

	// ===== Webhooks tab =====
	WHStats          *service.WebhookEventStats
	WHEvents         []LogsWebhookRow
	WHTotal          int64
	WHPage           int
	WHTotalPages     int
	WHShowingStart   int
	WHShowingEnd     int
	WHPaginationBase string
	WHFilterProvider string
	WHFilterStatus   string
}

// LogsConnectionOption represents one <option> in the connection filter
// select. The handler builds these from db.ListBankConnectionsRow so the
// templ doesn't have to know about pgtype.
type LogsConnectionOption struct {
	ID       string // formatted UUID
	Name     string
	Selected bool
}

// LogsSyncRow is a view-model wrapper around service.SyncLogRow that
// pre-renders relative times so the templ does not have to call the
// admin funcMap helper. Mirrors the field set the old html/template
// read off SyncLogRow plus a derived StartedAtRelative.
type LogsSyncRow struct {
	ID                   string
	InstitutionName      string
	Trigger              string
	Status               string
	StartedAtRelative    string // "2 minutes ago" — pre-rendered
	Duration             *string
	AccountsAffected     int64
	FriendlyErrorMessage *string
	ErrorMessage         *string
	WarningMessage       *string
	AddedCount           int32
	ModifiedCount        int32
	RemovedCount         int32
	UnchangedCount       int32
}

// LogsWebhookRow mirrors service.WebhookEventRow with pre-rendered
// relative time + formatted "Received at" timestamp so the templ stays
// free of time-formatting helpers.
type LogsWebhookRow struct {
	ID                string
	Provider          string
	EventType         string
	Status            string
	ConnectionID      *string
	InstitutionName   *string
	PayloadHash       string
	ErrorMessage      *string
	CreatedAtRelative string // "12 minutes ago"
	CreatedAtFull     string // "Jan 2, 2006 3:04 PM"
}

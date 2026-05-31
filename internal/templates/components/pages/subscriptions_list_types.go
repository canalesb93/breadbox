//go:build !headless && !lite

package pages

// SubscriptionsListProps is the typed input for the /subscriptions admin page.
// Series are pre-split into Candidates (awaiting human adjudication) and Active
// (confirmed / live), and the stat tiles are pre-computed in the handler.
type SubscriptionsListProps struct {
	CSRFToken string

	// Stat tiles.
	ActiveCount    int
	CandidateCount int
	// Monthly-equivalent spend, one entry per currency (never summed across).
	MonthlyTotals []SubscriptionMonthlyTotal

	// Household-member filter strip. Only rendered when len > 1.
	Users []SubscriptionUserFilter

	// status == 'candidate' — get the Confirm / Not-a-subscription actions.
	Candidates []SubscriptionRow
	// status in (active, paused, cancelled) — the confirmed/live ledger.
	Active []SubscriptionRow
}

// SubscriptionMonthlyTotal is one currency's monthly-equivalent spend tile.
type SubscriptionMonthlyTotal struct {
	Currency string
	Amount   float64
}

// SubscriptionUserFilter — one chip in the "filter by household member" strip.
type SubscriptionUserFilter struct {
	ID    string // formatted UUID — drives the x-show filter value
	Name  string
	First string // first letter for the avatar circle
}

// SubscriptionRow is one series card. Fields are pre-formatted in the handler.
type SubscriptionRow struct {
	ShortID     string // short_id — drives detail link + verdict PATCH
	Name        string
	MerchantKey string

	Cadence      string // canonical: monthly, annual, ...
	CadenceLabel string // "Monthly", "Annual", "Every 2 weeks"

	Status      string // active | paused | cancelled | candidate
	StatusLabel string
	StatusTone  string // success | warning | neutral | info

	// Renewal-health attention chip (active series only). Empty when the
	// subscription renews comfortably or has no projection — only the states a
	// user should act on (due soon / overdue / likely cancelled) get a chip.
	RenewalLabel string // "Renews in 3d" | "5d overdue" | "Likely cancelled"
	RenewalTone  string // info | warning | error

	HasAmount bool
	Amount    float64 // last_amount in dollars
	Currency  string

	NextExpected string // formatted "May 30" or "" when none
	LastSeen     string // formatted "May 1" or ""

	OccurrenceCount int

	// Confidence (candidates only) — derived coarse band from detection_signals.
	ConfidenceBand string // "High" | "Medium" | "Low"
	BandTone       string // success | warning | neutral
	SignalSummary  string // "11 charges · monthly · ±0% spread"
	SignalsJSON    string // pretty-printed detection_signals for the disclosure

	Source       string // deterministic | agent | user | rule
	SourceLabel  string
	CategoryName string
	Tags         []string // tag slugs attached to the series (inherited by members)

	// Monthly-equivalent contribution (active rows) — used nowhere in the row
	// itself but kept for parity / future per-row display.
	MonthlyEquiv float64

	// Filter support.
	UserID    string // formatted UUID, "" for shared/household
	OwnerName string
	Search    string // lowercase haystack for the filter input
}

// SubscriptionDetailProps is the typed input for /subscriptions/{short_id}.
type SubscriptionDetailProps struct {
	CSRFToken string

	Series SubscriptionRow // reuses the row shape for header chrome

	// Config grid values (pre-formatted).
	ExpectedAmount  string
	AmountTolerance string
	ExpectedDay     string
	NextExpected    string
	LastSeen        string
	Confidence      string // auto | confirmed | rejected
	CreatedAt       string

	// Linked charges, newest first.
	Members []SubscriptionMember

	// Price-change history derived from members (oldest → newest change points).
	PriceChanges []SubscriptionPriceChange
}

// SubscriptionMember is one linked charge in the detail timeline.
type SubscriptionMember struct {
	ShortID   string
	Date      string // "May 1, 2026"
	Name      string
	HasAmount bool
	Amount    float64
	Currency  string
}

// SubscriptionPriceChange marks a point where the charge amount changed.
type SubscriptionPriceChange struct {
	Date     string
	From     float64
	To       float64
	Currency string
}

//go:build !headless && !lite

package pages

import (
	"time"

	"breadbox/internal/service"
	"breadbox/internal/templates/components"
)

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
	// Upcoming spend in the next 30 days — active series whose next charge lands
	// within [0,30] days, summed per currency. UpcomingCount is how many.
	UpcomingTotals []SubscriptionMonthlyTotal
	UpcomingCount  int

	// Household-member filter strip. Only rendered when len > 1.
	Users []SubscriptionUserFilter
	// Type filter strip (subscription/bill/loan/other present in the data).
	// Only rendered when len > 1 — no point offering a filter for one type.
	Types []SubscriptionTypeFilter

	// ActiveTab selects which tab renders: "active" (default) or "review".
	ActiveTab string

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

// SubscriptionTypeFilter — one option in the "filter by type" segmented control.
type SubscriptionTypeFilter struct {
	Value string // subscription | bill | loan | other — matches row data-type
	Label string // "Subscriptions" | "Bills" | "Loans" | "Other"
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

	Type      string // subscription | bill | loan | other (raw, drives the filter)
	TypeLabel string // "Subscription" | "Bill" | "Loan" | "Other"

	// Renewal-health attention chip (active series only). Empty when the
	// subscription renews comfortably or has no projection — only the states a
	// user should act on (due soon / overdue / likely cancelled) get a chip.
	RenewalLabel string // "Renews in 3d" | "5d overdue" | "Likely cancelled"
	RenewalTone  string // info | warning | error
	// DaysUntilRenewal is the signed day count to the next charge (negative =
	// overdue), nil when there's no projection. Drives the ledger's
	// renewal-urgency sort. Active series only.
	DaysUntilRenewal *int
	// PriceChanged flags a series the detector saw steadily raising its price
	// (detection_signals amount_branch == "monotonic_drift") — surfaced as a
	// "Price ↑" chip on the ledger. The detail page shows the full from→to history.
	PriceChanged bool

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
	SignalsJSON    string // pretty-printed detection_signals (raw fallback)
	// SignalFacts is the parsed "Why detected" surface: the raw detection_signals
	// rendered as labelled criteria (charges seen, cadence, timing regularity,
	// amount pattern) instead of a raw JSON blob.
	SignalFacts []SubscriptionSignalFact

	Source       string // deterministic | agent | user | rule
	SourceLabel  string
	CategoryName string
	Tags         []string // tag slugs attached to the series (inherited by members)

	// Monthly-equivalent contribution (active rows) — used nowhere in the row
	// itself but kept for parity / future per-row display.
	MonthlyEquiv float64

	// MemberRows holds a bounded sample of the linked charges as canonical
	// transaction rows (rendered via the shared TxRowCompact), populated only
	// for candidates so the review card shows the evidence — account, category,
	// date — before the user commits to confirming.
	MemberRows []service.AdminTransactionRow

	// Filter support.
	UserID    string // formatted UUID, "" for shared/household
	OwnerName string
	Search    string // lowercase haystack for the filter input
}

// SubscriptionDetailProps is the typed input for /subscriptions/{short_id}.
type SubscriptionDetailProps struct {
	CSRFToken string

	Series SubscriptionRow // reuses the row shape for header chrome

	// Config grid values (pre-formatted, for read-only display fallbacks).
	ExpectedAmount  string
	AmountTolerance string
	ExpectedDay     string
	NextExpected    string
	LastSeen        string
	Confidence      string // auto | confirmed | rejected
	CreatedAt       string

	// Raw editable values — drive the inline-edit inputs (the formatted strings
	// above are display fallbacks). Name / Type / Cadence / Currency are read
	// off Series directly.
	HasExpectedAmount    bool
	ExpectedAmountValue  float64
	AmountToleranceValue float64
	ExpectedDayValue     int    // 0 = unset
	CurrentCategoryID    string // selected category UUID, "" = none
	// Categories is the full vocabulary for the suggested-category <select>.
	Categories []SubscriptionCategoryOption

	// Linked charges (newest first) as canonical transaction rows, rendered
	// with the shared TxRowCompact so the "Charges in this series" list reads
	// identically to the /transactions list.
	MemberRows []service.AdminTransactionRow

	// Price-change history derived from members (oldest → newest change points).
	PriceChanges []SubscriptionPriceChange

	// AvailableTags is every tag in the vocabulary (slug + name), for the
	// interactive tag editor's add-control. The template hides tags already on
	// the series client-side.
	AvailableTags []SubscriptionTagOption
	// TagChips is the resolved chip data (display/color/icon) for the tags
	// currently on the series — rendered through the shared TagChip component.
	TagChips []components.TagChipData

	// --- Detection-forward panels (assembled in the handler) ---
	Detection components.SeriesDetectionProps
	Evidence  components.SeriesEvidenceProps
	Facts     components.SeriesFactStripProps

	// --- Shared-picker payloads ---
	// CategoryTree seeds window.__bbCategories for the shared categoryPicker.
	CategoryTree []service.CategoryResponse
	// AllTags seeds window.__bbAllTags + the tag picker's availableTags list.
	AllTags []service.TagResponse
	// CurrentTagSlugs are the tags already on the series (the picker shows them
	// as "present" so the user can add/remove in one session).
	CurrentTagSlugs []string
}

// SubscriptionTagOption is one option in the detail page's add-tag picker.
type SubscriptionTagOption struct {
	Slug string
	Name string
}

// SubscriptionCategoryOption is one option in the suggested-category select.
type SubscriptionCategoryOption struct {
	ID   string // category UUID (resolves cleanly server-side)
	Name string
}

// RecurringSeriesFormProps drives the /recurring/new create form. On a
// validation error the handler re-renders with Error set and the entered
// values preserved (sticky form).
type RecurringSeriesFormProps struct {
	CSRFToken      string
	Error          string
	Name           string
	Type           string
	Cadence        string
	Currency       string
	ExpectedAmount string
	ExpectedDay    string
	CategoryID     string
	Categories     []SubscriptionCategoryOption
}

// SubscriptionMember is one linked charge in the detail list. Carries the
// category color/icon + pending + tag count so it renders through the shared
// TxRowFeed transaction-row component.
type SubscriptionMember struct {
	ShortID       string
	Date          string // "May 1, 2026"
	Name          string // raw provider description
	HasAmount     bool
	Amount        float64
	Currency      string
	Pending       bool
	CategoryColor *string
	CategoryIcon  *string
	TagCount      int
}

// SubscriptionSignalFact is one parsed detection criterion for the "Why
// detected" surface — a labelled, human-readable reading of a single
// detection_signals field (replaces the raw JSON dump).
type SubscriptionSignalFact struct {
	Icon  string // lucide name
	Label string // "Timing regularity"
	Value string // "Very regular (±4%)"
	Tone  string // success | warning | neutral
}

// seriesChargeDate formats an AdminTransactionRow.Date ("2006-01-02") as the
// leading date label in the series charge list ("Jan 2, 2006"); raw on parse fail.
func seriesChargeDate(s string) string {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Format("Jan 2, 2006")
	}
	return s
}

// subscriptionSignalFactClass maps a signal-fact tone to the value text color.
func subscriptionSignalFactClass(tone string) string {
	switch tone {
	case "success":
		return "text-success"
	case "warning":
		return "text-warning"
	default:
		return "text-base-content/80"
	}
}

// SubscriptionPriceChange marks a point where the charge amount changed.
type SubscriptionPriceChange struct {
	Date     string
	From     float64
	To       float64
	Currency string
}

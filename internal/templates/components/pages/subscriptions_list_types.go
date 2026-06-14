//go:build !headless && !lite

package pages

import (
	"fmt"
	"strings"
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

// SubscriptionPriceChange marks a point where the charge amount changed.
type SubscriptionPriceChange struct {
	Date     string
	From     float64
	To       float64
	Currency string
}

// SubscriptionStatusGroup is one status bucket of the ledger — Active, Paused,
// or Ended — rendered as a quiet label line (status · count · optional monthly
// subtotal) over its list-rows, the /accounts connection-group idiom applied to
// the recurring surface. Status lives on the group header, so the rows below it
// don't repeat a status badge.
type SubscriptionStatusGroup struct {
	Status string // active | paused | cancelled | (other, last)
	Label  string // "Active" | "Paused" | "Ended"
	Rows   []SubscriptionRow

	// Subtotal is the group's monthly-equivalent spend. Only populated
	// (HasSubtotal=true) for the Active group when every amount-bearing row
	// shares a single currency — paused/ended series aren't billing monthly,
	// and we never sum across iso_currency_code. SubtotalCurrency carries that
	// shared currency for the renderer.
	Subtotal         float64
	SubtotalCurrency string
	HasSubtotal      bool
}

// subscriptionStatusOrder is the canonical group order: active charges first,
// then paused, then ended/cancelled. Any status outside this set sorts after
// these, in first-seen order. Pure data so the grouping stays testable.
var subscriptionStatusOrder = []struct{ status, label string }{
	{service.SeriesStatusActive, "Active"},
	{service.SeriesStatusPaused, "Paused"},
	{service.SeriesStatusCancelled, "Ended"},
}

// GroupSubscriptionsByStatus buckets the ledger rows by status into Active →
// Paused → Ended groups (unknown statuses appended last, first-seen order),
// preserving the incoming row order within each group (the handler pre-sorts
// Active by renewal urgency). The Active group carries a monthly-equivalent
// subtotal when all its amount-bearing rows share one currency; never sums
// across currencies. Pure function — unit-tested without a DB.
func GroupSubscriptionsByStatus(rows []SubscriptionRow) []SubscriptionStatusGroup {
	label := make(map[string]string, len(subscriptionStatusOrder))
	rank := make(map[string]int, len(subscriptionStatusOrder))
	keys := make([]string, 0, len(subscriptionStatusOrder))
	for i, o := range subscriptionStatusOrder {
		label[o.status] = o.label
		rank[o.status] = i
		keys = append(keys, o.status)
	}

	byStatus := make(map[string][]SubscriptionRow)
	for _, r := range rows {
		if _, known := label[r.Status]; !known {
			if _, seen := byStatus[r.Status]; !seen {
				keys = append(keys, r.Status) // unknown status, appended last
			}
		}
		byStatus[r.Status] = append(byStatus[r.Status], r)
	}

	groups := make([]SubscriptionStatusGroup, 0, len(keys))
	for _, st := range keys {
		rs := byStatus[st]
		if len(rs) == 0 {
			continue
		}
		lbl := label[st]
		if lbl == "" {
			lbl = subscriptionStatusFallbackLabel(st)
		}
		g := SubscriptionStatusGroup{Status: st, Label: lbl, Rows: rs}
		if st == service.SeriesStatusActive {
			applySubscriptionMonthlySubtotal(&g)
		}
		groups = append(groups, g)
	}
	return groups
}

// applySubscriptionMonthlySubtotal fills the Active group's monthly-equivalent
// subtotal — but only when every amount-bearing row shares one currency and the
// total is positive. Mixed currencies leave the group subtotal-less rather than
// summing across them.
func applySubscriptionMonthlySubtotal(g *SubscriptionStatusGroup) {
	sum := 0.0
	cur := ""
	single := true
	any := false
	for _, r := range g.Rows {
		if !r.HasAmount {
			continue
		}
		rc := r.Currency
		if rc == "" {
			rc = "USD"
		}
		if !any {
			cur, any = rc, true
		} else if rc != cur {
			single = false
		}
		sum += r.MonthlyEquiv
	}
	if any && single && sum > 0 {
		g.Subtotal = sum
		g.SubtotalCurrency = cur
		g.HasSubtotal = true
	}
}

// subscriptionStatusFallbackLabel title-cases an unexpected status value so an
// unknown bucket still renders a readable header.
func subscriptionStatusFallbackLabel(status string) string {
	if status == "" {
		return "Other"
	}
	return strings.ToUpper(status[:1]) + status[1:]
}

// subscriptionsGroupCount renders the dimmed "N active charges" suffix on a
// group header.
func subscriptionsGroupCount(g SubscriptionStatusGroup) string {
	noun := "charge"
	if len(g.Rows) != 1 {
		noun = "charges"
	}
	return fmt.Sprintf("%d %s", len(g.Rows), noun)
}

// subscriptionsRowBodyLine is the row's single body line — cadence then the next
// renewal date ("Monthly · next Jun 28"). Falls back to a dash so the line is
// never empty.
func subscriptionsRowBodyLine(s SubscriptionRow) string {
	parts := make([]string, 0, 2)
	if s.CadenceLabel != "" {
		parts = append(parts, s.CadenceLabel)
	}
	if s.NextExpected != "" {
		parts = append(parts, "next "+s.NextExpected)
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, " · ")
}

// subStatusTileBg / subStatusTileIcon / subStatusTileIconColor map a series
// status to its leading status-tile treatment (principle #2 — status lives in
// one color-coded tile). Active is the only vivid-success state; paused warns;
// ended is a muted, non-error terminal state (a deliberate cancellation isn't an
// error, so it stays neutral rather than red).
func subStatusTileBg(status string) string {
	switch status {
	case service.SeriesStatusActive:
		return "bg-success/10"
	case service.SeriesStatusPaused:
		return "bg-warning/10"
	default:
		return "bg-base-200"
	}
}

func subStatusTileIcon(status string) string {
	switch status {
	case service.SeriesStatusActive:
		return "repeat"
	case service.SeriesStatusPaused:
		return "pause"
	case service.SeriesStatusCancelled:
		return "circle-slash-2"
	default:
		return "repeat"
	}
}

func subStatusTileIconColor(status string) string {
	switch status {
	case service.SeriesStatusActive:
		return "text-success"
	case service.SeriesStatusPaused:
		return "text-warning"
	default:
		return "text-base-content/50"
	}
}

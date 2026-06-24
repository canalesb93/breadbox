//go:build !headless && !lite

package pages

import (
	"fmt"
	"strings"

	"breadbox/internal/service"
	"breadbox/internal/templates/components"
)

// SubscriptionsListProps is the typed input for the /recurring admin page —
// the thin, rule-maintained recurring-series ledger. There is no detector, so
// there are no candidates, no monthly/upcoming totals, and no lifecycle status:
// just every live series, each with its name, type, and linked-charge count.
type SubscriptionsListProps struct {
	CSRFToken string

	// Rows is every live series, alphabetical by name.
	Rows []SubscriptionRow

	// Type filter strip (subscription/bill/loan/other present in the data).
	// Only rendered when len > 1 — no point offering a filter for one type.
	Types []SubscriptionTypeFilter
}

// SubscriptionTypeFilter — one option in the "filter by type" segmented control.
type SubscriptionTypeFilter struct {
	Value string // subscription | bill | loan | other — matches row data-type
	Label string // "Subscriptions" | "Bills" | "Loans" | "Other"
}

// SubscriptionRow is one series row on the ledger (and the header chrome on the
// detail page). A thin series carries only an identity (short_id, name), a
// structured type, its linked-charge count, and its inherited tags.
type SubscriptionRow struct {
	ShortID            string // short_id — drives the detail link
	Name               string
	Type               string // subscription | bill | loan | other (raw, drives the filter)
	TypeLabel          string // "Subscription" | "Bill" | "Loan" | "Other"
	MemberCount        int    // linked live charges
	GoverningRuleCount int    // assign_series rules that define membership
	Tags               []string

	// Search is the lowercase haystack for the client-side filter input.
	Search string
}

// SubscriptionDetailProps is the typed input for /recurring/{short_id}. The page
// makes the rule-substrate relationship explicit: the LINKED CHARGES (what's in
// the series) sit beside the GOVERNING RULES (the assign_series rules that
// define its membership). Name + type edit in a thin drawer; tags edit inline.
type SubscriptionDetailProps struct {
	CSRFToken string

	Series    SubscriptionRow // reuses the row shape for header chrome
	CreatedAt string

	// Linked charges (newest first) as canonical transaction rows, rendered
	// with the shared TxRowCompact so the "Charges in this series" list reads
	// identically to the /transactions list.
	MemberRows []service.AdminTransactionRow

	// GoverningRules are the rules whose assign_series action targets this
	// series — the durable definition of its membership (rules-as-substrate).
	GoverningRules []components.GoverningRule

	// AvailableTags is every tag not already on the series (slug + name), for
	// the inline tag editor's add-control.
	AvailableTags []SubscriptionTagOption
	// TagChips is the resolved chip data (display/color/icon) for the tags
	// currently on the series — rendered through the shared TagChip component.
	TagChips []components.TagChipData

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

// RecurringSeriesFormProps drives the /recurring/new create form. On a
// validation error the handler re-renders with Error set and the entered values
// preserved (sticky form). A thin series needs only a name and a type.
type RecurringSeriesFormProps struct {
	CSRFToken string
	Error     string
	Name      string
	Type      string
}

// BuildGoverningRule flattens a service.TransactionRuleResponse into the
// pure-view components.GoverningRule the governing-rules panel renders. The
// condition + action summaries reuse the same service helpers the /rules list
// uses, so a series' governing rules read identically to the rules ledger.
func BuildGoverningRule(r service.TransactionRuleResponse) components.GoverningRule {
	categoryName := ""
	if r.CategoryName != nil {
		categoryName = *r.CategoryName
	}
	return components.GoverningRule{
		ShortID:          r.ShortID,
		Name:             r.Name,
		ConditionSummary: service.ConditionSummary(r.Conditions),
		ActionSummary:    service.ActionsSummary(r.Actions, categoryName),
		Enabled:          r.Enabled,
		HitCount:         r.HitCount,
		CreatedByType:    r.CreatedByType,
		CreatedByName:    r.CreatedByName,
	}
}

// subscriptionMemberCount renders the dimmed "N charges" suffix on a row.
func subscriptionMemberCount(n int) string {
	if n == 1 {
		return "1 charge"
	}
	return fmt.Sprintf("%d charges", n)
}

// subscriptionRowMeta renders the "N charges · M rules" body line on a series
// row, mirroring counterpartyRowMeta: the charge count always shows; the rule
// count is appended only when at least one governing rule exists (keeping the
// line quiet for an as-yet-ungoverned series).
func subscriptionRowMeta(memberCount, ruleCount int) string {
	parts := []string{subscriptionMemberCount(memberCount)}
	switch ruleCount {
	case 0:
		// nothing — keep the line quiet when no rules govern it
	case 1:
		parts = append(parts, "1 rule")
	default:
		parts = append(parts, fmt.Sprintf("%d rules", ruleCount))
	}
	return strings.Join(parts, " · ")
}

// subscriptionTypeTone maps a series type to a calm, distinct EntityAvatar tint.
// It deliberately uses only the *vivid* semantic tones (success/warning/info) +
// neutral, because in the dark theme primary/secondary/accent collapse to gray
// (see the theme's vivid-tones rule) — a primary tint would read identically to
// neutral. So: subscription → success (green), bill → warning (amber), loan →
// info (blue), other → neutral (gray, the quiet bucket).
func subscriptionTypeTone(typ string) components.IconTone {
	switch typ {
	case "bill":
		return components.IconToneWarning
	case "loan":
		return components.IconToneInfo
	case "subscription":
		return components.IconToneSuccess
	default:
		return components.IconToneNeutral
	}
}

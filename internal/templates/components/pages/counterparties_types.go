//go:build !headless && !lite

package pages

import (
	"fmt"
	"strings"

	"breadbox/internal/service"
	"breadbox/internal/templates/components"
)

// CounterpartiesListProps is the typed input for the /counterparties admin page —
// the canonical, cross-provider "other side" of a charge (merchants AND
// non-merchants). A counterparty is a thin, rule-maintained entity: an identity +
// name + optional enrichment, whose membership comes from `assign_counterparty`
// rules. The list is a flat directory: name · logo · linked-charge count ·
// governing-rule count. No candidate/review concepts.
type CounterpartiesListProps struct {
	CSRFToken string
	Rows      []CounterpartyRow
}

// CounterpartyRow is one counterparty on the directory (and the header chrome on
// the detail page).
type CounterpartyRow struct {
	ShortID            string
	Name               string
	LogoURL            string // optional 16×16 thumbnail on the row
	MemberCount        int    // linked live charges
	GoverningRuleCount int    // assign_counterparty rules that define membership

	// Search is the lowercase haystack for the client-side filter input.
	Search string
}

// CounterpartyDetailProps is the typed input for /counterparties/{short_id}. The
// page pairs the manual ENRICHMENT form (name, category, mcc, website, logo) with
// the LINKED CHARGES (what's bound to it) and the GOVERNING RULES (the
// assign_counterparty rules that define its membership). No auto-fetch.
type CounterpartyDetailProps struct {
	CSRFToken string

	Counterparty CounterpartyRow
	CreatedAt    string

	// Enrichment field values (raw, for the edit form).
	WebsiteURL string
	LogoURL    string
	MCC        string
	CategoryID string // current category short_id ("" when unset)

	// CategoryOptions is the flat category picker (parent / child), each carrying
	// its short_id value and a Selected flag.
	CategoryOptions []CounterpartyCategoryOption

	// Linked charges (newest first) as canonical transaction rows, rendered with
	// the shared TxRowCompact so they read identically to the /transactions list.
	MemberRows []service.AdminTransactionRow

	// GoverningRules are the rules whose assign_counterparty action targets this
	// counterparty — its durable definition (rules-as-substrate).
	GoverningRules []components.GoverningRule
}

// CounterpartyFormProps drives the /counterparties/new create form. On a
// validation error the handler re-renders with Error set and the entered name
// preserved (sticky form).
type CounterpartyFormProps struct {
	CSRFToken string
	Error     string
	Name      string
}

// CounterpartyCategoryOption is one option in the enrichment form's category
// select. Value is the category short_id; resolveCategoryID accepts it.
type CounterpartyCategoryOption struct {
	Value    string
	Label    string
	Selected bool
}

// counterpartyMemberCount renders the dimmed "N charges" suffix on a row.
func counterpartyMemberCount(n int) string {
	if n == 1 {
		return "1 charge"
	}
	return fmt.Sprintf("%d charges", n)
}

// counterpartyGoverningSubtitle renders the governing-rules panel subtitle.
func counterpartyGoverningSubtitle(n int) string {
	switch n {
	case 0:
		return "Rules that assign charges here"
	case 1:
		return "1 rule defines membership"
	default:
		return fmt.Sprintf("%d rules define membership", n)
	}
}

// counterpartyRowMeta renders the "· N rules" governing-rule suffix on a list row.
func counterpartyRowMeta(memberCount, ruleCount int) string {
	parts := []string{counterpartyMemberCount(memberCount)}
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

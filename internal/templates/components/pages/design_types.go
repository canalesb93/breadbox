//go:build !headless && !lite

package pages

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"breadbox/internal/templates/components"

	"github.com/a-h/templ"
)

// DesignGalleryProps is the prop bag for the /design page — the full
// component gallery rendered on one scrollable page with section anchors.
type DesignGalleryProps struct {
	Breadcrumbs []components.Breadcrumb
	Sections    []DesignSection
}

// DesignComponentProps is the prop bag for /design/c/{slug} — a single
// component rendered in isolation so agents (and humans) can focus
// screenshots on one piece at a time.
type DesignComponentProps struct {
	Breadcrumbs []components.Breadcrumb
	Section     DesignSection
}

// DesignSection describes one entry in the design system gallery.
// Render must be a no-arg constructor for the section's templ component
// (kept as a closure so the slice of sections can be built once at
// package init without templ generics gymnastics).
//
// Group categorises the section into one of the top-level buckets used
// by the sandbox sidebar (see DesignSectionGroups). Sections with the
// same Group render under one collapsible header.
type DesignSection struct {
	Slug        string
	Title       string
	Description string
	Group       string // one of the slugs in DesignSectionGroups
	Render      func() templ.Component
}

// DesignSectionGroup is one collapsible top-level bucket in the
// sandbox sidebar. Groups render in the declared order — the first
// group is open by default.
type DesignSectionGroup struct {
	Slug  string // URL-safe id, used for the <input> name + anchors
	Label string // visible header
}

// DesignSectionGroups returns the canonical ordered list of top-level
// sandbox groups. Group slugs must match the Group field on each
// DesignSection. Order is fixed and shapes the sidebar.
func DesignSectionGroups() []DesignSectionGroup {
	return []DesignSectionGroup{
		{Slug: "foundations", Label: "Foundations"},
		{Slug: "layout", Label: "Layout"},
		{Slug: "navigation", Label: "Navigation"},
		{Slug: "forms", Label: "Forms"},
		{Slug: "data", Label: "Data display"},
		{Slug: "feedback", Label: "Feedback"},
		{Slug: "patterns", Label: "Patterns"},
	}
}

// SectionsByGroup returns the subset of `sections` whose Group matches
// `groupSlug`, preserving the input order.
func SectionsByGroup(sections []DesignSection, groupSlug string) []DesignSection {
	out := make([]DesignSection, 0, len(sections))
	for _, s := range sections {
		if s.Group == groupSlug {
			out = append(out, s)
		}
	}
	return out
}

// DesignSections returns the canonical, ordered list of gallery sections.
// Each entry maps a URL slug to a templ component that demonstrates the
// component family. To add a new section: write a `templ SectionFoo()`
// component in design_sections.templ and append an entry here.
//
// The slice is rebuilt on every call (cheap — just struct literals) so
// new sections are picked up without server restarts under
// BREADBOX_DEV_RELOAD when paired with `templ generate`.
func DesignSections() []DesignSection {
	return []DesignSection{
		// ── Foundations ─────────────────────────────────────────────
		{
			Slug:        "foundations",
			Title:       "Foundations",
			Description: "Color tokens, typography scale, spacing, radius — the raw material every component is built from.",
			Group:       "foundations",
			Render:      func() templ.Component { return SectionFoundations() },
		},
		{
			Slug:        "icons",
			Title:       "Icons & tiles",
			Description: "Lucide sizing convention, bb-icon-tile color modifiers.",
			Group:       "foundations",
			Render:      func() templ.Component { return SectionIcons() },
		},
		{
			Slug:        "kbd",
			Title:       "Keyboard shortcuts",
			Description: "Kbd, KbdChord, KbdCombo — single key, sequential \"g then d\" chord, and modifier-fused pill. Hidden on touch devices via $store.device.isTouch + sm: breakpoint.",
			Group:       "foundations",
			Render:      func() templ.Component { return SectionKbd() },
		},

		// ── Layout ──────────────────────────────────────────────────
		{
			Slug:        "page-header",
			Title:       "Page header",
			Description: "bb-page-header + bb-page-title + optional primary action slot.",
			Group:       "layout",
			Render:      func() templ.Component { return SectionPageHeader() },
		},
		{
			Slug:        "section-header",
			Title:       "Section header",
			Description: "Icon + h2 + count + optional action — for section headings INSIDE pages. Use components.SectionHeader (PageHeader is for top-of-page titles).",
			Group:       "layout",
			Render:      func() templ.Component { return SectionSectionHeader() },
		},
		{
			Slug:        "cards",
			Title:       "Cards",
			Description: "bb-card variants — simple, sectioned, interactive, danger-zone, empty-state.",
			Group:       "layout",
			Render:      func() templ.Component { return SectionCards() },
		},
		{
			Slug:        "empty-states",
			Title:       "Empty states",
			Description: "Standard no-data and no-results patterns. Use components.EmptyState.",
			Group:       "layout",
			Render:      func() templ.Component { return SectionEmptyStates() },
		},
		{
			Slug:        "stat-tiles",
			Title:       "Stat tiles",
			Description: "4-up dashboard metric tiles — icon-on-left, big tabular-nums value. Use components.StatTile + StatTileRow.",
			Group:       "layout",
			Render:      func() templ.Component { return SectionStatTiles() },
		},

		// ── Navigation ──────────────────────────────────────────────
		{
			Slug:        "tabs",
			Title:       "Tabs",
			Description: "Daisy tabs-border (navigation) and tabs-box (filter-as-tabs). Use components.TabBar. Nest a second TabBar inside an active tab's content for multi-level.",
			Group:       "navigation",
			Render:      func() templ.Component { return SectionTabs() },
		},
		{
			Slug:        "menus-dropdowns",
			Title:       "Menus & dropdowns",
			Description: "DaisyUI dropdown / menu, overflow action menu pattern.",
			Group:       "navigation",
			Render:      func() templ.Component { return SectionMenusDropdowns() },
		},
		{
			Slug:        "overflow-menu",
			Title:       "Overflow menu",
			Description: "Kebab dropdown for row actions. Use components.OverflowMenu.",
			Group:       "navigation",
			Render:      func() templ.Component { return SectionOverflowMenu() },
		},

		// ── Forms ───────────────────────────────────────────────────
		{
			Slug:        "buttons",
			Title:       "Buttons",
			Description: "Primary / ghost / outline / destructive / icon-only variants at sm + xs sizes.",
			Group:       "forms",
			Render:      func() templ.Component { return SectionButtons() },
		},
		{
			Slug:        "form-controls",
			Title:       "Form controls",
			Description: "Inputs, selects, textareas, checkboxes, toggles, file inputs.",
			Group:       "forms",
			Render:      func() templ.Component { return SectionFormControls() },
		},
		{
			Slug:        "filter-search-input",
			Title:       "Filter search input",
			Description: "Client-side filter input — daisy input + leading search icon + x-model binding for Alpine-driven row filtering. Use components.FilterSearchInput on /categories, /tags, and future inline-filter list pages.",
			Group:       "forms",
			Render:      func() templ.Component { return SectionFilterSearchInput() },
		},
		{
			Slug:        "category-picker",
			Title:       "Category picker",
			Description: "Shared `categoryPicker` Alpine factory + `bb-cat-picker` container paired with the four canonical button shells used across the admin (inline tx row, filter bar, assign panel, form select). Every variant routes through the same global overlay (base.html) via data-source-id so behaviour stays consistent.",
			Group:       "forms",
			Render:      func() templ.Component { return SectionCategoryPicker() },
		},

		// ── Data display ────────────────────────────────────────────
		{
			Slug:        "tables",
			Title:       "Tables",
			Description: "Zebra, sm/md/xs, hover row, sticky header, amount columns.",
			Group:       "data",
			Render:      func() templ.Component { return SectionTables() },
		},
		{
			Slug:        "badges",
			Title:       "Badges",
			Description: "Status chips (badge-soft) and metadata chips (badge-ghost). Pairs with statusBadge() helper.",
			Group:       "data",
			Render:      func() templ.Component { return SectionBadges() },
		},
		{
			Slug:        "tags",
			Title:       "Tags",
			Description: "Pill-shaped colored tags — bb-tag, bb-tag-sm, bb-tag-lg, bb-tag-ghost, bb-tag-add.",
			Group:       "data",
			Render:      func() templ.Component { return SectionTags() },
		},
		{
			Slug:        "amounts",
			Title:       "Amounts",
			Description: "components.Amount — the canonical renderer for monetary values. Three intents (transaction / balance / cost), three formats (standard / abbreviated / compact), pending modifier. Adopt for every new amount display so coloring and sign don't drift across pages.",
			Group:       "data",
			Render:      func() templ.Component { return SectionAmounts() },
		},
		{
			Slug:        "transaction-rows",
			Title:       "Transaction rows",
			Description: "TxRow / TxRowCompact / TxRowFeed and their building blocks (bb-tx-avatar, bb-tx-owner-badge, bb-tx-amount). The same avatar + amount shapes carry across every surface that lists transactions.",
			Group:       "data",
			Render:      func() templ.Component { return SectionTransactionRows() },
		},

		// ── Feedback ────────────────────────────────────────────────
		{
			Slug:        "alerts",
			Title:       "Alerts & flash",
			Description: "Page-level alert variants, inline bb-form-error, soft alerts.",
			Group:       "feedback",
			Render:      func() templ.Component { return SectionAlerts() },
		},
		{
			Slug:        "toast",
			Title:       "Toast",
			Description: "Floating notification pill. Fire from anywhere via the bb-toast custom event — every admin page mounts the global container in base.html. Use components.Toast for embedded contexts.",
			Group:       "feedback",
			Render:      func() templ.Component { return SectionToast() },
		},
		{
			Slug:        "loading",
			Title:       "Loading & skeletons",
			Description: "DaisyUI loading spinners, progress bars, skeleton placeholders.",
			Group:       "feedback",
			Render:      func() templ.Component { return SectionLoading() },
		},
		{
			Slug:        "modals",
			Title:       "Modals",
			Description: "DaisyUI modal-bottom sm:modal-middle, rounded-xl modal-box.",
			Group:       "feedback",
			Render:      func() templ.Component { return SectionModals() },
		},

		// ── Patterns ────────────────────────────────────────────────
		{
			Slug:        "multi-select-toolbar",
			Title:       "Multi-select toolbar",
			Description: "Floating bottom toolbar that surfaces bulk actions on a multi-selection. Reference: the transactions list's bulk action bar. Use components.MultiSelectToolbar.",
			Group:       "patterns",
			Render:      func() templ.Component { return SectionMultiSelectToolbar() },
		},
		{
			Slug:        "timeline",
			Title:       "Activity timeline",
			Description: "GitHub-style row-on-rail primitives shared by /feed and /transactions/{id} — Timeline wrapper (card + prominent variants), day separators, system rows (built-in tones + custom tile), comment rows, inline actor references, and the empty-state.",
			Group:       "patterns",
			Render:      func() templ.Component { return SectionTimeline() },
		},
	}
}

// designTimelineNow returns the render-time anchor used by the timeline
// sandbox examples. Centralised so every example shares a single now
// across midnight, matching the contract real callers (/feed and
// /transactions/{id}) hold with components.Timeline.
func designTimelineNow() time.Time { return time.Now() }

// designTimelineAgo returns an RFC3339 timestamp `d` before
// designTimelineNow() — the format components.Timeline accepts on
// TimelineRowProps.Timestamp / TimelineCommentRow / TimelineSystemRowCustomTile.
func designTimelineAgo(d time.Duration) string {
	return designTimelineNow().Add(-d).Format(time.RFC3339)
}

// toastDispatchExample returns the canonical dispatch snippet used in
// the Toast section of the sandbox. Lives here as a helper because
// literal `{` / `}` characters confuse templ's lexer when embedded in
// markup.
func toastDispatchExample() string {
	return "window.dispatchEvent(new CustomEvent('bb-toast', {\n" +
		"  detail: { message: 'Saved', type: 'success' }\n" +
		"}));"
}

// FindDesignSection looks up a section by URL slug.
func FindDesignSection(slug string) (DesignSection, bool) {
	for _, s := range DesignSections() {
		if s.Slug == slug {
			return s, true
		}
	}
	return DesignSection{}, false
}

// amountSandboxCode renders an AmountProps literal as a short Go-style
// code string for the sandbox copy-paste reference rows. Mirrors only
// the fields a reader is likely to type — drops zero-valued defaults so
// "{Value: 12.34}" stays terse instead of stringifying every field.
func amountSandboxCode(p components.AmountProps) string {
	parts := []string{fmt.Sprintf("Value: %s", trimFloat(p.Value))}
	switch p.Intent {
	case components.AmountBalance:
		parts = append(parts, "Intent: AmountBalance")
	case components.AmountCost:
		parts = append(parts, "Intent: AmountCost")
	}
	switch p.Format {
	case components.AmountFormatAbbreviated:
		parts = append(parts, "Format: AmountFormatAbbreviated")
	case components.AmountFormatCompact:
		parts = append(parts, "Format: AmountFormatCompact")
	}
	if p.Precision > 0 {
		parts = append(parts, fmt.Sprintf("Precision: %d", p.Precision))
	}
	if p.Pending {
		parts = append(parts, "Pending: true")
	}
	return "AmountProps{" + strings.Join(parts, ", ") + "}"
}

// trimFloat formats a float without a trailing ".0" when the value is a
// whole number, so "Value: 50" reads naturally next to "Value: 12.34"
// in the sandbox copy-paste reference rows. Uses FormatFloat with -1
// precision to drop trailing zeros, and 'f' to avoid the scientific
// notation %g switches to at ≥ 1e6 — abbreviated-format examples
// (Value: 1_234_567) would otherwise render as "1.234567e+06" and
// defeat the rows' copy-paste purpose.
func trimFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

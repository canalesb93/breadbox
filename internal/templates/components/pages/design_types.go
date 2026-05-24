//go:build !headless && !lite

package pages

import (
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
type DesignSection struct {
	Slug        string
	Title       string
	Description string
	Render      func() templ.Component
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
		{
			Slug:        "foundations",
			Title:       "Foundations",
			Description: "Color tokens, typography scale, spacing, radius — the raw material every component is built from.",
			Render:      func() templ.Component { return SectionFoundations() },
		},
		{
			Slug:        "buttons",
			Title:       "Buttons",
			Description: "Primary / ghost / outline / destructive / icon-only variants at sm + xs sizes.",
			Render:      func() templ.Component { return SectionButtons() },
		},
		{
			Slug:        "badges",
			Title:       "Badges",
			Description: "Status chips (badge-soft) and metadata chips (badge-ghost). Pairs with statusBadge() helper.",
			Render:      func() templ.Component { return SectionBadges() },
		},
		{
			Slug:        "alerts",
			Title:       "Alerts & flash",
			Description: "Page-level alert variants, inline bb-form-error, soft alerts.",
			Render:      func() templ.Component { return SectionAlerts() },
		},
		{
			Slug:        "cards",
			Title:       "Cards",
			Description: "bb-card variants — simple, sectioned, interactive, danger-zone, empty-state.",
			Render:      func() templ.Component { return SectionCards() },
		},
		{
			Slug:        "form-controls",
			Title:       "Form controls",
			Description: "Inputs, selects, textareas, checkboxes, toggles, file inputs.",
			Render:      func() templ.Component { return SectionFormControls() },
		},
		{
			Slug:        "tables",
			Title:       "Tables",
			Description: "Zebra, sm/md/xs, hover row, sticky header, amount columns.",
			Render:      func() templ.Component { return SectionTables() },
		},
		{
			Slug:        "tags",
			Title:       "Tags",
			Description: "Pill-shaped colored tags — bb-tag, bb-tag-sm, bb-tag-lg, bb-tag-ghost, bb-tag-add.",
			Render:      func() templ.Component { return SectionTags() },
		},
		{
			Slug:        "menus-dropdowns",
			Title:       "Menus & dropdowns",
			Description: "DaisyUI dropdown / menu, overflow action menu pattern.",
			Render:      func() templ.Component { return SectionMenusDropdowns() },
		},
		{
			Slug:        "modals",
			Title:       "Modals",
			Description: "DaisyUI modal-bottom sm:modal-middle, rounded-xl modal-box.",
			Render:      func() templ.Component { return SectionModals() },
		},
		{
			Slug:        "loading",
			Title:       "Loading & skeletons",
			Description: "DaisyUI loading spinners, progress bars, skeleton placeholders.",
			Render:      func() templ.Component { return SectionLoading() },
		},
		{
			Slug:        "icons",
			Title:       "Icons & tiles",
			Description: "Lucide sizing convention, bb-icon-tile color modifiers.",
			Render:      func() templ.Component { return SectionIcons() },
		},
		{
			Slug:        "page-header",
			Title:       "Page header",
			Description: "bb-page-header + bb-page-title + optional primary action slot.",
			Render:      func() templ.Component { return SectionPageHeader() },
		},
		{
			Slug:        "empty-states",
			Title:       "Empty states",
			Description: "Standard no-data and no-results patterns. Use components.EmptyState.",
			Render:      func() templ.Component { return SectionEmptyStates() },
		},
		{
			Slug:        "stat-tiles",
			Title:       "Stat tiles",
			Description: "4-up dashboard metric tiles — icon-on-left, big tabular-nums value. Use components.StatTile + StatTileRow.",
			Render:      func() templ.Component { return SectionStatTiles() },
		},
		{
			Slug:        "tabs",
			Title:       "Tabs",
			Description: "Daisy tabs-border (navigation) and tabs-box (filter-as-tabs). Use components.TabBar. Nest a second TabBar inside an active tab's content for multi-level.",
			Render:      func() templ.Component { return SectionTabs() },
		},
		{
			Slug:        "overflow-menu",
			Title:       "Overflow menu",
			Description: "Kebab dropdown for row actions. Use components.OverflowMenu.",
			Render:      func() templ.Component { return SectionOverflowMenu() },
		},
		{
			Slug:        "section-header",
			Title:       "Section header",
			Description: "Icon + h2 + count + optional action — for section headings INSIDE pages. Use components.SectionHeader (PageHeader is for top-of-page titles).",
			Render:      func() templ.Component { return SectionSectionHeader() },
		},
	}
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

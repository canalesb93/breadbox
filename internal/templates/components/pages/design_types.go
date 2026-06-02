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
	Sections    []DesignSection
}

// DesignComponentProps is the prop bag for /design/c/{slug} — a single
// component rendered in isolation so agents (and humans) can focus
// screenshots on one piece at a time.
type DesignComponentProps struct {
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
		{Slug: "onboarding", Label: "Onboarding"},
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

// DesignSectionGroupMap returns a slug → group-slug lookup used by the
// gallery's scroll-spy: when a section becomes active the Alpine factory
// resolves the containing group via this map so it can force-open the
// collapsed group in the sidebar.
func DesignSectionGroupMap(sections []DesignSection) map[string]string {
	m := make(map[string]string, len(sections))
	for _, s := range sections {
		m[s.Slug] = s.Group
	}
	return m
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
		{
			Slug:        "theme-controls",
			Title:       "Theme controls",
			Description: "ThemeToggle (compact cycling icon button — system → light → dark) and ThemeControl (explicit System/Light/Dark segmented control). Both bind to the shared $store.theme; \"system\" follows the OS via prefers-color-scheme.",
			Group:       "foundations",
			Render:      func() templ.Component { return SectionThemeControls() },
		},

		// ── Layout ──────────────────────────────────────────────────
		{
			Slug:        "page-header",
			Title:       "Page header",
			Description: "bb-page-header + bb-page-title + optional secondary + primary action slots.",
			Group:       "layout",
			Render:      func() templ.Component { return SectionPageHeader() },
		},
		{
			Slug:        "entity-header",
			Title:       "Entity header",
			Description: "Detail-page header: icon tile + title + badges + bullet-separated meta + actions. Use components.EntityHeader for account/connection/rule/sync-log/transaction detail pages.",
			Group:       "layout",
			Render:      func() templ.Component { return SectionEntityHeader() },
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
		{
			Slug:        "topbar-breadcrumb",
			Title:       "Topbar breadcrumb",
			Description: "The app's single location trail, rendered in the topbar (and a scrollable mobile strip). Section group + navigable parent crumbs + bold current page. Use components.TopbarBreadcrumb; the layout owns it — pages don't render their own breadcrumb.",
			Group:       "navigation",
			Render:      func() templ.Component { return SectionTopbarBreadcrumb() },
		},
		{
			Slug:        "sidebar-user-menu",
			Title:       "Sidebar user menu",
			Description: "Consolidated identity + actions popover at the bottom of the sidebar. Trigger shows avatar + display name + role; popover holds Documentation, GitHub, an editor-only /design shortcut, and a destructive Sign out below an <hr> separator. (Settings lives in the sidebar System section, not here.) Use components.SidebarUserMenu.",
			Group:       "navigation",
			Render:      func() templ.Component { return SectionSidebarUserMenu() },
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
			Slug:        "server-filter-toolbar",
			Title:       "Server filter toolbar",
			Description: "GET-driven filter bar for server-rendered list pages — a daisy search `input` + N daisy `select`s on one row, auto-submitting on `change`. Used on /rules (search + category + status + creator). Inline `<input type=hidden>` carries unrelated query state (sort, per_page) across submits; an active-filters `Clear` link resets the URL.",
			Group:       "forms",
			Render:      func() templ.Component { return SectionServerFilterToolbar() },
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
			Slug:        "recurring-chips",
			Title:       "Recurring chips",
			Description: "Shared chips for the Recurring (recurring-series) surfaces: SeriesTypeChip (type), SeriesRenewalChip (renewal health), SeriesConfidenceChip (detection band). Used across the ledger, candidate cards, and detail page.",
			Group:       "data",
			Render:      func() templ.Component { return SectionRecurringChips() },
		},
		{
			Slug:        "recurring-detail",
			Title:       "Recurring detail panels",
			Description: "Detection-forward panels for the recurring-series detail page: SeriesDetectionPanel (match-strength badge + plain-language summary + match-window range viz), SeriesEvidenceTimeline (charge timeline with matched/prior/projected markers + price-change inset), and SeriesFactStrip (read-only derived facts). The handler assembles the values; these are pure presentation.",
			Group:       "data",
			Render:      func() templ.Component { return SectionRecurringDetail() },
		},
		{
			Slug:        "tags",
			Title:       "Tags",
			Description: "Pill-shaped colored tags — bb-tag, bb-tag-sm, bb-tag-lg, bb-tag-ghost, bb-tag-add.",
			Group:       "data",
			Render:      func() templ.Component { return SectionTags() },
		},
		{
			Slug:        "tag-picker-button",
			Title:       "Tag picker button",
			Description: "Triggers that open the global tag picker — the inline bb-tag-add chip (transaction detail) and the bulk-toolbar btn-ghost variant (transactions list). Both fire the same open-tag-picker window event.",
			Group:       "data",
			Render:      func() templ.Component { return SectionTagPickerButton() },
		},
		{
			Slug:        "amounts",
			Title:       "Amounts",
			Description: "components.Amount — the canonical renderer for monetary values. Three intents (transaction / balance / cost), three formats (standard / abbreviated / compact), pending modifier. Adopt for every new amount display so coloring and sign don't drift across pages.",
			Group:       "data",
			Render:      func() templ.Component { return SectionAmounts() },
		},
		{
			Slug:        "user-avatar",
			Title:       "User avatar",
			Description: "components.UserAvatar — the shared identity badge used by the sidebar profile, transaction owner badge, activity-timeline rail tile + inline actor, and settings preview. Backed by the DiceBear /avatars/{id} endpoint with components.AvatarURL as the single source of truth for URL construction.",
			Group:       "data",
			Render:      func() templ.Component { return SectionUserAvatar() },
		},
		{
			Slug:        "editable-avatar",
			Title:       "Editable avatar",
			Description: "components.EditableAvatar — a live avatar whose entire surface is a shuffle/regenerate control (corner badge as the cue). The \"change this identity's picture\" affordance used by the Workflows reconfigure-drawer header. SrcExpr/OnShuffle are Alpine expressions so it drops into any seed-owning scope; the ring + badge tint primary and the glyph spins on hover.",
			Group:       "data",
			Render:      func() templ.Component { return SectionEditableAvatar() },
		},
		{
			Slug:        "transaction-rows",
			Title:       "Transaction rows",
			Description: "TxRow / TxRowCompact / TxRowFeed and their building blocks (bb-tx-avatar, bb-tx-owner-badge, bb-tx-amount). The same avatar + amount shapes carry across every surface that lists transactions.",
			Group:       "data",
			Render:      func() templ.Component { return SectionTransactionRows() },
		},
		{
			Slug:        "agent-run-rows",
			Title:       "Agent run rows",
			Description: "components.AgentRunRow + AgentRunRowList — the row shape for /agents (runs landing). Daisy list-row with a single status-toned icon tile, agent name + compact time, one optional body line (error → top report → skipped note), and cost. Trigger / cap details ride along as tooltip-only icons.",
			Group:       "data",
			Render:      func() templ.Component { return SectionAgentRunRows() },
		},
		{
			Slug:        "workflow-preset-card",
			Title:       "Workflow preset card",
			Description: "The /workflows gallery row (workflowPresetCard) — a clean Mintlify-style single line: a leading status tile (gray → green once set up; a red dot flags a failed last run), name + clamped description, and the run toggle + a settings gear. The gear opens the configure / reconfigure drawer, where Set up, Run now, Reconfigure, and Preview prompt all live. Flows in a 2-up grid.",
			Group:       "data",
			Render:      func() templ.Component { return SectionWorkflowPresetCard() },
		},
		{
			Slug:        "reports-table",
			Title:       "Reports table",
			Description: "components.ReportsTable + ReportTableRow + ReportPriorityBadge — the agent-reports index (/reports). A clean daisy table: Agent (name + time, primary dot for unread), Status (soft priority badge or em-dash), Summary (the title), and a trailing mark-read action. The whole row links to the report; the body lives on the detail page. The priority chip is reused on the report detail header.",
			Group:       "data",
			Render:      func() templ.Component { return SectionReportsTable() },
		},
		{
			Slug:        "agent-run-chat",
			Title:       "Agent run chat thread",
			Description: "The chat-thread variants the /agents/runs/{id} detail page composes — assistant + user bubbles (daisy chat), tool_use + tool_result rows (expand to JSON viewer), final result bubble, and the soft-alert error row. Pairs with the markdown-prose + json-viewer sections.",
			Group:       "data",
			Render:      func() templ.Component { return SectionAgentRunChat() },
		},
		{
			Slug:        "markdown-prose",
			Title:       "Markdown prose",
			Description: "components.Markdown renders agent-emitted text as a safe HTML fragment wrapped in .bb-prose. Supports paragraphs, headers (##/###), emphasis, inline + fenced code, lists (one nest level), GFM tables, blockquotes, and http/https/mailto links. Drives the assistant chat bubble on the run-detail page and will land in the reports body next.",
			Group:       "data",
			Render:      func() templ.Component { return SectionMarkdownProse() },
		},
		{
			Slug:        "json-viewer",
			Title:       "JSON viewer",
			Description: "components.JSONViewer pretty-prints a JSON payload with collapsible objects + arrays and an optional copy-to-clipboard button. Powers the tool_use / tool_result rows on the run-detail transcript.",
			Group:       "data",
			Render:      func() templ.Component { return SectionJSONViewer() },
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
		{
			Slug:        "drawers",
			Title:       "Drawer (slide-over)",
			Description: "Right-side slide-over sheet for focused create/edit flows without leaving the page — backdrop + sliding panel + DrawerHeader + scrollable body + DrawerFooter. Opened from anywhere via $store.drawers.open('<id>'). Drives the Workflows Set up / Configure / Reconfigure flows; reach for it for simple inline edits instead of a full page or a cramped modal.",
			Group:       "feedback",
			Render:      func() templ.Component { return SectionDrawer() },
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
			Slug:        "tag-picker",
			Title:       "Tag picker",
			Description: "Global picker overlay (singleton in base.html, driven by globalTagPicker()) — multi-tx diff editor: each chip cycles through absent/pending-add/pending-remove/present/mixed states, nothing fires until Apply. Opens via the open-tag-picker window event (see tag-picker-button section for triggers).",
			Group:       "patterns",
			Render:      func() templ.Component { return SectionTagPicker() },
		},
		{
			Slug:        "timeline",
			Title:       "Activity timeline",
			Description: "GitHub-style row-on-rail primitives shared by /feed and /transactions/{id} — Timeline wrapper (card + prominent variants), day separators, system rows (built-in tones + custom tile), comment rows, inline actor references, and the empty-state.",
			Group:       "patterns",
			Render:      func() templ.Component { return SectionTimeline() },
		},
		{
			Slug:        "command-palette",
			Title:       "Command palette",
			Description: "The ⌘K cmdk shell (bb-cmdk-*) rendered inline — catalogues the dialog, generic command rows (default + active), the tx variant that wraps TxRowCompact, and the loading + empty states. Pins down the current row markup so the queued migration to a daisy <dialog class=\"modal\"> shell has a target.",
			Group:       "patterns",
			Render:      func() templ.Component { return SectionCommandPalette() },
		},
		{
			Slug:        "settings",
			Title:       "Settings",
			Description: "SettingsSection / SettingsRow / SettingsAutoSaveForm — the shared shape every /settings/* tab uses. See .claude/rules/settings.md for the design language (anatomy, width, auto-save vs single-Save-per-section, danger variant).",
			Group:       "patterns",
			Render:      func() templ.Component { return SectionSettings() },
		},

		// ── Onboarding ──────────────────────────────────────────────
		{
			Slug:        "onboarding-hero",
			Title:       "Onboarding hero",
			Description: "The /getting-started banner: a circular ProgressRing + a headline that warms up by progress toward an all-set celebration, with a time-remaining estimate. Use components.OnboardingHero + components.ProgressRing.",
			Group:       "onboarding",
			Render:      func() templ.Component { return SectionOnboardingHero() },
		},
		{
			Slug:        "setup-step",
			Title:       "Setup step",
			Description: "The stateful onboarding step row: active (elevated, \"you are here\"), in_progress (spinner), complete (collapsed confirmation), and pending (muted). Topical icon, time estimate, optional badge, doc link, single reflowing CTA. Use components.SetupStep.",
			Group:       "onboarding",
			Render:      func() templ.Component { return SectionSetupStep() },
		},
		{
			Slug:        "onboarding-stats",
			Title:       "Onboarding stats",
			Description: "Always-on Connections/Accounts/Transactions/Syncs strip for /getting-started, with a graceful muted zero state. Composes StatTile + StatTileRow. Use components.OnboardingStats.",
			Group:       "onboarding",
			Render:      func() templ.Component { return SectionOnboardingStats() },
		},
		{
			Slug:        "onboarding-next-steps",
			Title:       "Onboarding next steps + footer",
			Description: "The celebratory \"what's next\" destination grid shown once onboarding completes, plus the redesigned dismiss/resume footer (both states). Use components.OnboardingNextSteps + components.OnboardingFooter.",
			Group:       "onboarding",
			Render:      func() templ.Component { return SectionOnboardingNextSteps() },
		},
		{
			Slug:        "onboarding-alt-path",
			Title:       "Onboarding alt path",
			Description: "Calm CSV-import alternative + documentation resources aside, shown while setup is in progress. Copy adapts to whether a provider is configured. Use components.OnboardingAltPath.",
			Group:       "onboarding",
			Render:      func() templ.Component { return SectionOnboardingAltPath() },
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

// tagPickerDispatchExample returns the canonical dispatch snippet for
// opening the global tag picker. Mirrors the live call sites
// (transaction_detail.js, transactions.js bulk bar + 't' shortcut). The
// literal `{` / `}` characters live here as a Go string for the same
// templ-lexer reason as toastDispatchExample.
func tagPickerDispatchExample() string {
	return "window.dispatchEvent(new CustomEvent('open-tag-picker', {\n" +
		"  detail: {\n" +
		"    sourceId:       'txd-tag',          // echoed back in tag-selection-commit\n" +
		"    transactionIds: [txId],             // 1 for inline chip, N for bulk\n" +
		"    txCount:        1,\n" +
		"    appliedCounts:  { recurring: 1 },   // {slug: count present across selection}\n" +
		"    availableTags:  window.__bbAllTags, // registered tags for the chip grid\n" +
		"  },\n" +
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

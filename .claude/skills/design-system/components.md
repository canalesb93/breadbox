# Component catalog

The shared surface components, their props, and the one-line rule for each.
All live in `internal/templates/components/`. This is a reference — read the
`.templ`/`.go` source for the full contract. Generated `_templ.go` siblings are
committed; run `templ generate` after editing a `.templ`.

> Enum values below are the source-of-truth constants. When you add a variant,
> update both the component and this file in the same PR (this skill is living).

---

## PageHeader — `page_header.templ`

The chrome every surface opens with. Title + optional subtitle + trailing actions.

```go
PageHeaderProps{
    Title                string        // required
    Subtitle             string        // optional
    Icon                 string        // optional lucide name
    SubtitleHideOnMobile bool          // hide subtitle < sm to save vertical space
    Leading              templ.Component // optional avatar/visual left of the title
    TitleBadge           templ.Component // optional inline chip (e.g. "Beta")
    SecondaryAction      templ.Component // PageHeaderSecondaryAction(...)
    Action               templ.Component // PageHeaderAction(...) — primary, trailing
}
```

- One primary action max; everything else is `SecondaryAction` or recedes into the body.
- Build the common single-action case with `PageHeaderAction(href, icon, label)`.

## TabBar — `tab_bar.templ`

Two jobs, one component. `border` = section navigation; `box` = in-page filter.

```go
TabBarProps{
    Items      []TabBarItem
    Variant    string // "border" (default, nav) | "box" (filter)
    AriaLabel  string // REQUIRED
    Scrollable bool   // flex-nowrap; wrap caller in overflow-x-auto for mobile
}
TabBarItem{ Label string; Href templ.SafeURL; Active bool; Icon string; Count *int }
```

- `border` for Gallery/Runs-style section tabs (give each an `Icon`).
- `box` for status/type filters — give each item a `Count` (use
  `components.SectionHeaderIntPtr(n)`). Pair with a `<select>` for a second axis;
  don't stack two tab rows.
- Never stretch a `box` TabBar full-width. It scrolls horizontally on mobile.

## Filter controls — three blessed shapes, not duplicates

There are three filter primitives. They are **not** redundant — pick by the
filtering model, don't invent a fourth:

| Use | Shape | Example |
| --- | --- | --- |
| Single-axis **client-side text** filter on a short list | `FilterSearchInput` (`filter_search_input.templ`) | `/tags`, `/categories`, `/subscriptions` |
| **Multi-axis server-side** filter (search + status + category + creator) | server filter toolbar (sandbox slug `server-filter-toolbar`) | `/rules` |
| **Status segments** with live counts | box-variant `TabBar` + a `<select>` for the secondary axis | `/workflows/runs` |

Complex multi-field panels (date ranges + pickers, e.g. `/transactions`,
`/logs`) are a collapsible panel — fine, but hold them to one shape. A
low-cardinality *view* toggle (Grouped/List) is a daisy `join`, not a filter.

## Badge tone & contrast — vivid only

Badges are **terminal/categorical** state only — never a loading/running/
in-progress state (that's a spinner + text). And on the dark theme only
`info`/`success`/`warning`/`error` are vivid; `primary`/`secondary`/`accent`/
`neutral` soft badges read as gray-on-gray and all but disappear
(`reference_theme_vivid_tones`). Map meaning to a vivid tone, or use
`badge-ghost` for a deliberately quiet neutral. Standard shape:
`badge badge-soft badge-{vivid-tone} badge-sm`.

## SectionHeader — `section_header.templ`

A titled section divider with an optional count + trailing action.

```go
SectionHeaderProps{ Icon string; Title string; Count *int; CountLabel string /* "items" */; Action templ.Component }
```

`SectionHeaderIntPtr(n)` builds the `*int` for `Count` (and for `TabBarItem.Count`).

## IconTile — `icon_tile.templ`

The single color-coded status tile that leads a row (principle 2).

```go
@components.IconTile(tone, icon)  // tone drives bg + currentColor of the glyph
```

- Tone is a semantic word (`success`/`error`/`warning`/`info`/…) mapped to a
  `bb-icon-tile--{tone}` class. The inner glyph inherits `currentColor` — don't
  add `text-{tone}` yourself.
- This is the *status* affordance. Do not also render a "Success" text badge.

## Drawer — `drawer.templ`

The slide-over for create / configure / edit (principle 5). Opened via
`$store.drawers.open('<id>')`; closed by Escape or backdrop.

```go
DrawerProps{ ID string /* required, store key */; Title string /* aria-label */; Size string /* sm|md(default)|lg|xl */; Class string }
DrawerHeaderProps{
    Icon string; BrandIcon string  // BrandIcon = vendored brand SVG (brand.templ)
    Title string; TitleExpr string // TitleExpr = Alpine x-text, overrides Title
    Subtitle string; SubtitleExpr string
    Right templ.Component           // optional control on the header top line (e.g. enable toggle)
}
@components.DrawerFooter() { /* sticky action pair — primary on the right */ }
```

Anatomy: `DrawerHeader` → scrollable body → sticky `DrawerFooter`. Namespace ids
per surface so two drawers on one page never collide.

## RadioCard — `radio_card.templ`

Choice cards inside a drawer (trigger type, model, mode).

```go
RadioCardProps{
    Name string; Value string          // required — radio group + this card's value
    Icon string; Title string; Subtitle string
    ModelExpr string; ChangeExpr string // Alpine x-model / @change (empty = native)
    Checked bool; Class string
}
```

## OverflowMenu — `overflow_menu.templ`

Per-row kebab of secondary actions.

```go
OverflowMenuProps{ AriaLabel string /* required */; Items []OverflowMenuItem; Position string /* end(default)|start */; Size string /* sm(default)|xs */ }
```

- Use `Size: "sm"` on list-rows (the `xs` circle reads as too small/fiddly — see #1793).
- Render it *outside* a row's main `<a>` link so the menu button isn't swallowed by the navigation hit area.

## UserAvatar — `user_avatar.templ`

Stable DiceBear identity for people and agents (principle 7).

```go
UserAvatarProps{
    ID string; Name string /* tooltip+alt+fallback glyph */; Version string /* updated_at for ?v= */
    Size UserAvatarSize; IsAgent bool; Ring bool; Inline bool; Decorative bool; Lazy bool; Loading bool
    Class string; Title string; SrcOverride string
}
// Sizes: UserAvatarXS(16) · UserAvatarSm · UserAvatarMd · UserAvatarLg · UserAvatarXL
```

- `IsAgent: true` routes to the agent DiceBear style (or a bot-tile fallback).
- `Decorative: true` (alt="") when an adjacent `<strong>` already names the actor.
- Use `XS` on dense list-rows (owner badge), larger on profile/header surfaces.

## Amount — `amount.templ`

Every monetary value. **Private by default.**

```go
AmountProps{
    Value float64; Intent AmountIntent; Format AmountFormat; Precision int
    Pending bool; Class string; Public bool // Public = opt OUT of privacy (rare)
}
// Intent: AmountTransaction("") · AmountBalance("balance") · AmountCost("cost")
// Format: AmountFormatStandard("") · AmountFormatAbbreviated · AmountFormatCompact
```

- Sign convention is intent-aware; pair every amount with its `iso_currency_code`
  upstream and never sum across currencies.
- Leave `Public` false for all real balances/totals — it stays `data-private="amount"`.

## StatTile / StatTileRow — `stat_tile.templ`

Summary numbers when figures lead the page (Net Worth / Assets / Liabilities).

```go
StatTileProps{
    Icon string; Label string; Value string; Description string
    Tone StatTileTone; Href templ.SafeURL
    ValuePrivateKind string; DescriptionPrivateKind string // "amount" → marks data-private
}
// Tone: StatToneNeutral · StatTonePrimary · StatToneSuccess · StatToneWarning · StatToneError · StatToneInfo
@components.StatTileRow() { @components.StatTile(...) ... } // the divided strip
```

Set `ValuePrivateKind: "amount"` on money tiles so they obfuscate with privacy mode.

## EmptyState — `empty_state.templ`

Zero state for an empty list/group/page.

```go
EmptyStateProps{
    Icon string; Title string /* required */; Body string
    CTAHref templ.SafeURL; CTAIcon string; CTALabel string; CustomCTA templ.Component
    Compact bool; InCard bool // Compact = small inline; InCard wraps the compact variant
}
```

## CollapsibleSection — `collapsible_section.templ`

A quiet, reusable disclosure: a header row (lucide glyph + label + optional
right-slot indicator) toggling its body open/closed with a rotating chevron.

```go
CollapsibleSectionProps{
    Icon string; Label string /* required */
    DefaultOpen bool            // data-driven open-on-render decision
    Right templ.Component       // optional header indicator (rendered in scope)
    Class string
}
```

- Disclosure state is a tiny Alpine `{ open: <bool> }`; the body renders with
  `x-show` (NOT `<details>`), so **form inputs inside a collapsed section still
  submit** — this is the reason to use it over daisy `collapse`/`<details>`
  inside a `<form>`.
- The component exposes `open` in its Alpine scope. A `Right` indicator guarded
  with `x-show="!open"` surfaces only when collapsed (the "active filter hidden
  here" cue). Nest the component inside an ancestor `x-data` to let the
  indicator read reactive state (e.g. the Tags section reads `selectedSlugs`).
- The header is a real `<button>` with a comfortable `py-3` touch target and
  full interaction states (per the *Interaction states* principle in
  [`SKILL.md`](SKILL.md)): `hover:bg-base-200/60` with `transition-colors`,
  `cursor-pointer`, and a keyboard `focus-visible:ring-2 focus-visible:ring-primary/40`
  ring. The chevron rotates smoothly (`transition-transform duration-200`).
- Backs the `/transactions` filter drawer's When / Where / What / Tags / Search
  sections: each defaults open when it holds an active filter (computed in
  `TransactionsProps.*SectionOpen`) and collapses with a count/dot indicator
  otherwise. Sandbox: `/design/c/collapsible-section`.

## Skeletons — `*_skeleton.templ`

Loading placeholders that mirror the real component's shape so the swap-in
doesn't shift layout. Use the daisy `skeleton` primitive (the hand-rolled
`bb-skeleton*` family is retired). See `accounts_list_skeleton.templ` for the
grouped-list shape.

## Settings dialect — `settings_section.templ` / `settings_row.templ` / `settings_autosave_form.templ`

Settings tabs use a local dialect of this language. See
[`.claude/rules/settings.md`](../../rules/settings.md) — `SettingsSection` +
`SettingsRow`, auto-save single controls, no accordions, danger-zone variant.

---

## ReportRow / ReportsList — `report_table.templ`

The agent-reports inbox row, a list-row sibling of `AgentRunRow` (same
shape, so `/reports` and `/agents/runs` read as one product).

```go
ReportRowProps{
    ID string; Title string /* summary */; Priority string /* "" | "info" | "warning" | "critical" */
    Author string; AuthorID string /* avatar seed */; IsAgent bool
    Time string /* pre-rendered relative */; IsRead bool
}
@components.ReportRowList() { for _, r := range rows { @components.ReportRow(r) } }
@components.ReportsList(rows) // convenience: flat list in one card
```

- Leading status tile = priority → vivid `IconTile` tone (critical→error,
  warning→warning, routine→info). No parallel priority text badge in the row.
- Title line: `UserAvatar` (IsAgent) + agent name + an info **unread dot** + the
  relative time; the summary is the one body line below. Read rows fade
  (`opacity-60`) and drop the dot; unread rows keep a heavier summary.
- `OverflowMenu` (Size `"sm"`) — mark read/unread + open — renders OUTSIDE the
  row's `<a>` link. The `/reports` All view groups rows unread-first under quiet
  label lines (`partitionReportsByRead`). `ReportPriorityBadge` is now
  detail-header-only.

## Reference implementation

`agent_run_row.templ` (`AgentRunRowList` + `AgentRunRow`) is the cleanest single
example of principles 2–7: status `IconTile`, one priority body line, bare-icon
actions, `UserAvatar` identity, list-row spacing in a `bb-card`. Read it before
authoring a new feed/list row. `report_table.templ` is its inbox sibling.

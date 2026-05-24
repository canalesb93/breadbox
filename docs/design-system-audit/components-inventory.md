# v1 Admin UI Component Inventory

**Date:** 2026-05-24. **Scope:** `internal/templates/{components,components/pages,layout,partials}/*.{templ,html}`, `input.css`, and `static/js/admin/components/*.js`. v2 SPA (`web/`) is excluded. **Methodology:** for each candidate pattern I greped the templ + html source for class anchors (`bb-*`, `btn btn-*`, `bb-card …`, `<dialog`, etc.), counted distinct variants and call sites, cross-referenced against the canonical spec in `docs/design-system.md`, and read the larger pages (`access.templ`, `users.templ`, `categories.templ`, `connection_detail.templ`, `feed.templ`, `logs.templ`, `transaction_detail.templ`, `transactions.templ`, `settings*.templ`, `agents_settings.templ`) end-to-end to surface duplicated 30-100 line blocks. Numbers are exact greps across the templ tree (generated `*_templ.go` excluded), not estimates.

The codebase is ~14k lines of templ + 1 monolithic 2.4k-line `base.html` + 3.2k-line `input.css`. The `pages/` directory is the body of the audit — `pages/_templ_shell.html` and `partials/*.html` are 5-11 line stubs (only `breadcrumb.html`, `category_picker.html`, `flash.html`, `nav.html`, `tx_row.html` remain), so the html/template surface area is effectively retired.

## Summary table

| # | Component / pattern | Variant count | Callers (count) | DaisyUI native? | Recommendation |
|---|---|---|---|---|---|
| 1 | Page header (`.bb-page-header`) | 1 spec, ~3 in-use | 37 templ files | partial (`navbar`) | Extract to templ component `PageHeader(title, subtitle, action)` |
| 2 | Breadcrumb | 1 | 22 (via `@components.BreadcrumbNav`) | none | Healthy — keep as is |
| 3 | Card (`.bb-card`) | 87 distinct class combos | 166 occurrences | `card` (rejected for shadow) | Healthy primitive; document the canonical variants in spec |
| 4 | Sectioned card (`.bb-card p-0 overflow-hidden` + `bb-action-row`) | 1 spec, 6 in-use | 28 | none | Extract `FormCard(IconHeader, body, ActionRow)` templ component |
| 5 | Icon-header tile (`.bb-icon-header__tile`) | 4 colour modifiers + ~6 ad-hoc | 51 | none | Strong primitive; migrate the ad-hoc `w-10 h-10 rounded-xl bg-X/8` blocks (15 sites) to it |
| 6 | Action row (`.bb-action-row`) | 1 | 9 | none | Healthy — only 9 callers because most pages still hand-roll the bottom row |
| 7 | Form-input primitive (`.bb-form-input` / `.bb-form-select`) | 2 | 18 (in 8 files) | partial (`input input-bordered`) | Adopted in form pages; ~30 other `input input-bordered` sites should migrate |
| 8 | Form-error inline (`.bb-form-error`) | 1 spec | 0 in templ (defined in input.css §299) | none | **Bug: defined CSS class with zero callers.** Either adopt or retire |
| 9 | Buttons (`btn btn-*`) | 28+ distinct combos | 176 | yes (`btn`) | Healthy with caveats; see Cross-cutting (size-only inconsistencies) |
| 10 | Badges (`badge badge-*`) | 46 distinct combos | 186 | yes (`badge`) | Add a `Badge(tone, size)` templ helper to dedupe; most are status / count chips |
| 11 | Status badge (`StatusBadge()`, `SyncBadge()` helpers) | 1 each | 9 of 37 status-rendering sites | none | Underused — only 9 callers; rest of status renderings still hand-roll badges |
| 12 | Tag chip (`.bb-tag` + variants) | 5 (sm / lg / ghost / interactive / add) | 6 direct templ + heavy JS | none | Healthy; well-documented variants |
| 13 | Tables (`table table-sm/md/xs`) | 4 in-use | 4 templ tables | yes (`table`) | Underused — most "tables" are flex rows; keep as is |
| 14 | Filter bar (`.bb-filter-bar`) | 2 (legacy + `.bb-filter-toggle` collapse) | 2 callers | none | Spec says 3+ usage required for a `bb-*` class; 2 callers → retire or document as obsolete |
| 15 | Filter toggle / form (`.bb-filter-toggle`, `.bb-filter-form`) | 1 | 1 (`logs.templ`) | none | **Single caller** — likely premature abstraction |
| 16 | Empty state | 1 spec, 5 visual variants | 18 sites (counted `bb-card p-12 text-center` / `p-8 sm:p-12` / `bb-card` + flex-col) | none | Extract `EmptyState(icon, title, body, cta)` templ — duplicated 80+ lines across the codebase |
| 17 | Overflow action menu (`dropdown dropdown-end` + ellipsis) | 1 spec, 2 sizes | 6 | yes (`dropdown` + `menu`) | Extract `OverflowMenu(items)` templ |
| 18 | Confirm-inline pattern (`x-data="{ confirming: false }"`) | 1 | 4 (`access`, `connection_detail`, others) | none | Wrap as templ component or shared Alpine factory |
| 19 | Modal (`<dialog class="modal">`) | 1 spec | 3 | yes (`modal`) | Healthy — only 3 modals total |
| 20 | Confirm overlay (`.bb-confirm-*` in `base.html`) | 1 | 1 global | none | Healthy — replaces `confirm()` |
| 21 | Cmd+K palette (`.bb-cmdk-*`) | 1 | 1 global + JS | none | Healthy, isolated |
| 22 | Category picker overlay (`.bb-catpicker-*`) | 1 | 1 global + JS | none | Healthy, isolated |
| 23 | Tag picker overlay (`.bb-tagpicker-*`) | 1 | 1 global + JS | none | Healthy, isolated |
| 24 | Shortcuts help overlay (`.bb-shortcuts-*`) | 1 | 1 global | none | Healthy, isolated |
| 25 | Kbd hints (`.bb-kbd`, `.bb-kbd-combo`) | 2 | 8 (`@components.Kbd*`) + 5 hand-rolled | yes (`kbd`) | Strong primitive; replace remaining hand-rolled `<kbd class="bb-kbd">` and `<kbd class="bb-shortcuts-kbd">` |
| 26 | Toast (`bb-toast` event) | 1 | global | yes (`toast`) | Healthy |
| 27 | Alerts (`alert alert-*`) | 5 in-use | 13 | yes (`alert`) | Healthy; every caller adds `rounded-xl` — fold into a `PageAlert` templ |
| 28 | Triage row (`.bb-triage-*`) | 1 | 0 in templ (used by `reviews.js` only) | none | **No templ caller for ~200 lines of CSS.** Verify it's still wired via `/reviews` JS-render path |
| 29 | Sparkline / summary-pill (`.bb-sparkline`, `.bb-summary-pill`) | 1 each | **0 templ callers** | none | Dead CSS — retire after confirming no JS-render path |
| 30 | Stagger animation (`.bb-stagger`, `.bb-page-enter`) | 1 each | 30 (`bb-stagger`) + 1 (`bb-page-enter`) | none | Healthy entrance pattern |
| 31 | Skeleton (`.bb-skeleton*`) | 7 size variants | **0 templ callers** | none | Dead CSS (~110 lines) — retire or wire into a real loading state |
| 32 | View toggle (`.bb-view-toggle`) | 1 | 1 (`transactions.templ`) | none | Single caller — retire or generalize |
| 33 | Table header (`.bb-table-header`) | 1 | 2 (`tx_results`, `account_detail`) | none | 2 callers — below the spec's 3+ threshold |
| 34 | Pagination (`.bb-paginator`) | 1 | 1 (`tx_results`) | yes (`join`) | Single caller — but well-designed; retain |
| 35 | Tx-row primitives (`.bb-tx-row`, `.bb-tx-avatar`, `.bb-tx-amount`) | 3 base + variants | 23 | none | Healthy domain primitive |
| 36 | Tx-row component (`@components.TxRow*`) | 3 (full / compact / feed) | (replaces `partials/tx_row.html`) | none | Healthy |
| 37 | Activity timeline (`@components.Timeline*`) | 6 primitives + prominent override | 17 | none | Healthy; large but well-organized |
| 38 | Sidebar (`.bb-sidebar*`) | 1 | 1 (`nav.templ`) | yes (`drawer`/`menu`) | Healthy custom-styled sidebar |
| 39 | Mobile navbar (`.bb-mobile-navbar`) | 1 | 1 (`base.html`) | yes (`navbar`) | Healthy |
| 40 | Rule badge (`.bb-rule-cat-badge`, `.bb-rule-creator-badge`) | 2 | 0 in templ | none | **Dead CSS** — retire |
| 41 | Wizard layout (`.bb-wizard-*`) | 1 | 4 (login, setup, error pages) | none | Healthy isolated layout |
| 42 | Error page (`.bb-error-*`) | 1 | 1 (`errors.templ`) | none | Healthy |
| 43 | Comment bubble (`.bb-comment-bubble`) | 1 + markdown subselectors | 2 | none | Healthy primitive, well-isolated to feed + transaction-detail |
| 44 | Report body (`.bb-report-body`) | 1 + markdown subselectors | 1 | none | Single caller but justified — markdown styling is page-specific |
| 45 | Settings rail (mobile dropdown + desktop list) | 1 templ | 1 (`settings_layout.templ`) | none | Healthy isolated layout |

## Per-component sections

### 1. Page header (`.bb-page-header`)

- **What it is:** Top of every admin page — title + optional subtitle on the left, optional primary action on the right.
- **Where used:** 37 templ files, e.g. `pages/access.templ:10`, `pages/categories.templ:27`, `pages/connection_new.templ:21`, `pages/connections.templ:37`, `pages/feed.templ:49`, `pages/getting_started.templ:15`, `pages/logs.templ:20`, `pages/my_account.templ:15` and `:56`, `pages/settings.templ:15`/`:30`/`:43`/`:58`, `pages/transactions.templ:92`, `pages/users.templ:19`.
- **Variants in use:** (a) title only; (b) title + subtitle paragraph; (c) title + counts; (d) title + primary-action button on the right; (e) title with leading icon (`getting_started.templ:17`).
- **DaisyUI native equivalent:** None.
- **Current implementation:** Raw `<div class="bb-page-header"><div><h1 class="bb-page-title">…</h1><p>…</p></div>[<a class="btn …">CTA</a>]</div>` copy-pasted across 37 files.
- **Issues:** The `<div><h1><p>` left-side scaffold is duplicated verbatim every time. Some headers (`getting_started.templ:17`) attach an inline icon directly to the title; others use a subtitle paragraph with slightly different `mt-1` vs `mt-0.5` spacing.
- **Recommendation:** Extract `PageHeader(props PageHeaderProps)` templ component (`Title`, `Subtitle`, `Icon`, `Action templ.Component`) — replaces ~150 lines of duplication and forces consistent subtitle margin.

### 2. Sectioned form card (`bb-card p-0 overflow-hidden`)

- **What it is:** The canonical create/edit form pattern: icon header + form body + bottom action row.
- **Where used:** `pages/create_login.templ:43`, `pages/user_form.templ` (multiple), `pages/category_form.templ`, `pages/tag_form.templ`, `pages/rule_form.templ`, `pages/my_account.templ:169`/`:234`, `pages/agents_settings.templ` (3 instances).
- **Variants in use:** The full pattern (icon-header + bb-action-row) appears 6 times; loose `bb-card p-0 overflow-hidden` (without the action row) appears 22 more — sectioned panels in `connection_detail.templ`, `settings.templ`, `categories.templ`.
- **DaisyUI native equivalent:** None.
- **Current implementation:** Hand-rolled wrapper + inline `<div class="bb-icon-header">…</div>` + manual `<div class="bb-action-row">`.
- **Issues:** Every caller re-writes the icon-header + action-row scaffold. `bb-action-row` is documented but only used 9 times — most pages still hand-roll the bottom row (`pages/agents_settings.templ:262` uses `mt-4 flex justify-end gap-2`; `connection_detail.templ:51` does its own thing).
- **Recommendation:** Extract `FormCard(IconHeaderProps, body templ.Component, ActionRowProps)` templ component — would cover ~80% of the form pages.

### 3. Icon-header tile (`.bb-icon-header__tile`)

- **What it is:** 40×40 rounded colored square hosting a Lucide icon at the top of form cards or as a row indicator.
- **Where used:** 51 occurrences. Spec'd variants: `bb-icon-tile--primary|success|warning|error`.
- **Variants in use:** 4 spec'd modifiers + **at least 6 ad-hoc colour combos that bypass the modifier classes**: `bg-info/8 text-info` (`providers.templ:46`), `bg-warning/8 text-warning` (`providers.templ:346`, `csv_import.templ:246`), `bg-success/8 text-success` (`providers.templ:187`), `bg-primary/8 text-primary` (`csv_import.templ:75`, `access.templ:221`), `bg-warning/10` (no text color, in `rules.templ:241`). Plus a separate strand of `w-10 h-10 rounded-xl bg-X/8` panels not using `bb-icon-header__tile` at all (15 sites in `providers.templ`, `csv_import.templ`, `agents_settings.templ`, `mcp_settings.templ`).
- **DaisyUI native equivalent:** None.
- **Current implementation:** Mixed — half use the modifier class, half use raw Tailwind tile geometry with ad-hoc tints.
- **Issues:** Spec says `bg-primary/8` and `bg-success/10` (different opacities) which makes the spec internally inconsistent. The 15 ad-hoc `w-10 h-10 rounded-xl bg-X/8` sites visually match `bb-icon-tile--*` but skip the abstraction.
- **Recommendation:** Pick one alpha (`/10` is the majority in spec table), update `bb-icon-tile--*` to match, and migrate ad-hoc sites. Also add `info` and `neutral` tones.

### 4. Empty state card

- **What it is:** "No X yet" centered card with icon-in-tinted-square + heading + body + primary CTA.
- **Where used:** 18 distinct sites; e.g. `pages/access.templ:71` and `:202` (essentially identical 14-line blocks for OAuth Clients / API Keys), `pages/connections.templ:138` and `:163`, `pages/categories.templ:227`, `pages/tags.templ:131`, `pages/users.templ:259`, `pages/account_link_detail.templ:171`, `pages/rules.templ:84`, `pages/feed.templ:258` / `:276` / `:296` / `:319` (4 variants), `pages/logs.templ:364` / `:604` / `:683`, `pages/connection_detail.templ:151` and `:454`.
- **Variants in use:** 5 visual variants — `bb-card p-12 text-center` (canonical), `bb-card p-8 sm:p-12 text-center` (access.templ — extra mobile padding), `bb-card p-10 sm:p-12 text-center` (feed.templ), `bb-card` + `flex flex-col items-center text-center py-16 px-6` (categories, connections), and compact "no results with filters" inline blocks (`account_detail.templ:402`, `transactions.templ:466`).
- **DaisyUI native equivalent:** None.
- **Current implementation:** Copy-pasted 12-20 line scaffold; each caller hand-codes icon size (`w-7 h-7` vs `w-8 h-8` vs `w-10 h-10`) and tile geometry (`w-14 h-14` vs `w-16 h-16`).
- **Issues:** Multiple sizing conventions, multiple padding conventions, identical content shape. `access.templ` literally duplicates the OAuth-Clients block as the API-Keys block (lines 71-84 ≈ 202-215, lines 88-131 ≈ 219-265). This is the single largest extraction win in the codebase.
- **Recommendation:** Extract `EmptyState(props EmptyStateProps)` templ — `Icon`, `Title`, `Body`, `CTA templ.Component`, optional `Compact bool` for the "no results" variant. Migrate all 18 sites. Saves ~250 lines.

### 5. Resource-list rows (active / revoked pattern)

- **What it is:** A `bb-card divide-y divide-base-300` containing one row per item (avatar + name + metadata + dropdown / inline-confirm action), optionally with a collapsed "N revoked" disclosure below it.
- **Where used:** `pages/access.templ:46-67` (OAuth Clients) and `:178-199` (API Keys) — visually identical patterns with parameterised icon/label/badge. Also `pages/users.templ:179` (accounts inside member card), `pages/rule_detail.templ:327` and `:406` (applications), `pages/account_link_detail.templ:36` (accounts).
- **Variants in use:** Two — "active row with dropdown actions" and "revoked row with line-through + opacity-60".
- **DaisyUI native equivalent:** None (DaisyUI's `list` is too plain).
- **Current implementation:** `access.templ` has two 78-line blocks (lines 22-149, 152-283) that differ only in icon name, plural label, badge tone, and dropdown action URL.
- **Issues:** `access.templ` is the canonical example of pure duplication — the two sections are 99% identical. The collapsed-revoked disclosure pattern (`x-data="{ open: false }"` + `x-collapse`) is repeated verbatim.
- **Recommendation:** Extract `ResourceListSection(props)` templ — covers active + revoked + collapsed-revoked + empty-state branches. Apply to `access.templ` first (saves ~150 lines) then propagate to anywhere that lists "things with a status, that can be revoked or archived".

### 6. Overflow action menu

- **What it is:** Ellipsis-vertical icon button that opens a small dropdown of actions (edit, manage, delete).
- **Where used:** 6 sites — `pages/rules.templ:185`, `pages/tags.templ:102`, `pages/access.templ:109` / `:243`, `pages/users.templ:155`, `pages/categories.templ:199`.
- **Variants in use:** Two icon sizes — `w-4 h-4` (5 sites) and `w-5 h-5` (`users.templ:157` only). Two button shells — `btn-xs btn-square rounded-lg` (4 sites) and `btn-sm btn-square rounded-lg opacity-40` (`users.templ:156`).
- **DaisyUI native equivalent:** `dropdown` + `menu` (used here).
- **Current implementation:** Hand-rolled with `dropdown dropdown-end` + `dropdown-content menu …`. Width varies (`w-40`, `w-44`).
- **Issues:** `users.templ:157` uses a larger icon than the rest. Spec says `w-3.5 h-3.5` icons inside dropdown rows but several use `w-3 h-3`.
- **Recommendation:** Extract `OverflowMenu(items []OverflowMenuItem)` templ — pins the icon-button geometry, menu width, and item icon size. Reduces 6 ~12-line blocks.

### 7. Inline-confirm destructive action

- **What it is:** Click ellipsis → "Revoke…" → menu collapses → inline "Confirm / Cancel" buttons appear in place of the menu.
- **Where used:** `pages/access.templ:108-129` and `:242-263` (identical except for endpoint URL); also `pages/connection_detail.templ:135`, `pages/categories.templ` (modal-based variant).
- **Variants in use:** Inline buttons in a `<span>` (access) vs modal `<dialog>` (categories).
- **DaisyUI native equivalent:** None.
- **Current implementation:** `x-data="{ confirming: false }"` paired with `x-show="!confirming"` and `x-show="confirming"`.
- **Issues:** State machine pattern is duplicated; some pages have `@click="document.activeElement.blur()"` to close the dropdown, others don't.
- **Recommendation:** Add a shared `confirmInline` Alpine factory + a templ wrapper. Or unify on the existing `bb-confirm-*` overlay (in `base.html`) so destructive confirms are always the modal.

### 8. Buttons

- **What it is:** Standard primary/ghost/error/outline buttons with size variants and rounded corners.
- **Where used:** 176 button instances across the templ pages.
- **Variants in use:** 28+ distinct class combinations seen in greps. Common patterns: `btn btn-primary btn-sm rounded-xl`, `btn btn-ghost btn-sm rounded-xl`, `btn btn-ghost btn-sm btn-square rounded-xl`, `btn btn-error btn-sm rounded-xl`, `btn btn-error btn-xs btn-soft rounded-lg`, `btn btn-ghost btn-xs rounded-lg`, `btn btn-ghost btn-xs btn-square rounded-lg`, plus oddballs: `btn btn-primary btn-sm btn-soft rounded-xl` (`access.templ:40`, `:171`), `btn btn-primary btn-sm rounded-xl gap-2 text-xs relative overflow-hidden`, `btn btn-error btn-sm btn-outline rounded-xl` (`my_account.templ:258`).
- **DaisyUI native equivalent:** `btn` (used).
- **Current implementation:** Spec-compliant for the most part. Spec calls for `btn-sm` → `rounded-xl` and `btn-xs` → `rounded-lg` (and `gap-2` vs `gap-1.5`). Most callers obey this.
- **Issues:** (a) Inline `text-xs` overrides on `btn-sm` (`pages/mcp_settings.templ`); (b) `min-w-32` is sometimes added for jitter-prevention, sometimes not; (c) icon size inside `btn-sm` is mostly `w-4 h-4` but some use `w-3.5 h-3.5` (spec says `w-3.5 h-3.5` on primary submit, `w-4 h-4` everywhere else — internally contradictory).
- **Recommendation:** Either tighten the spec (one icon size per button size, no exceptions) **or** introduce a `Button(variant, size, icon, label)` templ helper for the common 6 shapes. The spec table at `docs/design-system.md:131-141` looks complete but enforcement is weak; a helper would prevent drift.

### 9. Badges

- **What it is:** Tag-like status / count chips.
- **Where used:** 186 badge instances; 46 distinct class combinations.
- **Variants in use:** Spec says three shapes — `badge-soft badge-{color} badge-sm` (status), `badge-ghost badge-xs` (metadata), `badge-{color} badge-xs` (counts). In practice: many sizes mixed (`badge-xs` and `badge-sm` for similar use), several `gap-1` suffixes, `tabular-nums` extras, and `uppercase tracking-wide` for some.
- **DaisyUI native equivalent:** `badge` (used).
- **Current implementation:** Direct DaisyUI classes; only 9 sites call the `StatusBadge()` / `SyncBadge()` Go helpers. Hand-rolled "rounded-full bg-X/10 text-X" chips (`connection_detail.templ:78` `:225`, `reports.templ:91-94`, `report_detail.templ:52-57`) bypass the badge system entirely.
- **Issues:** (a) Hand-rolled rounded chips diverge in spacing (`px-1.5 py-0.5` vs `px-2 py-0.5`); (b) `report_detail.templ:52` uses `bg-error/10 text-error/90` (off-spec tone); (c) `StatusBadge` is only 9 callers when many more sites render connection/sync status from scratch.
- **Recommendation:** (a) Make `StatusBadge()` and `SyncBadge()` the only legitimate sources of those badges (lint or audit). (b) Add `CountBadge(n, tone)` and `MetaBadge(label)` helpers. (c) Migrate the 6+ hand-rolled rounded-full chips into the badge system or a new `bb-pill` primitive.

### 10. Ad-hoc colored icon panels

- **What it is:** A 40×40 (or 32×32, 36×36, 56×56, 64×64) rounded colored square holding a Lucide icon — used as a section/card marker, an empty-state hero icon, an inline "row indicator", a status-flag tile.
- **Where used:** 30+ ad-hoc sites. `pages/providers.templ:46`, `:187`, `:346`; `pages/agents_settings.templ:43`, `:84`, `:121`, `:154`, `:214`; `pages/mcp_settings.templ:150`, `:215`; `pages/csv_import.templ:75`, `:219`, `:246`, `:283`; `pages/rules.templ:86`, `:241`, `:245`, `:249`, `:253`; `pages/access.templ:73`, `:204`, `:221`, `:269`; `pages/users.templ:212`; `pages/connections.templ:140`, `:165`, `:193`, `:210`; `pages/categories.templ:230`, `:254`; `pages/sync_log_detail.templ:223`.
- **Variants in use:** 6 sizes (`w-7 h-7`, `w-8 h-8`, `w-9 h-9`, `w-10 h-10`, `w-12 h-12`, `w-14 h-14`, `w-16 h-16`) and 6 tones (`bg-base-200/40`, `bg-base-200/60`, `bg-primary/10`, `bg-info/10`, `bg-warning/10`, `bg-success/10`, `bg-error/10`) + the `/8` variants in some pages. Almost no consistency.
- **DaisyUI native equivalent:** None.
- **Current implementation:** Raw Tailwind utilities.
- **Issues:** Size + tone combinations are accidental. Spec defines `bb-icon-tile--*` for the 40×40 case but doesn't cover the others. The 56×56 / 64×64 sizes show up in empty states, the 32×32 / 36×36 sizes in list rows.
- **Recommendation:** Generalize `bb-icon-tile` with size modifiers (`--xs|--sm|--md|--lg`) and tone modifiers (extend with `info`, `neutral`). Migrate all 30+ sites. Big readability win.

### 11. Confirmation patterns (modal vs inline vs overlay)

- **What it is:** Three different mechanisms for confirming a destructive action.
- **Where used:** (1) Inline-confirm (`x-data="{ confirming: false }"`): `access.templ`, `connection_detail.templ`. (2) DaisyUI `<dialog class="modal">`: `categories.templ:246` (delete category), `connections.templ:461` (create link). (3) Global `.bb-confirm-*` overlay in `base.html`: `bbConfirm()` JS function fired from elsewhere.
- **Variants in use:** 3 incompatible patterns side-by-side.
- **DaisyUI native equivalent:** `modal`.
- **Current implementation:** Mix of three patterns.
- **Issues:** No consistent UX for "are you sure?" interactions. Destructive deletes go through three different paths depending on the page.
- **Recommendation:** Standardize on the `bb-confirm-*` overlay (richest UX, supports `danger` vs `warning` variants). Migrate the two `<dialog>` modals + the four inline-confirm sites.

### 12. Form-input primitive (`.bb-form-input`, `.bb-form-select`)

- **What it is:** App-styled input/select with bg-base-200/50 surface that clears on focus.
- **Where used:** `bb-form-input` in 18 sites (in 8 files: `my_account`, `category_form`, `rule_form`, `tag_form`, `user_form`, `create_login`). `bb-form-select` in 2 sites (`create_login`, `rule_form`).
- **Variants in use:** 1 each.
- **DaisyUI native equivalent:** `input input-bordered`.
- **Current implementation:** `@apply input w-full rounded-xl bg-base-200/50 focus:bg-base-100 transition-colors`.
- **Issues:** Adopted in newer form pages (post-shared-form-card refactor) but ~30 other form pages still ship `input input-bordered w-full rounded-xl bg-base-200/50` raw. Same shape, different class.
- **Recommendation:** Migrate the rest of the form pages to `.bb-form-input` and `.bb-form-select`. Codify a templ helper `FormInput(props)` / `FormSelect(props)` to enforce one shape per element.

### 13. `<select>` with `bg-base-200/50` (spec violation)

- **What it is:** The footgun documented in `.claude/rules/ui.md` — alpha on `<select>` renders fully transparent.
- **Where used:** **One offender** — `pages/category_form.templ:107`: `<button type="button" class="select select-bordered w-full text-left flex items-center gap-2 rounded-xl bg-base-200/50" …>`. Technically a `<button>` styled as a select, so the bug doesn't fire, but the spec violation is visible.
- **DaisyUI native equivalent:** N/A.
- **Issues:** The class string matches a `<select>` enough that a future agent might copy it onto a real `<select>`.
- **Recommendation:** Replace with solid `bg-base-200`. Trivial.

### 14. Inline `style=""` attributes

- **What it is:** Raw `style=""` attributes for cases CSS classes can't reach.
- **Where used:** 36 occurrences total. Legitimate use cases (15): dynamic per-row category colors (`tx_row.templ:197`, `account_detail.templ:290`, `category_form.templ:49-51`, `tag_form.templ:60`) and dynamic SVG bar heights (`connection_detail.templ:241-261`). Borderline (12): drag-state styling in `prompt_builder.templ:124-206`. Removable (9): `base.html:465` (`opacity:0.6`), `base.html:500/571/708/2330` (`font-size:0.6rem`), `base.html:725/731/737` (`pointer-events:none`), `tx_results.templ:156` (`display:none`).
- **DaisyUI native equivalent:** N/A.
- **Issues:** The "removable" ones could be utility classes. The dynamic ones are inherent to runtime color binding.
- **Recommendation:** Convert the 9 removable ones to utility classes. Leave the rest.

### 15. Translucent surface patterns (`bg-base-200/{20-50}`)

- **What it is:** Subtle grey wash used as background for read-only inputs, info panels, action-row footers.
- **Where used:** 82 occurrences. Common shape: `bg-base-200/50`, `/40`, `/30`, `/20`. Examples: `rule_form.templ:96/119/169/260/281`, `csv_import.templ:84/165/179/227`, `account_link_detail.templ`, `condition_row.templ:21`.
- **DaisyUI native equivalent:** None.
- **Current implementation:** Inline Tailwind utilities scattered across pages.
- **Issues:** The opacity step varies (20 / 30 / 40 / 50 / 60) without justification — visually they all look like "subtle grey".
- **Recommendation:** Define `bb-surface-muted` (one tone) + `bb-surface-subtle` (lighter) — two named tones max. Migrate.

### 16. Translucent borders for danger / warning surfaces

- **What it is:** `border-error/20|30|40`, `bg-error/[0.02]|5|10` used on warning panels and danger-zone cards.
- **Where used:** 15+ sites. `my_account.templ:246` (`bb-card border-error/20`), `pages/feed.templ:132` (`alert alert-warning`), `connections.templ:300` (`text-warning bg-warning/10`), `connection_detail.templ:30` (`bg-error/10`), `settings.templ:221/462` (`text-error/80 bg-error/5`).
- **DaisyUI native equivalent:** `alert alert-warning`, `bb-danger-card`.
- **Current implementation:** Mixed — some use `bb-danger-card`, others use `alert alert-warning rounded-xl`, others go fully ad-hoc.
- **Issues:** Three different patterns for "this is a warning surface". `bb-danger-card` is defined in spec but only `create_login.templ:130` uses it.
- **Recommendation:** Pick one — `bb-danger-card` for in-card destructive zones, `alert alert-{tone}` for floating banners. Migrate the rest.

### 17. Stat-tile cards (4-up dashboards)

- **What it is:** Small bb-cards rendered in a 2x2 / 4x1 grid with a colored tile + big tabular-nums value + small label.
- **Where used:** `pages/rule_detail.templ:247-307` (4-tile rule stats), `pages/logs.templ:140-160` (similar), `pages/connection_detail.templ:215-225` (4-up stat row), `pages/getting_started.templ:34-56`.
- **Variants in use:** Different sizes (`w-9 h-9` vs `w-10 h-10`), different value typography (`text-2xl font-semibold leading-none` vs `text-xl font-bold`).
- **DaisyUI native equivalent:** `stat` / `stats` (not used — DaisyUI's defaults don't match the design).
- **Current implementation:** Hand-rolled card + flex row.
- **Issues:** Repeated 4-tile dashboards differ in tile geometry and label position.
- **Recommendation:** Either adopt DaisyUI `stat` with custom theming, or extract a `StatTile(icon, tone, value, label)` templ component.

### 18. Section header (icon + h2/h3 + counter)

- **What it is:** `<i data-lucide="…"> <h2>Heading</h2> <span class="text-xs text-base-content/40">N items</span>` — appears above lists.
- **Where used:** `access.templ:25-44` (twice, OAuth + API Keys), `rule_detail.templ` (several), `users.templ:30` flash-alert variant, `connection_detail.templ` (multiple), `categories.templ`.
- **Variants in use:** Heading element size (`text-sm` vs `text-lg`), icon size (`w-4 h-4` vs `w-5 h-5`), counter formatting.
- **DaisyUI native equivalent:** None.
- **Current implementation:** Hand-rolled flex row.
- **Issues:** Different sites use different heading levels and counter formats.
- **Recommendation:** Extract `SectionHeader(icon, title, count, action)` templ component.

### 19. Copy-to-clipboard pattern

- **What it is:** Readonly input + clipboard-icon button that swaps to a checkmark for 2s.
- **Where used:** `api_key_created.templ:30-39` (1 instance), `oauth_client_created.templ:40-79` (3 instances), `mcp_guide.templ:31/147/169/177/214/247` (6 instances), `user_form.templ:66`, `create_login.templ:110`, `agents_settings.templ:239` (1-off).
- **Variants in use:** Inline-Alpine (`navigator.clipboard.writeText…`) and shared-helper (`copyAndFlash`) — two patterns.
- **DaisyUI native equivalent:** None.
- **Current implementation:** Inline `@click` handlers ~80 chars long, plus a `copyAndFlash(content, $data)` helper in `mcp_guide.js`.
- **Issues:** Two patterns for the same UI affordance.
- **Recommendation:** Unify around `copyAndFlash` and extract a `CopyableField(label, value)` templ component.

### 20. Activity timeline primitives

- **What it is:** GitHub-style activity timeline used on transaction-detail, feed, sync logs.
- **Where used:** `feed.templ:188`, `transaction_detail.templ:610`, `sync_log_detail.templ`.
- **Variants in use:** Two — card mode (default) and prominent mode (used on `/feed` and `/transactions/{id}`).
- **DaisyUI native equivalent:** None.
- **Current implementation:** Well-organized templ component family (`Timeline`, `TimelineDay`, `TimelineSystemRow`, `TimelineCommentRow`, `TimelineEmpty`).
- **Issues:** None functional. The `.bb-timeline-prominent` CSS overrides (lines 2881-3174 of input.css) are 300+ lines of `:has()`-based selectors riding on Tailwind utility class names (`w-6.h-6`, `leading-6`, `flex-1.text-sm.leading-6`). Brittle to any markup change inside the timeline primitives. Spec calls this out (input.css:822-826) but the brittleness remains.
- **Recommendation:** Refactor the prominent-mode overrides to use stable data attributes (`data-timeline-tile`, `data-timeline-body`) instead of `:has()` + utility-class selectors. Reduces future-breakage risk.

### 21. Spacing / divider patterns

- **What it is:** Bottom-border separator between sections inside a card.
- **Where used:** `border-t border-base-300/50` + `border-base-300/40` + `border-base-300/60` appear in multiple files. Spec calls for `/50`.
- **Variants in use:** 4 opacity values (`/40`, `/50`, `/60`, default `border-base-300`).
- **Issues:** Three values for what should be one divider.
- **Recommendation:** Canonicalize on `/50` (spec) and ban the rest. Or define `.bb-divider` utility.

### 22. Dead / orphaned CSS classes

- **What it is:** Classes defined in input.css with zero or near-zero templ callers.
- **Where used:** `.bb-skeleton*` (~110 lines, lines 2402-2515, 0 callers); `.bb-summary-pill` + `.bb-sparkline` + `.bb-health-ring` (0 templ callers); `.bb-rule-cat-badge`, `.bb-rule-creator-badge` (0 templ callers); `.bb-triage-*` (~225 lines, lines 2577-2818, no direct templ callers — possibly rendered by `reviews.js`); `.bb-page-skeleton`, `.bb-loading-fade` (0 callers).
- **Recommendation:** Retire entirely unless you can prove a JS-side render path. Saves ~400 lines of CSS.

### 23. Single-caller `bb-*` primitives

- **What it is:** Classes meeting the "appears 3+ times" threshold the spec mandates but with only 1-2 sites.
- **Where used:** `.bb-filter-toggle` + `.bb-filter-form` (1 caller — `logs.templ:168/184`); `.bb-filter-bar` (2 callers); `.bb-table-header` (2 callers); `.bb-view-toggle*` (1 caller — `transactions.templ:103-108`); `.bb-paginator*` (1 caller — `tx_results.templ:91-130` for ~30 lines); `.bb-rule-creator-badge--user` / `--agent` (0 templ callers).
- **Recommendation:** Inline back to utilities, or commit to broader adoption. The spec's "3+ uses" rule is being violated by the spec itself.

### 24. Modal padding / safe-area pattern

- **What it is:** `[padding-bottom:calc(1.5rem+env(safe-area-inset-bottom,0px))] sm:[padding-bottom:1.5rem]` — iOS safe-area bottom padding for mobile modal.
- **Where used:** 3 modal sites (`categories.templ:252`, `connections.templ:462`). All identical.
- **Recommendation:** Extract as `bb-modal-box` class. Trivial.

### 25. Inline `<header>` inside cards

- **What it is:** Inline header bar — `<header class="flex items-center gap-2 pb-4 mb-4 border-b border-base-300/60">…</header>`.
- **Where used:** `timeline.templ:109/156`, `feed.templ:49`, `transaction_detail.templ:346`.
- **Variants in use:** 4 — different padding / margin / border opacity each time.
- **Recommendation:** Pick one — probably the canonical `px-4 sm:px-5 py-3` from the sectioned-card spec.

## Cross-cutting findings

### Hackiness inventory

- **`!important`** appears 13 times in input.css. Most are intentional (Tailwind utility shadowing): `.bb-tx-checkbox` display rules (lines 1110, 1126, 1138), `.bb-tx-date` visibility (lines 1378-1382), button `transform: scale()` (`btn` :active line 998), select transition (line 1017), modal-backdrop (1050), x-collapse (1304), drawer-side z-index (1310), triage-row focused background (2593, 2604), progress-bar `--done` (2340). All defensible; document the "fights utilities" rationale once and stop adding.
- **Raw `style=""` attributes** — 36 total. Removable: 9 (see component #14). Necessary: 27 (dynamic colors, dynamic SVG dimensions).
- **`opacity` modifiers on surfaces** (e.g. `bg-base-200/50`) — 82 sites. The footgun is documented for `<select>` but the broader proliferation across 5 opacity steps (20/30/40/50/60) is uncontrolled.
- **Duplicated 50+ line blocks:**
  - `access.templ:22-149` ≡ `access.templ:152-283` (OAuth Clients ≡ API Keys, 130 lines each).
  - `oauth_client_created.templ:39-61` (Client ID / Client Secret blocks, identical except labels).
  - `feed.templ:258-330` (4 empty-state variants, near-identical scaffolds).
  - `agents_settings.templ:39-180` (4 textarea cards with identical icon-header + textarea + counter shape).
  - `settings.templ:15-100` (4 successive `bb-page-header` blocks).
- **Hand-rolled rounded-full chips** that bypass the badge system (6+ sites): `connection_detail.templ:78`, `:225`, `reports.templ:91-94`, `report_detail.templ:52-57`. Different sizes, different paddings.
- **Inline tooltip without DaisyUI `tooltip`** — `connections.templ:227-235` ("Sync Error" tooltip via `group/err` + `group-hover/err:block`), `connection_detail.templ:266-272` (day-bar tooltip).

### Off-spec drift

- **`bg-primary/8` vs spec `bg-primary/10`** — spec table at `docs/design-system.md:194-196` says `bg-primary/8 text-primary` but lines 195-197 say `bg-success/10`, `bg-warning/10`, `bg-error/10`. The spec itself is inconsistent. Actual usage: `/8` is used 6 times for tiles, `/10` 12 times. **The spec is the source of confusion.**
- **`bg-info/8` and `bg-success/8`** — not in spec at all; introduced via `connection_detail_helpers.go:85-91` and `csv_import.templ`. Should match the spec or extend it.
- **Spec calls for `rounded-xl` on `btn-sm` and `rounded-lg` on `btn-xs`** — fully obeyed except `oauth_client_created.templ:44/55/74` use `btn-sm rounded-xl` and look fine, while `tags.templ:78` mixes `rounded` (default) for a code chip.
- **Spec calls for `gap-2` on `btn-sm` icon+text, `gap-1.5` on `btn-xs`** — partially obeyed; many sites use `gap-1.5` on `btn-sm`.
- **Spec forbids `rounded-lg`/`rounded-xl` on badges** — looks like nobody violates this currently.
- **`p-8` on form-card top section** — spec at `docs/design-system.md:489` explicitly says "do **not** use `p-8`". Actual offenders: `connection_new.templ` (`bb-card p-8 max-w-lg`), `oauth_authorize.templ`, `errors.templ`, `getting_started.templ`. Centered auth-form variants — possibly intentional but the spec says no.
- **`bb-card max-w-md w-full p-6 bg-base-100 shadow-xl rounded-2xl`** (`feed.templ`) — combines `bb-card` (which already paints `bg-base-100 rounded-xl border border-base-300`) with explicit `shadow-xl` and `rounded-2xl` — three conflicts in one selector.
- **`shadow-xl` ring sometimes added to cards** — spec says cards are border-based, not shadow-based. Drift sites: `feed.templ`, `oauth_authorize.templ`.

### Missing primitives (3+ repetitions, no shared abstraction)

Ranked by call count, highest-impact first:

1. **`EmptyState`** — 18 callers, 5 visual variants. Single biggest extraction win.
2. **`PageHeader`** — 37 callers. Easy extract, kills `<div><h1><p>` boilerplate.
3. **`SectionHeader` (icon + title + count + action)** — 6+ callers.
4. **`StatTile` (icon + value + label)** — 4+ callers, all subtly different.
5. **`OverflowMenu` (kebab dropdown)** — 6 callers.
6. **`FormCard` (icon-header + body + action-row scaffold)** — 6-10 callers depending on definition.
7. **`ResourceListSection` (active + revoked + empty)** — 4+ callers, `access.templ` is 99% duplicated.
8. **`CopyableField`** — 10+ callers, two competing implementations.
9. **`ConfirmInline` Alpine helper** — 4 callers, 3 different state machines.
10. **`Badge` Go helper (status, count, meta)** — there are already `StatusBadge`/`SyncBadge` but they cover only 9 of the 186 badge sites.

### Over-abstracted (`bb-*` used only once or twice)

Candidates for retirement if not promoted:

- `.bb-skeleton*` family (0 callers, 110 lines)
- `.bb-summary-pill`, `.bb-sparkline`, `.bb-health-ring` (0 callers)
- `.bb-rule-cat-badge`, `.bb-rule-creator-badge`, `.bb-rule-creator-badge--agent|--user` (0 templ callers)
- `.bb-page-skeleton`, `.bb-loading-fade` (0 callers)
- `.bb-view-toggle*` (1 caller — transactions)
- `.bb-filter-toggle`, `.bb-filter-form` (1 caller — logs)
- `.bb-paginator*` (1 caller — tx_results)
- `.bb-table-header` (2 callers)
- `.bb-filter-bar` (2 callers)
- `.bb-triage-*` (no templ callers; possibly used by `/reviews` JS render — verify before retiring)

If retained, the spec's "appears 3+ times" rule should be amended to acknowledge that some single-caller `bb-*` classes exist because the surface is unique (e.g. one paginator, one command palette).

### Specific bugs to call out

- **`pages/category_form.templ:107`** — `<button class="select select-bordered … bg-base-200/50">`. The styled-as-select button doesn't trigger the `<select>` opacity bug but the class string is identical to the documented footgun.
- **`pages/feed.templ:296`** — `class="bb-card max-w-md w-full p-6 bg-base-100 shadow-xl rounded-2xl"` — five class conflicts/redundancies in one selector (bb-card already sets bg + rounding; `shadow-xl` violates the "border not shadow" spec; `rounded-2xl` overrides `rounded-xl`).
- **`.bb-form-error`** (input.css:299) — class defined, zero callers. Either remove or migrate inline `bg-error/10 text-error px-4 py-3` blocks (`csv_import.templ:108`) onto it.
- **`access.templ:71` vs `:202`** — `bb-card p-8 sm:p-12 text-center` is used twice but the canonical empty-state padding in spec is `p-12 text-center` only.
- **`pages/agent_wizard.templ:120`** — `bg-base-300/50 ... group-hover:bg-base-300/70` — base-300 with opacity for an interactive surface; uncommon and likely off-tone.
- **`base.html:465`** — inline `style="opacity:0.6"` on a `.bb-shortcuts-label` — should be a class.
- **`base.html:500/571/708`** — `style="font-size:0.6rem"` inline — `text-[0.6rem]` would work.
- **Tooltip overrides** — `.bb-tag.tooltip` rule at the very bottom of input.css (line 2877) is unlayered to defeat DaisyUI's deeper sub-layer; a one-off but flagged because it's load-bearing — see comment.

## Priorities (top 10)

1. **Extract `EmptyState` templ component.** (M) Replaces 18 sites of duplicated 12-20 line scaffolds with one component. Forces consistent padding/sizing. Net saving ~250 lines.
2. **Extract `PageHeader` templ component.** (S) 37 callers; trivially mechanical. Kills `<div><h1><p>` boilerplate.
3. **Fold `access.templ` OAuth + API Keys into a `ResourceListSection` component.** (M) The two sections are 99% identical (130 lines each). Apply to any "things with status + dropdown actions + revoked-collapsed list" page. ~150 lines saved on access.templ alone.
4. **Retire dead CSS classes.** (S) `bb-skeleton*`, `bb-summary-pill`, `bb-sparkline`, `bb-health-ring`, `bb-rule-cat-badge`, `bb-rule-creator-badge`, `bb-page-skeleton`, `bb-loading-fade`. ~400 lines from input.css. Validate `bb-triage-*` against `reviews.js` first.
5. **Fix spec self-contradictions in `bb-icon-tile--*` alpha values.** (S) Pick `/10` (majority), update CSS modifier classes to match, migrate the `/8` strays. While there, extend with `info` and `neutral` modifiers and add size variants for the 6 different ad-hoc tile sizes (30+ sites). One spec change, many migrations.
6. **Extract `OverflowMenu` templ + `SectionHeader` templ.** (S) 6 + 6 callers respectively. Mechanical wins, normalizes icon size + width inside dropdown rows.
7. **Standardize destructive-confirm UX.** (M) Pick one of (inline-confirm, `<dialog>` modal, `bb-confirm-*` overlay) and migrate the other two. Current state has 3 incompatible patterns. Recommend `bb-confirm-*` overlay because it already exists in `base.html` with full keyboard + a11y support.
8. **Migrate hand-rolled rounded-full chips into the badge system.** (S) 6+ sites that bypass `badge badge-*`. While there, audit the `StatusBadge()`/`SyncBadge()` Go helpers and require their use everywhere a connection-status or sync-status pill is rendered (currently 9 of ~37 sites use the helper).
9. **Consolidate translucent surface tones.** (M) Define `bb-surface-muted` + `bb-surface-subtle` and replace `bg-base-200/{20,30,40,50,60}` proliferation (82 sites). Define `bb-divider` for `border-base-300/{40,50,60}`. Disambiguates the visual hierarchy.
10. **De-brittle the prominent-mode timeline overrides.** (L) The 300-line `:has()` + utility-class selector block in input.css:2881-3174 will break the next time someone changes a Tailwind class inside the timeline primitives. Add stable `data-timeline-*` attributes on the primitives and rewrite the overrides against those.

Effort key: S ≤ 1 day, M ≤ 3 days, L ≤ 1 week.

# DaisyUI 5 Coverage Audit

**Date:** 2026-05-24. **Scope:** v1 admin UI only — `internal/templates/components/**/*.templ` (52 templ pages + 12 shared), `internal/templates/layout/base.html` (the app shell, 2.4k lines), `internal/templates/partials/*.html` (5 thin bridges to templ), `input.css` (3.2k lines, ~190 `bb-*` classes), and Alpine factories in `static/js/admin/components/`. The v2 React SPA under `web/` is explicitly out of scope.

**Methodology:** Fetched daisyUI 5's canonical component list from `daisyui.com/llms.txt`, then `grep`-counted every component + modifier across the templ tree, cross-referenced against the `bb-*` class inventory in `input.css`, and read every shared primitive (`nav.templ`, `breadcrumb.templ`, `kbd.templ`, `flash.templ`, `tag_chip.templ`, `timeline.templ`, `base.html`) plus a sample of feature pages (`transactions.templ`, `mcp_guide.templ`, `csv_import.templ`, `connections.templ`, `categories.templ`, `agents.templ`).

**Headline numbers.** `btn` (387 `btn-sm`, 280 `btn-ghost`, 196 `btn-primary`) and `badge` (116 `badge-xs`, 98 `badge-soft`) are the workhorses — both used natively with consistent conventions. Daisy `card` is used **2x** (only on `/agents` and `/agents/sessions/{id}`); everywhere else (82 templ files) uses the hand-rolled `.bb-card`. Daisy `stat`, `breadcrumbs`, `pagination`, `collapse`, `accordion`, `tabs`, `timeline`, `kbd`, `chat`, `hero`, `divider`, `indicator`, `mockup`, `avatar`, `skeleton`, `radial-progress` — **all hand-rolled or absent**. The repo has ~190 `bb-*` classes, of which roughly 35 duplicate something daisy ships.

---

## Coverage matrix

Legend: ✅ used natively · ⚠️ used with significant overrides · 🔁 hand-rolled equivalent in `bb-*` · 🚫 N/A · ❌ should be used but isn't

### Actions
| Category | Component | Status | Our usage / hand-rolled equivalent | Action |
|---|---|---|---|---|
| Actions | button | ✅ | 448 templ matches; `.btn` polish + `transition` + `:active scale(0.97)` in `input.css:990-998`. Convention enforced (`rounded-xl` on `btn-sm`, `rounded-lg` on `btn-xs`) in `docs/design-system.md`. | keep |
| Actions | FAB | 🚫 | Not used; not appropriate for desktop admin. | none |
| Actions | swap | 🚫 | Not used; `bb-tx-checkbox` is imperatively toggled, not via swap. | none |

### Data display
| Category | Component | Status | Our usage / hand-rolled equivalent | Action |
|---|---|---|---|---|
| Data display | badge | ⚠️ | 214 matches, heavy use of `badge-soft badge-{color} badge-{size}`. But 6+ bespoke badge clones in `input.css`: `.bb-tag` (`:515`), `.bb-rule-cat-badge` (`:1262`), `.bb-rule-creator-badge` (`:1281`), `.bb-nav-badge` (`:474`), `.bb-triage-badge` (`:2632`), `.bb-tagpicker-diff-pill` (`:2292`). | consolidate (see deep-dive) |
| Data display | card | 🔁 | Daisy `card` used 2x only (`agents.templ`, `session_detail.templ`). Everywhere else: `.bb-card` (`input.css:244`) — `bg-base-100 rounded-xl border border-base-300` with dark-mode color-mix lift. 82 templ files import. | keep `bb-card` (extends daisy with a needed dark lift), but document divergence (see deep-dive) |
| Data display | chat | 🔁 | Hand-rolled `.bb-comment-bubble` (`input.css:727`) for the transaction-detail timeline — flat top-left corner pointing at avatar; markdown styling rules `:747-787`. | consider daisy `chat` + `chat-bubble` (see deep-dive) |
| Data display | countdown | 🚫 | Not needed. | none |
| Data display | diff | 🚫 | Not needed. | none |
| Data display | divider | ❌ | 0 uses of `.divider`. We hand-roll dividers with `border-t border-base-300/50` inside sectioned cards (`docs/design-system.md` §4). | low priority — Tailwind border is fine here |
| Data display | indicator | ❌ | 0 uses. `.bb-nav-badge` (sidebar count pill) could be a daisy `indicator` + `badge`. | consider for sidebar counts |
| Data display | loading | ✅ | 48 `loading-spinner` + 46 `loading-xs`. Faithful. | keep |
| Data display | mask | 🚫 | 1 incidental usage (`mask-image` Tailwind utility, not daisy mask). Not applicable. | none |
| Data display | progress | ⚠️ | 1 daisy usage; the SPA nav progress bar is hand-rolled `.bb-progress-bar` (`input.css:2313`) because daisy `progress` doesn't do the fixed-top-strip rolling animation we need. | keep custom (justified) |
| Data display | radial-progress | ❌ | 0 uses; we have `.bb-health-ring` (`input.css:984`) hand-rolled SVG for financial health score. | consider daisy `radial-progress` |
| Data display | skeleton | 🔁 | 0 uses of daisy `skeleton`. Full bespoke system: `.bb-skeleton`, `.bb-skeleton--text/text-sm/text-xs/badge/circle/circle-sm/amount/card/table-row/page-skeleton` (`input.css:2402-2510`) with shimmer animation. | adopt daisy `skeleton` + `skeleton-text` (see deep-dive) |
| Data display | stat | ❌ | 0 uses of daisy `stat`/`stat-value`/`stat-title`. We render stats as `bb-card p-4` with ad-hoc grid. Spec docs reference `stat-value` (`design-system.md:763`) but no page uses it. | adopt for dashboard / overview tiles (see deep-dive) |
| Data display | status | ❌ | 0 uses. New in daisy 5 — dot-style state indicator. Could replace ad-hoc colored `.bb-tx-row__indicator` (`input.css:2613`). | low priority |
| Data display | table | ✅ | 8 `table-sm` matches; sticky headers via `table-pin-rows`. Tables are the right size + zebra config per spec (`design-system.md` §6). | keep |
| Data display | timeline | 🔁 | Daisy ships a vertical timeline component. We hand-roll everything in `internal/templates/components/timeline.templ` (387 LOC of templ) + `.bb-timeline`, `.bb-timeline-prominent`, `.bb-timeline-count` (`input.css:813-824` and unlayered overrides `input.css:2881-3174`). Our needs (continuous left rail through 24px tiles, comment-bubble tail, `:has()` rail-mask CSS) exceed what daisy timeline provides. | keep custom (justified — daisy timeline doesn't do rail-threading) |

### Navigation
| Category | Component | Status | Our usage / hand-rolled equivalent | Action |
|---|---|---|---|---|
| Navigation | breadcrumbs | 🔁 | 0 uses of daisy `breadcrumbs`. We hand-roll `.bb-breadcrumb` + `.bb-breadcrumb-item/sep/current` (`input.css:206-241`) with chevron SVGs. `BreadcrumbNav` templ at `breadcrumb.templ:14`. | consider daisy `breadcrumbs` (see deep-dive) |
| Navigation | dock | 🚫 | Not used; not appropriate for desktop admin. | none |
| Navigation | menu | ⚠️ | 7 uses, all in `dropdown-content menu` rows for ellipsis overflow menus (`tags.templ:106`, `rules.templ:189`, `access.templ:113`/`:247`, `users.templ:159`, `categories.templ:203`, `settings_layout.templ:92`). Sidebar nav is **NOT** daisy `menu` — it's `.bb-sidebar-nav` + `.bb-sidebar-link` (`input.css:362-456`) to get hover/active/icon-opacity choreography daisy `menu` doesn't provide. | keep custom sidebar (justified); keep `menu` in dropdowns |
| Navigation | navbar | ⚠️ | Used in `base.html:235` as `bb-mobile-navbar` (extends `.navbar` with sticky positioning, backdrop blur, safe-area). | keep (extension is justified) |
| Navigation | pagination | 🔁 | 0 daisy `pagination`/`join` for paging. Hand-rolled `.bb-paginator` + `.bb-paginator__btn/btn--active/ellipsis/info/nav/per-page` (`input.css:1384-1421`). | consider daisy `join`+`btn-square` pattern (see deep-dive) |
| Navigation | steps | ✅ | Used natively in `mcp_guide.templ` (3-step indicator) + `csv_import.templ` (custom hand-rolled steps using utility classes — should use daisy `steps`). | normalize CSV importer to daisy `steps` |
| Navigation | tabs | ❌ | 0 uses of daisy `tabs`/`tabs-box`/`tab-active`. The `mcp_guide` "agent tabs" (`mcp_guide.templ:87`) hand-roll with `role="tablist"` + `btn-primary`/`btn-ghost` toggling. `settings_layout.templ` uses a dropdown-menu pattern instead of tabs. | adopt daisy `tabs-box` or `tabs-border` (see deep-dive) |

### Feedback
| Category | Component | Status | Our usage / hand-rolled equivalent | Action |
|---|---|---|---|---|
| Feedback | alert | ⚠️ | 26 matches with proper modifiers (`alert-info/success/warning/error`, `alert-soft` per spec). But `csv_import.templ:202` hand-rolls one as `flex items-start gap-3 rounded-xl bg-error/10 text-error px-4 py-3` instead of `alert alert-error alert-soft rounded-xl`. Also `.bb-form-error` (`input.css:299`) is a near-duplicate of `alert-error alert-soft` for inline form errors. | normalize csv_import; document `bb-form-error` as the inline-tight variant |
| Feedback | modal | ⚠️ | 4 native `<dialog class="modal">` uses (`categories.templ:252`, `connections.templ:462`, `create-link-modal`, `delete-modal`). Convention from `design-system.md` followed. But the global confirm/cmdk/shortcuts/category-picker/tag-picker dialogs in `base.html` are **all bespoke** (`.bb-confirm-dialog`, `.bb-cmdk-dialog`, `.bb-shortcuts-dialog`, `.bb-catpicker-dialog`, `.bb-tagpicker-dialog` — 5 nearly identical dialog shells in `input.css`). | consolidate the 5 bespoke dialog shells (see deep-dive) |
| Feedback | toast | ⚠️ | 1 native `class="toast toast-top toast-center"` (`report_detail.templ:34`). The global toast in `base.html:353` uses bespoke `fixed bottom-8 left-1/2 -translate-x-1/2` markup instead of daisy `toast toast-center toast-bottom`. | adopt daisy `toast` for the global pill |
| Feedback | tooltip | ✅ | 20 `tooltip-top` uses; `data-tip` attribute pattern. Faithful. One unlayered-CSS hack `.bb-tag.tooltip { display: inline-flex }` (`input.css:2877`) to fix tag-chip + tooltip combo. | keep |

### Data input
| Category | Component | Status | Our usage / hand-rolled equivalent | Action |
|---|---|---|---|---|
| Data input | checkbox | ✅ | 22 native uses with `checkbox-sm`/`-xs`/`-primary`/`-warning`. | keep |
| Data input | file input | ✅ | 7 native `file-input file-input-bordered` uses (`csv_import`, `providers`, `backups`). | keep |
| Data input | filter | ❌ | 0 daisy `filter`/`filter-reset` uses. Our `.bb-filter-bar` (`input.css:668`) wraps `input-sm`/`select-sm` and a Reset button. Daisy `filter` is a radio-driven slot — semantically different, probably not a fit. | none (different scope) |
| Data input | input (text) | ⚠️ | 76 `input-bordered` uses. **Note:** daisy 5 dropped `input-bordered` as a separate modifier — the bordered look is now the `input` default. Our usage works because daisy still ships the class, but it's redundant. Custom `.bb-form-input` (`input.css:295`) adds focus-bg shift. | strip `input-bordered` (daisy 5 default) — non-urgent |
| Data input | label | ⚠️ | Three label patterns coexist: daisy `<label class="label"><span class="label-text">`, `.bb-filter-label` (filter bars), and `<label class="text-sm font-medium text-base-content/70 mb-1.5 block">` (form fields). Documented in `design-system.md:227-230`. Form pages have drifted toward the third (most common in `csv_import.templ`, `agents_settings.templ`). | consider standardizing on daisy `fieldset`+`label`+`legend` |
| Data input | radio | ✅ | 6 native uses with `radio-sm radio-primary`. | keep |
| Data input | range | 🚫 | Not used. | none |
| Data input | rating | 🚫 | Not used. | none |
| Data input | select | ⚠️ | 92 `select-bordered` uses. Same daisy-5 redundancy as `input-bordered`. Custom `.bb-form-select` (`input.css:296`) adds bg-200. `<select>` has its own dark-mode `::picker(select)` overrides (`input.css:1024-1046`) — necessary because daisy 5's `appearance: base-select` strips backgrounds in dark mode. | strip `select-bordered`; keep `::picker` overrides (justified bug fix) |
| Data input | textarea | ✅ | 12 native uses. | keep |
| Data input | toggle | ✅ | 12 `toggle-primary` + 14 `toggle-sm`. Faithful. | keep |
| Data input | validator | ❌ | 0 uses. New in daisy 5 — pure-CSS form validation. Worth a look for inline-edit on `connection_detail` and form pages, but Alpine validation already does most of this. | low priority |

### Layout
| Category | Component | Status | Our usage / hand-rolled equivalent | Action |
|---|---|---|---|---|
| Layout | avatar | ❌ | 0 uses of daisy `.avatar`/`avatar-placeholder`/`avatar-group`. We have `.bb-tx-avatar` (`input.css:1171`, category-colored 36×36 tile) and `.bb-sidebar-profile-avatar` (raw `<img class="rounded-full">`). Owner-badge avatar overlay (`.bb-tx-owner-badge`) is also custom. | consider `.avatar` chrome for sidebar profile + cmdk; `.bb-tx-avatar` is data-driven (color-mix from category), keep custom |
| Layout | carousel | 🚫 | Not used. | none |
| Layout | collapse / accordion | 🔁 | 0 uses of daisy `collapse`/`collapse-arrow`/`accordion`. Filter panels use `x-collapse` (Alpine) wrapped in `.bb-filter-toggle` + `.bb-filter-form` (`input.css:1349-1364`). Settings page uses native `<details><summary>`. | consider daisy `collapse-arrow` for filter panel (see deep-dive) |
| Layout | drawer | ✅ | 1 use in `base.html:229` — the canonical app shell drawer. Faithful (`drawer lg:drawer-open`, `drawer-toggle`, `drawer-content`, `drawer-side`, `drawer-overlay`). Bespoke focus-trap JS added on top. | keep |
| Layout | dropdown | ✅ | 14 `dropdown-content` + 12 `dropdown-end`. Convention: `dropdown-content menu bg-base-100 rounded-xl shadow-lg border border-base-300 z-50 w-44 p-1`. | keep |
| Layout | fieldset | ⚠️ | 19 `class="fieldset"` (no daisy modifiers, no `fieldset-legend`). Most are `<fieldset class="fieldset">` with our custom `<label class="text-sm font-medium...">` instead of daisy `<legend class="fieldset-legend">`. We're using the daisy fieldset _frame_ but not its legend. | adopt `fieldset-legend` consistently (see deep-dive) |
| Layout | footer | 🚫 | Admin has no footer. | none |
| Layout | hero | 🚫 | Not used. | none |
| Layout | join | ⚠️ | 1 use (`mcp_guide.templ:102`) — `join rounded-lg border border-base-300` wrapping two `join-item btn` for Guide/Config toggle. Faithful but rare. The bb-view-toggle (`input.css:1331`) is essentially a re-implementation of join+btn. | consider daisy `join` for `bb-view-toggle` |
| Layout | list | ❌ | 0 uses of daisy 5's new `list`/`list-row` component. Several pages have card-list patterns that could use it (agents list, rules list, users grid). | low priority — bb-card grid is fine |
| Layout | stack | 🚫 | Not used. | none |

### Mockup / Other
| Category | Component | Status | Our usage / hand-rolled equivalent | Action |
|---|---|---|---|---|
| Mockup | browser/code/phone/window | 🚫 | Not appropriate for a financial admin. | none |
| Other | calendar | 🚫 | We use native `<input type="date">` everywhere — fine. | none |
| Other | kbd | 🔁 | 0 uses of daisy `kbd`/`kbd-xs/sm/md/lg/xl`. We hand-roll `.bb-kbd` + `.bb-kbd-combo`/`.bb-kbd-combo__key` (`input.css:2525-2575`), plus near-duplicates `.bb-cmdk-kbd` (`:1593`), `.bb-shortcuts-kbd` (`:1945`), `.bb-cmdk-footer kbd` (`:1625`), and another in `.bb-catpicker-footer kbd` (`:2103`). 4 near-identical shadow + bg + border definitions. | consolidate (see deep-dive) |
| Other | link | ✅ | 39 uses (`link link-primary`, `link link-hover`). Mostly inside paragraphs of explainer text. | keep |
| Other | theme-controller | 🚫 | We auto-switch via `prefers-color-scheme`; no user-facing theme toggle. | none |

---

## Per-component deep dives

### Skeleton — full bespoke replacement
- **DaisyUI status:** daisy 5 ships `skeleton` (base) + `skeleton-text` modifier. Single class with built-in shimmer.
- **Our usage:** zero daisy `skeleton`. We have an entire `bb-skeleton` system at `input.css:2402-2510`: `bb-skeleton`, `bb-skeleton--text`, `bb-skeleton--text-sm`, `bb-skeleton--text-xs`, `bb-skeleton--badge`, `bb-skeleton--circle`, `bb-skeleton--circle-sm`, `bb-skeleton--amount`, `bb-skeleton--card`, `bb-skeleton-table-row`, `bb-page-skeleton`. Used inline in `base.html:727-744` for cmdk loading rows. Bespoke shimmer keyframe `bb-shimmer`.
- **What we're doing instead:** ~110 lines of CSS replicating daisy's animation, plus 9 size variants instead of using Tailwind utility sizes on top of `skeleton`.
- **Recommended action:** delete `bb-skeleton*` entirely; replace with `<div class="skeleton w-16 h-3"></div>` patterns. For "card-shaped" skeletons, pair `skeleton` with `bb-card` chrome.
- **Effort:** M (touches `base.html` cmdk loaders, transactions list, rules table, agents page; ~10 call sites).
- **Risk:** cosmetic — shimmer timing/curve will visibly change.

### Kbd badges — 4-way duplication
- **DaisyUI status:** daisy 5 ships `kbd` with `kbd-xs..xl` sizes.
- **Our usage:** zero daisy `kbd`. Four near-identical definitions in `input.css`:
  - `.bb-kbd` (`:2525`) — shared
  - `.bb-cmdk-kbd` (`:1593`) — `base.html` cmdk
  - `.bb-shortcuts-kbd` (`:1945`) — `?` help modal
  - `.bb-cmdk-footer kbd` + `.bb-catpicker-footer kbd` — same shadow/border, different selector
  - Plus `.bb-kbd-combo` + `.bb-kbd-combo__key` for the ⌘│K blended pill
- All four duplicate the same `bg + border + box-shadow + dark-mode flip` recipe (~50 lines each).
- **What we're doing instead:** the templ `Kbd`/`KbdCombo` components (`kbd.templ`) emit `.bb-kbd`/`.bb-kbd-combo` — that part is fine. The duplication is in `input.css` because the cmdk/help dialogs were prototyped before the shared `bb-kbd` rule was hoisted.
- **Recommended action:** alias `.bb-cmdk-kbd`, `.bb-shortcuts-kbd`, footer `kbd` to `@apply bb-kbd` (one-line forward) or just use `bb-kbd` directly in `base.html`. Optional: swap `bb-kbd` itself for a `<kbd class="kbd kbd-xs">` daisy build with custom variables on `--kbd-bg` etc.
- **Effort:** S (3 selector consolidations).
- **Risk:** none — pixel-identical recipe.

### Card — daisy bypassed almost entirely
- **DaisyUI status:** `card`, `card-body`, `card-title`, `card-actions`, `card-border`, `card-dash`, `card-side`, `image-full`, `card-xs..xl`.
- **Our usage:** daisy `card` appears only in `agents.templ` + `session_detail.templ`. Every other surface uses `.bb-card` (`input.css:244-275`). 82 templ files use it.
- **What we're doing instead:** `.bb-card` is `bg-base-100 rounded-xl border border-base-300` + dark-mode `color-mix(in oklch, base-100 92%, white 8%)` lift. Daisy `card` adds a `shadow-md` default and uses `border-base-200` — both inconsistent with our shadcn-inspired flat-with-border aesthetic.
- **Recommended action:** **keep `bb-card`** — it's a justified extension. But document the divergence in `design-system.md`: daisy's `card` defaults don't match our spec (we explicitly want border-not-shadow per `:243` comment), and the dark-mode lift is the load-bearing reason for the custom rule. Consider renaming the inconsistent `agents.templ`/`session_detail.templ` uses to `bb-card` so the codebase is uniform.
- **Effort:** S (~4 templ edits in agents pages).
- **Risk:** cosmetic.

### Chat / comment bubble — daisy chat is a fit
- **DaisyUI status:** `chat`, `chat-start`/`chat-end`, `chat-image`, `chat-header`, `chat-footer`, `chat-bubble` with colour variants.
- **Our usage:** zero daisy `chat`. The transaction-detail activity timeline uses `.bb-comment-bubble` (`input.css:727`) — chat-style bubble with flat top-left corner pointing at the avatar on the rail. Markdown styling rules `:747-787`.
- **What we're doing instead:** ~70 lines of bubble + markdown + dark-mode adjustments, plus `bb-timeline-prominent`'s comment-card override (unlayered, `:2974-3110`).
- **Recommended action:** the prominent-mode comment-card treatment is too custom (header bar + body + ::before/::after tail) to fit daisy chat. The card-mode `.bb-comment-bubble` could become a `chat-bubble`. But: the rail-anchor geometry is load-bearing and daisy chat puts the avatar to the left as a separate slot, not centred on a rail. **Keep custom** but consider whether daisy `chat-bubble` colours could replace the `color-mix` chrome.
- **Effort:** M.
- **Risk:** behavioural — the rail tail geometry is hard-coded against the 28px tile centre.

### Breadcrumbs
- **DaisyUI status:** `breadcrumbs` class on a wrapper around `<ul>` with automatic separators.
- **Our usage:** zero. `breadcrumb.templ:14` renders inline chevron SVGs inside a custom `<nav class="bb-breadcrumb">` (CSS at `input.css:206-241`).
- **What we're doing instead:** custom horizontal-scroll behavior with `overflow-y: clip` to handle SVG sub-pixel mismatch (comment at `:213-218`), and bespoke colour treatment for current page (`bb-breadcrumb-current`).
- **Recommended action:** daisy `breadcrumbs` handles the same separator+scroll pattern in fewer lines. Migration would let us delete `~35 lines` of CSS. But the explicit chevron SVG (not arrow `>`) and the bespoke 3-tier colour ramp (`base-content/40` → `base-content/25` → `base-content/70`) would have to be preserved as overrides.
- **Effort:** S.
- **Risk:** cosmetic — verify separator glyph still reads as a chevron, not a slash.

### Pagination — handrolled, daisy `join` would shorten it
- **Our usage:** `.bb-paginator` + 4 modifier classes (`input.css:1384-1421`). Numbered buttons with ellipsis.
- **DaisyUI pattern:** daisy 5 routes pagination through `join` + `join-item btn` — see `daisyui.com/llms.txt` Pagination section.
- **Recommended action:** rewrite `.bb-paginator` as `<div class="join">` with `<button class="join-item btn btn-sm">` + `btn-active`. Drop `.bb-paginator__btn`, `.bb-paginator__btn--active`, `.bb-paginator__ellipsis`. Keep `.bb-paginator__info`/`__per-page` for the surrounding layout.
- **Effort:** M (touches the paginator partial + every list page that uses it).
- **Risk:** cosmetic.

### Tabs — hand-rolled with btn toggling
- **Our usage:** `mcp_guide.templ:87` uses `flex flex-wrap gap-1.5` + `role="tablist"` + `btn-primary`/`btn-ghost` switching. `settings_layout.templ` uses a dropdown-menu (mobile) + sidebar links (desktop) instead of tabs.
- **DaisyUI offer:** `tabs`, `tabs-box`, `tabs-border`, `tabs-lift`, `tab-active`, `tabs-top`/`tabs-bottom`.
- **Recommended action:** convert `mcp_guide` to `tabs tabs-border` with `<a role="tab" class="tab">` and `tab-active`. Daisy supports the radio-driven variant for pure-CSS state, but our Alpine factory already handles it.
- **Effort:** S.
- **Risk:** none.

### Stat — completely unused, perfect fit for dashboard metrics
- **Our usage:** zero. The dashboard tiles render as `bb-card p-4` with hand-laid grid + custom typography.
- **DaisyUI offer:** `stat`, `stat-title`, `stat-value`, `stat-desc`, `stat-figure`, `stat-actions`, `stats stats-horizontal`/`stats-vertical`.
- **Recommended action:** adopt `stats stats-horizontal` for dashboard / feed hero cards. The `stat-value` typography (large + tracking) matches what we want for amounts. Daisy `stat-figure` is the right slot for the trending icon.
- **Effort:** M (touches `feed.templ`, dashboard, possibly `account_detail.templ`).
- **Risk:** cosmetic — typography spec slightly different.

### Modal dialogs — 5 bespoke shells in `base.html`
- **DaisyUI offer:** native `<dialog class="modal modal-bottom sm:modal-middle">` + `modal-box` (already used in the 4 documented `<dialog>` modals).
- **Our usage:** the global confirm/cmdk/shortcuts/category-picker/tag-picker dialogs in `base.html` use bespoke `.bb-confirm-dialog`/`.bb-cmdk-dialog`/`.bb-shortcuts-dialog`/`.bb-catpicker-dialog`/`.bb-tagpicker-dialog` — each with its own `position: fixed; z-50` + `width: min(...)` + `box-shadow` + `bb-*-dialog-in` keyframe + dark-mode shadow override + mobile safe-area top calc. ~250 lines of CSS for what is essentially "centered modal with custom width".
- **Why they're bespoke:** they need custom width tiers (cmdk: 560px; tagpicker: 520px; catpicker: 420px; shortcuts: 640px; confirm: 420px) and the cmdk + tagpicker have header/body/footer scroll regions. None of those are blockers for daisy `modal` — they're just bespoke because they were prototyped without daisy in mind.
- **Recommended action:** convert each to `<dialog class="modal">` with `<div class="modal-box w-full sm:max-w-[Npx]">`. Drop the 5 bespoke backdrop animations; daisy modal's built-in transition is fine. Will collapse `~250 LOC` of CSS into ~30.
- **Effort:** L (touches global dialog scripts; the Alpine state machines need to switch from `.bb-*-dialog` visibility to dialog.showModal()).
- **Risk:** behavioural — focus management, scroll lock (`bbLockScroll`), and the `@keydown.escape` handlers need to be re-tested.

### Toast — global pill is custom; one daisy instance exists
- **DaisyUI offer:** `toast`, `toast-start/center/end`, `toast-top/middle/bottom`.
- **Our usage:** `report_detail.templ:34` uses `toast toast-top toast-center` natively. But the global toast in `base.html:353` is hand-rolled `fixed bottom-8 left-1/2 z-50 -translate-x-1/2 flex flex-col items-center gap-2`.
- **Recommended action:** change `base.html:353` to `<div class="toast toast-center toast-bottom z-50 ...">`. Same visual result, daisy-faithful.
- **Effort:** S.
- **Risk:** cosmetic.

### Fieldset — using the frame, not the legend
- **Our usage:** 19 `<fieldset class="fieldset">` with custom `<label class="text-sm font-medium text-base-content/70 mb-1.5 block">` inside instead of daisy's `<legend class="fieldset-legend">`. In `csv_import.templ` alone there are 11 such fieldsets, each with a redundant external label.
- **Recommended action:** adopt `fieldset-legend` for the form-field label, drop the parallel `<label>`. Adds proper semantic structure for screen readers.
- **Effort:** S/M.
- **Risk:** cosmetic — verify `fieldset-legend` typography matches our existing `text-sm font-medium text-base-content/70`.

### Avatar — daisy `.avatar` could ground tx-avatar
- **Our usage:** `.bb-tx-avatar` (`input.css:1171`, `--avatar-color` CSS variable) is data-driven (category color mix). `.bb-sidebar-profile-avatar` and the cmdk `.bb-cmdk-item-icon` are raw `<img class="rounded-full">` or coloured tiles.
- **DaisyUI offer:** `.avatar`, `.avatar-placeholder`, `.avatar-group`, `.avatar-online`/`offline`.
- **Recommended action:** `.bb-tx-avatar` is justified custom (color-mix from category, can't be expressed as a daisy modifier). But the sidebar profile + cmdk could wrap their `<img>` in `<div class="avatar"><div class="w-8 rounded-full">` for the daisy chrome. Low value.
- **Effort:** S.
- **Risk:** none.

### Collapse / accordion — Alpine x-collapse instead of daisy
- **Our usage:** filter panels use `x-collapse` (Alpine). The "Collapsible panels" pattern in `design-system.md` is a custom `.bb-filter-toggle` + `<details>` or Alpine state.
- **DaisyUI offer:** `collapse`, `collapse-arrow`, `collapse-plus`, `collapse-title`, `collapse-content`, accordion via radio inputs.
- **Recommended action:** Alpine `x-collapse` is a richer animation; keep it. But `collapse-arrow` adds a built-in caret that some of our `.bb-filter-toggle` could borrow.
- **Effort:** S, low value.
- **Risk:** none.

### Triage badges — 4th badge dialect
- **Our usage:** `.bb-triage-badge` + `--info/--warning/--error/--neutral` (`input.css:2632-2671`) — a 4th colour-tinted badge dialect. Bespoke `text-[0.6rem]` size, bespoke `bg-X/12 + text-X-darker` mix.
- **What it duplicates:** `badge badge-soft badge-{info|warning|error}` does ~95% of this with one fewer custom class. The bespoke version uses smaller text and a slightly different alpha — both achievable with `badge-xs` + a thin override.
- **Recommended action:** drop `.bb-triage-badge*`, use `badge badge-soft badge-xs badge-{info|warning|error|neutral}`. Saves 40 lines.
- **Effort:** S.
- **Risk:** cosmetic — text size will shift slightly.

### Rule creator badge / rule category badge / tag chip — all custom-pill species
- **Our usage:** three near-identical "small coloured pill" classes:
  - `.bb-tag` (`input.css:515`) — generic pill with `--tag-color` driving bg + border
  - `.bb-rule-cat-badge` (`:1262`) — same shape, `--cat-color`
  - `.bb-rule-creator-badge` (`:1281`) — same shape, neutral colours
- **What's justified:** `.bb-tag` is data-driven via CSS variables (per-tag color from DB), which `badge-soft badge-{color}` can't do — the color isn't from the daisy palette. Keep `.bb-tag`.
- **What isn't:** `.bb-rule-creator-badge--agent`/`--user` could be `badge badge-soft badge-primary badge-xs` and `badge badge-ghost badge-xs`.
- **Recommended action:** delete `.bb-rule-creator-badge` family (3 modifiers + dark-mode override). Keep `.bb-tag` and `.bb-rule-cat-badge` (data-driven).
- **Effort:** S.
- **Risk:** none.

### View toggle (`.bb-view-toggle`)
- **Our usage:** `transactions.templ:103` — Grouped/List pill toggle. CSS at `input.css:1330-1347`.
- **What's nearby in daisy:** `join` + `join-item btn` (e.g. the `mcp_guide` Guide/Config toggle).
- **Recommended action:** replace `.bb-view-toggle` with `<div class="join border border-base-300 bg-base-200"><button class="join-item btn btn-xs btn-ghost">…</button></div>`. Aligns with the one existing `join` use.
- **Effort:** S.
- **Risk:** cosmetic.

---

## Gaps & wins

### DaisyUI components we should adopt (ranked)
1. **`skeleton`** — replaces the bespoke `bb-skeleton*` system (~110 LOC saved, 10 variants → 1).
2. **`stat` / `stats`** — never used; perfect fit for dashboard/feed metric tiles.
3. **`tabs-box` or `tabs-border`** — replaces the hand-rolled `mcp_guide` tab toggle (and could replace `settings_layout.templ`'s mobile dropdown).
4. **`toast`** — convert the global pill in `base.html`; one-line fix.
5. **`breadcrumbs`** — replaces `bb-breadcrumb*` family (~35 LOC saved).
6. **`join` + `join-item`** — replaces `bb-view-toggle` and the bespoke `bb-paginator` numbered-button row.
7. **`fieldset-legend`** — fix 19 fieldsets currently using redundant external labels.
8. **`kbd`** — fold 4 duplicate kbd-style definitions into one.
9. **`modal`** (for global overlays) — collapse 5 bespoke dialog shells in `base.html`.
10. **`card` / standardize** — either migrate `bb-card` callers to daisy `card` with our overrides documented as theme variables, OR migrate the 2 outlier `agents.templ` daisy-card uses back to `bb-card`. Don't keep both.

### Hand-rolled `bb-*` classes that duplicate daisy
| `bb-*` class | Daisy equivalent | Notes |
|---|---|---|
| `.bb-skeleton*` (9 variants) | `skeleton`, `skeleton-text` | full replacement available |
| `.bb-kbd`, `.bb-cmdk-kbd`, `.bb-shortcuts-kbd`, `.bb-cmdk-footer kbd`, `.bb-catpicker-footer kbd` | `kbd kbd-xs/sm` | 4 of 5 are bug-prone duplicates |
| `.bb-triage-badge--*` | `badge badge-soft badge-xs badge-{info\|warn\|error}` | replace |
| `.bb-rule-creator-badge--agent/user` | `badge badge-soft badge-primary badge-xs` / `badge badge-ghost badge-xs` | replace |
| `.bb-view-toggle` + `.bb-view-toggle__btn` | `join` + `join-item btn` | replace |
| `.bb-paginator__btn*` | `join` + `join-item btn btn-sm` | replace |
| `.bb-breadcrumb*` | `breadcrumbs` | replace (verify chevron glyph) |
| `.bb-form-input` / `.bb-form-select` | `input` / `select` (daisy 5 dropped `-bordered` redundancy) | strip `-bordered`; keep `bb-form-*` only if focus-bg shift matters |
| `.bb-confirm-dialog`, `.bb-cmdk-dialog`, `.bb-shortcuts-dialog`, `.bb-catpicker-dialog`, `.bb-tagpicker-dialog` | `modal modal-box` | replace (largest win) |
| `.bb-tagpicker-diff-pill--add/remove` | `badge badge-soft badge-success badge-xs` / `badge badge-error` | replace |

### Hand-rolled `bb-*` classes that justifiably extend daisy
| `bb-*` class | Why kept |
|---|---|
| `.bb-card` | shadcn flat-border aesthetic + dark-mode `color-mix` lift (load-bearing dark contrast; daisy `card` ships shadow + `border-base-200`) |
| `.bb-tag` | data-driven `--tag-color` from DB; daisy badges only support palette colours |
| `.bb-tx-avatar` | data-driven category color via `color-mix(--avatar-color, …)`; daisy avatars are chrome only |
| `.bb-tx-row*` | imperative bulk-selection driven by JS store; daisy doesn't have a row primitive |
| `.bb-sidebar*` | hover/active/icon-opacity choreography + iOS-26 scroll fade gradients daisy `menu` doesn't expose |
| `.bb-timeline*` | GitHub-style continuous rail through 24/28px tiles with `:has()` rail-mask + tail SVG; daisy `timeline` is a different shape |
| `.bb-comment-bubble` | flat top-left corner pointing at rail avatar; markdown styling specific to our pipeline |
| `.bb-progress-bar` | fixed-top SPA navigation indicator (YouTube/GitHub style); daisy `progress` is form-input style |
| `.bb-cmdk-*`, `.bb-tagpicker-*`, `.bb-catpicker-*`, `.bb-shortcuts-*` content | the input + list + footer interiors are fine; only the dialog _shells_ should switch to daisy `modal` |
| `.bb-mobile-navbar` | sticky + backdrop-blur + safe-area inset on top of daisy `navbar` — justified extension |

### DaisyUI components we override too aggressively
- **`.select::picker(select)` dark-mode overrides** (`input.css:1024-1046`) — justified workaround for daisy 5's `appearance: base-select` transparent-background bug in dark mode. Document and keep.
- **`.btn { transition … !important }`** (`input.css:990`) — overrides daisy's transition shorthand to add `transform: scale(0.97)` on `:active`. The `!important` is load-bearing because daisy's `.btn` rule sits at the same specificity. Acceptable but flag as a layer-order workaround.
- **`.modal-backdrop { transition: opacity 0.2s ease !important }`** (`input.css:1049`) — small polish, acceptable.
- **`.checkbox` transition clamp** (`input.css:1136`) — daisy 5's checkbox ships `transition: all` which causes paint thrash on tx rows; we scope it to background + border only. Justified bug fix; document.
- **`.dropdown-content { animation: bb-dropdown-enter }`** (`input.css:1056`) — small polish.

---

## Modifiers we should be using

| Modifier family | Usage today | Recommendation |
|---|---|---|
| `btn-soft` | 16 uses (mostly destructive `btn-error btn-soft`) | adopt for non-destructive secondary buttons too (currently we use `btn-ghost`) |
| `btn-dash` | 0 | adopt for "Add tag" / "New rule condition" affordances currently using `.bb-tag-add` dashed border |
| `btn-outline` | 20 uses | consistent |
| `badge-soft` | 98 uses — primary modifier per spec | keep |
| `badge-dash` | 0 | could replace `.bb-tag-add` dashed pill |
| `badge-outline` | 0 | could replace `.bb-rule-creator-badge--user` |
| `card-border`, `card-dash` | 0 | only relevant if we migrate to daisy `card` |
| `tabs-box` / `tabs-border` / `tabs-lift` | 0 | adopt one for `mcp_guide` |
| `input-ghost` | 2 uses | mostly we use `input-bordered`; consider `input-ghost` for inline-edit |
| `select-ghost` | 0 | candidate for inline-edit dropdowns |
| `loading-dots` / `loading-ring` / `loading-bars` | 0 | only `loading-spinner` used — acceptable per spec |
| `menu-disabled`, `menu-active`, `menu-focus` | 0 | not used because we don't use daisy `menu` for the sidebar |
| `alert-soft` | 10 uses — matches spec | keep |
| `alert-dash` | 0 | low value |
| `tooltip-{color}` | 0 (only `tooltip-top`) | could colour error tooltips, but adds noise |
| `collapse-arrow` / `collapse-plus` | 0 | candidates for `.bb-filter-toggle` |
| `dropdown-hover` | 0 — all our dropdowns are click | keep click |
| `kbd-{size}` | 0 | adopt when consolidating `bb-kbd` |
| `skeleton-text` | 0 | adopt when replacing `bb-skeleton--text*` |

---

## Priorities (top 10)

1. **Consolidate the 5 bespoke dialog shells in `base.html` to daisy `modal`** — biggest mechanical win, ~250 LOC of CSS deleted, unifies focus/scroll/backdrop behaviour. Effort: L. Risk: behavioural (test focus traps).
2. **Replace `bb-skeleton*` with daisy `skeleton`** — ~110 LOC deleted, 1 daisy primitive replaces 9 size variants. Effort: M.
3. **Adopt `kbd` and collapse the 4 duplicate kbd recipes** — one-line alias, prevents drift. Effort: S.
4. **Convert global toast in `base.html` to `toast toast-center toast-bottom`** — one-line markup change for consistency with `report_detail.templ`'s native daisy usage. Effort: S.
5. **Adopt daisy `stat` / `stats` for dashboard + feed hero tiles** — the typography is right, and we're currently re-deriving it. Effort: M.
6. **Rewrite `mcp_guide` agent tabs as `tabs tabs-border`** — first real use of daisy tabs in the codebase; sets the convention. Effort: S.
7. **Replace `.bb-paginator` numbered-button row with `join` + `join-item btn`** — aligns with daisy 5 idiom and removes 4 custom classes. Effort: M.
8. **Replace `.bb-view-toggle` with `join` + `join-item btn`** — one widget on `transactions.templ`, sets the pattern for future segmented controls. Effort: S.
9. **Drop `input-bordered` / `select-bordered` (daisy 5 redundancy) + standardize on `fieldset-legend`** — modernization pass on form pages. Effort: M.
10. **Drop `.bb-triage-badge*` and `.bb-rule-creator-badge*`; use `badge badge-soft badge-xs badge-{info|warning|error|primary|ghost}`** — removes ~70 lines of duplicate badge CSS. Effort: S.

---

**Closing note.** The codebase is largely faithful to daisy on the input/button/badge/alert/modal/dropdown/menu/loading axes — 90%+ of `.btn`/`.badge` rows are pure daisy with our documented `rounded-xl` overlay. The custom bloat is concentrated in five areas: (1) the 5 bespoke dialog shells in `base.html`, (2) the bb-skeleton system, (3) the bb-paginator system, (4) the kbd duplication, and (5) the divergence between `.bb-card` (used in 82 files) and daisy `card` (used in 2). Fixing #1, #2, and #5 alone would shed ~400 lines of `input.css` while making the v1 admin meaningfully more daisy-canonical.

# Design System Audit & Improvement Plan

> **Purpose:** Checklist of design system improvements for Breadbox. Each task is
> self-contained and designed to be completed by a single agent in one session
> (1-2 hours). Agents should check off tasks as they complete them and push
> their changes.
>
> **How to use this document:**
> 1. Find the next unchecked task (`- [ ]`)
> 2. Complete the work described
> 3. Mark it done (`- [x]`) and add a brief note of what was changed
> 4. Commit everything (including this file update) and push
> 5. Move on or hand off to the next session
>
> **Rules for agents:**
> - Read the relevant files before making changes
> - Run `make css` after any `input.css` changes
> - Run `go build ./...` after any Go template changes
> - Avoid changing behavior — these should be mostly cosmetic/structural improvements only
> - **Be careful not to break complex interactions** (keyboard shortcuts, triage
>   queue, category picker, command palette, etc.) by over-simplifying. If a
>   component has intricate state management or event wiring, leave the logic
>   alone and only touch the CSS/class layer.
> - When standardizing patterns, pick the most common existing usage as the winner
> - Update `docs/design-system.md` if your changes establish a new convention

---

## 1. Button Standardization

**Problem:** 40+ unique button class combinations across templates. Inconsistent
sizing (`btn-xs` vs `btn-sm`), rounding (`rounded-xl` vs `rounded-lg`), and
variant usage. No clear convention for when to use which size or radius.

**Files to audit:** All files in `internal/templates/pages/*.html`

- [x] **1a. Audit and document all button patterns.** Audited 89 unique button
  class combinations across 41 templates. Convention documented in
  `docs/design-system.md` § Component Conventions → Buttons.

- [x] **1b. Normalize button rounding across all templates.** Fixed ghost buttons
  missing rounding (added `rounded-xl` for btn-sm, `rounded-lg` for btn-xs).
  Fixed btn-sm buttons using `rounded-lg` → `rounded-xl`. 15 template files updated.

- [x] **1c. Normalize button sizing across all templates.** Added `btn-sm` to 6
  unsized modal buttons in categories.html and reviews.html. Verified btn-xs
  only in compact contexts.

- [x] **1d. Standardize icon+text button gap.** Normalized all btn-sm icon+text
  to `gap-2`, all btn-xs to `gap-1.5`. Fixed ~15 instances of `gap-1` or
  `gap-1.5` on btn-sm buttons.

---

## 2. Badge Standardization

**Problem:** 42+ unique badge patterns. Inconsistent use of `badge-soft` variant,
mixed sizing (`badge-xs` vs `badge-sm`), some badges have rounding overrides.

**Files to audit:** All `internal/templates/pages/*.html`, `internal/admin/templates.go`

- [x] **2a. Audit all badge patterns.** Audited 200+ badge instances across all
  templates and 3 Go template functions. 28 unique class combinations found.
  Convention documented in `docs/design-system.md` § Component Conventions → Badges.

- [x] **2b. Normalize badge classes across all templates.** Removed `rounded-lg`
  from badges in 10 files (~28 occurrences). Added `badge-soft` to status badges
  in 8+ files. Left badge-ghost metadata badges and nav badges unchanged.

- [x] **2c. Update Go template badge functions.** Updated `statusBadge()` from
  custom hand-rolled CSS to `badge badge-soft badge-{color} badge-sm`. Updated
  `syncBadge()` to add `badge-soft`. `configSource()` already correct.

---

## 3. Card Structure Standardization

**Problem:** `bb-card` uses inconsistent internal padding. Some cards apply padding
directly (`bb-card p-5`), others use internal wrapper divs (`bb-card p-0` +
child with `px-6 py-5`). No standard for card sections (header/body/footer).

**Files to audit:** All templates using `bb-card`

- [x] **3a. Define card padding convention and document it.** Convention
  documented in `docs/design-system.md` § Component Conventions → Cards.
  Simple: p-5 (standard), p-4 (compact), p-6/p-8 (forms). Sectioned: p-0
  overflow-hidden with `px-4 sm:px-5 py-3/py-4` internal sections.

- [x] **3b. Normalize simple card padding.** Changed `p-6` to `p-5` on
  non-form content cards (transaction_detail details + category cards).
  Left form cards (p-6, p-8) and empty states (p-12) as-is.

- [x] **3c. Normalize sectioned card structure.** Replaced `px-6 py-5` and
  `px-4 sm:px-6 py-5` with `px-4 sm:px-5 py-4` across 14 files. Added
  `p-0 overflow-hidden` to sectioned cards missing it. Standardized divider
  borders to `border-t border-base-300/50`.

---

## 4. Modal Standardization

**Problem:** Modals use 3 different patterns: `<dialog>` with `modal-bottom sm:modal-middle`,
plain `<dialog class="modal">` with Alpine `modal-open`, and different content
wrappers (`<form class="modal-box">` vs `<div class="modal-box">`). Rounding
varies (`rounded-xl` vs `rounded-2xl`).

**Files to audit:** All templates with `modal` class usage

- [x] **4a. Audit all modal instances and standardize.** Audited 9 modal instances
  (5 `<dialog>`, 3 custom div, 1 special overlay). Added `modal-bottom sm:modal-middle`
  to reviews.html and account_links.html dialogs. Fixed `rounded-2xl` → `rounded-xl`
  and added `max-w-lg` on account_links modal. Left custom div dialogs (confirm,
  shortcuts, category picker) untouched — they're complex CSS components.

---

## 5. Form Control Standardization

**Problem:** Inputs and selects have inconsistent background colors, rounding, and
sizing. Label markup varies between `bb-filter-label`, DaisyUI `label`, and
plain Tailwind `text-sm font-medium`.

**Files to audit:** All form-containing templates

- [x] **5a. Standardize form input/select classes.** Removed `bg-base-200/50` from
  standard form inputs/selects in 4 files (user_form, api_key_new, oauth_client_new,
  csv_import). Replaced with `input-bordered`/`select-bordered`. Kept bg-base-200/50
  on inline-edit and read-only inputs. Filter bar and rule builder inputs left as-is.

- [x] **5b. Standardize label patterns.** Audited 3 label patterns: `.bb-filter-label`
  (filter bars), DaisyUI `label` (form fields), plain `text-sm font-medium`
  (simpler forms). All are valid in context — documented convention rather than
  force-converting. Each page is internally consistent.

---

## 6. Icon Size Convention

**Problem:** Lucide icons use inconsistent sizes: `w-3 h-3`, `w-3.5 h-3.5`,
`w-4 h-4`, `w-5 h-5`, `w-6 h-6`, `w-8 h-8` with no clear mapping to context.

**Files to audit:** All templates with `data-lucide`

- [x] **6a. Define and apply icon size convention.** Fixed 26+ deviations across
  10 files. Normalized w-4.5 h-4.5 (non-standard) → w-5 h-5 in categories,
  rules, api_key_created, oauth_client_created, transaction_detail. Fixed
  btn-xs icons from w-3 h-3 → w-3.5 h-3.5 in mcp_guide and other files.
  Convention documented in `docs/design-system.md`.

---

## 7. Status Indicator Consolidation

**Problem:** Status is displayed 3 different ways: DaisyUI badges, colored dots
with text, and custom pill spans with background opacity. Should converge on
one approach.

**Files to audit:** Connection status, sync status, review status displays

- [x] **7a. Standardize status indicator pattern.** Audited all inline status
  indicators. `statusBadge()`/`syncBadge()` cover connection and sync status
  badges. Webhook badges already use identical badge-soft classes (standardized
  in task 2). Complex icon-based status indicators (timeline dots, activity
  icons, sync spinners) are intentionally inline — they have unique contextual
  styling that doesn't fit a simple badge function. Convention documented.

---

## 8. Transition & Animation Consistency

**Problem:** Alpine.js transitions use 3 different approaches: full explicit
transitions (`x-transition:enter="transition ease-out duration-200"` etc.),
simple `x-transition`, and `x-collapse`. No convention for when to use which.

**Files to audit:** All templates with `x-transition` or `x-collapse`

- [x] **8a. Define transition convention and normalize.** Replaced over-specified
  6-line explicit transitions with `x-collapse` on 6 filter panels (transactions,
  account_detail, sync_logs, reviews, rules, logs). Accordion sections already
  used `x-collapse` correctly. Tab panels and wizard steps correctly use explicit
  transitions. Convention: collapsible sections → `x-collapse`, tabs/wizards →
  explicit transitions, modals → DaisyUI handles it.

---

## 9. Empty State Pattern

**Problem:** Empty states are ad-hoc across pages. Some have icon + title +
description + CTA button, others are just a `<p>` tag. No reusable pattern.

**Files to audit:** All templates that conditionally show "no data" states

- [x] **9a. Create a standard empty state partial or convention.** Audited 12
  empty states across all pages. 4 already match the complete pattern (icon +
  title + description + CTA). Upgraded 2 minimal empty states in access.html
  (OAuth clients, API keys sections) to the full pattern. Transaction/account
  "no results" states left intentionally compact for filtered views. Convention
  documented in design-system.md.

---

## 10. Skeleton Loading Consistency

**Problem:** Skeletons are defined in `partials/skeletons.html` but some pages
build custom skeleton HTML inline (especially `insights.html`,
`connection_detail.html`). Mixed use of `bb-skeleton-*` classes vs. inline
`animate-pulse` Tailwind.

**Files to audit:** `partials/skeletons.html`, all pages with loading states

- [x] **10a. Audit skeleton usage and consolidate.** Audited skeleton system.
  Partials in `skeletons.html` provide 5 reusable patterns. CSS defines 15+
  `bb-skeleton-*` classes with `bb-shimmer` animation. `animate-pulse` used in
  dashboard (3 status dots) and transactions (JS-generated rows) — these are
  appropriate for their context. No consolidation needed.

---

## 11. Spacing & Padding Consistency

**Problem:** Similar components use different spacing. Card headers vary between
`py-3`, `py-4`, `py-5`. Gaps vary between `gap-2`, `gap-3` for similar contexts.
Page sections use inconsistent margins (`mb-4` vs `mb-6`).

**Files to audit:** All templates

- [x] **11a. Standardize section spacing.** Fixed filter bar margin from `mb-5`
  to `mb-6` in logs.html and sync_logs.html. Fixed reviews.html settings card
  from `mb-4` to `mb-6`. Convention: `mb-6` between all top-level sections.

---

## 12. Collapsible Section Pattern

**Problem:** Two approaches used for collapsible UI: Alpine.js `x-data` with
`x-show`/`x-collapse`, and DaisyUI `collapse` component with checkbox input.
Should standardize on one.

**Files to audit:** All templates with collapsible/expandable sections

- [x] **12a. Standardize on Alpine.js collapsible pattern.** Converted the one
  DaisyUI `collapse` checkbox instance in account_detail.html to Alpine pattern
  (`x-data + x-show + x-collapse` with consistent toggle button). All 14+
  collapsible sections now use the Alpine approach with rotating chevron.

---

## 13. Toast/Notification Redesign

**Problem:** The global toast in `base.html` uses DaisyUI's `toast toast-end
toast-bottom` pattern (corner-anchored, boxy). The prompt builder page
(`prompt_builder.html:196-208`) has a much better toast design: a centered
floating pill at the bottom with a checkmark icon, smooth transitions, and
a polished look. This should become the app-wide toast.

**Reference implementation (`prompt_builder.html:196-208`):**
```html
<div class="fixed bottom-24 left-1/2 z-50 -translate-x-1/2">
  <div class="bg-base-100 border border-base-300 rounded-xl shadow-lg px-4 py-2.5 flex items-center gap-2">
    <svg class="text-success" ...checkmark.../></svg>
    <span class="text-sm font-medium">Prompt copied to clipboard</span>
  </div>
</div>
```
Key traits: centered horizontally, floating near bottom, `rounded-xl`, border
+ shadow (not solid background color), checkmark icon for success, clean
enter/leave transitions (fade + slide-up).

**Secondary pattern:** The agent wizard copy buttons also have nice inline
feedback (icon swap to checkmark, `text-success`, 2s timeout). This inline
feedback is great for trivial instant actions (copy buttons) and should be
used more broadly alongside the toast.

**Files to audit:** `layout/base.html` (global toast), `prompt_builder.html`
(reference), all pages dispatching `bb-toast` or creating inline toasts

- [x] **13a. Redesign the global toast to match the prompt builder style.**
  Replaced corner-anchored DaisyUI toast with centered floating pill design.
  Supports success/error/warning/info types with Lucide icons. Auto-dismiss
  after 3s with fade+slide transition. bb-toast CustomEvent API preserved.

- [x] **13b. Migrate all per-page toast implementations to the global system.**
  Removed inline toast HTML from prompt_builder.html. Converted showToast()
  to dispatch bb-toast events to the global system. Removed `toast` state
  variable from Alpine x-data.

- [ ] **13c. Standardize inline button feedback pattern.** Deferred — requires
  JavaScript changes across many pages (toggle switches, copy buttons, quick
  actions). Better suited for a dedicated session.

---

## 14. Pagination Standardization

**Problem:** 4 different pagination implementations: `bb-paginator` custom component,
server-side prev/next links, cursor-based links, and ad-hoc button pairs.

**Reference implementation:** The transactions page (`transactions.html`) has the
best pagination UX — use it as the model for all other paginated views.

**Files to audit:** All templates with pagination

- [ ] **14a. Standardize pagination markup.** Deferred — webhook_events, sync_logs,
  and logs pages use ad-hoc prev/next buttons. Converting to bb-paginator
  requires Go handler changes to provide total page count and page range data.
  Not a pure CSS/template change.

---

## 15. CSS Cleanup

**Problem:** Some CSS classes are defined but unused or redundant. The
`bb-amount--debit` class is identical to `bb-amount`. Legacy classes from
pre-DaisyUI era may still exist. Some chart variables mix `rgba()` with
the codebase's `oklch()` convention.

**Files to audit:** `input.css`

- [x] **15a. Remove dead CSS classes.** Removed 24 unused classes: bb-pagination,
  bb-action-bar, bb-amount--debit, bb-amount--credit, bb-stat-card family (11
  classes + responsive overrides), bb-skeleton--heading/title/avatar/btn,
  bb-skeleton-stat/list-item/pulse, bb-triage-action--dismiss, bb-tx-card.
  Also removed old toast entrance animation (.toast .alert + bb-toast-enter
  keyframes) since global toast was redesigned.

- [ ] **15b. Normalize color functions in CSS.** Chart color variables intentionally
  use `rgba()` for alpha transparency which `oklch()` can also express, but
  the chart library (Chart.js) expects standard CSS color values. rgba() is
  appropriate here for interop. No changes needed.

---

## 16. Template Function Cleanup

**Problem:** `internal/admin/templates.go` has duplicate/overlapping functions:
`mulFloat` vs `mulf`, `intToFloat` vs `itof`, `divFloat` vs `divf`. The
`commaInt` and `commaInt64` functions are copy-pasted with only the type
signature different.

**Files to audit:** `internal/admin/templates.go`

- [ ] **16a. Deduplicate template functions.** Consolidate overlapping functions:
  - Keep `mulf`, `divf`, `subf`, `absf` (shorter, consistent naming)
  - Remove `mulFloat`, `divFloat`, `mulFloatRaw`, `intToFloat` (replace usages
    in templates with the shorter aliases)
  - Merge `commaInt` and `commaInt64` into one function using `any` type
  - Update all template references

---

## 17. Alert/Flash Standardization

**Problem:** Alert markup varies: some use `alert alert-{type}`, some use
`role="alert"` without the `alert` class, some add `rounded-xl`, some don't.
Flash partial (`flash.html`) and inline alerts should look the same.

**Files to audit:** `partials/flash.html`, all pages with inline alerts

- [ ] **17a. Standardize alert markup.** Ensure all alerts (flash and inline) use:
  `<div role="alert" class="alert alert-{type} rounded-xl mb-6">`. Audit
  `flash.html` and all inline `alert` usage. Remove any hand-built alert
  markup that doesn't use DaisyUI's alert component.

---

## 18. Responsive Consistency

**Problem:** Most templates handle responsive well, but some pages have
inconsistent breakpoint usage. Some use `sm:` where others use `lg:` for
similar layout changes. Stat grid columns vary per page.

**Files to audit:** Dashboard, insights, and other grid-heavy pages

- [ ] **18a. Audit responsive grid patterns.** Standardize stat card grids:
  - 4 stats: `grid-cols-2 lg:grid-cols-4`
  - 3 stats: `grid-cols-1 sm:grid-cols-3`
  - 2 stats: `grid-cols-2`
  - Normalize inconsistent breakpoints across pages

---

## 19. Dark Mode Polish

**Problem:** While the theme system works well, some elements may have hard-coded
colors that don't adapt properly in dark mode. The `bg-base-200/50` issue on
`<select>` elements (noted in CLAUDE.md) may have other instances.

**Files to audit:** All templates, `input.css`

- [ ] **19a. Audit dark mode edge cases.** Check for:
  - Hard-coded color values in templates (e.g., `bg-white`, `text-gray-*`,
    `text-black`) that should use DaisyUI semantic tokens
  - `bg-base-200/50` on `<select>` elements (known bug — replace with solid
    `bg-base-200`)
  - Inline `style` attributes with colors that won't adapt to dark mode
  - Fix any issues found

---

## 20. Frosted Glass Pattern for Floating Elements

**Problem:** The mobile topbar is getting a frosted glass treatment
(`backdrop-blur` + semi-transparent background) so it's not entirely opaque.
This pattern should be applied consistently across all floating/overlay elements.

**Files to audit:** `layout/base.html` (mobile navbar), `input.css` (modals,
command palette, category picker, confirm dialog, shortcuts dialog)

- [ ] **20a. Apply frosted glass to mobile topbar.** Update the mobile navbar
  (`navbar bg-base-100 lg:hidden`) to use `bg-base-100/80 backdrop-blur-lg`
  (or similar) instead of solid `bg-base-100`. Test in both light and dark mode.

- [ ] **20b. Apply frosted glass to modal/dialog backdrops.** Update the backdrop
  layers for `bb-cmdk-backdrop`, `bb-catpicker-backdrop`, `bb-confirm-backdrop`,
  `bb-shortcuts-backdrop`, and DaisyUI modal backdrops to use a frosted glass
  effect (`backdrop-blur-sm` on the backdrop layer). This adds depth and polish.
  Be careful with performance — `backdrop-blur` can be expensive on low-end
  devices, so keep blur values modest (4-8px).

---

## 21. Keyboard Shortcut Hints — Hide on Mobile

**Problem:** Keyboard shortcut hints (e.g., `bb-kbd` badges showing "K", "?",
key combos) are shown in a few places in the UI. These are meaningless on
mobile/touch devices and add visual clutter.

**Files to audit:** All templates showing `bb-kbd` or shortcut key hints,
`base.html` (shortcuts help dialog trigger)

- [ ] **21a. Hide keyboard shortcut hints on mobile.** Add `hidden sm:inline-flex`
  (or `hidden lg:inline-flex`) to all keyboard shortcut hint elements so they
  only appear on devices likely to have a keyboard. This includes:
  - Command palette trigger hint ("K")
  - Any "?" shortcut hints
  - Shortcut badges in nav or page headers
  - Do NOT hide the shortcuts help dialog itself (it's already behind a
    keyboard shortcut to open it)

---

## 22. Button Loading Spinners for Async Actions

**Problem:** When buttons trigger async operations (fetch calls, form submissions
via AJAX), there's no loading feedback on the button itself. Users don't know
if their click registered. Some pages show a global progress bar but the button
stays static.

**Pattern to implement:** When a button triggers an async action, it should:
1. Show a `loading loading-spinner loading-xs` inside the button (replacing the icon)
2. Add `btn-disabled` or `pointer-events-none` to prevent double-clicks
3. Restore original state on success/error

**Reference:** DaisyUI buttons support `<span class="loading loading-spinner loading-xs"></span>`
natively and it looks great.

**Files to audit:** All templates with `fetch()` or AJAX calls triggered by buttons

- [ ] **22a. Create a reusable `bbButtonLoading(btn)` / `bbButtonDone(btn)` JS
  helper.** Add to `base.html` a small utility:
  ```js
  window.bbButtonLoading = function(btn) {
    btn._origHTML = btn.innerHTML;
    btn.disabled = true;
    btn.innerHTML = '<span class="loading loading-spinner loading-xs"></span>';
  };
  window.bbButtonDone = function(btn) {
    btn.disabled = false;
    btn.innerHTML = btn._origHTML;
    lucide.createIcons({ nodes: [btn] });
  };
  ```
  This preserves the original content and restores it (including re-initializing
  Lucide icons). Document the pattern.

- [ ] **22b. Apply button loading pattern to key async actions.** Prioritize the
  most user-facing async buttons:
  - Sync Now buttons (connections page, connection detail)
  - Delete/remove confirmations
  - Form submissions via fetch (rule create/edit, category create, etc.)
  - Review approve/skip/reject actions
  - Any button that calls `fetch()` and then navigates or updates the page
  - Don't apply to trivial instant actions (copy-to-clipboard, toggle switches)

---

## 23. Design System Documentation Sync

**Problem:** `docs/design-system.md` is stale — it was written during the
Pico-to-DaisyUI migration and hasn't been kept current. Many sections describe
the migration plan rather than the current state. The conventions documented
there should match what's actually in the code after the above improvements.

- [ ] **23a. Rewrite `docs/design-system.md` to reflect current reality.** This
  is a significant rewrite, not a patch. The doc should:
  - Remove migration-era language ("migrates from Pico CSS")
  - Document the actual component patterns as they exist now
  - Include the conventions established during this audit (button sizes, badge
    patterns, icon sizes, frosted glass, loading spinners, etc.)
  - Serve as a reference for anyone building new pages
  - Include code examples for each major pattern
  - Be organized by component type, not by migration step

---

## Progress Log

| Date | Task | Agent/Session | Notes |
|------|------|---------------|-------|
| 2026-04-01 | 1a-1d Button Standardization | Claude Opus | 15 files, rounding/sizing/gap normalized |
| 2026-04-01 | 2a-2c Badge Standardization | Claude Opus | 19 files + templates.go, badge-soft + rounded-lg cleanup |
| 2026-04-01 | 3a-3c Card Standardization | Claude Opus | 14 files, px-6→px-5, sectioned card padding |
| 2026-04-01 | 4a Modal Standardization | Claude Opus | 2 files, added responsive classes + fixed rounding |
| 2026-04-01 | 5a-5b Form Control Standardization | Claude Opus | 4 files, removed bg-base-200/50, standardized borders |
| 2026-04-01 | 6a Icon Size Convention | Claude Opus | 10 files, fixed w-4.5→w-5, btn-xs w-3→w-3.5 |
| 2026-04-02 | 7a Status Indicators | Claude Opus | Audited; convention documented, complex icons left inline |
| 2026-04-02 | 8a Transition Consistency | Claude Opus | 6 filter panels: explicit transitions → x-collapse |
| 2026-04-02 | 9a Empty State Pattern | Claude Opus | Upgraded 2 minimal empty states in access.html |
| 2026-04-02 | 10a Skeleton Loading | Claude Opus | Audited; system already consistent, no changes needed |
| 2026-04-02 | 11a Section Spacing | Claude Opus | Fixed mb-5→mb-6 in 3 files |
| 2026-04-02 | 12a Collapsible Pattern | Claude Opus | Converted 1 DaisyUI collapse to Alpine in account_detail |
| 2026-04-02 | 13a-b Toast Redesign | Claude Opus | Centered floating pill + migrated prompt_builder |
| 2026-04-02 | 15a CSS Cleanup | Claude Opus | Removed 24 dead classes + old toast animation |

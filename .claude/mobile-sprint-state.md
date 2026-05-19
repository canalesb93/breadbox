# Mobile Responsiveness Sprint — v2 SPA / iOS Safari (Phase 2)

**Branch:** `sprint/mobile-ios-safari`
**Goal:** v2 SPA works excellently in iOS mobile Safari (reference: iPhone 14 Pro, 393×852, dvh-aware).
**Cadence:** Autonomous /loop iteration every 20 min. One subagent PR per iteration.

## Phase status

**Phase 1: SHIPPED.** PR [#1326](https://github.com/canalesb93/breadbox/pull/1326) merged into main with 12 fixes (MOBILE-1..MOBILE-21). Sprint branch was deleted on origin, then recreated from main for continued Phase 2 work.

Phase 1 shipped fixes (now in main):
- Sidebar close-on-tap, popover/select/dropdown collision + width clamps, 44pt tap targets via `pointer-coarse:` pseudo, `viewport-fit=cover` + safe-area, table scroll-shadow + iOS momentum, dvw/dvh viewport units, PageHeader wrap + FormFooter stacking, agent-form CSS-order reshuffle, empty-state width + error pre momentum, transactions filter chip rail, settings tab scroll.

## Workflow — PROACTIVE MODE

The loop NEVER reports idle. Every iteration must produce a merge, a dispatch, or a scout-then-dispatch chain.

Priority ladder — try each tier until something dispatches:
- **T1 User-reported bugs** (highest)
- **T2 Backlog items in state doc** (P0/P1/P2 including deferred)
- **T3 Fresh scout on a previously-unaudited surface** (rotate; don't repeat within 5 iterations)
- **T4 Polish on existing patterns** — extend `scroll-shadow-x`, hunt straggler `100vh`/`100vw`, verify `pointer-coarse:` on icon-button consumers, safe-area on dialogs, FormFooter pattern on forms
- **T5 Mobile a11y** — ARIA, focus traps, reduced-motion, contrast, screen-reader labels
- **T6 Sandbox completeness** — every component+variant exercised in `web/src/sandbox/sections/*`
- **T7 Documentation drift** — `.claude/rules/v2-frontend.md` codifies new patterns

Each iteration:
1. Sync sprint branch, merge ready PRs.
2. Pick from priority ladder.
3. Spawn ONE subagent in worktree isolation. Brief includes file paths + iOS quirks + visual evidence requirement via img402.dev.
4. Record which tier the work came from in state doc.
5. When backlog → main accumulates 3+ merged PRs, open a fresh `sprint → main` bundle PR.

## iOS Safari quirks reference

- `100vh` is broken; prefer `100dvh` / `100svh`. `100vw` similarly; prefer `100dvw` for floating containers.
- Inputs with `font-size < 16px` auto-zoom on focus. Always ≥16px on inputs.
- Tap targets < 44×44pt fail Apple HIG. Use `@media (pointer: coarse)` for touch-only rules (NOT viewport queries).
- `env(safe-area-inset-*)` requires `viewport-fit=cover` in `<meta viewport>`.
- Sticky elements interact poorly with the keyboard — test forms.
- Popovers/dropdowns must respect viewport bounds (Radix `collisionPadding`).
- Mobile sheet is shadcn Sidebar's built-in behavior (`useSidebar().openMobile`).
- Reuse `scroll-shadow-x` @utility from globals.css for any horizontal-scroll container.

## Backlog (Phase 2 — SPA-pitfall audit, iter ~14)


## Backlog (T3 scout — rules / agents / prompts, iter ~17)

- [ ] **MOBILE-37 (HIGH)** Agent runs table column widths explode beyond viewport on iOS. `web/src/routes/agents.runs.tsx` (~lines 303-388). Rigid `w-[22%] min-w-[160px]` + 8 fixed columns ⇒ horizontal scroll required. Verify whether the existing `scroll-shadow-x` from Table primitive helps; if not, hide-on-mobile or collapse columns into a Metrics expander.
- [ ] **MOBILE-39 (MEDIUM)** Rules filter toolbar three fixed-width selects wrap awkwardly. `web/src/routes/rules.tsx` (~line 290). Apply chip-rail pattern from #1324 (`max-sm:scroll-shadow-x max-sm:flex-nowrap max-sm:overflow-x-auto`).
- [ ] **MOBILE-41 (LOW)** Command palette input `h-11` vs `h-12` threshold from #1317. `web/src/components/command-palette.tsx` (~line 218). Debatable; bumping to `h-12` for consistency.
- [ ] **MOBILE-42 (LOW)** `detail-sheet-header.tsx` missing safe-area-top in iPhone landscape with notch. `web/src/components/detail-sheet-header.tsx` (~lines 54-59). `pt-5` → `pt-[calc(1.25rem+env(safe-area-inset-top))]`.

## Deferred / low-value (won't fix without evidence)

- **scroll-padding-top for anchor links** — niche, no anchor-link UX in the app today.
- **Icon-only button aria-label audit** — already mostly correct; only outliers in `calendar.tsx` day grid (low impact, Radix handles).
- **`role="alert"` on FormMessage** — Sonner toasts already announce form errors.
- **`gcTime` tuning** — speculative; current 5min default is fine.

**Carried over from Phase 1 (deferred):**
- [ ] **MOBILE-8** Sticky `<main>` header may overlap iOS keyboard. `web/src/routes/__root.tsx`. Requires real-device validation; no simulator fully reproduces keyboard occlusion.
- [ ] **MOBILE-19** HeroGrid 20px padding feels cramped on 375px. `web/src/components/hero-grid.tsx`. Subjective — defer until visual evidence shows a clear problem.

**New (T1 — user-reported):**
- [ ] **MOBILE-26** iOS Safari swipe-back gesture freezes / shows blank page. Tap-back works fine. _(in flight)_
  - Likely cause: bfcache restore with no `pageshow` handler — TanStack Query refetches on focus, a stale-session 401 fires `navigate({ to: "/login" })` mid-swipe-animation. SPA is bfcache-eligible (no `unload`/WS/etc.) but doesn't react when restored.
  - Files: `web/src/main.tsx`, `web/src/routes/__root.tsx` (401 handler).
  - Fix: `pageshow` listener with `event.persisted` check that calls `router.invalidate()` + `queryClient.invalidateQueries()`; optionally gate the 401 redirect on `document.visibilityState === "visible"`.

**Closed by audit:**
- ✅ **MOBILE-API-COPY (closed as already-handled)** Scout flagged that API key copy on iOS might silently fail. Verified: `onCopy` at `api-key-created.tsx:47` already has try/catch with a clear error toast ("Couldn't access the clipboard. Select the value and copy manually.") and the readonly Input has `onFocus={e => e.currentTarget.select()}` for manual-copy ergonomics. No code change required.
- ✅ **MOBILE-24 (closed as non-issue)** Scout flagged CSV column-mapping labels as wrapping aggressively on narrow viewports. Verified: `grid-cols-1 sm:grid-cols-2` means mobile gets ONE column per row — each ColumnSelect has the full 375px row width and the `text-xs` label easily fits "Separate Debit and Credit" in one line. Two-column wrap is a tablet+ concern where verticality is cheap. No code change.
- ✅ **MOBILE-25 (closed as non-issue)** Scout flagged file input with `className="hidden"` as potentially not opening picker on iOS. Verified: `<label htmlFor="csv-file">` + `<input type="file" className="hidden">` IS the canonical React/Tailwind pattern and iOS Safari opens the picker correctly when you tap the associated label. `display:none` removes from layout but preserves label-association interactivity. No code change.

## In-flight PRs

- **PR #1334** sprint→main Phase 2 bundle. **Awaiting user merge** — now includes #1328, #1330, #1331, #1332, #1333, #1335, #1336, #1337, #1338, #1339 (state-doc merge included).
- **fix/mobile-overscroll-contain** (subagent `a9ce28c0`) — **MOBILE-34 + MOBILE-40 (T2 MEDIUM bundle)**. Adds `overscroll-contain` to the Table primitive wrapper and the transcript-viewer `<pre>` to block iOS pull-to-refresh / back-swipe leaks from inner scroll containers. PR # TBD.

## Completed (Phase 2 — direct-to-main)

- ✅ **MOBILE-26** iOS swipe-back bfcache fix — PR #1329 merged into MAIN per user auth (`34dee658`). Adds `pageshow` listener that calls `router.invalidate()` + `queryClient.invalidateQueries()` on `event.persisted === true`, eliminating the freeze/blank-page race between bfcache restore and stale-query 401 redirect.

## Completed (Phase 2)

- ✅ **MOBILE-22/23** Button stacking polish — PR #1328 merged into sprint branch (`543599c8`). API-key Copy button gains `w-full sm:w-auto`; disconnect confirmation rewrapped as `flex-col-reverse sm:flex-row` with destructive Disconnect button on top + full-width-when-stacked. Follows the FormFooter pattern from #1321.
- ✅ **MOBILE-27** Mobile navbar blur — PR #1330 merged into sprint branch (`70f8518d`). `<header>` now uses solid `bg-background` at <640px (no `backdrop-blur`, no translucency), restoring the glassy look at sm+ via `sm:backdrop-blur` / `sm:bg-background/95`. Eliminates the visible seam between solid safe-area zone and previously-blurred header on iOS.
- ✅ **MOBILE-28** Viewport-unit polish (T4) — PR #1331 merged into sprint branch (`84bdb932`). 5 viewport-unit straggler swaps: `min-h-screen` → `min-h-dvh` on auth-shell wrapper + grid; `min-h-svh` → `min-h-dvh` on sidebar outer wrapper; `max-w-[calc(100vw-1rem)]` → `max-w-[calc(100dvw-1rem)]` in popover/select/dropdown content clamps. Finishes the dvh/dvw family.
- ✅ **MOBILE-29** iOS web-app shell polish — PR #1332 merged into sprint branch (`658e69f6`). Adds iOS PWA meta tags (`apple-mobile-web-app-capable=yes`, `status-bar-style=black-translucent`, `apple-mobile-web-app-title=Breadbox`, `apple-touch-icon` → favicon.svg), inline cold-load splash in `<div id="root">` (auto-vanishes when React hydrates, respects `prefers-reduced-motion` and `prefers-color-scheme`), and global `-webkit-tap-highlight-color: transparent`.
- ✅ **MOBILE-30** 401 visibility-gate — PR #1333 merged into sprint branch (`73e53940`). `AuthenticatedGate` in `__root.tsx` now defers the redirect-to-login while `document.visibilityState !== "visible"`, attaching a `visibilitychange` listener that fires the redirect when the user re-engages. Closes the residual bfcache-restore race left by PR #1329; cleanup-on-unmount included.
- ✅ **MOBILE-35** Per-row CategoryPicker lazy body — PR #1335 merged into sprint branch (`fb8439c3`). Splits `CategoryPicker` into a lightweight always-mounted shell (just `useState(open)` + trigger) and a `PickerBody` that mounts only when `open === true` and owns `useCategoryEditor` (the mutation hook). Audit's actual finding: 50× `useMutation` observers per page (not popover content) were the leak. Reduces transactions-list React memory, improving iOS Safari bfcache eligibility.
- ✅ **MOBILE-36** Prompts add-block dialog footer stacking — PR #1336 merged into sprint branch (`943dec09`). Inner action wrapper rewrapped as `flex w-full flex-col-reverse gap-2 sm:w-auto sm:flex-row sm:items-center` so "Done" (affirmative) sits on top on mobile per the #1321 convention.
- ✅ **MOBILE-31** Global `prefers-reduced-motion` (T2 HIGH a11y) — PR #1337 merged into sprint branch (`46323665`). Adds one `@media (prefers-reduced-motion: reduce)` block to `globals.css` using the CSS-tricks pattern (compress animation/transition to 0.01ms so `animationend` handlers still fire). Covers ~51 `animate-spin` usages + all shadcn primitive transitions without touching call sites. Cold-load splash (#1332) retains its own per-element `animation: none` override.
- ✅ **MOBILE-32/33** iOS form ergonomics (T2 HIGH/MEDIUM) — PR #1338 merged into sprint branch (`d00c6305`). `SearchInput` gets defaults (`inputMode="search"`, `enterKeyHint="search"`, `autoCapitalize="none"`, `autoCorrect="off"`, `spellCheck={false}`) so every search consumer benefits. Numeric inputs (agent `max_turns`, `budget_usd_cents`, link-account tolerance, rule values) get `inputMode="numeric|decimal"`. Identifier fields (API key name/prefix, rule values) get autocorrect/autocapitalize off.
- ✅ **MOBILE-38** Prompts builder mobile layout (T2 HIGH) — PR #1339 merged into sprint branch (`d6ada115`). `grid grid-cols-[10rem_1fr]` swapped to `flex flex-col sm:grid sm:grid-cols-[10rem_1fr]`; nav becomes a horizontal scroll-shadow chip rail on `<sm` with dividers flipped from horizontal rules to vertical separators between pills. Pattern matches #1324 (transactions filter) / MOBILE-21 (settings tabs).

## Notes for next iteration

- Backlog is intentionally thin. If nothing actionable on a fire, do a focused scout on a previously-unaudited flow (auth/setup-account/CSV import/connection creation) rather than dispatching a pointless PR.
- New user-reported bugs ALWAYS take priority over deferred / scout items.
- The `sprint/mobile-ios-safari` branch tip and `main` should diverge only by in-flight PRs; if it gets stale by >24h with no PRs, hard-reset it to `origin/main` to keep the diff base honest.

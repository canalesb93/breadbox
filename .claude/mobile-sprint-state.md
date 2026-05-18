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

## Backlog (Phase 2)

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

_(none)_

## Completed (Phase 2 — direct-to-main)

- ✅ **MOBILE-26** iOS swipe-back bfcache fix — PR #1329 merged into MAIN per user auth (`34dee658`). Adds `pageshow` listener that calls `router.invalidate()` + `queryClient.invalidateQueries()` on `event.persisted === true`, eliminating the freeze/blank-page race between bfcache restore and stale-query 401 redirect.

## Completed (Phase 2)

- ✅ **MOBILE-22/23** Button stacking polish — PR #1328 merged into sprint branch (`543599c8`). API-key Copy button gains `w-full sm:w-auto`; disconnect confirmation rewrapped as `flex-col-reverse sm:flex-row` with destructive Disconnect button on top + full-width-when-stacked. Follows the FormFooter pattern from #1321.
- ✅ **MOBILE-27** Mobile navbar blur — PR #1330 merged into sprint branch (`70f8518d`). `<header>` now uses solid `bg-background` at <640px (no `backdrop-blur`, no translucency), restoring the glassy look at sm+ via `sm:backdrop-blur` / `sm:bg-background/95`. Eliminates the visible seam between solid safe-area zone and previously-blurred header on iOS.
- ✅ **MOBILE-28** Viewport-unit polish (T4) — PR #1331 merged into sprint branch (`84bdb932`). 5 viewport-unit straggler swaps: `min-h-screen` → `min-h-dvh` on auth-shell wrapper + grid; `min-h-svh` → `min-h-dvh` on sidebar outer wrapper; `max-w-[calc(100vw-1rem)]` → `max-w-[calc(100dvw-1rem)]` in popover/select/dropdown content clamps. Finishes the dvh/dvw family.

## Notes for next iteration

- Backlog is intentionally thin. If nothing actionable on a fire, do a focused scout on a previously-unaudited flow (auth/setup-account/CSV import/connection creation) rather than dispatching a pointless PR.
- New user-reported bugs ALWAYS take priority over deferred / scout items.
- The `sprint/mobile-ios-safari` branch tip and `main` should diverge only by in-flight PRs; if it gets stale by >24h with no PRs, hard-reset it to `origin/main` to keep the diff base honest.

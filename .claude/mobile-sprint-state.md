# Mobile Responsiveness Sprint — v2 SPA / iOS Safari (Phase 2)

**Branch:** `sprint/mobile-ios-safari`
**Goal:** v2 SPA works excellently in iOS mobile Safari (reference: iPhone 14 Pro, 393×852, dvh-aware).
**Cadence:** Autonomous /loop iteration every 20 min. One subagent PR per iteration.

## Phase status

**Phase 1: SHIPPED.** PR [#1326](https://github.com/canalesb93/breadbox/pull/1326) merged into main with 12 fixes (MOBILE-1..MOBILE-21). Sprint branch was deleted on origin, then recreated from main for continued Phase 2 work.

Phase 1 shipped fixes (now in main):
- Sidebar close-on-tap, popover/select/dropdown collision + width clamps, 44pt tap targets via `pointer-coarse:` pseudo, `viewport-fit=cover` + safe-area, table scroll-shadow + iOS momentum, dvw/dvh viewport units, PageHeader wrap + FormFooter stacking, agent-form CSS-order reshuffle, empty-state width + error pre momentum, transactions filter chip rail, settings tab scroll.

## Workflow (unchanged)

1. Loop wakes → sync branch → merge ready PRs → pick next backlog item.
2. Spawn ONE subagent in worktree isolation. Brief includes file paths + iOS quirks.
3. Subagent opens PR against this branch with `img402.dev` screenshots at 375×812 / 768×1024 / 1280×800.
4. Orchestrator reviews next iteration. Merges when green. Never merge to `main` directly without explicit auth.
5. When backlog is meaningfully accumulated (3+ merged), open a fresh `sprint → main` bundle PR.

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

**New (Phase 2 scout, iter 12):**
- [ ] **MOBILE-22** API key copy button isn't full-width when stacked — `web/src/routes/api-key-created.tsx` (~lines 91-112). _(in flight, see below)_
- [ ] **MOBILE-23** Disconnect confirmation buttons cramped on mobile — `web/src/routes/connection-detail.tsx` (~lines 379-401). _(in flight, see below)_
- [ ] **MOBILE-24** CSV column-mapping label wraps aggressively on narrow viewports — `web/src/features/connections/csv-import-form.tsx` (~lines 422-478). Needs visual evidence before fix.
- [ ] **MOBILE-25** CSV drag-drop file input may not open picker reliably on iOS — `web/src/features/connections/csv-import-form.tsx` (~lines 166-190). Needs simulator verification.

**Closed by audit:**
- ✅ **MOBILE-API-COPY (closed as already-handled)** Scout flagged that API key copy on iOS might silently fail. Verified: `onCopy` at `api-key-created.tsx:47` already has try/catch with a clear error toast ("Couldn't access the clipboard. Select the value and copy manually.") and the readonly Input has `onFocus={e => e.currentTarget.select()}` for manual-copy ergonomics. No code change required.

## In-flight PRs

- **fix/mobile-button-stacking-polish** (subagent `a0ad94fa`) — bundles MOBILE-22 + MOBILE-23. Adds `w-full sm:w-auto` to api-key Copy button; rewraps disconnect confirmation as `flex-col-reverse sm:flex-row` (destructive on top) with full-width-when-stacked buttons. Follows the FormFooter pattern from PR #1321. PR # TBD.

## Notes for next iteration

- Backlog is intentionally thin. If nothing actionable on a fire, do a focused scout on a previously-unaudited flow (auth/setup-account/CSV import/connection creation) rather than dispatching a pointless PR.
- New user-reported bugs ALWAYS take priority over deferred / scout items.
- The `sprint/mobile-ios-safari` branch tip and `main` should diverge only by in-flight PRs; if it gets stale by >24h with no PRs, hard-reset it to `origin/main` to keep the diff base honest.

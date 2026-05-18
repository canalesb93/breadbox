# Mobile Responsiveness Sprint — v2 SPA / iOS Safari

**Branch:** `sprint/mobile-ios-safari`
**Goal:** v2 SPA works excellently in iOS mobile Safari (reference: iPhone 14 Pro, 393×852, dvh-aware).
**Cadence:** Autonomous /loop iteration every 30 min. One subagent PR per iteration.

## Workflow

1. Loop wakes → sync branch → merge ready PRs → pick next backlog item.
2. Spawn ONE subagent in worktree isolation. Brief includes file paths + iOS quirks.
3. Subagent opens PR against this branch with `img402.dev` screenshots at 375×812 / 768×1024 / 1280×800.
4. Orchestrator reviews next iteration. Merges when green. Never merge to `main`.

## iOS Safari quirks to keep in mind

- `100vh` is broken (counts the address bar). Prefer `100dvh` / `100svh`.
- Inputs with `font-size < 16px` auto-zoom on focus. Always ≥16px on inputs/textareas/selects.
- Tap targets < 44×44pt are rejected by Apple HIG.
- `env(safe-area-inset-*)` for notch/home indicator.
- Sticky elements behave oddly with the keyboard — test forms.
- Popovers/dropdowns must respect viewport bounds (Radix `collisionPadding`).
- Sheet on mobile is shadcn Sidebar's built-in behavior (`useSidebar().openMobile`).

## Backlog (P0)

_(empty — both user-reported P0 bugs shipped)_

## Backlog (P1 — re-scout)

- [ ] **MOBILE-17** Empty state `max-w-sm` (384px) exceeds 375px viewport. `web/src/components/empty-state.tsx` (~line 73). Bump to `max-w-xs` or rely on viewport padding.
- [ ] **MOBILE-18** Error page `<pre>` block lacks iOS momentum. `web/src/routes/error.tsx` (~line 152). Add `[-webkit-overflow-scrolling:touch]` (same pattern as #1319 table fix).
- [ ] **MOBILE-19** HeroGrid tight 10px padding on 375px. `web/src/components/hero-grid.tsx` (~lines 43-45). `px-5 sm:px-7` feels cramped. Consider `px-4 sm:px-5` (more breathing room actually means less mobile padding) — verify against design intent.
- [ ] **MOBILE-20** Transactions toolbar wraps pills above sort/select buttons. `web/src/features/transactions/transactions-toolbar.tsx` (~line 202). `flex flex-wrap` pushes sort/select to a third row when there are several filter chips. Consider horizontal-scroll chip rail on mobile or fixed positioning.

## Backlog (P1-P2 — scout-seeded)

- [ ] **MOBILE-8** Sticky `<main>` header may overlap iOS keyboard / backdrop-blur glitches. `web/src/routes/__root.tsx` (`sticky top-0`). Validate in real device. (Header safe-area top inset handled in MOBILE-7 PR; keyboard-overlap concern still open.)

> Items touching `web/src/components/ui/*` should be solved by composing over the primitive (wrap, prop-pass) — only edit `ui/*` if there is no other path. Use `/shadcn` skill to verify if a primitive can be upgraded via prop instead.

## In-flight PRs

- **fix/mobile-empty-state-and-error-pre** (subagent `aaf9e92d`) — MOBILE-17 + MOBILE-18. `max-w-sm` → `max-w-xs` on empty-state description; `[-webkit-overflow-scrolling:touch]` added to error page `<pre>`. PR # TBD.

## Completed

- ✅ **MOBILE-1** Sidebar close-on-tap — PR #1312 merged (`ab6dd40a`). `useSidebar().setOpenMobile(false)` wired into `NavMain` (link + modal) and `NavUser` (logout). Desktop unaffected.
- ✅ **MOBILE-2/3/4** Popover/Select/DropdownMenu onscreen — PR #1316 merged (`cfd98db9`). Added `collisionPadding={8}` (overridable default), `max-w-[calc(100vw-1rem)]`, and `max-h-[min(60vh, available-height)]` to all three primitive Content components.
- ✅ **MOBILE-5/12** 44pt tap targets — PR #1317 merged (`3b2f91f5`). Tailwind v4 `pointer-coarse:` variant adds invisible 44×44 tap zone via `::before` pseudo to all icon Button sizes; CommandInput bumps to `h-12` on touch. Desktop visuals unchanged (verified via mouse `pointer: fine`).
- ✅ **MOBILE-6 (verified non-issue, closed)** Input font-size auto-zoom — `web/src/components/ui/input.tsx` and `textarea.tsx` already use `text-base` (16px) on mobile and `md:text-sm` (14px) at desktop ≥768px. iOS auto-zoom only fires on inputs <16px on mobile viewports; both meet the threshold. No code change required.
- ✅ **MOBILE-7/9** iOS safe-area pass — PR #1318 merged (`78c814d9`). Added `viewport-fit=cover` to `web/index.html` (activates `env(safe-area-inset-*)`), per-side safe-area padding to `SheetContent`, mobile sidebar inner content respects bottom inset, and `pt-[env(safe-area-inset-top)]` on the sticky `<main>` header so it sits below the notch/Dynamic Island.
- ✅ **MOBILE-11** Table scroll affordance — PR #1319 merged (`66d7d8d2`). Added `scroll-shadow-x` utility (4-layer CSS-only gradient, dark-mode aware) + `[-webkit-overflow-scrolling:touch]` to the `Table` primitive's container. Clipped tables now show a fade indicating scrollability; flat right edge confusion resolved.
- ✅ **MOBILE-10/13** Dynamic viewport units — PR #1320 merged (`b3411af3`). Selection action bar swapped `100vw` → `100dvw` so it stays within the dynamic viewport as iOS chrome collapses; desktop sidebar swapped `h-svh` → `h-full` (parent-relative) so it fills exactly its container instead of the worst-case small-viewport height.
- ✅ **MOBILE-15/16** Header + footer mobile stacking — PR #1321 merged (`669cfed6`). `PageHeader` actions cluster now wraps on mobile (`flex-wrap` + `sm:flex-nowrap sm:shrink-0`) so pages with multiple action buttons no longer overflow horizontally. `FormFooter` action cluster goes full-width column with primary on top (`flex-col-reverse w-full`) at <640px, restoring inline row on ≥640px — affirmative-on-top matches iOS / Material.
- ✅ **MOBILE-14** Agent form mobile order — PR #1322 merged (`1bad8159`). CSS `order-first` / `md:order-none` reorders the agent-form cards so the operational knobs card (Model, Schedule, Tool scope, Allowed tools, Max turns, Budget) appears ABOVE the long prompt body on <768px. DOM and tab order unchanged.

## Notes for next iteration

- Always `git fetch && git pull` before starting.
- Run `gh pr list --base sprint/mobile-ios-safari --state open` to see in-flight work.
- Run `gh pr list --base sprint/mobile-ios-safari --state merged` to track velocity.
- If a subagent PR has failing CI, comment summary, skip merging, mark as blocked here.

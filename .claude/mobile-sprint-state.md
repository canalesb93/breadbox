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

## Backlog (P1-P2 — scout-seeded)

- [ ] **MOBILE-6** Input font-size triggers Safari auto-zoom. `web/src/components/ui/input.tsx` (`text-base` then `md:text-sm`). Keep ≥16px on mobile viewports.
- [ ] **MOBILE-7** Viewport meta missing safe-area + zoom intent. `web/index.html`. Add `viewport-fit=cover`, leave `user-scalable=yes` (accessibility).
- [ ] **MOBILE-8** Sticky `<main>` header may overlap iOS keyboard / backdrop-blur glitches. `web/src/routes/__root.tsx` (`sticky top-0`). Validate in real device.
- [ ] **MOBILE-9** Sheet content not safe-area aware. `web/src/components/ui/sheet.tsx` (lines 62-71). Add `env(safe-area-inset-*)` padding (compose, don't edit ui/*).
- [ ] **MOBILE-10** `100vw` math in `selection-action-bar.tsx` causes occasional horizontal scroll on iOS. `web/src/features/connections/selection-action-bar.tsx` (line 121). Replace with `w-full` + parent constraint.
- [ ] **MOBILE-11** Table horizontal scroll lacks touch affordance. `web/src/components/ui/table.tsx`, `web/src/components/data-table.tsx`. Add scroll-shadow indicator + `-webkit-overflow-scrolling: touch`.
- [ ] **MOBILE-13** Sidebar uses `h-svh` instead of `h-full`. `web/src/components/ui/sidebar.tsx` (line 230). Switch to `h-full` with parent height constraint.

> Items touching `web/src/components/ui/*` should be solved by composing over the primitive (wrap, prop-pass) — only edit `ui/*` if there is no other path. Use `/shadcn` skill to verify if a primitive can be upgraded via prop instead.

## In-flight PRs

_(none)_

## Completed

- ✅ **MOBILE-1** Sidebar close-on-tap — PR #1312 merged (`ab6dd40a`). `useSidebar().setOpenMobile(false)` wired into `NavMain` (link + modal) and `NavUser` (logout). Desktop unaffected.
- ✅ **MOBILE-2/3/4** Popover/Select/DropdownMenu onscreen — PR #1316 merged (`cfd98db9`). Added `collisionPadding={8}` (overridable default), `max-w-[calc(100vw-1rem)]`, and `max-h-[min(60vh, available-height)]` to all three primitive Content components.
- ✅ **MOBILE-5/12** 44pt tap targets — PR #1317 merged (`3b2f91f5`). Tailwind v4 `pointer-coarse:` variant adds invisible 44×44 tap zone via `::before` pseudo to all icon Button sizes; CommandInput bumps to `h-12` on touch. Desktop visuals unchanged (verified via mouse `pointer: fine`).

## Notes for next iteration

- Always `git fetch && git pull` before starting.
- Run `gh pr list --base sprint/mobile-ios-safari --state open` to see in-flight work.
- Run `gh pr list --base sprint/mobile-ios-safari --state merged` to track velocity.
- If a subagent PR has failing CI, comment summary, skip merging, mark as blocked here.

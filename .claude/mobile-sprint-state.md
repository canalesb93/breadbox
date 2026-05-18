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

## Backlog (P0 — broken / user-reported)

- [ ] **MOBILE-1** Sidebar stays open after tapping a nav item on mobile.
  - Files: `web/src/components/nav-main.tsx`, `web/src/components/app-sidebar.tsx`, `web/src/components/ui/sidebar.tsx`
  - Cause: `NavRow` for `kind: "link"` renders `<Link>` directly inside `SidebarMenuButton asChild` — no `setOpenMobile(false)` on tap. Modal items also miss this.
  - Fix: call `useSidebar().setOpenMobile(false)` on nav-link tap on mobile.
  - Status: PR not yet opened.

- [ ] **MOBILE-2** Menus / popovers / pickers render offscreen on mobile.
  - Files: audit `Popover`, `DropdownMenu`, `Command`, `Combobox`, `Select` usages across `web/src/`.
  - Cause: missing `collisionPadding`, fixed `side`/`align` that overflow viewport, or content wider than `100vw - 2 * padding`.
  - Fix: tighten Radix props (`collisionPadding`, `sideOffset`), constrain widths with `max-w-[calc(100vw-2rem)]`, prefer `Sheet` on mobile for category-picker-class pickers.

## Backlog (P1-P2 — to be seeded by scout)

_Pending: Explore agent is scouting routes for additional issues. Will be folded in next iteration._

## In-flight PRs

_(none yet)_

## Completed

_(none yet)_

## Notes for next iteration

- Always `git fetch && git pull` before starting.
- Run `gh pr list --base sprint/mobile-ios-safari --state open` to see in-flight work.
- Run `gh pr list --base sprint/mobile-ios-safari --state merged` to track velocity.
- If a subagent PR has failing CI, comment summary, skip merging, mark as blocked here.

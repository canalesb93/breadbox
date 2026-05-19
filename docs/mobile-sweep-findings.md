# Mobile Safari sweep — findings (May 2026)

Output of Phase 1.5 of the mobile-Safari-perfect sweep. Drove webkit (Playwright `devices['iPhone SE' / 'iPhone 13' / 'iPhone 15 Pro Max']`) through 16 authed v2 routes, captured screenshots, console errors, layout overflow, and interactive-element hit-area sizes.

Sweep script: `web/scripts/mobile-sweep.ts` (`PORT=8080 bun run mobile-sweep`).
Latest output: `tmp/mobile-sweep-<stamp>/findings.md` (gitignored screenshots).

## Headline findings

### 1. Horizontal overflow on table-with-actions routes — **fix in Phase 2 PR #1**

Four routes overflow visibly on iPhone SE (and the worst two also overflow on every iPhone viewport up to 15 Pro Max). All share the same DataTable + trailing kebab-actions column pattern, where the actions column sits to the right of the visible viewport.

| route                | iPhone SE | iPhone 13 | iPhone 15 PM |
| -------------------- | --------: | --------: | -----------: |
| `/v2/tags`           | **244px** | **174px** |    **134px** |
| `/v2/api-keys`       | **135px** |  **65px** |     **25px** |
| `/v2/agents`         |  **62px** |       0px |          0px |
| `/v2/categories`     |  **39px** |       0px |          0px |
| `/v2/` (home)        |  **81px** |  **11px** |          0px |

Tags + api-keys are the worst because the table is rendered at fixed widths the viewport can't accommodate. Agents/categories overflow only on the narrowest device.

**Fix**: route the wide tables through a `scroll-shadow-x overflow-x-auto` container (the pattern is already documented in `.claude/rules/v2-frontend.md` under "Horizontal-scroll affordance") so the actions column scrolls into view rather than spilling off. Or collapse the trailing actions into a row-tap detail sheet on mobile.

The 81px on `/v2/` is likely a hidden offscreen element (sidebar drawer) — visually the layout looks clean — but worth verifying once the easy wins land.

### 2. Title truncation on `/v2/agents` is too aggressive

Agent card titles render as `M...`, `Spe...`, `Routi...`, `Quick ...` — 3–5 chars before ellipsis. Likely caused by a flex container missing `min-w-0` paired with a long peer (badge + `READ WRITE` chip). Fix in the same PR as the agents overflow.

### 3. Zero JS errors across every route × viewport — excellent baseline

No `pageerror`s, no real console errors (after filtering benign 401s from the auth probe). The SPA's React/TanStack/Tailwind plumbing is solid; remaining work is layout, not stability.

### 4. Tap-target detection needs refinement (and the data is noisier than the issue)

The sweep flags 6–52 interactive elements per route as having a hit area < 44pt. The detector measures the bounding rect ∪ the `::before` pseudo-element rect (which is how the v2 Button primitive enlarges icon-only hit areas via the `pointer-coarse:before:size-11` recipe).

In practice the listing surfaces three kinds of elements:
- **Compact text buttons in toolbars** (e.g. `<Button variant="ghost" size="sm">View all</Button>` = 80×28). The `pointer-coarse` recipe deliberately doesn't apply here — `size="sm"` text buttons aren't icon-only — but on iPhone SE they're still small. Decision: bump `size="sm"` text buttons to `size="default"` (h-9) on mobile, or accept and document. **Open question.**
- **Compact inputs with submit affordance** (search input clear `×`, command palette trigger) — usually fine because text/glyph indicates affordance, but worth a tap-area pass.
- **Detector blind spots** — the SidebarTrigger reports 28×28 even though it uses `size="icon"` (which should attach the recipe). Two probes were inconclusive about whether `tailwind-merge` strips the recipe under className override, or whether the production bundle was built before the recipe landed. **Open question** — verify before counting these as real defects.

A dedicated follow-up should: (a) confirm the recipe survives in current builds, (b) update the sweep to ignore confirmed-recipe-protected buttons, (c) re-run and produce a clean list of REAL tap-target offenders.

## What the data does NOT show

- Real-device performance (synthetic webkit lacks Touch Bar latency, GPU compositing quirks).
- iOS keyboard reveal / hide interactions — Playwright doesn't simulate the keyboard.
- Bfcache restore — would need a multi-page navigation test.
- Visual regression — screenshots are captured but not pixel-diffed.
- Hover affordances — webkit's mobile emulation forces `pointer: coarse` + `hover: none`.

Each of these is a real follow-up; none should block the Phase 2 overflow fixes.

## Phase 2 — proposed PR sequence

Ordered by user impact:

1. **Table overflow on `/v2/tags` and `/v2/api-keys`** — apply the `scroll-shadow-x` pattern or collapse the kebab into a row-tap sheet.
2. **`/v2/agents` overflow + title truncation** — fix `min-w-0` chain on the title row; verify the action icons aren't escaping the card on iPhone SE.
3. **`/v2/categories` overflow** — same family of fix as #1, single-PR change.
4. **Tap-target sweep, take 2** — refined detector + targeted bumps.
5. **Confirm-before-leave wiring** — apply the existing `useConfirmLeave` hook to dirty forms in `agents/$slug/edit`, `rules` editor, etc.
6. **View Transitions** — wrap route transitions with `startViewTransitionIfSupported`, scoped to two high-traffic transitions first (transactions list → detail; agents list → detail).

Each PR ships independently; CI must be green at every tip per the stack rules.

## How to reproduce

```
make dev                   # backend on :8080
cd web && bun install && bun run e2e:install
PORT=8080 bun run mobile-sweep   # produces tmp/mobile-sweep-<stamp>/findings.md
```

`tmp/` is gitignored; commit only this summary doc, not the raw output.

# Mobile QA Matrix

Regression checklist for the v2 SPA before tagging a release. The v2 SPA is browsed from iOS Safari as a first-class target (Add-to-Home-Screen, swipe-back, dynamic chrome), so every release should walk the route surface at the four representative viewports below.

**Supported baseline:** iOS 26.2 / iPhone Safari, iPadOS 26.2 / Safari, latest desktop Safari. Older versions render a soft-warn banner but still load.

Patterns that make these checks pass live in `.claude/rules/v2-frontend.md` under "Mobile / iOS Safari patterns" — when a pass surfaces a regression, fix it against that canon rather than re-inventing.

## Viewports to test

| Device                | CSS px        | Portrait | Landscape |
| --------------------- | ------------- | -------- | --------- |
| iPhone SE             | 375 × 667     | yes      | optional  |
| iPhone 13             | 390 × 844     | yes      | optional  |
| iPhone 15 Pro Max     | 430 × 932     | yes      | yes (notch + safe-area edges) |
| iPad Mini             | 768 × 1024    | yes      | yes (drawer transitions to sidebar) |

Landscape matters most on iPhone 15 Pro Max (notch + home-indicator) and iPad Mini (where the sidebar layout flips). On the smaller iPhones, landscape is optional unless you're chasing a viewport-specific bug.

## Routes to walk

Each row is a routable page in `web/src/routes/`. Tick the box after a clean pass on that viewport. Detail routes (with `$id` / `$slug`) need a representative record to open — pick a category, transaction, tag, agent, etc. that exists in your dev DB.

| Route URL                          | SE 375 | 13 390 | 15PM 430 | iPad 768 |
| ---------------------------------- | :----: | :----: | :------: | :------: |
| `/v2/` (home)                      | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/login`                        | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/setup-account/<token>`        | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/transactions`                 | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/transactions/<id>`            | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/categories`                   | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/categories/new`               | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/categories/<id>`              | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/tags`                         | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/tags/new`                     | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/tags/<slug>`                  | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/connections`                  | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/connections/<id>`             | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/accounts`                     | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/accounts/<id>`                | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/api-keys`                     | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/api-keys/new`                 | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/api-keys/created`             | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/providers`                    | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/rules`                        | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/rules/new`                    | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/rules/<id>`                   | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/rules/<id>/edit`              | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/agents`                       | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/agents/new`                   | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/agents/runs`                  | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/agents/<slug>/edit`           | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/agents/<slug>/runs`           | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/prompts/build`                | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/sandbox`                      | [ ]    | [ ]    | [ ]      | [ ]      |
| `/v2/<unknown>` (not-found)        | [ ]    | [ ]    | [ ]      | [ ]      |
| error boundary                     | [ ]    | [ ]    | [ ]      | [ ]      |

Placeholder routes (any nav leaf without a `PAGE_OVERRIDES` entry — see `web/src/main.tsx`) render the shared `placeholder.tsx`. Walking one placeholder per release is enough; they all share the same shell.

## Interactions to verify on each pass

Each numbered check should hold on every viewport above. If one fails, file the regression with the route + viewport and link to the relevant pattern in `.claude/rules/v2-frontend.md`.

1. **No horizontal overflow on cold load.** The page paints once and the body never scrolls sideways. Watch for a stray fixed-width element bleeding past `100dvw`.
2. **Sidebar drawer behavior.** Hamburger opens the drawer; tapping the scrim closes it; tapping a nav item closes it before navigating; landscape iPad keeps the sidebar pinned (no drawer).
3. **Top safe-area clears the notch.** Status bar and notch never sit on top of header content. Sticky headers respect `env(safe-area-inset-top)`; full-screen dialogs use `mt-[env(safe-area-inset-top)]` on iPad-landscape.
4. **Bottom safe-area clears the home-indicator.** Floating action bars and sheet footers offset via `bottom-[max(1rem,env(safe-area-inset-bottom))]` so the home-indicator never overlaps a tap target. Long-scroll pages don't trap content under the indicator at the scroll-end.
5. **Sticky headers stay glued.** During rubber-band overscroll at the top or bottom, the sticky header doesn't decouple, double-render, or jitter.
6. **Tap targets ≥ 44pt.** Icon-only buttons, tag-chip × closers, and overflow-menu triggers all hit cleanly on first try. Buttons going through the Button primitive inherit the 44pt hit area on `(pointer: coarse)`; raw `<button>`s should apply the inline recipe.
7. **Inputs do not zoom on focus.** All `<input>`, `<select>`, `<textarea>` are at least 16px font-size on mobile — focusing them must not trigger Safari's auto-zoom.
8. **Correct iOS keyboard per input.** Email fields show the email keyboard, numeric/decimal fields show the number pad, search fields show the search layout, identifier fields show the standard keyboard with autocorrect off.
9. **Return-key glyph matches affordance.** `enterKeyHint` paints "go" on URL fields, "send" on composer textareas, "search" in `SearchInput`, "done" on form-end inputs. The glyph in the keyboard's bottom-right matches what tapping it will do.
10. **Identifier fields don't autocorrect.** Slugs, API key names, regex values, and other technical strings retain their typed value verbatim — no first-letter capitalization, no autocorrect substitution, no spellcheck underline.
11. **Filter pills scroll horizontally with shadow affordance.** On transactions and prompts-builder, the chip row scrolls left-right on phones with a faded gradient at the clipped edge. Rubber-band doesn't propagate to the page (`overscroll-contain`).
12. **Long lists scroll smoothly.** Transactions, sync logs, agent runs all momentum-scroll without dropped frames. No jank from per-row layout thrash.
13. **Detail sheets open and close.** Sheet content slides in with safe-area-aware padding; swipe-back from a detail page returns to the parent list with scroll position restored; swipe-back from a modal closes it without leaving the route.
14. **Bfcache restore refreshes data.** Open the SPA, navigate to an external URL (e.g. via `<a target="_blank">`), then swipe-back. The page repaints from bfcache, the `pageshow` listener invalidates router + queries, and you see fresh data — not stale numbers from before you left.
15. **Theme switch is flash-free.** Toggling system light/dark while the SPA is open transitions without a white flash; cold-loading in dark mode shows the dark splash inside `#root` until React hydrates.
16. **Add to Home Screen launches standalone.** From Safari's share sheet → Add to Home Screen → tap the icon. The app launches without Safari chrome (no URL bar, no tab strip), the splash respects the system color scheme, and the status bar style is `black-translucent`.
17. **Reduced motion is near-instant.** With Settings → Accessibility → Motion → Reduce Motion **on**, page transitions and modal open/close finish within ~10ms — no spinning, no extended slide-in, no parallax.

## How to run automated coverage

The Playwright e2e suite includes mobile-safari projects at the four viewports above. Run it from the worktree root:

```sh
cd web
bun run e2e
```

For one-off screenshot evidence — useful when attaching a responsive snapshot to a PR — use the validate script:

```sh
cd web
bun run validate /v2/transactions
```

Both fall back to headed Chromium when WebKit isn't available, but the mobile-safari projects are the canonical signal — Chromium catches layout regressions but misses iOS-specific bugs (rubber-band, safe-area, bfcache).

## When this matrix is wrong

The matrix is generated from inspection of `web/src/routes/`. If you add a route, add a row. If you remove a route, drop the row. The Playwright config is the runtime ground truth; this file is the human ground truth.

# Mobile QA Matrix

Regression checklist for the v2 SPA before tagging a release. The v2 SPA is browsed from iOS Safari as a first-class target (Add-to-Home-Screen, swipe-back, dynamic chrome), so every release should walk the route surface at the four representative viewports below.

**Supported baseline:** iOS 26.2 / iPhone Safari, iPadOS 26.2 / Safari, latest desktop Safari. Older versions render a soft-warn banner but still load.

Patterns that make these checks pass live in `.claude/rules/v2-frontend.md` under "Mobile / iOS Safari patterns" — when a pass surfaces a regression, fix it against that canon rather than re-inventing.

## Viewports to test

Playwright's `mobile-sweep` walks the six listed below on every run; manual passes should at least cover the four "yes" portrait rows + iPad landscape (which flips the sidebar from drawer to pinned).

| Device                       | CSS px        | Portrait | Landscape | Notes |
| ---------------------------- | ------------- | -------- | --------- | ----- |
| iPhone SE (1st-gen)          | 320 × 568     | yes      | optional  | The narrowest realistic mobile — catches min-content + slug-pill overflow |
| iPhone 13                    | 390 × 844     | yes      | yes       | Median modern phone |
| iPhone 15 Pro Max            | 430 × 932     | yes      | yes       | Widest phone — notch + home-indicator safe-area |
| iPad Mini                    | 768 × 1024    | yes      | yes       | Tablet portrait. md breakpoint active; sidebar visible |
| —                            | 1024 × 768    | —        | yes       | iPad Mini landscape. lg breakpoint active; 3-col grids activate |

Landscape matters for: iPhone 15 Pro Max (notch + home-indicator), iPad Mini (sidebar transitions from drawer to pinned at md+; 3-col agent form layout activates at lg+ on the wider landscape). iPhone SE 1st-gen is the narrowest realistic device and the canary for min-content overflow; if it passes, every other phone passes. iPhone 13 landscape (750×342) has a very short height — useful for keyboard-open / dropdown-clipping regressions.

## Routes to walk

Each row is a routable page in `web/src/routes/`. Tick the box after a clean pass on that viewport. Detail routes (with `$id` / `$slug`) need a representative record to open — pick a category, transaction, tag, agent, etc. that exists in your dev DB.

| Route URL                          | SE 320 | 13 390 | 15PM 430 | iPad 768 | iPad-L 1024 |
| ---------------------------------- | :----: | :----: | :------: | :------: | :---------: |
| `/v2/` (home)                      | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/login`                        | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/setup-account/<token>`        | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/transactions`                 | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/transactions/<id>`            | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/categories`                   | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/categories/new`               | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/categories/<id>`              | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/tags`                         | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/tags/new`                     | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/tags/<slug>`                  | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/connections`                  | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/connections/<id>`             | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/accounts`                     | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/accounts/<id>`                | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/api-keys`                     | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/api-keys/new`                 | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/api-keys/created`             | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/providers`                    | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/rules`                        | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/rules/new`                    | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/rules/<id>`                   | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/rules/<id>/edit`              | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/agents`                       | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/agents/new`                   | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/agents/runs`                  | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/agents/<slug>/edit`           | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/agents/<slug>/runs`           | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/prompts/build`                | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/sandbox`                      | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| `/v2/<unknown>` (not-found)        | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |
| error boundary                     | [ ]    | [ ]    | [ ]      | [ ]      | [ ]         |

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

## iPad-specific checks

The phone breakpoints (sm 640 / md 768) trigger differently on a tablet, and the sidebar drawer flips to a pinned sidebar at md+ when the device is wide enough. These passes catch breakpoint regressions that don't surface on phones.

1. **Sidebar is pinned (no drawer) at iPad portrait + landscape.** No hamburger control; the sidebar is always visible.
2. **3-column form layouts only activate at landscape.** On `/v2/agents/new` and `/v2/agents/<slug>/edit`, the page should stack to one column in iPad portrait (768) and split into a 2:1 grid only in iPad landscape (1024+). Squeezing 3 columns into 768 produces unreadable narrow inputs.
3. **Table columns still fit at iPad portrait.** `/v2/tags` and `/v2/api-keys` hide their less-essential columns (description, actor) below the `lg` breakpoint (1024) so the trailing kebab doesn't push off-screen with the sidebar visible.
4. **Detail-page Cards don't overflow.** Long select values (e.g. "Claude Sonnet 4.6 (balanced)") should respect their grid track. If a SelectTrigger has `whitespace-nowrap` and the parent isn't `min-w-0`, content can push past 1fr.
5. **PWA standalone mode is sensible on iPad.** When launched from home screen, the SPA fills the tablet without Safari chrome; orientation flips work without re-layout glitches.
6. **Form scroll behavior in landscape.** With the iOS keyboard visible (less likely on iPad but possible), input fields should scroll into view, not behind the keyboard.

## How to run automated coverage

Two complementary scripts. Both target webkit (Playwright's bundled WebKit ≈ Safari) and skip cleanly when the backend isn't reachable.

```sh
# Smoke test — auth + a couple of canonical golden paths
cd web && bun run e2e

# Sweep — walk every authed route at six viewports, capturing
# overflow, JS errors, and tap-target violations. Output to
# tmp/mobile-sweep-<stamp>/findings.md + per-route screenshots.
cd web && BASE_URL=http://localhost:6080 bun run mobile-sweep

# Memory sweep — drive list ↔ detail navigation N=20 times,
# look for structural DOM growth (a proxy for memory leaks
# since WebKit doesn't expose `performance.memory`).
cd web && BASE_URL=http://localhost:6080 bun run memory-sweep
```

For one-off screenshot evidence — useful when attaching a responsive snapshot to a PR — use the validate script:

```sh
cd web
bun run validate /v2/transactions
```

Both fall back to headed Chromium when WebKit isn't available, but the mobile-safari projects are the canonical signal — Chromium catches layout regressions but misses iOS-specific bugs (rubber-band, safe-area, bfcache).

## When this matrix is wrong

The matrix is generated from inspection of `web/src/routes/`. If you add a route, add a row. If you remove a route, drop the row. The Playwright config is the runtime ground truth; this file is the human ground truth.

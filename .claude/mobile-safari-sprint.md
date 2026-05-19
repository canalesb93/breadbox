# Mobile-Safari-perfect sprint

**Sprint branch (persistent, accumulates all iterations):** `mobile-safari/sprint`
**Worktree:** `.claude/worktrees/mobile-safari-playwright`
**Started:** 2026-05-19 off origin/main @ 5d16af1a (Phase 1) + e45aa673 (Phase 2)
**Loop cron:** `13,43 * * * *` (every 30 min; job id 109e97f4, durable)
**Authorization & workflow:** Ricardo has granted standing approval to open AND squash-merge iteration PRs INTO the sprint branch (NOT into main). At end of sprint, ONE final PR opens from `mobile-safari/sprint` → `main` for Ricardo to review the full feature in one place. **Do not enable GitHub auto-merge.** **Do not merge to main.**
**Per-iteration branches:** each iteration creates `mobile-safari/iter-N-<slug>` (e.g. `mobile-safari/iter-3-confirm-leave`) off the sprint branch, opens a PR into the sprint branch, merges, and is auto-deleted on merge. The sprint branch is never deleted.
**Driver:** `/loop 30m` autonomous continuation (cron job set up 2026-05-19).
**Local servers:** Vite dev server runs on :6080 (HMR), backend on :8080 (proxied for /api + /web/v1). `BASE_URL=http://localhost:6080 bun run mobile-sweep` validates changes.

## Goal

Perfect mobile support for the v2 SPA at `web/`, prioritizing iOS Safari (26.2+ baseline). Open-ended: keep iterating until Ricardo stops the loop.

## Done

- ✅ Phase 1 (PR #1355 → main): Playwright webkit infra, iOS 26.2 banner, Nav API foundations (`useConfirmLeave`, `startViewTransitionIfSupported`), docs.
- ✅ Phase 1.5: Mobile sweep tool (`bun run mobile-sweep`) + `docs/mobile-sweep-findings.md`.
- ✅ Phase 2 (PR #1356 → sprint): overflow fixes on `/v2/tags`, `/v2/api-keys`, `/v2/agents`, `/v2/categories`. All at 0px overflow on iPhone 13 / 15 Pro Max.
- ✅ Iter 3 (PR #1357 → sprint): `LeaveGuard` component + wired to `agent-form.tsx` and `rule-form.tsx`.
- ✅ Iter 4 (PR #1358 → sprint): PWA assets — 180/192/512 PNG icons rasterized from favicon.svg via `bun run generate-icons`, `manifest.webmanifest` with maskable variant, sized apple-touch-icon. 404 + error pages confirmed to have back-to-home links (no Safari chrome in standalone mode).
- ✅ Iter 5 (PR #1359 → sprint): restore bfcache on iOS Safari swipe-back. Pulled `/v2/*` out of the session-middleware group in `internal/api/router.go` so the static SPA bundle stops carrying `Vary: Cookie` (which WebKit treats as a bfcache blocker). Tightened the `pageshow` handler in `web/src/main.tsx` to only re-validate `["me"]` instead of invalidating every cached query, eliminating the post-restore refetch storm. Verified via curl against a freshly-built binary: `Vary: Cookie` is gone from `/v2/` responses.
- ✅ Iter 6 (PR #1360 → sprint): `bun run memory-sweep` — Playwright-based structural-leak detector. Drives list↔detail N=20 times under webkit, samples DOM node count + shadcn slot count at iter 1/5/10/15/20, reports verdict per flow. Baseline: `/v2/accounts` clean (0 growth across 20 iters); `/v2/transactions` clean iter 1-15 (1838→1742, attrition) then drops to 252 at iter 20 — likely session loss after rapid navigations; flagged for follow-up investigation. WebKit doesn't expose `performance.memory`, so this is a structural proxy; real iOS heap data needs Safari Web Inspector.
- ✅ Iter 7 (PR #1361 → sprint): rolled `LeaveGuard` out to `tag-form`, `category-form`, `api-key-form`, and the password-change form in `account-section.tsx`. Each navigates only on success and now resets the form before navigating (so the guard doesn't intercept the post-save nav). The password form uses a custom title/description.
- ✅ Iter 8 (PR #1362 → sprint): Web Vitals listener at `web/src/lib/web-vitals.ts`. Uses standard `PerformanceObserver` for `largest-contentful-paint`, `event` (INP-proxy), and `layout-shift` (CLS). Gated by `VITE_REPORT_VITALS` (defaults to on in dev, off in prod). Logs structured `[vitals] ✓ LCP=504 (good) path=/v2/sandbox`-style lines. Verified: LCP=504ms and INP=112ms captured live on `/v2/sandbox`.
- ✅ Iter 9 (PR #1363 → sprint): extended `mobile-sweep` to walk 7 detail/edit flows (transactions, accounts, categories, tags, connections, rules, agents-edit) after the static routes. Each flow scrapes one short_id from its list and inspects the resolved detail URL. **All 7 detail pages clean across iPhone SE 1st-gen, iPhone 13, and iPhone 15 Pro Max** — 0 overflow, no JS errors. Detail surfaces inherit the layout work from earlier iterations.
- ✅ Iter 10 (PR #1364 → sprint): iOS keyboard-hint audit + targeted fixes. Added `autoCapitalize="none"` / `autoCorrect="off"` / `spellCheck={false}` to: `action-row.tsx` tag-slug input, `cron-field.tsx` cron-expression input, `account-section.tsx` 3 password fields. Added `type="search"` + `inputMode="search"` + `enterKeyHint="search"` + same anti-correction set to `transcript-viewer.tsx` search box. Audit cleared all other identifier-style inputs as already-handled.
- ✅ Iter 11 (PR #1365 → sprint): TanStack Query cache hygiene. Explicit `gcTime: 5 * 60 * 1000` on the QueryClient defaults (no functional change vs the implicit default but locks the contract). `gcTime: Infinity` on `["me"]` so the auth snapshot doesn't get GC'd during long iOS Safari sessions (would force a `/web/v1/me` refetch + auth-splash flicker on tab-away/return). Memory sweep + mobile sweep clean — no regressions.
- ✅ Iter 12 (PR pending → sprint): Playwright regression test for bfcache eligibility — asserts `/v2/` has no `Vary: Cookie` header (the iter-5 fix). Test also caught a real iter-4 bug: `/v2/manifest.webmanifest` was serving as `text/plain` because Go's net/http doesn't know `.webmanifest` MIME by default — iOS Safari would silently ignore the manifest during Add-to-Home-Screen. Fixed by registering `application/manifest+json` via `mime.AddExtensionType` in an init() in `web/embed.go`. Both tests now pass against a fresh sprint binary.

## Queue (priority order)

Each iteration: pick the next item, branch off `mobile-safari/sprint`, ship a sub-PR, merge after CI green.

1. **Tap-target detection refinement + fixes** — current sweep flags 6-50+ "small" elements per route, but most are `size="sm"` text buttons (80×28) without the `pointer-coarse:before` recipe (by design). Decide: extend the recipe vertically to all sizes? Audit raw `<button>` elements not going through the primitive. Need a careful design call.

2. **View Transitions wiring** — wrap `transactions list → detail` and `agents list → detail` route transitions with `startViewTransitionIfSupported`. Helper is in `lib/navigation/feature.ts`. Test fallback path (older browsers see instant navigation).

3. **iPhone SE 1st-gen (320px) edge** — `/v2/api-keys` still shows ~55px overflow on the 320px profile. Consider `table-layout: fixed` opt-in on DataTable or other approaches. Lowest priority — 320px devices are ~8 years old.

4. **LeaveGuard — remaining settings sub-forms** — audit `household-section`, `backups-section`, `agents-section` for `useForm` instances that need LeaveGuard. Those files are 600+ lines each and likely contain multiple sub-forms; needs per-form judgment on whether the field is sensitive enough to warrant a guard.

5. **Memory phase — virtualize transactions list** — long-history households render the entire TanStack Table row tree in DOM (1800+ nodes at iter 1 of the memory sweep). Add `@tanstack/react-virtual` row windowing to DataTable when row count exceeds N. Defer if a profile shows no real iOS pain.

6. **Memory phase — `useEffect` cleanup audit** — grep for `useEffect` returning no cleanup against patterns that hold a listener (`addEventListener`, `setInterval`, `IntersectionObserver`, `MutationObserver`, `ResizeObserver`). Trace common offenders in `features/transactions/*` and `components/data-table.tsx`.

7. **Transactions session-loss after rapid navigations** — `bun run memory-sweep` shows `/v2/transactions` dropping to 252 nodes at iter 20 (AuthSplash territory). Likely session lifetime or scs middleware behavior under hammered navs. Reproduce, then either bump session ttl on /v2 boundary or stabilize the auth gate.

## Operating notes

- Always work from the `mobile-safari-playwright` worktree at `.claude/worktrees/mobile-safari-playwright/`.
- Run `BASE_URL=http://localhost:6080 bun run mobile-sweep` after each phase to confirm no regressions.
- `bun run lint` clean before any merge.
- Use subagents for parallelizable work (banner-type new components, doc updates).
- Update this file after each iteration: mark item done, add new findings.
- If the queue is empty, run a fresh exploratory pass (sweep at new viewports, audit a specific concern) and produce new queue items.
- If a Vite dev server or backend isn't running, check `lsof -i :6080` / `lsof -i :8080` and start whatever's missing.
- Always pair PR links with `([graphite](...))` per Ricardo's preference.
- This sprint state file is at `.claude/mobile-safari-sprint.md` (NOT `.claude/sprint-state.md` which belongs to the agents sprint).

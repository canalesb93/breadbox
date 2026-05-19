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
- ✅ Iter 5 (PR pending → sprint): restore bfcache on iOS Safari swipe-back. Pulled `/v2/*` out of the session-middleware group in `internal/api/router.go` so the static SPA bundle stops carrying `Vary: Cookie` (which WebKit treats as a bfcache blocker). Tightened the `pageshow` handler in `web/src/main.tsx` to only re-validate `["me"]` instead of invalidating every cached query, eliminating the post-restore refetch storm. Verified via curl against a freshly-built binary: `Vary: Cookie` is gone from `/v2/` responses.

## Queue (priority order)

Each iteration: pick the next item, branch off `mobile-safari/sprint`, ship a sub-PR, merge after CI green.

1. **Tap-target detection refinement + fixes** — current sweep flags 6-50+ "small" elements per route, but most are `size="sm"` text buttons (80×28) without the `pointer-coarse:before` recipe (by design). Decide: extend the recipe vertically to all sizes? Audit raw `<button>` elements not going through the primitive. Need a careful design call.

2. **View Transitions wiring** — wrap `transactions list → detail` and `agents list → detail` route transitions with `startViewTransitionIfSupported`. Helper is in `lib/navigation/feature.ts`. Test fallback path (older browsers see instant navigation).

3. **Web Vitals beacon** — small listener using Event Timing API + LCP (both new in Safari 26.2). Beacon INP/LCP/CLS to a `/api/v1/web-vitals` endpoint or `console.log` behind a `VITE_REPORT_VITALS` flag. Establishes perf baseline.

4. **Detail-page sweep** — extend `mobile-sweep.ts` to scrape one ID per list route and visit the corresponding detail/edit/new pages. The current sweep only covers parameter-less routes.

5. **iPhone SE 1st-gen (320px) edge** — `/v2/api-keys` still shows ~55px overflow on the 320px profile. Consider `table-layout: fixed` opt-in on DataTable or other approaches. Lowest priority — 320px devices are ~8 years old.

6. **bfcache verification** — write a Playwright test that actually exercises bfcache restore (multi-page traversal + back gesture). Confirms the `pageshow` + `event.persisted` handler in `main.tsx` fires correctly.

7. **Form-field iOS keyboard audit** — verify every `<Input>` consumer passes appropriate `inputMode` / `enterKeyHint` / `autoCapitalize` defaults. `SearchInput` already does; others might not.

8. **LeaveGuard for other forms** — apply LeaveGuard to remaining dirty-state forms: category-form, tag-form, settings forms, API key new form.

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

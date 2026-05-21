# v3 Browser-Native MPA — Sprint State

**Goal (re-anchor every loop):** Deliver a full, working v3 admin application — a browser-native,
server-rendered Go MPA at `/app/*` — at feature/UX/flow parity with the v2 React SPA. The browser
owns navigation/history/scroll/bfcache (the back button is free). Execute autonomously, fan out
subagents, validate every surface with Chrome DevTools, merge PRs into this sprint branch.

**Plan of record:** `~/Documents/obsidian/Breadbox/planned-features/v3-browser-native-mpa-plan.md`
**Branch:** `worktree-v3-mpa` (worktree: `.claude/worktrees/v3-mpa`)
**Sprint started:** 2026-05-21 00:21 PDT

## Locked decisions (resolved at sprint start)
1. Mount path: `/app/*` (302 from `/v2/*` at cutover)
2. Package: `internal/webapp/` (package `webapp`)
3. Components: Basecoat-styled + own thin templ wrappers
4. Settings: real `/app/settings/*` routes
5. Charts: server-rendered SVG default (revisit per surface)
6. Alpine.js: available, used sparingly

## Integration anchors (from codebase map)
- Router: `internal/api/router.go` → `NewRouter(a *app.App, version string)`. Mount `/app` BEFORE `/` catch-all (~line 322), gate `!a.Config.NoDashboard`.
- Service: `service.New(a.Queries, a.DB, a.SyncEngine, a.Logger)` → `*service.Service`. Accounts: `ListAccounts(ctx, *string)`, `GetAccount(ctx, id)`, `GetAccountDetailResponse(ctx, id, limit)`; `ErrNotFound` on miss.
- Session: `admin.NewSessionManager`, `admin.SessionAccountID(sm,r)`, `admin.SetLoginSessionKeys(ctx,sm,account,queries)`, `admin.RequireAuth(sm,queries)` (redirects to `/login` — webapp needs its own → `/app/login`).
- Login: `queries.GetAuthAccountByUsername` + bcrypt + `sm.RenewToken` + `SetLoginSessionKeys`.
- SameOrigin: `internal/middleware` → `mw.SameOrigin(r) bool`, `mw.IsUnsafeMethod`, `mw.WriteError`.
- Embed: mirror `web/embed.go` (`//go:embed`, fallback handler, immutable cache for hashed assets).
- Tailwind: `tailwind-cli-extra` standalone binary (Node-free), see Makefile `css-install`.
- Theme: SPA uses localStorage `breadbox:theme`+`.dark`; webapp adds a server-read cookie to avoid flash.

## Phase status
- [ ] Phase 1 — Foundation (build pipeline, package, shell, primitives, login, Accounts slice, validate, rules)
- [ ] Phase 2 — Read surfaces
- [ ] Phase 3 — Write surfaces/forms + settings
- [ ] Phase 4 — Islands (⌘K, dnd)
- [ ] Phase 5 — Streaming (Datastar+SSE)
- [ ] Phase 6 — Cutover + parity audit

## Progress log (newest first)
- 2026-05-21 00:21 — Sprint init: worktree created, decisions locked, integration mapped, tasks #1–13 created. Starting Phase 1.

## Execution rules (from Ricardo)
- Deploy subagents wherever reasonable (parallelize per-resource; keep orchestrator context lean).
- Every PR includes evidence: Chrome DevTools/Playwright screenshots + build/test output.
- Merge PRs into this sprint branch (full permission). Validate before merge.

## Standing permissions (from Ricardo)
- Download any resources/tools needed to implement (templ, tailwind binary, esbuild, deps).
- Kill locked/stale processes blocking testing (Chrome DevTools lockups, stale `breadbox serve`).
- Seed/read the dev DB for testing. **NEVER drop/destroy the DB** — account & connection data
  is mostly manual-only and needed for testing. Additive only.

## CI/deploy requirement (from Ricardo)
The sprint PR must merge to main and deploy gracefully. The webapp asset build (templ generate +
tailwind app.css) MUST be wired into `make build`/`make generate` and the CI/release/deploy workflows
so embedded `/app/static/css/app.css` ships in the release binary and `breadbox.exe.xyz` serves /app
after auto-deploy. Keep headless/lite build tags correct so the CI matrix stays green. (Task #14.)

## Loop end-state (pivot here once core v3 delivery feels complete)
Once /app reaches functional+flow parity with the SPA and the foundation is solid, the loop switches to:
1. Make pending improvements skipped during the initial build.
2. Clean up & deprecate the v1 admin UI AND the React SPA, plus all references to both.
3. Run Playwright against the new app: catch bugs / UI rendering issues, polish interactions,
   improve mobile responsiveness. Iterate until clean.

## Notifications
- last_notified_epoch: 1779348104  (2026-05-21 00:21 — sprint start)
- cadence: hourly at :37 via cron job `aecc8a60` (re-anchors plan + sends push + continues work)

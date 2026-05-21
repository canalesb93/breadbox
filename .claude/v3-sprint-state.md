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
- [x] Phase 1 — Foundation **DONE & MERGED** (PR #1402 → worktree-v3-mpa). /app shell, Accounts slice,
      login gate, Node-free build, app-mpa.md rule. Validated desktop+mobile+dark, no console errors.
- [x] Phase 2 — Read surfaces **DONE** (transactions, connections, providers, categories, tags, rules,
      api-keys, agents+runs, placeholders). Built via 4 fanned-out subagents, integrated, Chrome-validated.
- [ ] Phase 3 — Write surfaces/forms + settings
- [ ] Phase 4 — Islands (⌘K, dnd)
- [ ] Phase 5 — Streaming (Datastar+SSE)
- [ ] Phase 6 — Cutover + parity audit

## Progress log (newest first)
- 2026-05-21 01:0x — Phase 2 read surfaces DONE & validated (no console errors). 9 surfaces live under /app. Fixed missing </div> in categories/tags templ from subagents. templ+build+vet green. Opening Phase 2 PR. Next: Phase 3 write surfaces/forms (+ real settings).
- 2026-05-21 00:57 — Phase 2 in flight. 4 subagents fanned out per read surface. Done: Transactions (registerTransactions), Connections+Providers (registerConnections/registerProviders), Categories+Tags (registerCategories/registerTags). Pending: Rules+APIKeys+Agents+placeholders. Next: wire registrars into handler.go Router(), templ generate, build, fix collisions (watch `detailRow` helper), Chrome-validate, PR. Registrars NOT yet wired.
- 2026-05-21 00:53 — Phase 1 foundation MERGED (PR #1402). Server runs on :8088 (worktree). Chrome-validated login→accounts→detail→back, mobile+dark. Fixed pre-existing sqlc drift (worktreeinclude stale agent_runs). Starting Phase 2: fanning out subagents per read surface.
- 2026-05-21 00:21 — Sprint init: worktree created, decisions locked, integration mapped, tasks #1–13 created. Starting Phase 1.

## Dev server (for validation)
- Run: `bash /Users/canales/.claude/jobs/31d11a34/run-server.sh 8088` (background). Login: admin@example.com / password.
- Theme cookie: `bb_theme=dark`. Screenshots → `.shots/` (gitignored) → upload `curl -F image=@f.jpeg https://img402.dev/api/free`.
- Chrome MCP profile can lock; kill: `pkill -f chrome-devtools-mcp/chrome-profile` (sandbox-disabled).
- git/PR ops need dangerouslyDisableSandbox (worktree .git lives in main repo, sandbox-blocked).

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
- last_notified_epoch: 1779350260  (2026-05-21 00:57 — Phase 2 heartbeat)
- cadence: hourly at :37 via cron job `aecc8a60` (re-anchors plan + sends push + continues work)

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
- [x] Phase 3 — Write surfaces/forms + settings **DONE & MERGED** (PR #1404). Forms for category/tag/
      api-key/rule/agent + agent settings + real /app/settings + setup-account. Validated (creates persist).
- [ ] Phase 4 — Islands (⌘K, dnd)
- [x] Phase 5 — Streaming **DONE & MERGED** (PR #1407): agent-run live transcript via SSE + static fallback. Follow-ups: sync-progress + activity-timeline streaming.
- [ ] Phase 6 — Cutover + parity audit (do NOT retire SPA until parity audited)

## Parity audit punch-list (2026-05-21, SPA vs /app — definitive)
Core surfaces AT PARITY. Reports/Insights/Reviews are PLACEHOLDER-MATCHED (SPA also stubs them — no action). v3 EXCEEDS SPA: server-side validation, SSE streaming, native nav/bfcache.
**Tier 1 (cutover blockers):**
  1. Tx list filters: amount min/max + pending (+ sorting sort_by/sort_order) — EASY, handler+form params.
  2. Bulk tx operations (select → categorize/tag) — island + multi-id POST.
  3. Inline category edit on tx list — small island AJAX update.
  4. Connection reauth flow — provider-specific (Plaid/Teller link); COMPLEX/risky for autonomous — may need real link tokens; defer/flag.
**Tier 2 (high-value):**
  5. Connection sync-all button (POST trigger).
  6. Activity timeline on tx detail (list_annotations → render) — explains categorization.
  7. Connection user/family filter tabs.
  8. Rule form: warn when OR/NOT conditions can't be visually edited.
**Tier 3/4 (post-cutover OK):** settings modals-vs-pages decision; prompt builder (or hide from nav); backups export; household mgmt; Reports/Insights/Reviews real content.
**Loop end-state also:** #15 drag-drop rule builder + asset fingerprinting; confirm()→<dialog> on api-key revoke; Playwright suite + mobile pass.

## Progress log (newest first)
- 2026-05-21 07:0x — Heartbeat. 12 PRs; review-ready + deploy-correct. Next unit (delegated): drag-drop rule builder island (#15) — richer than the Phase 3 form editor (nested AND/OR/NOT + category/tag pickers). It's the last marquee parity nicety; will only merge if it's clean (form editor stays as the solid fallback). After this, only Tier 3/4 minor items + cutover (Ricardo) remain — loop is winding toward done.
- 2026-05-21 06:1x — Asset fingerprinting MERGED (PR #1413): app.css/app.js content-hashed + immutable; islands now immutable too (fixed looksFingerprinted for base32). Stale-CSS-on-deploy SOLVED. 12 PRs total. Remaining loop items: drag-drop rule builder island (#15; fingerprinting half now done); Tier 3/4 (settings modals-vs-pages, prompt builder, backups, household). Cutover proposal still awaiting Ricardo.
- 2026-05-21 06:0x — Heartbeat. 11 PRs; review-ready. Next unit (delegated): asset fingerprinting for app.css/app.js (extend the islands manifest+IslandSrc pattern to a general AssetURL resolver; embed.go already long-caches fingerprinted names via looksFingerprinted). Fixes stale-CSS-on-deploy. Then drag-drop rule builder, Tier 3/4.
- 2026-05-21 05:1x — Polish batch MERGED (PR #1412): native <dialog> for api-key revoke (no confirm()), mobile tx-filter collapse (<details>). 11 PRs total. Validated. NOTE: stale-CSS cache bit validation a 3rd time (had to hard-reload to see new .dialog styles) — **asset fingerprinting (#15) is now the top remaining item** (deploy correctness; islands already fingerprinted, app.css/app.js are not). Next cron cycle: do fingerprinting, then drag-drop builder, then Tier 3/4. Cutover still awaiting Ricardo.
- 2026-05-21 05:0x — Heartbeat. 10 PRs merged; cutover proposal awaiting Ricardo. Next unit (delegated): polish batch — (1) confirm()→native <dialog> on api-key revoke (per no-blocking-dialogs convention), (2) collapse tx filter bar behind <details> on mobile. Then #15 (drag-drop builder + asset fingerprinting) + Tier 3/4 remain.
- 2026-05-21 04:2x — Playwright e2e suite + CRITICAL mobile-drawer fix MERGED (PR #1411). 10 PRs total. Playwright (`make webapp-e2e`) 15 pass/1 skip. The mobile drawer was fully broken (peer-checked couldn't reach nested aside) — fixed + validated. Verified the 1 skipped scroll-restoration test is a HEADLESS artifact (headed Chrome restores correctly 2000→2000); core promise holds. Wrote **cutover proposal** for Ricardo: `~/Documents/obsidian/Breadbox/planned-features/v3-cutover-proposal.md`. Phase 6 cutover gated on his review (do NOT auto-retire SPA). Loop continues with minor polish: confirm()→<dialog> on api-key revoke, mobile tx-filter collapse, #15 (drag-drop builder + asset fingerprinting), Tier 3/4.
- 2026-05-21 04:0x — Heartbeat. Full parity holds (9 PRs). Next unit: delegating #21 Playwright e2e suite for /app (back/forward/scroll/deep-link/iOS-emulated across key surfaces; reuse web/e2e harness, install deps if needed per standing perms). Then remaining polish (confirm()→<dialog>, mobile filter collapse, #15) + cutover proposal.
- 2026-05-21 03:4x — Bulk tx ops + inline category edit MERGED (PR #1410). ALL Tier-1 parity gaps from the audit CLOSED. 9 PRs total. v3 at full functional parity with the SPA (exceeds it: server validation, SSE, native nav). Mobile pass: overview + transactions + accounts validated at 390px (responsive, usable; minor future polish: collapse tx filter bar behind a disclosure on mobile). Inline edit confirmed persisting server-side. NEXT (loop carries): #21 Playwright suite; remaining polish (confirm()→<dialog> on api-key revoke; mobile filter collapse; #15 drag-drop builder + asset fingerprinting; full-native reauth island); Tier 3/4 (settings modals decision, prompt builder, backups, household). THEN: propose Phase 6 cutover (302 /v2→/app) + v1/SPA deprecation for Ricardo's review — do NOT auto-retire.
- 2026-05-21 03:3x — Parity wave MERGED (PR #1409): tx amount/pending filters + header sorting + activity timeline; connections sync-all + family tabs + hosted-link reauth. 8 PRs total. Tier-1/2 server-side parity closed. Launched LAST Tier-1 gap: bulk tx ops + inline category edit (islands, #20). Remaining after that: #21 Playwright polish + mobile pass (loop end-state #3); #15 drag-drop builder + asset fingerprinting; full-native reauth island; confirm()→<dialog> on api-key revoke; Tier 3/4 (settings modals decision, prompt builder, backups, household — post-cutover OK). Then propose Phase 6 cutover (302 /v2→/app) for Ricardo's review — do NOT auto-retire SPA.
- 2026-05-21 03:0x — Heartbeat. Core v3 complete (7 PRs). Parity-audit Explore agent running (read-only, SPA vs /app). Awaiting its punch-list to drive final polish; then loop end-state. No code change this tick (sequencing on the audit; ad-hoc edits would risk redundancy with audit findings + my context is large).
- 2026-05-21 03:0x — Home overview dashboard MERGED (PR #1408): metrics + server-SVG spending chart + recent activity + multi-currency-safe. Phase 5 MERGED (PR #1407). 7 PRs total; core v3 app is COMPLETE & deploy-ready. NEXT: parity audit (delegated, read-only) → then loop end-state (pending improvements #15, deprecate v1+SPA after audit, Playwright polish/mobile). Phase 6 cutover (302 /v2→/app) only after parity audit confirms no critical gaps.
- 2026-05-21 02:2x — Phase 4 islands MERGED (PR #1406): esbuild-via-Go pipeline + ⌘K command palette (validated, centered, no console errors). 5 PRs merged; v3 is a near-complete deploy-ready app. Deferred (task #15): drag-drop rule builder island + app.css/js fingerprinting (currently max-age=3600 → stale CSS up to 1h post-deploy; the islands manifest already fingerprints). NEXT: Phase 5 streaming (Datastar+SSE: sync progress, agent run live transcript, activity timeline) — delegated. Then parity audit → Phase 6 cutover (302 /v2→/app) → loop end-state (deprecate v1+SPA, Playwright polish/mobile). NOTE: do NOT retire the SPA until parity (incl. streaming) is reached + audited; SPA still has streaming/charts/reviews v3 lacks.
- 2026-05-21 02:0x — #14 CI/deploy MERGED (PR #1405): webapp build-tag injection in CI, CSS in CI/release/Docker, Caddyfile passes /app. 4 PRs merged; v3 is deploy-ready. NEXT: Phase 4 islands (esbuild-via-Go pipeline + ⌘K palette delegated; drag-drop rule builder is a follow-up), then Phase 5 streaming, Phase 6 cutover, loop end-state (deprecate v1+SPA, Playwright polish).
- 2026-05-21 01:4x — Phase 3 MERGED (PR #1404). Core CRUD app complete (read+write+settings+auth). 3 PRs merged into sprint branch. NEXT: (#14) CI/deploy wiring — CRITICAL gotcha: CI runs `templ generate` directly and only injects the !headless&&!lite build tag into internal/templates/components; it MUST also inject into internal/webapp generated files or headless/lite CI cells break. Delegated #14 to a subagent. After that: Phase 4 (esbuild-via-Go islands: ⌘K palette, drag-drop rule builder), Phase 5 (Datastar+SSE streaming: sync progress, agent transcripts, activity timeline), Phase 6 cutover (302 /v2→/app, retire SPA), then loop end-state (deprecate v1+SPA, Playwright polish/mobile).
- 2026-05-21 01:2x — Phase 3 form pattern PROVEN: Category create/edit validated in Chrome (create→303→detail, slug auto-gen, persisted). Reusable components/forms.templ established. Created a "V3 Test Category" in dev DB (harmless test data; no delete UI yet). Fanned out 4 more form agents: (A) tags+api-keys, (B) rules, (C) agents+settings-tokens, (D) settings routes+setup-account. INTEGRATION TODO when they return: wire `h.registerSettings(r)` into AUTHED group + `h.registerSetup(r)` into PUBLIC group in handler.go, and REMOVE `/settings` from registerPlaceholders (settings replaces it). Then templ generate, build, fix collisions, rebuild binary, restart :8088, Chrome-validate, PR. Watch: secrets must stay masked (agents tokens, api-key plaintext only on create page); category_override sacred.
- 2026-05-21 01:1x — Phase 2 MERGED (PR #1403). Starting Phase 3 forms: launched 1 subagent to establish the reusable form-component pattern + Category create/edit exemplar. When it returns: validate, then fan out tags/api-keys/rules/agents/settings/setup-account forms following that pattern.
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
- last_notified_epoch: 1779368309  (2026-05-21 ~07:0x — heartbeat; pushed "12 PRs, fingerprinted, drag-drop builder next")
- cadence: hourly at :37 via cron job `aecc8a60` (re-anchors plan + sends push + continues work)

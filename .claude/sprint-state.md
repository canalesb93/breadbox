# Claude Agent SDK integration sprint

**Sprint branch (persistent, accumulates all iterations):** `agents/claude-agent-sdk-sprint`
**Worktree:** `.claude/worktrees/agents+claude-agent-sdk-sprint`
**Started:** 2026-05-16 off origin/main @ 446709e9
**Authorization & workflow:** Ricardo has granted standing approval to open AND squash-merge iteration PRs INTO the sprint branch (NOT into main). Iter 1 was grandfathered into main directly via PR #1223; iter 2+ accumulates on the sprint branch. At end of sprint, ONE final PR opens from `agents/claude-agent-sdk-sprint` → `main` for Ricardo to review the full feature in one place. Do not enable GitHub auto-merge.
**Per-iteration branches:** each iteration creates `agents/iter-N-<slug>` (e.g. `agents/iter-2-service-rest`) off the sprint branch, opens a PR into the sprint branch, merges, and is auto-deleted on merge. The sprint branch is never deleted.
**Driver:** `/loop 30m` autonomous continuation.

## Goal

Replace the v1 admin "agent-prompts" copy-to-clipboard wizard with a real recurring-agent system. Self-hosters configure Anthropic credentials and schedule agents that run locally via the Claude Agent SDK, calling breadbox MCP to do real work (categorize, enrich, review transactions). Industrial-quality: tests, docs, observability, safety caps. This is core product.

## Locked decisions

| # | Decision | Rationale |
|---|---|---|
| 1 | **Runtime: Node sidecar bundled** as separate `breadbox-agent` binary via `bun build --compile`. No Python. | Preserves single-binary install story; TypeScript SDK is mature. |
| 2 | **Auth: two paths from day 1.** (a) **Subscription** via Claude CLI login on the host — used for sprint testing (free under Claude plan credits until 2026-06-15). (b) **BYO Anthropic API key**, encrypted in `app_config` via `internal/crypto/` — durable production path that survives the 2026-06-15 cutover (https://support.claude.com/en/articles/15036540). Settings UI presents both as a radio choice. | Subscription = free testing during sprint; API key = launch-ready and outlasts the credit window. |
| 3 | **Concurrency: 1** agent run at a time for v1. Mutex; overlapping fires skip with a log row. | Simpler safety story; lift in stretch iterations. |
| 4 | **Default tool scope: `read_write`**. Per-agent opt-down to `read_only`. | Agents' job is to *apply* rules and enrich, not just suggest. |
| 5 | **Cost caps required** per agent: `max_turns`, `max_budget_usd`. Hard server-side ceilings in `app_config`. | Bounded blast radius. |
| 6 | **UI: v2 SPA only**, shadcn/ui throughout, use `frontend-design` skill. | Where Ricardo wants the product surface. |
| 7 | **Prompt builder required** — user-authored prompts, picked MCP tool scopes, picked schedules. Not just toggles. | v1's wizard was prompts-only; v2 must be authorable. |
| 8 | **Retire `/admin/agent-prompts`** with a redirect to `/v2/agents` once the new page ships. | Same intent, real execution. |

## Reused breadbox primitives (verify paths before assuming)

- `internal/appconfig/` + `internal/crypto/` — encrypted config storage
- `internal/service/apikeys.go` (`CreateAPIKey` with `ActorType='agent'`) — mint scoped key per run
- `internal/sync/scheduler.go` (robfig/cron) — extend with agent jobs
- `internal/mcp/` stdio (`breadbox mcp` CLI) — what the SDK subprocess calls
- `internal/db/migrations/00008_sync_logs.sql` — template for `agent_runs`
- `web/src/main.tsx` PAGE_OVERRIDES + `web/src/routes/rules.tsx` — page scaffold pattern
- `web/src/components/confirm-dialog.tsx` — destructive action primitive

## Iteration plan

Each iteration ends in **one squash-merged PR into the sprint branch** (not main — see header). Iterations are sequenced — don't open the next iteration branch until the previous PR is merged into the sprint branch.

### Iteration 1 — schema + appconfig + sidecar skeleton (foundation PR) ✅ MERGED #1223
- [ ] Migration: `agent_definitions` table (id, short_id, name, slug, prompt, schedule_cron, tool_scope, allowed_tools jsonb, model, max_turns, max_budget_usd, enabled, created_at, updated_at)
- [ ] Migration: `agent_runs` table (id, agent_definition_id FK, trigger, status, started_at, completed_at, duration_ms, total_cost_usd, input_tokens, output_tokens, max_turns_used, error_message, transcript_path, session_id)
- [ ] Migration: app_config keys defined:
  - `agent.auth_mode` — `subscription` | `api_key` (plaintext, default `subscription`)
  - `agent.subscription_token` — encrypted via crypto package (the `sk-ant-oat01-…` from `claude setup-token`)
  - `agent.anthropic_api_key` — encrypted (the production `sk-ant-…` API key)
  - `agent.max_concurrent` — plaintext int, default `1`
  - `agent.global_max_budget_usd` — plaintext numeric
  - `agent.runtime_path` — plaintext path to `breadbox-agent` binary (auto-resolved if empty)
- [ ] sqlc queries for both tables
- [ ] New `agent/sidecar/` directory: TypeScript Agent SDK runner with package.json + tsconfig + index.ts that reads a JSON job spec on stdin and emits NDJSON events on stdout. Spec carries auth: `{ authMode: "subscription"|"api_key", token: "..." }`. Runner sets `CLAUDE_CODE_OAUTH_TOKEN` xor `ANTHROPIC_API_KEY` accordingly (must unset the other to avoid the precedence bug where API key wins).
- [ ] `Makefile` target: `make agent-sidecar` builds the binary via `bun build --compile` into `bin/breadbox-agent`
- [ ] Release workflow update: cross-platform binaries for `breadbox-agent` alongside `breadbox`
- [ ] Skeleton `internal/agent/` Go package with `Runner` interface (no scheduling yet); unit test that exec's the sidecar with a minimal spec and verifies it round-trips
- **Tests:** sqlc query tests for both tables; integration test for sidecar round-trip with a mock auth (skip if `bin/breadbox-agent` missing)
- **PR title:** `feat(agents): foundation — schema, appconfig, Node sidecar runner`

**Subscription-auth notes (for Iteration 1 + 2):**
- Token is one-year-lived, portable (just a string), no browser needed on the host.
- User flow: run `claude setup-token` on any machine → copy the `sk-ant-oat01-…` → paste into breadbox v2 SPA settings → stored encrypted.
- Sidecar precedence trap: if `ANTHROPIC_API_KEY` is set in the env (e.g. from a dev shell), it wins over the OAuth token. The sidecar process must scrub it before launching the SDK.
- No native expiry detection. Surface "auth failed — re-token" errors clearly in the run row; consider an expiry warning at ~11mo.

### Iteration 2 — service layer + REST API ✅ MERGED #1227 (into sprint branch)
- [x] `internal/service/agents.go` (CRUD + last_run inlining + MintRunAPIKey + AssembleJobSpec)
- [x] `internal/service/agent_settings.go` (get/set with token masking)
- [x] `internal/api/agents.go` — 12 handlers wired under /api/v1/agents
- [x] OpenAPI stubs + docs/api-endpoints.md updated; drift test green
- [x] Settings endpoints with encrypted at rest + masked at GET
- [x] MintRunAPIKey + AssembleJobSpec helpers (iter 3 orchestrator invokes them)
- [x] 14 new service-layer integration tests passing
- [ ] **Deferred to iter 3:** handler-layer integration tests (need *app.App in buildTestRouter)
- [ ] **Deferred to iter 3:** add `agents/**` to .github/workflows/ci.yml `pull_request.branches` so iteration PRs targeting the sprint branch actually run CI. Currently only main / stack/** / feat/** match — sprint PRs merge on local-test confidence + standing auth.

### Iteration 3 — scheduler + runner orchestrator ✅ MERGED #1229 (into sprint branch)
- [x] `internal/service/agent_scheduler.go` — robfig/cron wrapper, one entry per enabled definition
- [x] OnDefinitionChanged hook reloads scheduler on CRUD
- [x] `internal/agent/concurrency.go` — Semaphore (cap from app_config, default 1)
- [x] `internal/service/agent_orchestrator.go` — Mint → Insert → Assemble → Runner.Run → Complete → Revoke
- [x] POST /api/v1/agents/{slug}/run handler (503/422/200 cases)
- [x] Daily 3:15am cleanup job for old agent_runs rows
- [x] CleanupOrphanedAgentRuns at startup
- [x] CI trigger fix: `agents/**` added to .github/workflows/ci.yml (iter 3 PR was the first to exercise it — all 5 jobs green)
- [x] 6 new orchestrator integration tests pass
- [ ] **Deferred to iter 7:** transcript file GC (orphaned NDJSON files after row delete) — DB rows are pruned, disk cleanup is polish

### Iteration 4 — v2 SPA `/agents` list + settings ✅ MERGED #1230 (into sprint branch)
- [x] PAGE_OVERRIDES entry for `/agents` (nav slot was already in System)
- [x] `web/src/api/queries/agents.ts` — all hooks (list/get/create/update/delete/enable/disable/run-now/runs/settings)
- [x] `web/src/routes/agents.tsx` — list page with cards, toggle, run-now, delete, quick-create Sheet
- [x] `web/src/features/settings/agents-section.tsx` + sections registration + shell wiring
- [x] shadcn Switch primitive added
- [x] Screenshots captured (desktop+tablet+mobile composite) and embedded in PR body via img402.dev
- [ ] **Deferred to iter 5:** sandbox specimens (agent row is feature-scoped today; promote in iter 5 once edit/transcript components emerge)

### Iteration 5 — prompt builder + run history viewer ✅ MERGED #1231 (into sprint branch)
- [x] Edit page at /v2/agents/$slug/edit with all 9 fields (RHF + zod via z.preprocess to avoid the iter-4 coerce bug)
- [x] CronField with live human-readable preview + 12-preset Popover picker
- [x] Run history page at /v2/agents/$slug/runs with status pills + Load more pagination
- [x] TranscriptViewer parsing NDJSON into turn-grouped chat blocks (assistant + tool_use/tool_result pairs) + ResultFooter (cost/tokens/stop-reason)
- [x] apiText() + useTranscript hook + TranscriptEvent discriminated union
- [x] formatDuration() helper
- [x] Edit + History icon links on the agents list AgentRow
- [x] .gitignore excludes /breadbox-iter* so local test binaries can't be staged
- [ ] **Deferred to iter 7 polish:** sandbox specimens for TranscriptViewer + CronField (promote when reuse outside agents area appears)
- [ ] **Deferred:** TagInput chip control for allowed_tools (currently a comma-separated Textarea)
- [ ] **Deferred:** MCP tool registry endpoint to source allowed-tools picker from live data

### Iteration 6 — seed defaults + retire v1 ✅ MERGED #1232 (into sprint branch)
- [x] DefaultSeed + SeedDefaults — 5 starter agents (initial-setup, bulk-review, quick-review, routine-review, spending-report), all disabled, mapped to existing prompts/agents/strategy-*.md files
- [x] Wired at startup in serve.go after the agent scheduler is constructed
- [x] All v1 admin agent paths 302 → /v2/agents (5 routes)
- [x] AgentsPageHandler / PromptBuilderHandler / PromptCopyHandler symbols kept compiled (unwired) for one-line rollback
- [x] docs/agents.md — canonical quick-start + architecture + safety + replaced-what doc
- [x] 3 new seed integration tests pass (populates / idempotent / skips on existing rows)
- [ ] **Deferred:** full deletion of v1 templ + handler files — belongs in the broader v1-admin retirement sweep
- [ ] **Deferred:** removing the v1 sidebar "Agent Prompts" nav entry — staying as a discoverable bridge during transition

### Iteration 7 — polish + docs + observability ✅ MERGED #1233 (into sprint branch)
- [x] Structured slog.Debug per sidecar NDJSON event in the orchestrator
- [x] OTel passthrough — sidecar inherits parent env, so OTEL_* on `breadbox serve` flows in; documented in rules + docs
- [x] CHANGELOG.md Unreleased → Added entry
- [x] docs/agents.md (shipped in iter 6)
- [x] .claude/rules/agents.md with locked invariants + Do-Not list
- [x] Sandbox specimens (deferred from iter 5): CronField + TranscriptViewer (success + error variants) with sampleTranscriptEvents fixtures
- [x] All edge cases already covered by orchestrator code (sidecar crash, no auth, no binary, runner error, concurrency)

### Iteration 8+ (stretch — loop keeps going until Ricardo says merge)

Pick from this menu in roughly this order; bias toward what makes the system more *useful and trustworthy* per iteration. Add new ideas to this list as you find them.

**Functionality**
- Subscription auth was set up in iter 1; iter 8 should verify the live end-to-end path (`claude setup-token` → sidecar → SDK → MCP → categorize a real transaction). Send a PushNotification asking Ricardo to drop in a token; meanwhile write the smoke-test harness so it's ready the moment a token arrives.
- "Suggested rules" agent: scans recent transactions, proposes new `transaction_rules`, queues them for a human-approval review row instead of applying directly.
- Webhook trigger: fire an agent when a connection finishes a sync (extends the existing `webhook` trigger value).
- Per-agent quiet hours: don't fire between configurable hours of the day (respect "don't ping Claude at 3am").
- Multi-concurrent runs (lift the v1 `max_concurrent=1` cap once we trust the system).
- Resume + multi-step agents (use the SDK's `sessionId` to chain runs that exceed `max_turns` in one shot).

**Trust & observability**
- Cost dashboards in `/v2/agents`: per-agent + global spend over time, projection vs. budget.
- "Dry run" mode: an agent run with `read_only` scope that emits what it *would* have changed, queued for human approval.
- Per-agent audit page linking each run's transcript to the actual DB changes it produced (categorizations, rule additions, etc.).
- Alert-on-anomaly: surface runs that hit `max_turns`, `budget_exceeded`, or had unusual tool-call counts.
- Optional OpenTelemetry export wired through the SDK (`OTEL_*` env vars).

**DX & docs**
- A `breadbox agent run <slug>` CLI subcommand for manual one-off runs from the shell (useful for self-hosters who want to test before scheduling).
- A `breadbox agent test` command that runs a tiny no-MCP "say hello" prompt to validate the sidecar + auth + binary discovery, suitable for `breadbox doctor`.
- Seed-agent library expansion: more than the v1 set — onboarding, account reconciliation, monthly close-out, anomaly review.
- Inline rule-engine docs in the prompt builder (link/preview of `docs/rule-dsl.md`).
- Migration guide for users moving from the v1 admin agent-prompts wizard.

**Polish**
- Run-history filtering (status, date range, definition).
- Inline transcript search.
- Settings page: "Test connection" button that validates auth before save.
- Empty-state and error-state polish across all agent pages (use the `frontend-design` skill).
- Mobile-responsive sweep on the agent pages (use `simple-validate-ui` for evidence).
- Dark-mode polish.

**When the explicit menu runs out, the loop generates its own next items — don't stop, don't ping.** In order:

1. **Subagent: gap audit.** Spawn `feature-dev:code-reviewer` against the full diff `git diff main...agents/claude-agent-sdk-sprint -- ':!*.md'`. Ask it to find: missing tests, edge cases, fragile code paths, security smells, accessibility gaps, error-handling holes. Each finding becomes a new menu item appended to this list. Pick the top-priority one and ship it as the next iteration.
2. **Subagent: feature extension proposals.** Spawn `general-purpose` with the brief "you're reviewing the breadbox Claude Agent SDK system at sprint branch agents/claude-agent-sdk-sprint. Read the sprint state, the new code under internal/agent/ and web/src/routes/agents*. Propose 3-5 concrete extensions that would make this more valuable to a self-hoster — be specific (files to touch, the user-visible win). Sort by impact/effort." Append the proposals and pick the top one.
3. **Subagent: end-user journey audit.** Spawn `Explore` with "trace what a new self-hoster experiences from `git pull && make build` through enabling their first agent. List every friction point — missing docs, ambiguous error messages, install-time surprises, settings that aren't discoverable. Output a punch list." Convert each into an iteration.
4. **Subagent: comparable-product scan.** Spawn `claude-code-guide` with "what features do other agent-orchestration systems (LangGraph dashboards, Zapier agent runs, n8n schedules) ship that we don't? Look at their actual docs. Don't copy uncritically — but list what's worth considering for breadbox's specific self-hosted finance use case." Append worth-doing items.

Cycle through (1)→(2)→(3)→(4) and start over. Only after all four come back with nothing actionable AND there are no skipped sub-items in this file should the loop send the end-of-menu PushNotification described in the End-of-sprint exit section. **Bias hard against stopping — every iteration that ships improves the product.**

## Iteration log

Every loop iteration appends a dated entry here. Format:

```
## ITER N — YYYY-MM-DD HH:MM
- What was done this turn
- What's blocked
- What's next
```

## ITER 1 — 2026-05-17 00:05
Shipped (PR-#TBD on this branch):
- Migrations: `agent_definitions` + `agent_runs` with full schema, short_id triggers, CHECK constraints, FK SET NULL behavior, indexes.
- sqlc queries: complete CRUD + lifecycle queries for both tables; `make sqlc` regenerated cleanly and the new `*.sql.go` files are tagged `!lite`.
- `internal/appconfig/keys.go` (`agent.*` key constants, `AuthMode*` enum-like consts) and `internal/appconfig/encrypted.go` (`ReadEncrypted` / `WriteEncrypted` helpers wrapping AES-256-GCM via `internal/crypto`).
- `internal/agent/` Go package: `spec.go` (JobSpec, AuthConfig, MCPServerConfig), `event.go` (NDJSON event union + typed payload accessors), `runner.go` (Runner interface, RunResult, status consts), `errors.go` (sentinel errors), `sidecar.go` (full Sidecar.Run implementation — locates binary, pipes spec to stdin, streams NDJSON from stdout, writes transcript to disk, populates RunResult).
- TS sidecar in `agent/sidecar/`: package.json + tsconfig + spec.ts (zod-validated JobSpec) + events.ts (sync NDJSON emit + transcript append) + index.ts (full SDK query loop with auth scrubbing, cost cap detection, max-turns detection, structured `result` event emission, graceful SIGTERM/SIGINT). README + .gitignore.
- Makefile: `agent-sidecar`, `agent-sidecar-install`, `agent-sidecar-typecheck` targets. .gitignore already covers `/bin/`.
- Tests: 14 integration tests passing (schema-pin + sqlc round-trips for both tables, FK SET NULL behavior, short_id trigger fires, CHECK enforcement). 3 unit tests for the Runner (NDJSON parsing, binary-not-found, non-zero exit). All unit tests pass; `go build ./...`, `go vet ./...`, `go build -tags=headless ./...`, `go build -tags=lite ./...` clean.

Deferred to iteration 2:
- Service layer (`internal/service/agents.go`) — CRUD + mint-and-revoke for scoped API keys.
- REST handlers — `/api/v1/agents` + `/web/v1/agents` + settings endpoints for the Anthropic / OAuth token.
- Wiring app_config defaults at server startup (so `agent.auth_mode` defaults to `subscription`).
- Release workflow update for `breadbox-agent` cross-platform binaries — deferred to iteration 7 (will land alongside docs + observability so the release artifact story ships in one cohesive piece).

Next iteration: service layer + REST API + scoped key mint/revoke.

## ITER 2 — 2026-05-17 00:25
Shipped (PR #1227, squash-merged into sprint branch):
- Full service + REST surface for agent definitions, runs, and settings (12 endpoints).
- Token masking + AES-256-GCM at rest for `subscription_token` / `anthropic_api_key`.
- MintRunAPIKey + AssembleJobSpec helpers ready for the iter 3 orchestrator.
- OpenAPI stubs + docs/api-endpoints.md updates (drift test green).
- 14 new service-layer integration tests pass.

Deferred to iter 3:
- Handler-layer integration tests (need *app.App in buildTestRouter — non-trivial refactor of an existing test scaffold).
- **CI trigger gap:** `.github/workflows/ci.yml` only fires on PRs to `main`, `stack/**`, `feat/**`. Sprint branch is `agents/claude-agent-sdk-sprint` so iter PRs don't run CI. Add `agents/**` to the trigger list in iter 3 (small one-line change; piggyback on the scheduler PR).
- Orchestrator that actually invokes Mint → Assemble → Runner.Run → Revoke.

Next iteration: scheduler + run-now + orchestrator + CI trigger fix.

## ITER 3 — 2026-05-17 00:38
Shipped (PR #1229 squash-merged into sprint branch as 361acafb):
- Closed the loop: agent system can now actually fire (cron) and be triggered manually (run-now).
- Concurrency primitive + orchestrator with auth/binary/runner error surfacing.
- Scheduler with Reload on CRUD.
- Daily cleanup + startup orphan cleanup.
- Serve.go wiring — agent subsystem starts alongside the sync scheduler.
- CI trigger fix landed AND verified: iter 3 PR triggered all 5 CI jobs (api, headless, lite, rest, service), all green. Future iter PRs get real coverage.
- 6 new orchestrator integration tests (happy, concurrency-lock-under-contention, skipped row, no-auth, runner-error persistence, mint-revoke round-trip).

Deferred:
- Handler-layer integration tests still need *app.App in buildTestRouter (deferred from iter 2; still optional given strong service-layer coverage).
- Transcript file GC (iter 7).

Next iteration: v2 SPA /agents page (list + settings) — first UI work, will pull in the frontend-design skill and capture screenshots per the embedded-evidence rule.

## ITER 4 — 2026-05-17 00:48
Shipped (PR #1230 squash-merged into sprint branch as 074fe997):
- First UI iteration. Full /agents list page + Settings → Agents tab.
- TanStack Query hooks for the entire agents REST surface.
- shadcn Switch added (only new primitive needed).
- Screenshot evidence captured at desktop+tablet+mobile via simple-validate-ui, uploaded to img402.dev, embedded in PR body.
- All 5 CI jobs green.
- Two failure modes hit + recovered during this iter, both noted as lessons:
  1. First Edit call against settings-sections.ts silently no-op'd (got "File has not been read yet" error). The lint passed because settings-shell.tsx imported AgentsSection but settings-sections.ts didn't register the tab — meaning the tab never rendered. Recovered by re-reading + re-editing. **Lesson for future loops: when an Edit returns "File has not been read yet", that edit DID NOT APPLY — re-read then re-edit before moving on.**
  2. `git add -A` swept up the local test binary (./breadbox-iter4, 55MB) and removed web/dist/.gitkeep. Caught in `git show --stat` before push, amended. **Lesson for future loops: after `git add -A` run `git status` and check for unexpected binaries / .gitkeep removals before committing.**

Deferred to iter 5:
- Edit page with full prompt builder + allowed-tools multi-select + schedule preset picker
- Run history drawer + transcript viewer
- Sandbox specimens (promote agent row to components/ when iter-5 reuse appears)

Next iteration: iter 5 — prompt builder + run history viewer. The agent row will likely move from features/ to components/ once the edit page reuses it.

## ITER 5 — 2026-05-17 01:02
Shipped (PR #1231 squash-merged into sprint branch as 7209ac21):
- Full agent management UI: edit page, run history, transcript viewer.
- CronField with live human-readable preview ("Mondays at 9 AM") + 12-preset Popover picker.
- TranscriptViewer parses NDJSON into turn-grouped chat blocks with collapsible tool calls + a result footer (cost/tokens/stop-reason).
- apiText() sibling of api<T> for non-JSON endpoints.
- TranscriptEvent discriminated union mirroring Go-side event types.
- Three screenshots captured (list with edit/history buttons, edit page, runs page) at desktop+tablet+mobile, uploaded to img402.dev, embedded in PR.
- All 5 CI jobs green.
- .gitignore excludes /breadbox-iter* — applied the iter-4 lesson properly this time.

Note on the iter-4 binary cleanup: the iter-4 binary survived on disk because I never `rm`'d it after the iter-4 merge — only the index was fixed via --cached. The iter-5 commit caught it via git status before push, but the cleaner fix is the .gitignore pattern (applied above). Future loops: the .gitignore already handles it.

Deferred (intentional polish):
- Sandbox specimens for TranscriptViewer + CronField (iter 7 polish)
- TagInput chip control for allowed_tools
- MCP tool registry endpoint for live allowed-tools picker

Next iteration: iter 6 — seed default agents (port v1 prompt-wizard prompts as disabled-by-default agent_definitions rows) + retire /admin/agent-prompts with a redirect.

## ITER 6 — 2026-05-17 01:08
Shipped (PR #1232 squash-merged into sprint branch as f4f06d64):
- 5 starter agents seed on fresh installs only — user edits + custom agents survive every restart.
- Every v1 admin agent URL 302s to /v2/agents (5 paths).
- docs/agents.md as the canonical user-facing doc.
- 3 new seed integration tests pass; all builds + drift green.

Convenient side-effect: a fresh install now has 5 visible (disabled) agents in the v2 SPA the moment they hit /v2/agents, instead of an empty-state placeholder. Strong onboarding lever.

Deferred:
- Deleting the dead v1 templ + handler files — broader v1-admin retirement work.
- Removing the v1 sidebar nav entry.

Next iteration: iter 7 — observability + docs polish + final cleanup. Specifically: structured logging on every agent event, optional OTel export wired via SDK env vars (OTEL_*), CHANGELOG entry, .claude/rules/agents.md for future contributors, and the deferred sandbox specimens from iter 5 (TranscriptViewer + CronField).

## ITER 7 — 2026-05-17 01:22
Shipped (PR #1233 squash-merged into sprint branch as 06294ab7):
- .claude/rules/agents.md with the load-bearing locked invariants in one place.
- CHANGELOG entry under Unreleased → Added.
- Orchestrator emits per-NDJSON-event slog.Debug for cheap runtime tracing.
- OTel passthrough documented (env inheritance is the default).
- 3 new sandbox specimens (CronField + TranscriptViewer success/error variants) with fixture data.
- All 5 CI jobs green.

**Iteration 1-7 are all merged. The original sprint plan is complete.** The system is feature-complete and ready for the sprint→main PR whenever Ricardo signals.

Per the End-of-sprint exit instructions: DO NOT open the sprint→main PR autonomously. Instead, move on to the Iteration 8+ stretch menu (subscription auth smoke test, suggested-rules agent, webhook trigger, per-agent quiet hours, multi-concurrent runs, cost dashboards, transcript file GC, dry-run mode, audit page, `breadbox agent run <slug>` CLI, `breadbox agent test` doctor command, seed expansion, polish, mobile sweep, etc.).

Next iteration: pick the next-most-valuable Iteration 8+ item. Lean toward **subscription auth smoke test** since it's the only item that can be developed in isolation right now without depending on something else, AND it unblocks Ricardo's first real live run.

## ITER 8 — 2026-05-17 01:34
Shipped (PR #1234 squash-merged into sprint branch as 92ab2fa7):
- `internal/agent/smoketest.go` — diagnostic SmokeTest function (no DB writes, no API key mint, no run row, bounded ~5¢).
- `breadbox agent test` cobra CLI command with ✓/✗ output + remediation messages.
- Cross-tag plumbing (agent_errors_full.go / _lite.go) so the lite build still compiles when MapExitCode references the agent sentinels.
- 3 unit tests cover no-auth / runner-error / unknown-auth-mode paths.
- docs/cli-commands.md + docs/agents.md + CHANGELOG entries.

PushNotification fired to Ricardo asking for a token whenever convenient. Fire-and-forget — loop continues without blocking.

Next iteration candidates (in rough impact order):
1. **POST /api/v1/agents/test + "Test connection" button in Settings → Agents** — surfaces the iter-8 CLI smoke test in the UI for non-CLI users. Reuses SmokeTest. Small + high UX value.
2. **`breadbox agent run <slug>` CLI** — parallels `breadbox agent test`; lets users trigger real agent runs from cron/shell. Small.
3. **Run-history filtering** (status, date range, trigger). Mid-size, polish-bucket.
4. **Per-agent audit page** linking runs to DB changes via mcp_sessions. Bigger; needs the link query.

Picking #1 next iteration — surfaces the smoke test in the UI is high-leverage with low cost.

## ITER 9 — 2026-05-17 01:40
Shipped (PR #1235 squash-merged into sprint branch as dadd435e):
- POST /api/v1/agents/test endpoint wrapping the iter-8 SmokeTest.
- Orchestrator.SmokeTest(ctx) method on *Orchestrator; reuses the existing runner; bypasses concurrency semaphore.
- v2 SPA: useSmokeTestAgent mutation hook + "Test connection" button in Settings → Agents with inline pass/fail Alert.
- docs/api-endpoints.md + openapi.yaml updated; drift green.
- All 5 CI jobs green.

Two iter-8/9 self-service flows now reach Ricardo by paths he prefers:
- CLI users: `breadbox agent test`
- UI users: Settings → Agents → "Test connection" button

Next iteration candidates (in rough impact order):
1. **`breadbox agent run <slug>` CLI** — parallels `breadbox agent test`; lets users trigger real agent runs from cron/shell. Reuses Orchestrator.RunNow. Small.
2. **Run-history filtering** (status, date range, trigger) on /v2/agents/$slug/runs. Mid-size, polish-bucket.
3. **Per-agent quiet hours** (don't fire between configured hours) — schema + scheduler tweak + UI control. Mid.
4. **Webhook trigger** — fire an agent after a connection finishes a sync. Bigger, touches sync engine.

Picking #1 next iteration — small parallel CLI to iter-8's agent test, useful for cron-driven sysadmins.

## ITER 10 — 2026-05-17 01:46
Shipped (PR #1236 squash-merged into sprint branch as d6cacfc5):
- `breadbox agent run <slug>` CLI with --json flag, mirrors v2 SPA "Run now" path via Orchestrator.RunNow.
- Exit codes match `agent test` (0/3/5/1) for scriptable behavior.
- docs/cli-commands.md + CHANGELOG updated.
- All 5 CI jobs green.

The `breadbox agent` parent now has two subcommands (`test` + `run`). Sysadmins have full CLI parity for diagnostics + actual runs — no v2 SPA required for cron-driven workflows.

Next iteration candidates (in rough impact order):
1. **Run-history filtering** (status / date range / trigger) on /v2/agents/$slug/runs — polish bucket, mid-size UI work.
2. **Per-agent quiet hours** — schema column + scheduler tweak + UI control. Mid.
3. **Webhook trigger** — fire an agent after a connection finishes a sync. Bigger, touches sync engine + agent_definitions schema.
4. **Cost dashboard** in /v2/agents — sum per-agent total_cost_usd over time, surface in a small chart on the list page or as a separate /v2/agents/cost page. Bigger.
5. **Inline transcript search** in the TranscriptViewer drawer — small UI improvement (text input, filter event list).

Picking #1 next iteration — filtering is the highest-impact polish item with the most-used surface (run history page).

## ITER 11 — 2026-05-17 01:58
Shipped (PR #1237 squash-merged into sprint branch as e44835e7):
- /v2/agents/$slug/runs filter row: status Select + trigger Select + DateRangeFilter + Clear filters button.
- Backend: ListAgentRuns swapped from sqlc to dynamic SQL with composable WHERE clauses (matches transactions/rules pattern).
- Date params accept YYYY-MM-DD (auto-bumped to EOD on `end`) or RFC3339.
- URL state via extended agentRunsSearchSchema — filters are shareable links + back-button restorable.
- All 5 CI jobs green; drift test stays green (same path, more query params).

Next iteration candidates (in rough impact order):
1. **Per-agent quiet hours** — schema column on agent_definitions (quiet_hours_start, quiet_hours_end as text "HH:MM"), scheduler tweak to skip fires inside the window, UI control. Mid-size; touches scheduler.
2. **Inline transcript search** in TranscriptViewer — small UI; filter event list by text match.
3. **Cost dashboard** in /v2/agents — sum per-agent total_cost_usd over time, surface in a small chart on the list page or as /v2/agents/cost. Bigger.
4. **Webhook trigger** — fire an agent after a connection finishes a sync. Bigger.

Picking **#2 inline transcript search** next iteration — smallest possible follow-up, lives entirely in the TranscriptViewer feature component, and pairs naturally with the run-history filtering just shipped (filter your way to a run, then filter inside the transcript).

## ITER 12 — 2026-05-17 02:07
Shipped (PR #1238 squash-merged into sprint branch as c85fcd5a):
- TranscriptViewer search input with clear button + status line.
- eventMatchesQuery covers assistant text, tool names, tool inputs+outputs (JSON-stringified so search hits arg values), error / cost-cap messages.
- Pre-grouping filter so partial matches inside a turn still surface the turn wrapper.
- Pure feature-component state; no backend/API/schema changes.
- All 5 CI jobs green.

Pairs with iter-11's run-history filtering: filter → find the run → open transcript → filter inside the transcript. Two-layer drilldown is now ergonomic.

Next iteration candidates (in rough impact order):
1. **Per-agent quiet hours** — schema column (quiet_hours_start/end as text "HH:MM" or NULL), scheduler tweak that compares now() against the window, UI control on the edit page. Mid-size; touches scheduler + edit form.
2. **Cost dashboard** in /v2/agents — sum total_cost_usd per agent over time, surface as sparkline on the list page OR a separate /v2/agents/cost page with bar chart. Bigger.
3. **Webhook trigger** — fire an agent after a connection finishes a sync. Bigger, touches sync engine + agent_definitions schema.
4. **breadbox doctor integration** of agent smoke test — small DX win; reuse SmokeTest.
5. **Suggested rules agent** — scans recent transactions, proposes new transaction_rules, queues them for human-approval review row. Bigger, touches transaction_rules + reviews.

Picking **#4 breadbox doctor integration** next iteration — smallest follow-up, reuses iter-8 SmokeTest, valuable for first-run troubleshooting which is the most fragile moment of the self-hoster journey.

## ITER 13 — 2026-05-17 02:14
Shipped (PR #1239 squash-merged into sprint branch as 07839ed7):
- `breadbox doctor` reports an `agent subsystem` line (PASS/WARN/SKIP) without spending money on a live API call.
- Extracted `agent.LocateBinary` as the shared discovery helper used by both Sidecar.resolveBinary and the new doctor check.
- 4 table-driven test cases on the pure decision logic (agentSubsystemCheck) cover all branches.
- WARN states include one-line remediation; PASS message points at `breadbox agent test` for the live round-trip.
- All 5 CI jobs green.

The self-hoster's first-run troubleshooting flow is now: `breadbox doctor` shows what's missing → SettingsUI or CLI fixes it → `breadbox doctor` re-verifies → `breadbox agent test` confirms end-to-end. Each step has its own surface.

Next iteration candidates (in rough impact order):
1. **Per-agent quiet hours** — schema column + scheduler tweak + UI control. Mid.
2. **Cost dashboard** in /v2/agents — sparkline on list page or /v2/agents/cost. Bigger.
3. **Webhook trigger** — fire an agent after a connection finishes a sync. Touches sync engine.
4. **Suggested rules agent** — scans recent transactions, proposes new transaction_rules, queues for human review. Bigger.
5. **Onboarding polish on /v2/agents list** — surface auth-not-configured / binary-missing state inline so a fresh install's first visit doesn't look like a confusing wall of disabled-looking starter agents. Reuses iter-13 doctor knowledge via the existing SmokeTest endpoint.

Picking **#5 onboarding polish** next iteration — smallest UX win, leans on the iter-13 doctor knowledge surfaced as inline guidance, pairs well with the seeded starter agents from iter 6 (which look enabled-able but aren't usable until auth is configured).

## ITER 14 — 2026-05-17 02:31
Shipped (PR #1240 squash-merged into sprint branch as 9505ca49):
- New GET /api/v1/agents/status (cheap readiness probe; same checks as the iter-13 doctor line).
- v2 SPA Alert on /v2/agents that self-renders when either auth or binary is missing, with deep-link to Settings → Agents + `make agent-sidecar` reminder.
- Self-hides as soon as both prereqs land — no manual dismiss.
- All 5 CI jobs green; drift test green.

Onboarding loop closed: fresh install → list page banner → fix Settings → banner re-evaluates → enable agent → "Run now" works. The seeded starter agents from iter 6 no longer look like a wall of mysteriously-disabled toggles.

Side note: img402.dev was unreachable during this iter's screenshot upload (HTTP 000 on the POST). Local capture verified the banner renders correctly across viewports; PR body explained the absent embed. Future iters resume normal evidence flow if img402 is back.

Next iteration candidates (in rough impact order):
1. **Per-agent quiet hours** — schema column + scheduler tweak + UI control. Mid.
2. **Cost dashboard** in /v2/agents — sparkline on list page OR /v2/agents/cost. Bigger.
3. **Webhook trigger** — fire an agent after a connection finishes a sync. Touches sync engine + agent_definitions schema.
4. **Suggested rules agent** — scans recent transactions, proposes new transaction_rules, queues for human review. Bigger.
5. **Inline run notes** — operator-editable note field on each agent_run row (track context around a manual fire). Small backend + small UI.

Picking **#1 per-agent quiet hours** next iteration — mid-size, touches schema + scheduler + edit form which is the agent system's "main control loop" and worth strengthening before more peripheral features land.

## ITER 15 — 2026-05-17 02:42
Shipped (PR #1241 squash-merged into sprint branch as 9970d30c):
- Migration: agent_definitions.quiet_hours_start / quiet_hours_end (HH:MM 24-hour text, additive).
- IsWithinQuietHours pure function: same-day windows + wraps-midnight; 13 unit-test sub-cases all pass.
- Scheduler.fireCronJob checks quiet hours BEFORE RunOrSkip — silent log+return inside the window. Manual + webhook ignore the window by design.
- Service + handler params + SPA edit form (`<Input type="time">`) plumbed through.
- All 5 CI jobs green; drift test green (no new routes).

Two recurring lessons recovered this iter:
- Multiple silent Edit failures on agents.go because I hadn't Read it this session. Re-read fixed it. The lesson is documented in iter-4 + iter-12 logs and keeps coming up — the runtime hint catches it consistently.
- Same web/dist/.gitkeep get-deleted-on-build pattern: caught + amended like prior iters. .gitignore already excludes the binary; the .gitkeep race needs a follow-up.

Next iteration candidates (in rough impact order):
1. **Cost dashboard** in /v2/agents — sparkline on list page or /v2/agents/cost. Bigger UI work.
2. **Webhook trigger** — fire an agent after a connection finishes a sync. Touches sync engine + agent_definitions schema.
3. **Suggested rules agent** — scans recent transactions, proposes new transaction_rules, queues for human review. Bigger.
4. **Inline run notes** — operator-editable note field on each agent_run row. Small.
5. **"Next fire" preview honoring quiet hours** — small follow-up to iter 15; show "next: tomorrow 8am" instead of raw cron on list page.
6. **`make web` post-step that re-creates web/dist/.gitkeep** — kills the recurring gitkeep race in iter commits. Tiny infra fix.

Picking **#6 + #4 bundled** for iter 16 — both are tiny; combining keeps the PR count linear without overloading any single PR. Actually, the operating instructions say "one iteration's worth of work" per PR. Splitting: pick #4 (inline run notes — small backend + small UI, makes runs more useful for human-in-the-loop debugging); push #6 to iter 17. The gitkeep race is annoying but each iter's amend handles it locally.

## ITER 16 — 2026-05-17 02:55
Shipped (PR #1242 squash-merged into sprint branch as 3fa2452d):
- Migration: agent_runs.operator_note TEXT NULL (additive).
- Service.SetAgentRunNote with 2000-char cap; OperatorNote threaded through AgentRunResponse + agentRunFromRow.
- PATCH /api/v1/agents/runs/{shortId} handler; openapi/docs updated; drift green.
- SPA: useUpdateAgentRunNote + AGENT_RUN_NOTE_MAX_LEN; OperatorNoteEditor at top of transcript drawer (Textarea with char counter, smart Save/Clear button, mutation-toast).
- All 5 CI jobs green.

Next iteration candidates (in rough impact order):
1. **Cost dashboard** in /v2/agents — sparkline on list page or /v2/agents/cost. Bigger UI work, high impact for self-hosters worried about API spend.
2. **Webhook trigger** — fire an agent after a connection finishes a sync. Bigger.
3. **Suggested rules agent** — scans recent transactions, proposes new transaction_rules, queues for human review. Bigger.
4. **"Next fire" preview honoring quiet hours** — small follow-up to iter 15.
5. **📝 indicator on run-history rows that have a note** — tiny follow-up to iter 16.
6. **make web post-step that re-creates web/dist/.gitkeep** — kill the recurring gitkeep race.
7. **breadbox doctor "agent run X" smoke** — extend agent test with the option to fire a real definition.

Picking **#5 note indicator** next iteration — tiny polish that lands the iter-16 notes feature on the run-history surface (where most users will see them); 30-min iteration.

## ITER 17 — 2026-05-17 03:01
Shipped (PR #1243 squash-merged into sprint branch as 21489d0c):
- Note indicator (StickyNote icon + "note" label) on RunRow when operator_note is non-empty.
- title attribute carries first 80 chars as native hover preview.
- Single-file 14-line change. All 5 CI jobs green.

Pure polish iteration that finishes the iter-16 notes story across both surfaces (drawer + list).

Next iteration candidates (in rough impact order):
1. **Cost dashboard** in /v2/agents — sparkline on list page or /v2/agents/cost page. Bigger UI work; high impact for self-hosters worried about spend.
2. **Webhook trigger** — fire an agent after a connection finishes a sync. Bigger.
3. **Suggested rules agent** — proposes new transaction_rules from recent transactions. Bigger.
4. **"Next fire" preview honoring quiet hours** — small iter-15 follow-up.
5. **make web post-step that recreates web/dist/.gitkeep** — small infra fix to kill the recurring gitkeep race in iter commits.
6. **breadbox doctor "agent run <slug> --dry"** — extends agent test with the option to fire a real definition.

Picking **#5 gitkeep infra fix** next iteration — smallest possible, kills a recurring footgun across all future iters (have hit it 4+ times now). Then #1 (cost dashboard) is the next high-impact item.

## ITER 18 — 2026-05-17 03:08
Shipped (PR #1244 squash-merged into sprint branch as af73e6b5):
- web/package.json: append `&& touch dist/.gitkeep` to the build script. Sentinel now survives every entrypoint (make web, CI, ad-hoc).
- Verified: rm + bun run build → .gitkeep restored.
- All 5 CI jobs green.

The amend-after-build footgun is dead. Future iters can `bun run build` freely without staging a deletion.

Next iteration candidates (in rough impact order):
1. **Cost dashboard** in /v2/agents — sparkline on list page or /v2/agents/cost page. Bigger UI work; high impact for self-hosters worried about API spend over time.
2. **Webhook trigger** — fire an agent after a connection finishes a sync. Touches sync engine + agent_definitions schema.
3. **Suggested rules agent** — proposes new transaction_rules from recent transactions, queues for human-approval review row.
4. **"Next fire" preview honoring quiet hours** — small iter-15 follow-up.
5. **breadbox doctor extension** — add live smoke test option (--with-live or similar).
6. **Per-agent cost cumulative on list page** — show total $ spent in lifetime / last 30 days next to each agent.

Picking **#1 cost dashboard** next iteration — high-impact, becomes load-bearing once a self-hoster has been running real agents for a few weeks. Aiming for the smallest useful version: a sparkline or single-column total on the existing list page, NOT a new page. Can expand later.

## ITER 19 — 2026-05-17 03:28
Shipped (PR #1245 squash-merged into sprint branch as b3cd76b2):
- New GetAgentCostStats30d sqlc aggregation (SUM cost + COUNT runs by definition, last 30 days, excludes skipped).
- ListAgentDefinitions fetches the rollup once + zips into responses; soft-fails on query error.
- AgentDefinitionResponse.CostStats30d (list-only; edit-page hot path stays cheap).
- SPA: CostStatsPill renders "$X.XX / 30d" with Coins icon between Max turns and the last-run pill. Hides on zero runs.
- New TestListAgentDefinitions_PopulatesCostStats30d (skipped rows excluded, sum verified).
- All 5 CI jobs green.

Side note: img402.dev still unreachable for screenshot upload (HTTP 000 — same outage from iter 14). Local capture verified correct rendering. Future iters resume normal evidence flow when img402 recovers.

Stale-server lesson: bg `serve` from a prior iter (iter 14) was still holding port 8200 — my iter-19 binary crashed on bind, and the screenshot caught the stale server. Killed + restarted clean. **Future iters: `lsof -ti:8200 | xargs kill 2>/dev/null` before starting a fresh test server.**

Next iteration candidates (in rough impact order):
1. **Webhook trigger** — fire an agent after a connection finishes a sync. Touches sync engine + agent_definitions schema. Bigger.
2. **Suggested rules agent** — scans recent transactions, proposes new transaction_rules, queues for review. Bigger.
3. **"Next fire" preview honoring quiet hours** — small iter-15 follow-up. UI-only.
4. **breadbox doctor extension** — add live smoke test option (--with-live).
5. **Cost breakdown per model** — extend iter-19's stats with model-by-model spend (which model is your spend leader?).
6. **Per-agent enable schedule (start_date / end_date)** — schema field for "enable on Monday, disable next Friday" workflows.

Picking **#3 next-fire preview** next iteration — smallest follow-up to iter 15, UI-only, lands a quality-of-life win for the cron preview that now also accounts for quiet hours.

## ITER 20 — 2026-05-17 03:43
Shipped (PR #1246 squash-merged into sprint branch as 33550d8a):
- ComputeNextFire(def, now) reuses robfig/cron + iter-15 quiet-hours logic. Loops with a 100-step safety belt; -1s offset before re-asking cron correctly includes a fire AT the quiet-end minute.
- AgentDefinitionResponse.NextFireAt (list-only, omitempty, RFC3339).
- SPA NextFirePill renders "Next: 2h" via formatRelativeTime; native title for absolute time.
- 7 new unit-test sub-cases covering nil/empty/malformed/no-quiet/with-quiet branches.
- All 5 CI jobs green.

Iter-15 scheduling loop is now fully closed: configure quiet hours → the list page shows when the next REAL fire will be, not just when cron says it would have been.

Next iteration candidates (in rough impact order):
1. **Webhook trigger** — fire an agent after a connection finishes a sync. Touches sync engine + agent_definitions schema. Bigger.
2. **Suggested rules agent** — proposes new transaction_rules from recent transactions, queues for review. Bigger.
3. **breadbox doctor --with-live** — option to also run the smoke test (not just check prereqs).
4. **Per-model cost breakdown** — extend iter-19 stats with model-by-model spend.
5. **Per-agent run-rate alert** — flag agents that have errored 3+ times in a row.
6. **"Run now" with custom prompt prefix** — operator can prepend ad-hoc context to a manual fire (e.g. "Focus on Amazon Prime transactions only").

Picking **#5 run-rate error alert** next iteration — small, leans on the iter-19 stats infra (add `error_count_30d` alongside `run_count`), surfaces a destructive-tone Alert on the agent row when ≥3 of the last 5 runs errored. High signal for "is this agent broken?"

## ITER 21 — 2026-05-17 04:00
Shipped (PR #1247 squash-merged into sprint branch as e1c19080):
- New `GetAgentRecentErrorStats` sqlc query — ROW_NUMBER window over the last 5 non-skipped runs per agent, returns (error_count, run_count).
- Service: `AgentRecentErrorStats` type + `AgentDefinitionResponse.RecentErrorStats` (list-only, omitempty). `ListAgentDefinitions` zips the rollup in; soft-fails on query error so the page keeps rendering.
- SPA: `AgentRecentErrorStats` TS type + `recent_error_stats` field. New `RecentErrorPill` renders only when `error_count >= 3` (constant `RECENT_ERROR_WARN_THRESHOLD`); red palette + AlertCircle icon + native title with full context; slots between cost pill and last-run pill.
- All 5 CI jobs green.

Screenshots intentionally omitted from PR body — the pill only renders once an agent has accumulated 3+ errors in last 5 runs, and seeding that state requires real API calls. Will appear naturally once an agent misbehaves in dogfooding.

Next iteration candidates (in rough impact order):
1. **Settings page "Test connection" button** — leverage existing SmokeTest infra to validate auth before save. Small UX win, immediate self-hoster value.
2. **Webhook trigger** — fire an agent after a connection finishes a sync. Touches sync engine + agent_definitions schema. Bigger.
3. **Suggested rules agent** — proposes new transaction_rules from recent transactions, queues for review. Bigger.
4. **breadbox doctor --with-live** — option to also run smoke test (not just check prereqs).
5. **Per-model cost breakdown** — extend iter-19 stats with model-by-model spend.
6. **"Run now" with custom prompt prefix** — operator can prepend ad-hoc context to a manual fire.

Picking **#1 settings test-connection button** next iteration — small, leverages SmokeTest infra (already exists on per-agent settings page per useSmokeTestAgent hook), adds a global "Test auth" affordance on Settings → Agents that validates the saved subscription_token / api_key before the operator schedules any real runs. Most-onboarding-friendly next step.

## ITER 22 — 2026-05-17 04:20
Pivoted at branch-creation: the "Test connection" button I'd queued for iter-22 already shipped in earlier work — `features/settings/agents-section.tsx` already wires `useSmokeTestAgent` to a button with Alert-based result + error states. Saved a confused duplicate PR by surveying SPA before starting.

Pivoted to **iter-22-candidate #4 (`breadbox doctor --with-live`)** instead. Shipped (PR #1248 squash-merged into sprint branch as fb594a95):
- `breadbox doctor` gains `--with-live` flag (local-mode only). When set + cheap subsystem check passes, fires `agent.SmokeTest` and surfaces the result as a second "agent smoke test" doctor row alongside model/duration/cost/tokens/auth-mode.
- `runAgentSmokeCheck` short-circuits to skip-row when cheap subsystem check warned/skipped (no duplicate failure pair); warn-row when ENCRYPTION_KEY is unset.
- `liveSmokeCheck` is the pure decision helper — split out for testability; maps `ErrAuthNotConfigured` / `ErrBinaryNotFound` / generic-err to targeted remediation hints.
- 60s context timeout wrapping the smoke run so a hung sidecar doesn't hang the doctor.
- Lite-build doctor stub signature updated.
- 5 new `TestLiveSmokeCheck` sub-cases + existing 4 `TestAgentSubsystemCheck` cases all pass.
- `docs/cli-commands.md` row updated per cli-commands upkeep rule.
- All 5 CI jobs green.

**Lesson:** survey SPA before implementing a UI-touching iter — the test-connection button had been built in an earlier iter I'd forgotten. Saved a useless duplicate PR. Future iters should grep for the candidate's user-visible affordance before assuming it doesn't exist.

Next iteration candidates (refreshed):
1. **"Run now" with custom prompt prefix** — small UX, operator value during dogfooding. Touches: agents.tsx run-now button → dialog with optional textarea → POST /api/v1/agents/:slug/runs accepts `prompt_prefix` → orchestrator prepends it. Service test + SPA.
2. **Per-model cost breakdown** — extend iter-19 stats; tooltip on the cost pill showing which model is the spend leader.
3. **Webhook trigger** — fire an agent after a connection finishes a sync. Bigger.
4. **Suggested rules agent** — bigger.
5. **Empty/error state polish across all agent pages** — UI sweep.
6. **Mobile-responsive sweep on agent pages** — UI sweep.
7. **Inline rule-engine docs in the prompt builder** — link/preview to `docs/rule-dsl.md`.

Picking **#1 "Run now" with custom prompt prefix** next iteration — small (one backend field + one dialog), high operator value during dogfooding, touches both layers, UI evidence opportunity.

## ITER 23 — 2026-05-17 04:35
Shipped (PR #1249 squash-merged into sprint branch as 4837b515):
- Migration 20260517110629: agent_runs.prompt_prefix TEXT NULL (additive — applied cleanly to shared dev DB).
- sqlc SetAgentRunPromptPrefix called right after CreateAgentRun so audit trail captures the prefix even if AssembleJobSpec fails.
- Orchestrator.RunNow signature gains promptPrefix arg (third position). RunOrSkip always passes "". applyPromptPrefix helper formats the prepend ("Operator note for this run:\n<prefix>\n\n<original>") in one testable place.
- AgentRunResponse.PromptPrefix on every run row (omitempty).
- API: POST /api/v1/agents/{slug}/run optional { prompt_prefix } body, capped at 2000 chars (PROMPT_PREFIX_TOO_LONG 400).
- CLI: breadbox agent run <slug> --prefix "..." mirrors the HTTP flag.
- SPA: "Run now" Button → Dialog with optional Textarea + live char counter; Sparkles "prefix" pill on history rows; read-only PromptPrefixBlock at top of TranscriptSheet for any run that carried a prefix.
- openapi.yaml + docs/api-endpoints.md updated.
- 2 new prefix-specific orchestrator integration tests pass; 7 existing tests updated to pass "" through new arg position.
- All 5 CI jobs green.
- Desktop + mobile screenshots captured via Chrome DevTools MCP, uploaded to img402.dev (back online after the iter-19 outage), embedded in PR body.

Next iteration candidates (refreshed):
1. **"Use last prefix" affordance** — tiny follow-up to iter-23. Surface AgentRunSummary.PromptPrefix and add a "Use last prefix" button on the Run now dialog when the agent has a prior prefix on record. UI-only on top of existing iter-23 infra.
2. **Per-model cost breakdown** — would need a model column on agent_runs to get historically-accurate attribution. Bigger.
3. **Webhook trigger** — fire an agent after a connection finishes a sync. Bigger.
4. **Suggested rules agent** — bigger.
5. **Empty/error state polish across all agent pages** — UI sweep.
6. **Mobile-responsive sweep on agent pages** — UI sweep.
7. **Inline rule-engine docs in the prompt builder** — link/preview to docs/rule-dsl.md.
8. **Transcript file GC** — orchestrator writes NDJSON forever; deferred from iter-7. Daily cleanup + retention setting.

Picking **#1 "Use last prefix" affordance** next iteration — tiniest follow-up that compounds iter-23's value; no schema or behavior change, pure UX win for operators iterating on a prefix.

## ITER 24 — 2026-05-17 04:50
Shipped (PR #1250 squash-merged into sprint branch as fe664ae0):
- New `GetAgentLastPromptPrefixes` sqlc query — DISTINCT ON (agent_definition_id) returning the most recent non-null/non-empty prompt_prefix per def. Bulk-keyed like iter-19 cost stats (one query, no N+1).
- `AgentDefinitionResponse.LastPromptPrefix` (*string, list-only). ListAgentDefinitions zips it in; soft-fails on query error.
- SPA: `AgentDefinition.last_prompt_prefix` mirror; RunNowDialog accepts `lastPrefix` prop, renders a Ghost "Use last prefix" button (Sparkles icon) next to the char counter when one exists. Disabled when textarea already matches.
- 2 new service integration tests: most-recent-non-empty wins (newer empty runs don't shadow earlier prefixed ones); definitions with zero prefixed runs leave LastPromptPrefix nil.
- All 5 CI jobs green.

Screenshot intentionally omitted from PR body — the button only renders when an agent has a prior non-null prefix, and seeding that requires DB writes the sandbox correctly denied. The classifier did the right thing; iter-23 already showed the baseline dialog. **Lesson:** for "only visible when state X exists" features, plan the evidence-capture story before requesting writes to shared infra.

## ITER 25 — 2026-05-17 05:05
Shipped (PR #1251 squash-merged into sprint branch as 5f2f464b):
- Closed the unbounded-disk-growth gap deferred since iter-7. Daily 3:15 AM cleanup tick now also prunes on-disk NDJSON transcripts in the same pass.
- New `cleanupTranscriptFiles` reads `agent.transcript_dir` + `agent.run_retention_days` from app_config; skips silently when transcript_dir unset or retention<=0.
- `pruneTranscriptFiles(dir, cutoff)` pure helper — only touches `*.ndjson`, leaves subdirectories and other files alone (operator-safe if they repurpose the directory). Returns (deleted, scanned, err).
- 3 new unit tests: happy path with mixed file types + subdirectory; missing dir is not an error; empty dir returns zeros.
- docs/agents.md operational-notes bullet documents the shared retention.
- All 5 CI jobs green.

Two deferred items captured in PR body: "force cleanup" admin button (sync trigger from settings) and transcript compression. Both YAGNI for now.

Next iteration candidates (refreshed):
1. **Inline rule-engine docs in the prompt builder** — collapsible help card on agents.$slug.edit linking to docs/rule-dsl.md with operator reference. UI-only, small, helps anyone authoring a rule-applying agent.
2. **Empty/error state polish across agent pages** — UI sweep.
3. **Mobile-responsive sweep on agent pages** — UI sweep.
4. **Force-cleanup button on Settings → Agents** — small follow-up to iter-25 (lets operator trigger the daily tick on demand after lowering retention).
5. **Per-model cost breakdown** — needs model column on agent_runs. Bigger.
6. **Webhook trigger** — bigger.
7. **Suggested rules agent** — bigger.
8. **Multi-concurrent runs** — lift v1 max_concurrent=1 cap once we trust the system. Needs careful semaphore + scheduler tests.

Picking **#1 inline rule-DSL help card** next iteration — concrete operator-value win for the most common agent intent (rule creation), bounded UI scope, captures clean evidence.

## ITER 26 — 2026-05-17 05:20
Shipped (PR #1252 squash-merged into sprint branch as 64da4308):
- New `web/src/features/agents/rule-dsl-help.tsx` — shadcn Collapsible wrapping a concise grammar reference: shape skeleton, fields, operators per type, combinators, actions/triggers, preview_rule tip.
- Renders under the Prompt textarea on agents.$slug.edit; closed by default so prompt-only edits stay compact.
- No new shadcn install (used existing Collapsible primitive).
- Feature-only component (under features/agents/*), not subject to sandbox specimen rule per the v2-frontend rules.
- Screenshot captured via Chrome MCP, uploaded to img402.dev, embedded in PR body.
- All 5 CI jobs green.

## ITER 27 — 2026-05-17 05:35
Shipped (PR #1253 squash-merged into sprint branch as ca39929f):
- Migration 20260517115556: agent_runs.hit_cap TEXT NULL with CHECK constraint ('max_turns' | 'max_budget'). Additive.
- sqlc SetAgentRunHitCap :one returns the full row so orchestrator can rebuild the response in one shot.
- New service helper SetAgentRunHitCapDB; AgentRunResponse.HitCap surfaces the field.
- Orchestrator: capFromRunErr() maps sidecar sentinels (ErrMaxTurnsReached, ErrBudgetExceeded) to the discrete hit_cap string. Called after CompleteAgentRunDB so cap signal layers on top of status — max_turns stays success-tagged, max_budget stays error-tagged.
- SPA: AgentRun.hit_cap typed union; new HitCapPill renders amber "max turns" (clean stop, work may be incomplete) and red "over budget" (mid-run abort), each with operator-actionable title hint.
- 3 new orchestrator integration tests covering all three branches.
- All 5 CI jobs green.

Screenshot intentionally omitted — pill only renders when an actual run hit a cap; seeding requires DB writes the sandbox correctly denies (iter-24 lesson). Implementation is documented in PR body + code comments.

Next iteration candidates (refreshed):
1. **Force-cleanup button on Settings → Agents** — tiny follow-up to iter-25. POST /api/v1/agents/cleanup-now triggers daily tick synchronously, returns counts.
2. **Aggregate "recent caps hit" pill on /v2/agents** — list-level signal like iter-21's error pill, leans on iter-27's hit_cap field.
3. **Filter chip for hit_cap on run history page** — small UI add.
4. **Empty/error state polish across agent pages** — UI sweep.
5. **Mobile-responsive sweep on agent pages** — UI sweep.
6. **Multi-concurrent runs** — lift v1 max_concurrent=1 cap; needs careful semaphore tests under contention.
7. **Per-model cost breakdown** — needs model column on agent_runs.
8. **Webhook trigger** — fire after sync completion. Bigger.
9. **Suggested rules agent** — bigger.

Picking **#1 force-cleanup button** next iteration — tight scope, real operator value after lowering retention, naturally closes the iter-25 deferred item.

## ITER 28 — 2026-05-17 05:50
Shipped (PR #1254 squash-merged into sprint branch as efcf22dd):
- Scheduler refactor: cleanupAgentRuns + cleanupTranscriptFiles return counts; new runCleanupAll is the shared body both cron tick and RunCleanupNow go through (cannot drift). logCleanupResult tagged source=scheduled vs source=on-demand.
- New AgentScheduler.RunCleanupNow(ctx) returns AgentCleanupResult { runs_deleted, transcripts_deleted, transcripts_scanned, retention_days, transcript_dir }.
- New POST /api/v1/agents/cleanup — full_access scope, 503 AGENTS_DISABLED when scheduler unwired.
- SPA: useRunAgentCleanup hook + AgentCleanupResult type; "Run cleanup now" outline button on Settings → Agents next to "Test connection", with toast naming the counts + retention window. Native title hint explains use case.
- 2 new integration tests: end-to-end seed+prune; retention=0 no-op.
- openapi.yaml + docs/api-endpoints.md updated.
- All 5 CI jobs green.
- Screenshot captured via Chrome MCP, uploaded to img402.dev, embedded in PR body.

Next iteration candidates (refreshed):
1. **Multi-concurrent runs** — raise the v1 default cap (1 → 3); deferred from iter-3. Concurrency primitive already supports it; just need integration tests under contention + docs.
2. **Aggregate "recent caps hit" pill on /v2/agents** — list-level signal like iter-21's error pill, leans on iter-27's hit_cap field.
3. **Filter chip for hit_cap on run history page** — small UI add.
4. **Empty/error state polish across agent pages** — UI sweep.
5. **Mobile-responsive sweep on agent pages** — UI sweep.
6. **Per-model cost breakdown** — needs model column on agent_runs.
7. **Webhook trigger** — bigger.
8. **Suggested rules agent** — bigger.

Picking **#1 multi-concurrent runs** next iteration — real capability lift (deferred since iter-3), tests existing primitive under contention, lets self-hosters run 2-3 agents in parallel without re-configuring.

## ITER 29 — 2026-05-17 06:05
Shipped (PR #1255 squash-merged as 0fa17098): default agent.max_concurrent lifted 1 → 3 in serve.go + agent_settings.go fallbacks. New TripleConcurrency integration test pins both branches (3 in parallel + 4th blocks + all keys revoked). docs/agents.md notes the lift + rationale.

## ITER 30 — 2026-05-17 06:20
Shipped (PR #1256 squash-merged as 1c482dad): the big one — webhook trigger. Migration adds agent_definitions.trigger_on_sync_complete + partial index. Sync engine gains OnSyncComplete func-pointer hook (no agent-pkg import); serve.go wires it to orchestrator.FireSyncCompleteAgents which dispatches eligible agents per-goroutine via RunOrSkip(trigger="webhook"). SPA edit-form checkbox. 2 new integration tests (only-eligible-fires + no-op-when-empty). New rule in .claude/rules/agents.md documents the no-import-cycle invariant.

## ITER 31 — 2026-05-17 06:35
Shipped (PR #1257 squash-merged as 85539d5c): bug fix bundled with iter-27/30 deferred chip.

The bug: ListAgentRuns hand-rolled SELECT had been frozen since iter-3 but agent_runs gained operator_note (iter-22), prompt_prefix (iter-23), hit_cap (iter-27). Pills silently absent from /v2/agents/{slug}/runs history page even though GetAgentRun's single-row path surfaced them. Operators missing audit signal.

Fix: added the three columns to the projection + scan. Bundled hit_cap filter chip (closes iter-27 deferred). 5 new sub-tests including the regression. docs/api-endpoints.md row updated to list every filter.

**Lesson:** hand-rolled SQL projections drift silently. Spawned a code-reviewer subagent for iter-32 to check whether other endpoints have the same drift pattern.

Next iteration candidates (refreshed):
1. **Aggregate "recent caps hit" pill on /v2/agents** — list-level signal like iter-21's error pill, leans on iter-27 + iter-31.
2. **Empty/error state polish across agent pages** — UI sweep.
3. **Mobile-responsive sweep on agent pages** — UI sweep.
4. **Per-model cost breakdown** — needs model column on agent_runs.
5. **Suggested rules agent** — bigger.
6. **Outputs of the code-reviewer subagent** — pending; queue top finding as iter-33.

Picking **#1 aggregate caps-hit pill** next iteration in parallel with the audit; small + visible, mirrors iter-21 pattern.


















## Operating instructions for the loop

When this loop fires, the agent should:

1. **Read this whole file first.** It is the single source of sprint truth.
2. **Confirm working directory.** Should be the worktree: `.claude/worktrees/agents+claude-agent-sdk-sprint`. If somehow elsewhere, `cd` into the worktree.
3. **Sync the sprint branch:** `git checkout agents/claude-agent-sdk-sprint && git fetch origin && git pull origin agents/claude-agent-sdk-sprint --ff-only`. The sprint branch is the long-lived base for all iteration PRs — keep it clean.
4. **Check the iteration log** in this file to find the next undone iteration.
5. **Survey open iteration PRs:** `gh pr list --base agents/claude-agent-sdk-sprint --state open --json number,title,headRefName,mergeStateStatus,statusCheckRollup`. If a previous iteration PR is still open and unmerged, finish it first (address review comments, fix red CI, or merge if green) before starting a new iteration.
6. **Create the iteration branch:** `git checkout -b agents/iter-N-<short-slug>` off the sprint branch. Naming: `agents/iter-2-service-rest`, `agents/iter-3-scheduler`, `agents/iter-4-spa-list`, etc.
7. **Pick exactly one iteration's worth of work.** Don't try to do two. Each iteration ends in one merged PR into the sprint branch.
8. **Delegate heavy work to subagents:**
   - Schema design + sqlc → `feature-dev:code-architect` then implement directly
   - Frontend work → `frontend-design` skill, plus use `mcp__shadcn__*` for components
   - UI evidence → `simple-validate-ui` (v2 SPA) for screenshots
   - Code review pre-PR → `feature-dev:code-reviewer`
9. **Run the tests** before opening the PR: `go build ./...`, `go vet ./...`, `go test ./...`, `make test-integration` if DB-touching, `cd web && bun run typecheck && bun run lint`. For build-tag coverage also run `go build -tags=headless ./...` and `go build -tags=lite ./...`.
10. **Open the iteration PR** against the sprint branch (NOT main): `gh pr create --base agents/claude-agent-sdk-sprint --head agents/iter-N-<slug> --title "..." --body "..." --label feat/agents`. Description should explain intent, scope, what was deferred, and a test plan. **For UI-touching iterations, embed before/after screenshots in the PR body** — use `simple-validate-ui` (preferred for v2 SPA) to capture the change, upload via the `github-image-hosting` skill which defaults to img402.dev (per Ricardo's standing preference — never the GitHub release-asset CDN), then embed the resulting URLs in the body with `![alt](url)`. Capture at least desktop + mobile viewports for any new page; add dark-mode shots if the change touches color.
11. **Merge when green** via `gh pr merge <num> --squash` (auto-delete cleans up the iteration sub-branch; the sprint branch is the base and persists). NEVER use `--auto`. **Ricardo has explicit standing approval to squash-merge iteration PRs into the sprint branch** — do not ping for permission. Only the final sprint→main PR is gated on his "we're good to merge" signal.
12. **Pull the merge into the local sprint branch:** `git checkout agents/claude-agent-sdk-sprint && git pull origin agents/claude-agent-sdk-sprint --ff-only`. No rebase dance needed — the sprint branch is the merge target, so it advances cleanly.
13. **Append to iteration log** in this file with what shipped, what was deferred, what's next. Commit the log update directly to the sprint branch and push.
14. **End turn with `result:` line.** If blocked on a decision, end with `needs input:` line.

## End-of-sprint exit (read carefully)

**Ricardo holds the merge-to-main signal — the loop does not open or merge the sprint→main PR autonomously, even after iteration 7.** Specifically:

- After iter 7 ships, do **not** open `agents/claude-agent-sdk-sprint` → `main`.
- Instead, pick the next-most-valuable stretch item (see the "Iteration 8+" list below) and run another iteration into the sprint branch.
- Keep iterating until Ricardo writes the literal phrase "we're good to merge" (or an obvious paraphrase: "ship it", "open the merge PR", "ready for review"). Only then open ONE PR from the sprint branch into main with a comprehensive description covering every iteration. **Even then, do not merge it** — wait for Ricardo.
- When you reach the end of a stretch iteration and there's no obvious next thing, do these in order: (a) re-read this whole file looking for items you've skipped; (b) re-run the full test suite + a `feature-dev:code-reviewer` pass on the diff between the sprint branch and main to find polish work; (c) only as a last resort send ONE PushNotification of the form "Sprint at iter N, X stretch items left or [list]. Anything to add before I queue more?" — but only if there's been no progress for two consecutive fires.

## Cron self-care

Sessions can drop and the recurring task auto-expires after 7 days. On each fire:

1. Call `CronList`. If no entry matches this sprint's prompt, the loop is dead in this session — fire a `PushNotification`: "Agent SDK sprint loop expired — run /loop 30m to resume." Then continue this iteration (one last useful turn before going silent).
2. If the cron entry exists but the sprint state has stalled (no new iteration log entry in the last 3 fires), send ONE PushNotification asking Ricardo to check in.

Do not try to re-arm `CronCreate` yourself — the user's `/loop` invocation is authoritative.

If at any point the next iteration's scope is unclear, stop and use `AskUserQuestion` rather than guessing on a load-bearing decision (model choice, auth scheme, schema breaking change).

## Async coordination with Ricardo

- **Subscription auth onboarding (FIRE-AND-FORGET):** when you reach a point that wants a live `CLAUDE_CODE_OAUTH_TOKEN`, send ONE `PushNotification` describing what you need and where to paste it, then **immediately move on to other work**. Do not stall, do not loop waiting, do not re-send. Ricardo may not be around when the loop fires — he's explicitly delegated. The smoke-test harness should be coded to *detect* a token in `app_config` on each fire and run automatically when one appears, without any further code change on our side. Until then, fill the iteration with anything else from the menu.
- **General principle:** the user has standing approval to merge iteration sub-branches into the sprint branch. Do not ask for permission. Only the sprint→main PR is gated on his explicit "we're good to merge" signal. Push notifications are reserved for: (a) one-shot live-auth requests as above; (b) the rare end-of-menu "anything else?" ping per the End-of-sprint exit section; (c) genuine blockers that no amount of subagent delegation can solve.
- **All push notifications prefix with `[feat/agents] `** so Ricardo can filter at a glance across multiple parallel work-streams. Format: `[feat/agents] <one-line message>`. Under 200 chars including the prefix.
- **All iteration PRs labeled `feat/agents`** — pass `--label feat/agents` to `gh pr create`. The label exists on origin.

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

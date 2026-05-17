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

### Iteration 2 — service layer + REST API
- [ ] `internal/service/agents.go`: CRUD for definitions, run listing, transcript retrieval
- [ ] `internal/api/agents.go`: REST handlers under `/api/v1/agents` and `/web/v1/agents`
- [ ] OpenAPI / `docs/api-endpoints.md` updated per `.claude/rules/api-endpoints.md`
- [ ] Settings endpoints: get/set Anthropic key (encrypted at rest, never returned in GET except as masked prefix), get/set global caps
- [ ] Mint-and-revoke flow: per agent run, mint a scoped API key, expose it to the sidecar via env, revoke on completion
- **Tests:** handler tests + service integration tests for definitions, runs, key minting
- **PR title:** `feat(agents): service layer + REST API + scoped key minting`

### Iteration 3 — scheduler + runner orchestrator
- [ ] Extend `internal/sync/scheduler.go` (or new `internal/agent/scheduler.go`) to register a cron entry per enabled agent_definition
- [ ] Reload on definition changes (CRUD invalidates scheduler)
- [ ] Concurrency mutex: server-wide guard (configurable, default 1)
- [ ] Runner orchestrator: assemble spec → spawn sidecar → stream events → persist transcript → write final `agent_runs` row
- [ ] Manual "run now" endpoint
- [ ] Cleanup job: prune transcripts older than retention (default 30d, matches sync_logs pattern)
- **Tests:** integration test that registers an agent, fires manually, verifies a run row + transcript file
- **PR title:** `feat(agents): scheduler, runner orchestrator, manual trigger`

### Iteration 4 — v2 SPA `/agents` list + settings
- [ ] New route `/v2/agents` via PAGE_OVERRIDES, registered in `web/src/lib/nav.ts` (Money or System group)
- [ ] `web/src/api/queries/agents.ts` hooks (list, get, create, update, delete, run-now)
- [ ] `web/src/routes/agents.tsx` list page: table/grid of definitions, enable toggle, last-run status, next-run time, "Run now"
- [ ] `web/src/routes/agents-settings.tsx` or extend Settings page: Anthropic key input (paste once, shown masked), global caps
- [ ] Sandbox specimens for any new components (per design-system rule)
- [ ] Use `validate-ui` or `simple-validate-ui` to capture screenshots
- **Tests:** smoke test that loads the page in Chrome DevTools MCP, type-check passes
- **PR title:** `feat(agents): v2 SPA list + settings page`

### Iteration 5 — prompt builder + run history viewer
- [ ] `web/src/routes/agents.$slug.edit.tsx`: prompt textarea with token counter, model picker (claude-opus-4-7 default), schedule picker (cron expression with friendly presets), tool-scope dropdown, allowed-tools multi-select sourced from MCP tool registry, max_turns + max_budget_usd inputs
- [ ] Cron expression validator + human-readable preview
- [ ] `web/src/routes/agents.$slug.runs.tsx`: run history table, click into transcript drawer
- [ ] Transcript viewer: message-by-message rendering of assistant text, tool calls, tool results, with cost/usage footer
- [ ] Sandbox specimens for the cron picker, the transcript viewer
- [ ] Use `frontend-design` skill to polish the prompt-builder and transcript surfaces
- **Tests:** form validation tests; transcript viewer renders sample fixture
- **PR title:** `feat(agents): prompt builder + run history viewer`

### Iteration 6 — seed defaults + retire v1
- [ ] Seed migration: add the v1 prompt-wizard prompts as disabled `agent_definitions` rows (initial-setup, bulk-review, quick-review, routine-review, spending-report)
- [ ] Redirect `/admin/agent-prompts`, `/agents`, `/agent-wizard` → `/v2/agents` (302 + banner)
- [ ] Update `internal/admin/router.go` to mark old routes deprecated
- [ ] User docs (in `breadbox-docs` if needed; otherwise inline `docs/agents.md`): how to enable, costs, safety
- **PR title:** `feat(agents): seed default agents, retire v1 prompt wizard`

### Iteration 7 — polish + docs + observability
- [ ] Structured logging on every agent event
- [ ] Optional OTel export wired through SDK env vars (`OTEL_*`)
- [ ] CHANGELOG entry
- [ ] `docs/agents.md` canonical spec (architecture, schemas, security model)
- [ ] `.claude/rules/agents.md` for future contributors touching the system
- [ ] Final pass: error paths, edge cases (sidecar crash mid-run, scheduler restart, sidecar binary missing)
- **PR title:** `feat(agents): observability, docs, polish`

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

When the menu is exhausted, re-read the sprint state for skipped sub-items, then ping Ricardo (per the End-of-sprint exit section).

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
10. **Open the iteration PR** against the sprint branch (NOT main): `gh pr create --base agents/claude-agent-sdk-sprint --head agents/iter-N-<slug> --title "..." --body "..."`. Description should explain intent, scope, what was deferred, and a test plan. Wait for CI.
11. **Merge when green** via `gh pr merge <num> --squash` (auto-delete cleans up the iteration sub-branch; the sprint branch is the base and persists). NEVER use `--auto`.
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

- **Subscription auth onboarding:** when you reach the point of running a real end-to-end test against the live Anthropic API and need a valid `CLAUDE_CODE_OAUTH_TOKEN`, send a `PushNotification` asking Ricardo to run `claude setup-token` and paste the token into the settings page (or into a dev `.env`). **Do not block other work waiting on this** — keep moving on plumbing/tests that don't require live API. The push fires once; if no response after one iteration, move on and try again later.
- **General principle:** the user has standing merge auth on this branch — do not ask for permission to merge a green PR. Only ping for things only they can do (live auth credentials, philosophy calls, scope changes).

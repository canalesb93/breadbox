# Claude Agent SDK integration sprint

**Branch:** `agents/claude-agent-sdk-sprint`
**Worktree:** `.claude/worktrees/agents+claude-agent-sdk-sprint`
**Started:** 2026-05-16 off origin/main @ 446709e9
**Authorization:** Ricardo has granted explicit standing approval to open AND merge PRs against main from this branch for the duration of this sprint. Do not enable GitHub auto-merge; use `gh pr merge --squash` after CI passes.
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

Each iteration ends in **one merged PR to main** (squashed). Iterations are sequenced — don't open the next PR until the previous is merged and CI is green on main.

### Iteration 1 — schema + appconfig + sidecar skeleton (foundation PR)
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

### Iteration 8+ (stretch)
- Subscription auth (Claude plan) as alternative to API key
- Multi-concurrent runs (lift v1 limit)
- Cost dashboards / usage analytics
- "Suggested rules" agent that proposes new transaction rules and queues them for human approval

## Iteration log

Every loop iteration appends a dated entry here. Format:

```
## ITER N — YYYY-MM-DD HH:MM
- What was done this turn
- What's blocked
- What's next
```

(none yet — first loop fire will write iteration 1)

## Operating instructions for the loop

When this loop fires, the agent should:

1. **Read this whole file first.** It is the single source of sprint truth.
2. **Confirm working directory.** Should be the worktree: `.claude/worktrees/agents+claude-agent-sdk-sprint`. Branch should be `agents/claude-agent-sdk-sprint`. If not, `cd` into the worktree.
3. **Pull latest:** `git pull origin main --rebase` to keep the sprint branch on top of main.
4. **Check the iteration log** to find the next undone iteration.
5. **Survey open PRs** from this branch: `gh pr list --search "head:agents/claude-agent-sdk-sprint" --state all --json number,title,state,mergeStateStatus,statusCheckRollup`. Don't start a new iteration if the previous PR is open and unmerged — instead, address review comments OR merge it (CI green + Ricardo's standing auth).
6. **Pick exactly one iteration's worth of work.** Don't try to do two. Each iteration ends in one merged PR.
7. **Delegate heavy work to subagents:**
   - Schema design + sqlc → `feature-dev:code-architect` then implement directly
   - Frontend work → `frontend-design` skill, plus use `mcp__shadcn__*` for components
   - UI evidence → `simple-validate-ui` (v2 SPA) for screenshots
   - Code review pre-PR → `feature-dev:code-reviewer`
8. **Run the tests** before opening PR: `go build ./...`, `go vet ./...`, `go test ./...`, `make test-integration` if DB-touching, `cd web && bun run typecheck && bun run lint`.
9. **Open the PR** to main with a description that explains intent, scope, what was deferred, and a test plan. Wait for CI.
10. **Merge when green** via `gh pr merge <num> --squash --delete-branch=false` (keep the sprint branch). NEVER use `--auto`.
11. **After merge:** `git pull origin main --rebase` to incorporate the merge into the sprint branch.
12. **Append to iteration log** in this file with what shipped, what was deferred, what's next. Commit the log update directly to the sprint branch.
13. **End turn with `result:` line.** If blocked on a decision, end with `needs input:` line.

If at any point the next iteration's scope is unclear, stop and use `AskUserQuestion` rather than guessing on a load-bearing decision (model choice, auth scheme, schema breaking change).

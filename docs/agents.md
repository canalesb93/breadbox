# Agents

Recurring AI-powered workflows that run via the Claude Agent SDK and call breadbox MCP to enrich, categorize, and review your data. Agents replace the v1 admin "agent prompts" wizard with a real scheduling + execution system.

## Quick start

1. **Authenticate**: Settings → Agents → pick auth mode.
   - **Subscription token (recommended for hobbyists)**: run `claude setup-token` on any machine, paste the resulting `sk-ant-oat01-…` token. Free under your Claude plan's monthly Agent SDK credit (until 2026-06-15; details: https://support.claude.com/en/articles/15036540).
   - **Anthropic API key**: paste an `sk-ant-…` key from console.anthropic.com. Billed per API call; durable past the 2026-06-15 cutover.
2. **Build the sidecar binary** (one-time):
   ```sh
   make agent-sidecar
   ```
   Outputs `bin/breadbox-agent`. Set `agent.runtime_path` in Settings → Agents if you put it elsewhere.
3. **Pick a starter agent** from the v2 SPA at `/v2/agents`. Five defaults are seeded on fresh installs (disabled):
   - **Initial Setup** — broad rule + category mapping after first sync.
   - **Bulk Review** — thorough categorization pass over a large queue.
   - **Quick Review** — fast batch-categorize, prioritizes speed.
   - **Routine Review** — daily/weekly pass over fresh transactions.
   - **Spending Report** — weekly category-grouped summary with anomalies.
4. **Edit prompt + schedule**, click Save, then flip the Enabled toggle. The agent fires on its cron schedule and shows up in the Runs page.
5. **Hit Run now** in the list page to trigger immediately. The result lands in the run history with a full transcript (tool calls + cost + token usage).

### Verify everything is wired

Before you run a real agent, sanity-check the chain with the diagnostic CLI:

```sh
./breadbox agent test
```

It spawns the sidecar with a tiny "say OK" prompt (no MCP servers, no agent definition, bounded to ~5¢). On success you'll see something like:

```
🔎 breadbox agent test

  ✓ auth          subscription
  ✓ binary        /Users/you/breadbox/bin/breadbox-agent
  ✓ model         claude-haiku-4-5
  ✓ duration      812ms
  ✓ cost          $0.000123 (15 in / 1 out tokens)
  ✓ response      "OK"

Smoke test passed. The agent subsystem is ready to run real definitions.
```

Exit code 3 = no Anthropic credential; exit code 5 = sidecar binary not found.

## Architecture (one-line view per layer)

| Layer | Lives at | What it does |
|---|---|---|
| v2 SPA | `web/src/routes/agents*.tsx` | List, edit, run history, transcript viewer |
| REST API | `internal/api/agents.go` | 13 endpoints under `/api/v1/agents` |
| Service layer | `internal/service/agents.go` | CRUD, mint scoped API key, assemble JobSpec |
| Orchestrator | `internal/service/agent_orchestrator.go` | Mint → Run → Persist → Revoke |
| Scheduler | `internal/service/agent_scheduler.go` | One cron entry per enabled definition |
| Sidecar | `agent/sidecar/index.ts` | TypeScript Agent SDK runner; spawned per run |
| Storage | `agent_definitions`, `agent_runs` | Plus transcript NDJSON files on disk |

## Safety controls

- **Per-agent caps**: `max_turns` and `max_budget_usd` on every definition.
- **Global ceiling**: `agent.global_max_budget_usd` in Settings → Agents.
- **Concurrency**: `agent.max_concurrent` (default 3 since iter-29; was 1 in iter-1 as a v1 safety net, lifted after the orchestrator's mint-and-revoke proved out under contention). Excess cron fires log as `skipped` rows; manual runs return 503 `CONCURRENCY_LOCKED`. Raise (or drop back to 1) in Settings → Agents.
- **Triggers**: `cron` (schedule_cron), `manual` (Run now / API /run), and `webhook` (iter-30 — opt-in per agent via `trigger_on_sync_complete`, fires after every successful bank sync). The orchestrator surfaces the trigger on every run row so the history shows which path fired it.
- **Scope**: per-agent `tool_scope` is `read_only` or `read_write`. Read-only agents mint a read-only API key that the MCP server rejects writes on.
- **Mint-and-revoke**: every run gets a fresh, scoped API key named `agent:<slug>:<runShortID>`. It's revoked the instant the run completes (or errors).
- **Encrypted at rest**: subscription tokens and API keys are AES-256-GCM encrypted in `app_config`. The full value never leaves the server after you save it — `GET /api/v1/agents/settings` returns a masked display string.

## Operational notes

- **Cleanup**: completed runs older than `agent.run_retention_days` (default 30) are pruned daily at 3:15 AM local time. The matching on-disk transcript files (`<agent.transcript_dir>/<runID>.ndjson`) are pruned in the same pass using the same retention so the two surfaces stay in sync.
- **Orphan recovery**: in-progress runs from a previous boot are marked `error` at startup so the run history doesn't lie.
- **Auth precedence trap**: if both `ANTHROPIC_API_KEY` and `CLAUDE_CODE_OAUTH_TOKEN` are in the sidecar's env, the API key wins. The sidecar scrubs the unused var before invoking the SDK so this can't bite you accidentally.

## What replaced what

The v1 admin "agent prompts" library at `/admin/agent-prompts` is retired. Every legacy path 302s to `/v2/agents`:
- `/agent-prompts` (and `/agent-prompts/builder/*`)
- `/agents`
- `/agent-wizard/*`

The five seeded starter agents are direct ports of the v1 wizard's prompts. Edit them in place; they're now real schedulable agents rather than copy-to-clipboard text.

## See also

- `docs/api-endpoints.md` — REST catalog for `/api/v1/agents/*`
- `internal/agent/seed.go` — list of starter agents and their prompt mappings
- `prompts/agents/` — the markdown source for every seeded prompt
- `.claude/sprint-state.md` — sprint history and design decisions

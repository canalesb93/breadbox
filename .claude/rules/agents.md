# Agents subsystem

The Claude Agent SDK integration. Lets a self-hoster schedule recurring AI runs that call breadbox MCP. Replaces the v1 admin "agent prompts" wizard.

User-facing reference: [docs.breadbox.sh/guides/multi-agent-reviewer](https://docs.breadbox.sh/guides/multi-agent-reviewer). Sprint history + design decisions: `.claude/sprint-state.md` on the `agents/claude-agent-sdk-sprint` branch.

## The five layers

```
admin UI (internal/admin/workflows_gallery_page.go, workflows_runs_page.go,
          agent_runs_page.go (run detail + transcript), agent_sdk_settings_page.go
          backed by internal/templates/components/pages/workflows_gallery.templ,
          workflows_runs.templ, agent_run_detail.templ, agent_sdk_settings.templ.
          Custom + preset workflows are created/edited via drawers on /workflows;
          the legacy /agents form + prompt-library wizard were removed.)
   │ REST /api/v1/agents/* (via browser session cookie)
   ▼
internal/api/agents.go            ← HTTP handlers, error envelope mapping
   │
internal/service/agents.go         ← CRUD, validation, helpers
internal/service/agent_settings.go ← AES-GCM masked-on-GET token storage
internal/service/agent_orchestrator.go ← mint → run → persist → revoke
internal/service/agent_scheduler.go    ← robfig/cron, one entry per enabled def
   │
internal/agent/                    ← Runner / Sidecar / Semaphore / Spec types
   │ exec ./bin/breadbox-agent
   ▼
agent/sidecar/index.ts             ← TypeScript Claude Agent SDK runner
   │ MCP stdio
   ▼
breadbox mcp                       ← MCP tool registry (read-only or full-access)
```

When in doubt about where new code belongs, ask: is it admin page shell / form state → `internal/admin/` + the matching `.templ`; is it user input/response shaping → `api/`; is it stateful behavior → `service/`; is it the runner protocol / event shapes → `internal/agent/`; is it sidecar logic → `agent/sidecar/`.

## Local dev across worktrees

Two paths bite worktree workflows because each `claude -w` worktree is a fresh checkout sharing one dev DB:

- **Sidecar binary**: install once with `make agent-sidecar-install-user`. Writes to `~/.breadbox/agent-bin/breadbox-agent` (the priority-4 discovery slot). Per-worktree `bin/breadbox-agent` is NOT in `.worktreeinclude` — it'd cost 50 MB per worktree to copy and the user-home install achieves the same thing once.
- **Transcript dir**: `agent.DefaultTranscriptDir()` (`internal/agent/transcript_dir.go`) checks `BREADBOX_AGENT_TRANSCRIPT_DIR` before falling back to the cwd-relative default. The session-start hook exports it to `~/.local/share/breadbox/transcripts/agents` for worktree sessions so every local server reads the same dir. Docker / prod containers don't set the env var, so they still default to `/app/transcripts/agents` (which the prod compose file mounts as a named volume). When you add a new call site that resolves `agent.transcript_dir`, use `agent.DefaultTranscriptDir()` as the appconfig fallback — don't hardcode `"transcripts/agents"`.

## Locked invariants

These are easy to break and painful to re-discover.

### Auth precedence trap

If both `ANTHROPIC_API_KEY` and `CLAUDE_CODE_OAUTH_TOKEN` are set in the sidecar's env, the API key silently wins. The sidecar must scrub the unused var before invoking the SDK (`agent/sidecar/index.ts::configureAuth`). Never remove that scrub; never assume the SDK does the right thing here.

### Mint-and-revoke

Every run mints a scoped `actor_type='agent'` API key (name format `agent:<slug>:<runShortID>`) and revokes it in a `defer` with a fresh context. Never leave a key valid past the run that minted it. If you add a new entry point that runs an agent, wrap it in `Orchestrator.RunNow` or `Orchestrator.RunOrSkip` — don't reimplement the mint-revoke dance.

### Sync-completion webhook is a hook, not a dependency

`sync.Engine.OnSyncComplete` is a function pointer the engine fires after each successful sync. The agent orchestrator wires itself up in `serve.go` — the engine itself has no knowledge of agents (no import cycle, no agents-subsystem coupling). When you add another post-sync responsibility, prefer extending the hook (or stacking another one) over importing the agent package into `internal/sync/`.

### Concurrency split

- `RunNow` (manual trigger from HTTP): `ErrConcurrencyLocked` returns WITHOUT creating an `agent_runs` row. Caller maps to 503; user can retry without polluting history.
- `RunOrSkip` (cron trigger): always leaves a row. Status `skipped` when locked, so the run history shows what was missed.

Don't blur these. Cron callers must use `RunOrSkip`; HTTP callers must use `RunNow`.

### Fresh-ctx revocation

`Orchestrator.runLocked` defers the API key revoke with `context.WithTimeout(context.Background(), 10*time.Second)` — a NEW root context, not the request ctx. A cancelled parent (user closed the run-now request, scheduler restart, server shutdown) must NOT prevent revocation.

### Hot-reload via `OnDefinitionChanged`

CRUD mutations on agent_definitions call `svc.OnDefinitionChanged()` which the orchestrator wires to `AgentScheduler.Reload(ctx)`. The full sequence: service method updates DB → notifies hook → scheduler removes all per-agent entries → re-registers from DB. Don't add new mutation paths without calling the hook.

### Run-key attribution — the run key IS the actor

Every write a run makes must be attributed to its minted run key (`agent:<slug>:<runID>`, `actor_type='agent'`, `actor_name=def.Name`, `workflow_id=def.ID`), NOT to the MCP `clientInfo`. The Claude Agent SDK presents a generic shared `clientInfo` ("claude-code") on every connection; if attribution falls back to it, every agent collapses onto one identity and the feed shows one session under several names + avatars.

Two load-bearing pieces enforce this:

1. `AssembleJobSpec` passes the run key in `BREADBOX_API_KEY`; `runMCPStdio` (`internal/cli/mcp.go`) binds it as the actor floor via `ValidateAPIKey`. If you change how the sidecar reaches MCP, keep the run key as the bound actor.
2. `MCPServer.rebindActorFromClientInfo` (`internal/mcp/server.go`) is gated on `service.AgentRunShortIDFromContext(ctx) == ""` — it upgrades anonymous stdio / external clients to a per-`clientInfo` key but **never** clobbers a real run key. Don't remove that guard.

`api_keys.workflow_id` (set at mint) is the durable link; `Service.ResolveAgentSlugForActor` / `GetAgentIdentityByApiKeyID` resolve any agent actor → its workflow (name + slug avatar). Render an agent's identity through these, not through the raw stamped `actor_name`.

## Tests + CI

- Service-layer integration tests: `internal/service/agents_test.go`, `agent_orchestrator_test.go` (one TestMain shared with the rest of `service_test`).
- Schema + sqlc integration tests: `internal/db/agent_tables_integration_test.go`, `agent_queries_integration_test.go`.
- Runner unit tests: `internal/agent/sidecar_test.go` (no DB; uses a fake shell-script sidecar).
- OpenAPI drift: every new HTTP route MUST land in `openapi.yaml` in the same PR, or `TestOpenAPIDrift` will red-fail CI.
- Iteration PRs on the agents sprint branch get the full CI matrix because the branch is in `.github/workflows/ci.yml::pull_request.branches`.

## Observability

The sidecar inherits the parent process env, so any `OTEL_*` env var set on `breadbox serve` flows through to the Anthropic SDK automatically. To enable distributed tracing:

```sh
OTEL_TRACES_EXPORTER=otlp \
OTEL_LOGS_EXPORTER=otlp \
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4317 \
OTEL_LOG_TOOL_DETAILS=1 \
./breadbox serve
```

Without OTel, the orchestrator emits structured slog at run boundaries (`orchestrator: run starting / finished`) and the scheduler emits at register/reload/cleanup boundaries. Every run also persists a full NDJSON transcript at `<agent.transcript_dir>/<runID>.ndjson` viewable from the admin agent-runs page.

## Do not

- Don't call `RunNow` from a cron callback — it skips the skipped-row semantics.
- Don't return the full plaintext of `subscription_token` or `anthropic_api_key` from any endpoint. The mask helper in `agent_settings.go::maskToken` is the only shape that leaves the server.
- Don't bypass the concurrency semaphore (e.g., a "force run" debug endpoint). One safety net should not be optional.
- Don't store the per-run minted API key in `agent_runs`. It's deliberately ephemeral — `actor_type='agent'` + the `agent:<slug>:<runID>` name in `api_keys` is the audit trail.

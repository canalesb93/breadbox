# Agents subsystem

The Claude Agent SDK integration. Lets a self-hoster schedule recurring AI runs that call breadbox MCP. Replaces the v1 admin "agent prompts" wizard.

User-facing reference: `docs/agents.md`. Sprint history + design decisions: `.claude/sprint-state.md` on the `agents/claude-agent-sdk-sprint` branch.

## The five layers

```
v2 SPA (web/src/routes/agents*.tsx, features/agents/*)
   │ REST /api/v1/agents/*
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

When in doubt about where new code belongs, ask: is it user input/response shaping → `api/`; is it stateful behavior → `service/`; is it the runner protocol / event shapes → `internal/agent/`; is it sidecar logic → `agent/sidecar/`.

## Locked invariants

These are easy to break and painful to re-discover.

### Auth precedence trap

If both `ANTHROPIC_API_KEY` and `CLAUDE_CODE_OAUTH_TOKEN` are set in the sidecar's env, the API key silently wins. The sidecar must scrub the unused var before invoking the SDK (`agent/sidecar/index.ts::configureAuth`). Never remove that scrub; never assume the SDK does the right thing here.

### Mint-and-revoke

Every run mints a scoped `actor_type='agent'` API key (name format `agent:<slug>:<runShortID>`) and revokes it in a `defer` with a fresh context. Never leave a key valid past the run that minted it. If you add a new entry point that runs an agent, wrap it in `Orchestrator.RunNow` or `Orchestrator.RunOrSkip` — don't reimplement the mint-revoke dance.

### Concurrency split

- `RunNow` (manual trigger from HTTP): `ErrConcurrencyLocked` returns WITHOUT creating an `agent_runs` row. Caller maps to 503; user can retry without polluting history.
- `RunOrSkip` (cron trigger): always leaves a row. Status `skipped` when locked, so the run history shows what was missed.

Don't blur these. Cron callers must use `RunOrSkip`; HTTP callers must use `RunNow`.

### Seed-on-empty, never-overwrite

`agent.SeedDefaults` runs ONLY when `agent_definitions` is empty (fresh install). User edits to seeded agents, custom agents, and renamed agents must survive every restart. Never add an "upsert" path here — that would silently undo user customization.

### Fresh-ctx revocation

`Orchestrator.runLocked` defers the API key revoke with `context.WithTimeout(context.Background(), 10*time.Second)` — a NEW root context, not the request ctx. A cancelled parent (user closed the run-now request, scheduler restart, server shutdown) must NOT prevent revocation.

### Hot-reload via `OnDefinitionChanged`

CRUD mutations on agent_definitions call `svc.OnDefinitionChanged()` which the orchestrator wires to `AgentScheduler.Reload(ctx)`. The full sequence: service method updates DB → notifies hook → scheduler removes all per-agent entries → re-registers from DB. Don't add new mutation paths without calling the hook.

## Adding a new starter agent

1. Drop the markdown prompt at `prompts/agents/strategy-<slug>.md`.
2. Add an entry to `agent.DefaultSeed` in `internal/agent/seed.go`.
3. Existing installs are unaffected (seed is fresh-install-only). New installs get it on next boot.

That's the whole flow.

## Tests + CI

- Service-layer integration tests: `internal/service/agents_test.go`, `agent_orchestrator_test.go` (one TestMain shared with the rest of `service_test`).
- Schema + sqlc integration tests: `internal/db/agent_tables_integration_test.go`, `agent_queries_integration_test.go`.
- Seed tests: `internal/agent/seed_test.go`.
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

Without OTel, the orchestrator emits structured slog at run boundaries (`orchestrator: run starting / finished`) and the scheduler emits at register/reload/cleanup boundaries. Every run also persists a full NDJSON transcript at `<agent.transcript_dir>/<runID>.ndjson` viewable from the v2 SPA.

## v1 retirement state

Every legacy admin agent URL 302s to `/v2/agents`. Symbols (`AgentsPageHandler`, `PromptBuilderHandler`, `PromptCopyHandler`, the `agent_wizard.templ` template) stay compiled but unwired so a rollback is one router line. The full deletion belongs in the broader v1-admin retirement sweep — don't bundle it with agent-subsystem changes.

## Do not

- Don't call `RunNow` from a cron callback — it skips the skipped-row semantics.
- Don't add an upsert path to `SeedDefaults`.
- Don't return the full plaintext of `subscription_token` or `anthropic_api_key` from any endpoint. The mask helper in `agent_settings.go::maskToken` is the only shape that leaves the server.
- Don't bypass the concurrency semaphore (e.g., a "force run" debug endpoint). One safety net should not be optional.
- Don't store the per-run minted API key in `agent_runs`. It's deliberately ephemeral — `actor_type='agent'` + the `agent:<slug>:<runID>` name in `api_keys` is the audit trail.

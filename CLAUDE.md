# Breadbox

Self-hosted financial data aggregation for families. Syncs bank data via Plaid, stores it in PostgreSQL, exposes it to AI agents via MCP and REST API.

## Tech Stack

Go 1.24+ single binary. PostgreSQL, chi/v5 router, pgx/v5 + sqlc, goose migrations, robfig/cron. Admin UI: Go html/template + Pico CSS. MCP: github.com/modelcontextprotocol/go-sdk (Streamable HTTP). Plaid: github.com/plaid/plaid-go.

## Architecture

One HTTP server (`breadbox serve`) hosts everything: REST API (`/api/v1/...`), MCP server (`/mcp`), admin dashboard (`/admin/...`), webhooks (`/webhooks/:provider`). Bank data providers are abstracted behind a `Provider` Go interface (Plaid first, Teller + CSV later).

## Key Design Decisions

- REST API is the core data layer; MCP tools and dashboard consume the service layer directly (no HTTP round-trip)
- Amounts are NUMERIC(12,2), always with `iso_currency_code` per transaction — never sum across currencies
- Pending→posted: Plaid removes pending ID, creates new posted ID linked via `pending_transaction_id`
- Soft deletes: transactions use `deleted_at`, connections set to `disconnected` status
- FK policy: accounts/transactions use SET NULL on connection delete (preserve history), sync_logs use CASCADE
- Config precedence: environment variables → app_config DB table → defaults
- Access tokens AES-256-GCM encrypted at rest
- API key auth: `X-API-Key: bb_xxxxx` header, SHA-256 hashed, `revoked_at` for soft-revoke
- Admin sessions: `alexedwards/scs` + `pgxstore`, cookies `HttpOnly; SameSite=Lax; Secure`
- Error codes: `UPPER_SNAKE_CASE` in JSON envelope `{ "error": { "code": "...", "message": "..." } }`
- Service layer (`internal/service/`): shared between REST API handlers and MCP tools. Converts `pgtype.*` → Go primitives for clean JSON. Takes `*db.Queries` + `*pgxpool.Pool` (for dynamic queries).
- MCP server (`internal/mcp/`): wraps service layer as 6 MCP tools. Streamable HTTP at `/mcp` (API key auth), stdio via `breadbox mcp-stdio` (no auth). Uses `github.com/modelcontextprotocol/go-sdk` v1.4.0. Tool handlers use typed input structs with `jsonschema` tags. Errors: `IsError: true` with `{"error": "..."}` text content.
- Transaction queries use dynamic SQL with positional `$N` params (not sqlc) for composable filters + cursor pagination
- API key format: `bb_` + base62 body (32 random bytes). Stored as SHA-256 hex hash. Prefix stored for display.

## Canonical Enums

- Connection status: `active`, `error`, `pending_reauth`, `disconnected`
- Sync status: `in_progress`, `success`, `error`
- Sync trigger: `cron`, `webhook`, `manual`, `initial`
- Provider type: `plaid`, `teller`, `csv`

## Spec Documents

Detailed specs live in `docs/`. The canonical source for schema and enums is `docs/data-model.md`. The canonical source for the Provider interface is `docs/architecture.md`. Implementation order is in `docs/ROADMAP.md`.

## Workflow Rules

> If you are a subagent or teammate executing a specific task, ignore this section — just do your work. These rules are for the top-level orchestrating agent only.

### How We Work (Orchestrator → Ricardo)

- If it makes sense for the current task **follow `docs/ROADMAP.md`** phase by phase. Don't skip ahead.
- **Checkpoint before moving on.** At the end of each phase, pause and let Ricardo verify the checkpoint steps before starting the next phase.
- **Commit after each completed phase.** One clean commit per phase, not mid-phase.
- **No surprises.** If a task is ambiguous or a design decision comes up that isn't covered in the specs, ask Ricardo rather than guessing.

### Parallelism Strategy

- **Agent teams** for phase-level work where multiple independent modules can be built simultaneously (e.g., separate REST endpoints, dashboard pages). Teammates get their own context windows and can work on different files without conflicts.
- **Git worktrees** (`isolation: "worktree"`) for teammates that write code, so they each get an isolated copy of the repo. Merge results back when done.
- **Subagents** (via Agent tool) for quick, focused tasks within the orchestrator's session: research, code review, running tests, reading specs. These don't need worktrees since they report results back.
- **Avoid file conflicts.** Never assign two agents to edit the same file. Break work so each agent owns distinct files.
- **3-5 teammates max** per team. More adds coordination overhead without proportional benefit.

### Task Sizing

- Each teammate should have 3-6 tasks to stay productive.
- Tasks should be self-contained: one package, one endpoint group, one dashboard page, etc.
- Include spec references in every task description so teammates have full context.

### Keeping Docs Current

After completing a phase or making a significant decision:

- **`docs/ROADMAP.md`**: Mark completed tasks/phases so progress is visible across sessions.
- **`CLAUDE.md`** (this file): Update if a design decision changes, a new convention is established, or the tech stack evolves. Keep it concise.
- **Spec docs** (`docs/*.md`): Update if implementation reveals the spec was wrong or incomplete. Specs should reflect reality, not aspirations.

It's critical to also keep this CLAUDE.md up to date. If you are the orchestrator agent include in your plans to come back to this at the end and make updates it necessary.
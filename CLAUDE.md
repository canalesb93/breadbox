# Breadbox

Self-hosted financial data aggregation for families. Syncs bank data via Plaid and Teller, stores it in PostgreSQL, exposes it to AI agents via MCP and REST API.

## Tech Stack

Go 1.24+ single binary. PostgreSQL, chi/v5 router, pgx/v5 + sqlc, goose migrations, robfig/cron. Admin UI: Go html/template + DaisyUI 5 + Tailwind CSS v4 + Alpine.js v3. MCP: github.com/modelcontextprotocol/go-sdk (Streamable HTTP). Plaid: github.com/plaid/plaid-go. Teller: hand-written HTTP client with mTLS (no SDK).

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
- MCP server (`internal/mcp/`): wraps service layer as 7 MCP tools + 1 resource. Streamable HTTP at `/mcp` (API key auth), stdio via `breadbox mcp-stdio` (no auth). Uses `github.com/modelcontextprotocol/go-sdk` v1.4.0. Tool handlers use typed input structs with `jsonschema` tags. Errors: `IsError: true` with `{"error": "..."}` text content. Resource: `breadbox://overview` returns live stats.
- Transaction queries use dynamic SQL with positional `$N` params (not sqlc) for composable filters + cursor pagination
- API key format: `bb_` + base62 body (32 random bytes). Stored as SHA-256 hex hash. Prefix stored for display.
- Multi-provider DB columns: `bank_connections` uses generic `external_id` + `encrypted_credentials` (not provider-specific column names). Unique constraint on `(provider, external_id)`.
- Shared crypto: `internal/crypto/encrypt.go` — AES-256-GCM encrypt/decrypt used by all providers
- Provider errors: `provider.ErrReauthRequired`, `provider.ErrSyncRetryable` — provider-agnostic sentinels in `internal/provider/errors.go`. Each provider wraps these.
- Provider interface: `CreateReauthSession` and `RemoveConnection` accept a `Connection` struct (not a string ID) so providers can decrypt credentials internally.
- Teller auth: mTLS (app-level cert/key files) + per-connection access token via HTTP Basic Auth. Config: `TELLER_APP_ID`, `TELLER_CERT_PATH`, `TELLER_KEY_PATH`, `TELLER_ENV`, `TELLER_WEBHOOK_SECRET`.
- Teller sync: date-range polling with 10-day overlap (no cursor). After sync, auto soft-delete stale *pending* transactions not returned by the API. Posted transactions are never auto-deleted.
- Teller amounts: sign is opposite to Plaid. Teller negative=debit, Plaid positive=debit. Provider negates amounts before returning.
- CSV provider: import-only, stub `Provider` interface (`ErrNotSupported` for sync/webhook/reauth, nil for `RemoveConnection`). Bypasses provider interface for actual import — uses service layer directly.
- CSV dedup: `external_transaction_id = SHA-256(account_id|date|amount|description)` per account. Standard `UpsertTransaction` ON CONFLICT handles it.
- Account settings: `display_name TEXT NULL` (template uses `COALESCE(display_name, name)`), `excluded BOOLEAN DEFAULT FALSE` (skips transaction upsert only, balances still refresh)
- Connection pause: `paused BOOLEAN DEFAULT FALSE` orthogonal to `status`. Only cron respects pause; manual "Sync Now" bypasses it.
- Per-connection sync interval: `sync_interval_override_minutes INTEGER NULL`. Cron fires at minimum interval, checks each connection's staleness individually.
- Alpine.js v3 for admin UI interactivity (CDN, no build step). Replaces `alert()`/`confirm()` with inline patterns.
- Dark mode: DaisyUI `light`/`dark` themes with `prefers-color-scheme` auto-switch (no hardcoded `data-theme`). Badge/flash colors use DaisyUI semantic classes.
- Badge rendering: `statusBadge()` and `syncBadge()` template functions replace copy-pasted if-chains.
- CSS spacing tokens: `--bb-gap-xs` (0.25rem) through `--bb-gap-xl` (2rem) in `:root`. Use these instead of hardcoded spacing values.
- Template data helper: `BaseTemplateData(r, sm, currentPage, pageTitle)` returns `map[string]any` with common fields. Handlers can extend the returned map.
- First-run: `CountAdminAccounts == 0` → redirect to `/admin/setup` (single-page admin creation form). No `setup_complete` flag. Wizard layout, no step indicator.
- Onboarding checklist: dashboard "Getting Started" card shown until dismissed (`onboarding_dismissed` in `app_config`). Checks: provider configured, family member exists, connection exists.
- CLI admin management: `breadbox create-admin` command with `--username`/`--password` flags or interactive prompts.
- Programmatic setup: `POST /admin/api/setup` creates admin + sets config (only works when no admin exists).
- Config source tracking: `ConfigSources map[string]string` populated in `Load()` (env) and `LoadWithDB()` (db/default). `configSource` template function renders badges.
- Settings password change: POST `/admin/settings/password`, validates current password via bcrypt, minimum 8 chars for new password
- Settings system info: `Version` and `StartTime` on `Config` struct, set in `main.go`. PostgreSQL version via inline `SELECT version()` query.
- Teller settings: All Teller config (app_id, env, webhook_secret, cert/key PEM) editable via `/admin/providers`. Cert/key PEM files uploaded through dashboard are AES-256-GCM encrypted and stored base64-encoded in `app_config`. Env file paths take precedence over DB PEM.
- Provider page: `/admin/providers` is a top-level nav page with equal-weight cards for Plaid, Teller, CSV. No "primary provider" concept. Settings page no longer contains provider configuration.
- Provider reinitialization: `app.ReinitProvider(name)` hot-reloads providers after dashboard config changes. Sync engine shares the same `map[string]provider.Provider` reference.
- Teller PEM client: `teller.NewClientFromPEM(certPEM, keyPEM)` creates mTLS client from in-memory PEM bytes (alternative to file-path constructor).
- Human-readable error messages: `errorMessage()` template function maps provider error codes to user-friendly strings
- Health check split: `/health/live` (basic HTTP 200) vs `/health/ready` (DB + scheduler verification)
- Sync writes wrapped in a single DB transaction for atomicity (Phase 14)
- Orphaned sync logs: on startup, mark stale `in_progress` logs as `error` with "interrupted by server restart"
- Per-sync timeout: configurable context deadline per connection sync (default 5 minutes)
- ENCRYPTION_KEY required at startup when Plaid or Teller providers are configured (fail fast, not runtime crash)
- LOG_LEVEL env var: debug/info/warn/error, overrides environment-based defaults
- Transaction responses include denormalized `account_name` and `user_name` (JOIN in dynamic query builder)
- MCP tool descriptions: domain-rich with amount conventions, filter docs, pagination behavior (not generic)
- MCP server instructions: domain-rich onboarding text (data model, amount convention, category system, recommended query patterns)
- Teller category mapping: `mapCategory()` returns `(primary, detailed)` pair. All Teller categories map to both primary and detailed Plaid-compatible categories.
- Transaction sort options: `sort_by` (date/amount/name) + `sort_order` (asc/desc). Cursor pagination only works with date sort.
- `ListDistinctCategories` returns `[]CategoryPair{Primary, Detailed}` (not `[]string`). Used by REST `/api/v1/categories`, MCP `list_categories`, and admin dashboard.
- MCP `min_amount`/`max_amount` use `*float64` (not `float64`) to allow filtering by zero values
- CSS framework: DaisyUI 5 + Tailwind CSS v4 via `tailwindcss-extra` standalone CLI binary. No Node.js. Replaces Pico CSS.
- CSS build: `make css` compiles `input.css` → `static/css/styles.css`. `make css-watch` for dev. Dockerfile runs `make css` in build stage.
- DaisyUI theme: `light` (default) + `dark` auto-switch via `prefers-color-scheme`
- Icons: Lucide via CDN, `data-lucide` attributes replaced with inline SVG by `lucide.createIcons()`
- DaisyUI components replace `bb-*` classes: `drawer` (sidebar), `stat` (metric cards), `table` (data tables), `badge` (status), `menu` (nav), `card` (sections), `modal` (dialogs), `toast`+`alert` (notifications), `steps` (wizard progress), `collapse` (accordions)
- Custom `@apply` classes in `input.css` for app-specific patterns: `.bb-filter-bar`, `.bb-pagination`, `.bb-action-bar`, `.bb-amount`, `.bb-info-grid`
- CI/CD: GitHub Actions (`.github/workflows/ci.yml`). PR → vet+test+build. Main → multi-arch push to `ghcr.io/canalesb93/breadbox:latest`. Tag → versioned image. Auto-deploy to dev VM via SSH.
- Production deployment: `deploy/docker-compose.prod.yml` with Caddy (auto HTTPS), PostgreSQL, Breadbox from ghcr.io. `deploy/install.sh` for one-liner setup. `deploy/update.sh` for CLI updates.
- Version checker: `internal/version/checker.go` — in-memory cached (1hr TTL) GitHub Releases API check. Shared by REST `GET /api/v1/version` (no auth) and dashboard handler.
- Docker socket detection: `os.Stat("/var/run/docker.sock")` at startup → `App.DockerSocketAvailable`. Used by update handler to pull images via Docker Engine API (`net/http` with Unix socket transport, no Docker SDK dependency).
- Update flow: dashboard banner shows when GitHub release is newer than current. Pull via Docker socket if available, then user runs `docker compose up -d`. Dismiss stores `update_dismissed_version` in `app_config`.
- Dev VM: `breadbox.exe.xyz` hosted on exe.dev. exe.dev handles TLS termination and HTTP proxying automatically (no Caddy needed on dev). Breadbox listens on a local port and exe.dev proxies `https://breadbox.exe.xyz` to it. Auto-deploy from GitHub Actions on main merge via SSH (`appleboy/ssh-action`). Secrets: `DEPLOY_SSH_KEY`, `DEPLOY_HOST`, `DEPLOY_USER`, `DEPLOY_PATH`.

## Canonical Enums

- Connection status: `active`, `error`, `pending_reauth`, `disconnected`
- Sync status: `in_progress`, `success`, `error`
- Sync trigger: `cron`, `webhook`, `manual`, `initial`
- Provider type: `plaid`, `teller`, `csv`

## Spec Documents

Detailed specs live in `docs/`. The canonical source for schema and enums is `docs/data-model.md`. The canonical source for the Provider interface is `docs/architecture.md`. Teller-specific details are in `docs/teller-integration.md`. CSV import details are in `docs/csv-import.md`. Design system (CSS framework, components, icons) is in `docs/design-system.md`. Implementation order is in `docs/ROADMAP.md`.

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
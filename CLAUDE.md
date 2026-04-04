# Breadbox

Self-hosted financial data aggregation for families. Syncs bank data via Plaid and Teller, stores it in PostgreSQL, exposes it to AI agents via MCP and REST API.

## Tech Stack

Go 1.24+ single binary. PostgreSQL, chi/v5 router, pgx/v5 + sqlc, goose migrations, robfig/cron. Admin UI: Go html/template + DaisyUI 5 + Tailwind CSS v4 + Alpine.js v3. MCP: github.com/modelcontextprotocol/go-sdk (Streamable HTTP). Plaid: github.com/plaid/plaid-go. Teller: hand-written HTTP client with mTLS (no SDK).

## Testing

- **Unit tests**: `go test ./...` (no DB needed). Covers crypto, CSV parsing, service utilities.
- **Integration tests**: `make test-integration` or `DATABASE_URL=... go test -tags integration -count=1 -p 1 ./...`. Requires a running PostgreSQL with `breadbox_test` database. Migrations run automatically via goose in `TestMain`. `-p 1` is required because multiple packages share the same test database.
- **Build tag separation**: Integration test files must have `//go:build integration` at the top. This ensures `go test ./...` (without `-tags integration`) only runs unit tests and doesn't require a database.
- **Test helper**: `internal/testutil/db.go` — call `testutil.RunWithDB(m)` from `TestMain`, then use `testutil.Pool(t)` or `testutil.Queries(t)` in tests. Tables are truncated between tests automatically.
- **Fixture helpers**: Use `testutil.MustCreateUser`, `MustCreateConnection`, `MustCreateTellerConnection`, `MustCreateAccount`, `MustCreateTransaction` to create test data. These fatal on error to catch silent setup failures.
- **Adding integration tests**: Any package that needs DB access should add a `TestMain` calling `testutil.RunWithDB(m)`. Use the `testutil.ServicePool(t)` helper to get pool+queries. See `internal/service/integration_test.go` for examples. Do NOT use `t.Parallel()` — tests share a database.
- **Session hook**: `.claude/hooks/session-start.sh` creates the `breadbox` role and `breadbox_test` database automatically on web sessions.
- **CI**: GitHub Actions spins up PostgreSQL, then runs `go test -tags integration -p 1 ./...` with `DATABASE_URL` set. Migrations run automatically via `testutil.RunWithDB`.
- **When adding new features**: Always add integration tests for new service layer methods and API endpoints. Prefer testing through the service layer rather than HTTP handlers.

## Architecture

One HTTP server (`breadbox serve`) hosts everything: REST API (`/api/v1/...`), MCP server (`/mcp`), admin dashboard (`/...`), webhooks (`/webhooks/:provider`). Bank data providers are abstracted behind a `Provider` Go interface (Plaid first, Teller + CSV later).

## Migrations

- **Timestamp-based naming**: New migrations use `YYYYMMDDHHMMSS_description.sql` format (e.g., `20260325153000_add_oauth.sql`). This prevents conflicts when parallel branches each need a migration. Existing sequential migrations (`00001`–`00029`) remain as-is — goose sorts by numeric prefix so timestamps (larger numbers) always run after them.
- **Creating a migration**: Use the current UTC timestamp as the prefix. Example: `date -u +%Y%m%d%H%M%S` gives `20260325153000`.
- **After adding a migration**: Run `sqlc generate` to regenerate Go code, then `go build ./...` to verify.
- **PL/pgSQL in migrations**: Goose cannot parse `$$`-quoted function bodies by default. Wrap each `CREATE FUNCTION ... $$ ... $$ LANGUAGE plpgsql;` block in `-- +goose StatementBegin` / `-- +goose StatementEnd`.

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
- Short IDs: every entity table has a `short_id TEXT NOT NULL UNIQUE` column — an 8-character base62 alias for the UUID primary key. Generated automatically by a Postgres BEFORE INSERT trigger (`set_short_id()` calling `generate_short_id()`). All service methods accept either UUID or short_id via resolver functions (`internal/service/resolve.go`). Response structs include both `id` (UUID) and `short_id`. MCP responses compact IDs: `jsonResult()` in `internal/mcp/tools.go` calls `compactIDs()` to replace `id` with the `short_id` value and remove the `short_id` field, so agents only see compact 8-char IDs. REST API continues returning both fields. `internal/shortid/` package provides `Generate()` and `IsShortID()`. Field selection always includes `short_id` alongside `id`.
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
- **Select element backgrounds**: Never use `bg-base-200/50` (or any `/opacity` modifier) on `<select>` elements — the alpha transparency renders as fully transparent in browsers. Use solid `bg-base-200` instead. `<input>` elements handle `bg-base-200/50` fine; `<select>` does not.
- **SPA progress bar**: `base.html` has a global progress bar and content fade (opacity/blur/pointer-events) that auto-starts on internal link clicks. When JS does async work (fetch + `window.location.href` on success), any error path **must** call `window.bbProgress.finish()` and restore `main` element styles (`opacity: '', filter: '', pointerEvents: ''`). Without this, the progress bar trickles forever and the page stays blurred/unclickable. Pattern: add a `restorePageState()` helper that does both, call it on every error/early-return path.
- CSS spacing tokens: `--bb-gap-xs` (0.25rem) through `--bb-gap-xl` (2rem) in `:root`. Use these instead of hardcoded spacing values.
- Template data helper: `BaseTemplateData(r, sm, currentPage, pageTitle)` returns `map[string]any` with common fields. Handlers can extend the returned map.
- First-run: `CountAdminAccounts == 0` → redirect to `/setup` (single-page admin creation form). No `setup_complete` flag. Wizard layout, no step indicator.
- Onboarding checklist: dashboard "Getting Started" card shown until dismissed (`onboarding_dismissed` in `app_config`). Checks: provider configured, family member exists, connection exists.
- CLI admin management: `breadbox create-admin` command with `--username`/`--password` flags or interactive prompts.
- Programmatic setup: `POST /api/setup` creates admin + sets config (only works when no admin exists).
- Config source tracking: `ConfigSources map[string]string` populated in `Load()` (env) and `LoadWithDB()` (db/default). `configSource` template function renders badges.
- Settings password change: POST `/settings/password`, validates current password via bcrypt, minimum 8 chars for new password
- Settings system info: `Version` and `StartTime` on `Config` struct, set in `main.go`. PostgreSQL version via inline `SELECT version()` query.
- Teller settings: All Teller config (app_id, env, webhook_secret, cert/key PEM) editable via `/providers`. Cert/key PEM files uploaded through dashboard are AES-256-GCM encrypted and stored base64-encoded in `app_config`. Env file paths take precedence over DB PEM.
- Provider page: `/providers` is a top-level nav page with equal-weight cards for Plaid, Teller, CSV. No "primary provider" concept. Settings page no longer contains provider configuration.
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
- Teller categories: raw Teller category strings (e.g., `dining`, `groceries`) are stored directly in `category_primary`. Transaction rules handle categorization during sync.
- Teller SHOPPING fix: historical migration corrected `SHOPPING*` to `GENERAL_MERCHANDISE*`. No longer relevant since raw Teller categories are now stored directly.
- Transaction sort options: `sort_by` (date/amount/name) + `sort_order` (asc/desc). Cursor pagination only works with date sort.
- Category system: `categories` table (UUID PK, slug, display_name, parent_id for 2-level hierarchy, icon, color). Transactions have `category_id` FK (SET NULL) + `category_override` boolean. Raw provider strings kept in `category_primary`/`category_detailed` for auditability. Transaction rules handle categorization during sync.
- Category slugs: `lowercase_with_underscores` format (e.g., `food_and_drink_groceries`). Immutable after creation. Display names are mutable.
- Category API: transaction responses include structured `category` object; raw fields renamed to `category_primary_raw`/`category_detailed_raw`. Filter by `category_slug` param.
- MCP `min_amount`/`max_amount` use `*float64` (not `float64`) to allow filtering by zero values
- Field selection: `?fields=` param on `GET /api/v1/transactions`, `GET /api/v1/transactions/{id}`, and MCP `query_transactions`. Supports individual field names (e.g., `fields=name,amount,date,account_name`) and aliases: `minimal` (name,amount,date — smallest useful set), `core` (id,date,amount,name,iso_currency_code), `category` (category,category_primary_raw,category_detailed_raw), `timestamps` (created_at,updated_at,datetime,authorized_datetime). `id` and `short_id` always included. Filtering happens in handlers, not service layer.
- Transaction summary: `GET /api/v1/transactions/summary` and MCP `transaction_summary` tool. Server-side aggregation with `group_by`: category, month, week, day, category_month. Multi-currency handled per-row. Default date range: 30 days.
- Merchant summary: `GET /api/v1/transactions/merchants` and MCP `merchant_summary` tool. Server-side GROUP BY on `COALESCE(merchant_name, name)` returning distinct merchants with transaction_count, total_amount, avg_amount, first_date, last_date, iso_currency_code. Supports same filters as transaction queries plus `min_count` (HAVING COUNT >= N) and `spending_only`. Default date range: 90 days. Limit 500.
- Exclude search: `exclude_search` param on `query_transactions`, `count_transactions`, and `merchant_summary`. Adds `NOT ILIKE` clause on name and merchant_name. Minimum 2 characters (REST API validation). Used to filter out known merchants when hunting for unknown charges.
- Search modes: `search_mode` param on `query_transactions`, `count_transactions`, `merchant_summary`, `list_transaction_rules`. Values: `contains` (default, substring ILIKE), `words` (split on spaces, AND all words — handles formatting differences like "Century Link" vs "CenturyLink"), `fuzzy` (pg_trgm similarity for typo tolerance). Comma-separated values in `search` are auto-ORed in all modes. Shared implementation in `internal/service/search.go` via `BuildSearchClause`/`BuildExcludeSearchClause`.
- Enhanced overview resource: `breadbox://overview` includes users list, accounts_by_type, connections with account counts, 30-day spending summary with top 5 categories, pending transaction count.
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
- MCP permissions: global `mcp_mode` (read_only/read_write), per-tool enable/disable via `mcp_disabled_tools` JSON array, custom instructions via `mcp_custom_instructions` — all stored in `app_config`. Default mode is `read_only`.
- MCP tool registry: `MCPServer.allTools []ToolDef` with `ToolClassification` (read/write). `BuildServer(MCPServerConfig)` creates a filtered `*mcpsdk.Server` per request. Write tools suppressed when mode is read_only, tool is in disabled list, or API key scope is read_only.
- MCP instruction templates: hardcoded Go constants in `internal/mcp/templates.go` (spend_review, monthly_analysis, reporting). Loaded into editor via Alpine.js, saved as `mcp_custom_instructions`.
- API key scope: `scope TEXT NOT NULL DEFAULT 'full_access'` column on `api_keys`. Values: `full_access`, `read_only`. `RequireWriteScope()` middleware blocks read-only keys from write REST endpoints. Full API key record stored in request context via `middleware.SetAPIKey()`/`GetAPIKey()`.
- MCP admin page: `/mcp` with four cards — global mode, tool access, server instructions, API key scope info. Nav item with `bot` Lucide icon between API Keys and Sync Logs.
- Review queue: `review_queue` table with `review_type` (new_transaction, uncategorized, low_confidence, manual) and `status` (pending, approved, rejected, skipped). Auto-enqueued during sync when `review_auto_enqueue` app_config is true. Confidence threshold from `review_confidence_threshold` app_config. `reviewer_complete` constraint ensures reviewer metadata is set on resolution.
- Review sync hook: `Engine.enqueueForReview()` called after each upsert inside the sync transaction. Priority: uncategorized > low_confidence > new_transaction. Skips `category_override=true` transactions. ON CONFLICT DO NOTHING for idempotency.
- Review service: `ListReviews` uses dynamic SQL with transaction JOINs and cursor pagination (ASC for pending FIFO, DESC for resolved). `SubmitReview` handles category override + comment creation. `BulkSubmitReviews` iterates individually.
- Review admin page: `/reviews` with filter bar (status, type, account, user), card-based review queue with inline approve/reject/skip/dismiss, Alpine.js `reviewQueue()` for AJAX actions with card fade-out animation.
- Review MCP tools: `list_pending_reviews` (ToolRead, supports `fields` param with aliases: `triage`, `review_core`, `transaction_core`), `submit_review` (ToolWrite), `batch_submit_reviews` (ToolWrite, max 500) in `internal/mcp/tools.go`. Limits: `list_pending_reviews` max 500, `batch_submit_reviews` max 500.
- Review REST API: read endpoints at `/api/v1/reviews`, `/api/v1/reviews/counts`, `/api/v1/reviews/{id}`; write endpoints at `POST /reviews/{id}/submit`, `POST /reviews/bulk`, `POST /reviews/enqueue`, `DELETE /reviews/{id}`.
- Transaction rules: `transaction_rules` table with recursive JSON condition tree (`conditions JSONB`). Rules auto-categorize future transactions during sync by matching conditions on transaction fields. Priority-ordered (higher wins). Support AND/OR/NOT logic with operators: eq, neq, contains, not_contains, matches (regex), gt, gte, lt, lte, in. Available fields: name, merchant_name, amount, category_primary, category_detailed, pending, provider, account_id, user_id, user_name.
- Transaction rules service: `internal/service/rules.go` — `ValidateCondition` (recursive validation), `CompileCondition` (pre-compile regexes), `EvaluateCondition` (short-circuit evaluation). CRUD via dynamic SQL with cursor pagination. `ruleRow` struct with `scanDest()`/`toResponse()` eliminates scan variable duplication.
- Transaction rules MCP tools: `create_transaction_rule` (ToolWrite, supports `apply_retroactively` flag), `list_transaction_rules` (ToolRead), `update_transaction_rule` (ToolWrite), `delete_transaction_rule` (ToolWrite), `apply_rules` (ToolWrite, applies rules retroactively to existing transactions — optional `rule_id` for single rule or all active rules), `preview_rule` (ToolRead, dry-run a condition against existing transactions — returns match count + samples).
- Transaction rules REST API: read endpoints at `/api/v1/rules`, `/api/v1/rules/{id}`; write endpoints at `POST /rules`, `PUT /rules/{id}`, `DELETE /rules/{id}`, `POST /rules/{id}/apply`, `POST /rules/apply-all`, `POST /rules/preview`.
- Retroactive rule application: `ApplyRuleRetroactively` and `ApplyAllRulesRetroactively` in `internal/service/rules.go`. Batched keyset pagination (1000/batch), evaluates conditions in Go, bulk UPDATEs matching non-overridden transactions. Respects `category_override=true`.
- Transaction rules admin: `/rules` page with table, filter bar (search, category, enabled), create/edit modal with JSON condition editor, toggle enable/disable, delete. Nav item with `list-filter` Lucide icon.
- Rule creator tracking: `created_by_type` (user/agent/system), `created_by_id`, `created_by_name` — uses Actor pattern from request context.
- Rule expiry: optional `expires_at` timestamp. `expires_in` param on create accepts duration strings (24h, 30d, 1w). Expired rules excluded from sync but remain visible.
- Rule hit tracking: `hit_count` and `last_hit_at` updated by `BatchIncrementHitCounts` during sync.
- Batch categorize: `batch_categorize_transactions` MCP tool (ToolWrite, max 500 items) and `POST /api/v1/transactions/batch-categorize`. Takes array of `{transaction_id, category_slug}` pairs. Pre-resolves slugs with cache.
- Bulk recategorize: `bulk_recategorize` MCP tool (ToolWrite) and `POST /api/v1/transactions/bulk-recategorize`. Server-side UPDATE with dynamic WHERE clause (same filter params as query_transactions) + target category. Requires at least one filter (safety). Sets `category_override=TRUE`.
- Review field selection: `list_pending_reviews` supports `fields` param. Aliases: `triage` (review id/type/status + transaction name/amount/date/category_primary_raw/account_name/user_name/merchant_name), `review_core` (review metadata only), `transaction_core` (key transaction fields). Implemented in `internal/service/fields.go` via `ParseReviewFields`/`FilterReviewFields`.
- Account linking: `account_links` table links dependent (authorized user) accounts to primary (cardholder) accounts for cross-connection transaction deduplication. `transaction_matches` table stores matched pairs. `transactions.attributed_user_id` overrides user attribution. `accounts.is_dependent_linked` flag excludes dependent transactions from totals at query time.
- Account link matching: `internal/sync/matcher.go` matches by date + exact amount. Runs post-sync via `Engine.matcher.ReconcileForConnection()`. Single candidate → auto-match. Multiple candidates → name similarity tiebreaker. Ambiguous → skip for manual review.
- Attribution-aware filtering: transaction queries use `COALESCE(t.attributed_user_id, bc.user_id)` for user filtering. "User's transactions" includes their own plus those attributed to them on shared cards. Dependent account transactions excluded from totals via `AND a.is_dependent_linked = FALSE`.
- Account link MCP tools: `list_account_links`, `create_account_link` (ToolWrite, auto-runs initial reconciliation), `delete_account_link`, `reconcile_account_link`, `list_transaction_matches`, `confirm_match`, `reject_match`.
- Account link REST API: `/api/v1/account-links` CRUD + `/reconcile` + `/matches`. `/api/v1/transaction-matches/{id}/confirm|reject`. `POST /api/v1/transaction-matches/manual`.
- Account link admin: `/account-links` page with link list, create modal, match detail view. Nav item with `link-2` Lucide icon between Categories and Family Members.
- Agent reports: `agent_reports` table for agents to submit summaries and flag transactions. `submit_report` MCP tool (ToolWrite) with title + markdown body. Dashboard widget shows unread reports with markdown rendering (marked.js CDN). Transaction deep-links via `[Name](/transactions/ID)` in markdown. Mark read/dismiss via admin API. Nav badge shows unread count.
- Agent report REST API: `GET /api/v1/reports`, `POST /api/v1/reports`, `GET /api/v1/reports/unread-count`, `PATCH /api/v1/reports/{id}/read`. Admin API: `POST /-/reports/{id}/read`, `POST /-/reports/read-all`.

## Canonical Enums

- Connection status: `active`, `error`, `pending_reauth`, `disconnected`
- Sync status: `in_progress`, `success`, `error`
- Sync trigger: `cron`, `webhook`, `manual`, `initial`
- Provider type: `plaid`, `teller`, `csv`
- API key scope: `full_access`, `read_only`
- MCP mode: `read_only`, `read_write`
- MCP tool classification: `read`, `write`
- Review type: `new_transaction`, `uncategorized`, `low_confidence`, `manual`, `re_review`
- Review status: `pending`, `approved`, `rejected`, `skipped`
- Reviewer type: `user`, `agent`
- Rule creator type: `user`, `agent`, `system`
- Condition operators (string): `eq`, `neq`, `contains`, `not_contains`, `matches`, `in`
- Condition operators (numeric): `eq`, `neq`, `gt`, `gte`, `lt`, `lte`
- Condition operators (bool): `eq`, `neq`
- Match confidence: `auto`, `confirmed`, `rejected`
- Match strategy: `date_amount_name`

## Spec Documents

Detailed specs live in `docs/`. The canonical source for schema and enums is `docs/data-model.md`. The canonical source for the Provider interface is `docs/architecture.md`. Teller-specific details are in `docs/teller-integration.md`. CSV import details are in `docs/csv-import.md`. Design system (CSS framework, components, icons) is in `docs/design-system.md`. Implementation order is in `docs/ROADMAP.md`.

## Local Dev & Git Worktrees

### Quick Start

Prerequisites: Go 1.24+, PostgreSQL (Homebrew or Docker).

```
make db       # start Postgres via Docker (skip if you have local Postgres)
make dev      # auto-installs sqlc + tailwind, generates code, runs migrations, starts server
```

On first run, `make dev` will download `tailwindcss-extra`, install `sqlc` via `go install`, and generate all build artifacts. Subsequent runs skip these steps if artifacts exist.

### Database

PostgreSQL credentials: `breadbox:breadbox`. All worktrees and dev server instances share the same database.

- **Dev database**: `postgres://breadbox:breadbox@localhost:5432/breadbox?sslmode=disable`
- **Test database**: `postgres://breadbox:breadbox@localhost:5432/breadbox_test?sslmode=disable`
- **Local Postgres**: Homebrew `postgresql@15` or `make db` (Docker). Both use port 5432.
- **Migrations**: Run automatically on `breadbox serve` startup. For manual control: `make migrate-up` / `make migrate-down`.
- **Shared state warning**: Multiple agents/worktrees may run simultaneously against the same `breadbox` database. Avoid destructive schema changes (DROP TABLE, DROP COLUMN, ALTER TYPE) without coordinating — another agent's running server will break. Additive migrations (ADD COLUMN, CREATE TABLE, CREATE INDEX) are safe.

### Required Environment Variables

The app requires these env vars when Plaid or Teller providers are configured in the DB:

- `DATABASE_URL` — pgx falls back to Unix socket with current OS user if unset, which may not work in all contexts (e.g., worktrees spawned by agents). Always set it explicitly.
- `ENCRYPTION_KEY` — 64-char hex (32-byte AES-256-GCM key). Required at startup if any provider is configured. To find the key from a running breadbox process: `ps eww -p $(pgrep -f "breadbox serve" | head -1) | tr ' ' '\n' | grep ENCRYPTION_KEY`

### Worktree Automation

Worktrees are fully automated via `claude --worktree` (or `claude -w`). Three mechanisms handle setup:

1. **`.worktreeinclude`**: Automatically copies gitignored build artifacts (sqlc files, `tailwindcss-extra`, `styles.css`, teller certs, `.local.env`) from the main repo into the worktree at creation time.
2. **`SessionStart` hook** (`.claude/hooks/session-start.sh`): Verifies `go build` works (falls back to `sqlc generate` if copied files are stale), injects `DATABASE_URL`, `ENCRYPTION_KEY`, and `PORT` via `CLAUDE_ENV_FILE`.
3. **Port assignment**: The hook scans ports 8081–8099 for the first available one and claims it with an atomic lock file under `.claude/port-locks/` to prevent races between concurrent sessions. Main repo uses 8080.

After setup, the agent just runs `make dev` — the `PORT` env var is already set, and `generate` skips since artifacts exist.

### Sandbox

OS-level sandboxing is enabled via `.claude/settings.json` with auto-allow mode. This replaces manual "bypass permissions" for most operations:

- **Filesystem**: Write access to project dir, `~/go`, `~/Library/Caches/go-build`, `/tmp`, `/var/folders`.
- **Network**: `localhost`, Go module proxy, GitHub.
- **Excluded commands**: `make dev*`, `make test*`, `go run *`, `go test *` run outside the sandbox (they need full network access for Postgres and HTTP binding).

### Manual Worktree Setup

If not using `claude -w`, you can set up manually:

1. `git worktree add -b <branch> .claude/worktrees/<name>`
2. Copy artifacts: `cp tailwindcss-extra static/css/styles.css internal/db/*.go .claude/worktrees/<name>/` (matching paths)
3. `cd .claude/worktrees/<name> && PORT=808X make dev`

### Cleanup

- Worktrees created by `claude -w` are cleaned up automatically when the session ends.
- Manual cleanup: `git worktree remove .claude/worktrees/<name>`
- Kill dev servers: `kill $(lsof -ti:<PORT>)` or `make dev-stop` (kills all instances)

## Releases & CI

- **Versioning**: Semantic versioning (`v0.1.0`). See `CHANGELOG.md`.
- **Releasing**: Manual. Create a git tag and push it — CI handles the rest.
  ```
  git tag -a v0.1.0 -m "Description"
  git push origin v0.1.0
  ```
- **What CI does on tag push**: runs tests, builds cross-platform binaries (linux/darwin, amd64/arm64), creates GitHub Release with binaries attached, builds + pushes versioned Docker image.
- **What CI does on merge to main**: runs tests, builds + pushes `latest` Docker image, auto-deploys to dev instance. Deploy job only runs on `canalesb93/breadbox` (skipped on forks).
- **What CI does on PR**: runs tests only. No builds, no deploys.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, testing, and contribution guidelines.

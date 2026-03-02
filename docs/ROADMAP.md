# Implementation Roadmap

Sequential build order for Breadbox MVP. Each phase builds on the previous one.
No dates or cost estimates — just ordered tasks with spec references.

Each phase ends with a **Checkpoint** — a hands-on verification you can run to
confirm everything works before moving on.

---

## Phase 1: Foundation ✅

Set up the project skeleton, database, and core infrastructure that everything else depends on.

**Status:** Complete. Committed as `f493ce3` + `50e8c52`. Checkpoint 1 verified.

### 1.1 Project Scaffold ✅

- [x] Initialize Go module (`breadbox`), set up directory layout per `architecture.md` Section 2
- [x] Configure `breadbox serve`, `breadbox migrate`, `breadbox version` subcommands (`architecture.md` Section 1.1)
- [x] Set up structured logging with `slog` (`architecture.md` Section 8)
- [x] Create `Makefile` with targets: `dev`, `build`, `test`, `lint`, `migrate-up`, `migrate-down`, `migrate-create`, `sqlc`, `seed`, `docker-up`, `docker-down`
- [x] Create `.env.example` with all required environment variables
- [x] Create `.gitignore`, `Dockerfile` (multi-stage Alpine), `docker-compose.yml` (app + db)

### 1.2 Configuration System ✅

- [x] Implement config loading: environment variables override `app_config` DB table override defaults
- [x] Define all config keys: `plaid_client_id`, `plaid_secret`, `plaid_env`, `sync_interval_hours`, `webhook_url`, `setup_complete`
- [x] AES-256-GCM encryption key loading from `ENCRYPTION_KEY` env var (64-char hex → 32 bytes)

### 1.3 Database Schema & Migrations ✅

- [x] All 11 migrations applied in correct order (00001–00011)
- [x] 8 tables: users, admin_accounts, bank_connections, accounts, transactions, sync_logs, api_keys, app_config
- [x] 4 enum types: provider_type, connection_status, sync_trigger, sync_status
- [x] All indexes per `data-model.md` Section 5 (including partial index, GIN trigram)
- [x] 6 seed config rows with ON CONFLICT DO NOTHING

### 1.4 sqlc Setup ✅

- [x] `sqlc.yml` configured with pgx/v5 backend
- [x] Initial queries: HealthCheck, GetAppConfig, ListAppConfig, SetAppConfig

### 1.5 Provider Interface ✅

- [x] `Provider` interface with 7 methods (CreateLinkSession, ExchangeToken, SyncTransactions, GetBalances, HandleWebhook, CreateReauthSession, RemoveConnection)
- [x] All shared types defined with `shopspring/decimal` for monetary amounts
- [x] Provider registry (`map[string]Provider`) in App struct
- [x] AES-256-GCM encrypt/decrypt in `internal/provider/plaid/encrypt.go`

### 1.6 Health Endpoint & HTTP Server ✅

- [x] chi/v5 router with middleware: RequestID, RealIP, slog Logger, Recoverer
- [x] `GET /health` → `{"status":"ok","version":"..."}`
- [x] Graceful shutdown on SIGINT/SIGTERM with 30s deadline

### Checkpoint 1 ✅

All 7 checks passed:

1. ✅ `go build ./cmd/breadbox/` compiles with no errors
2. ✅ PostgreSQL started
3. ✅ `breadbox migrate` — 11 migrations applied (8 tables, 4 enums, 6 seed rows)
4. ✅ `breadbox version` prints "dev"
5. ✅ `breadbox serve` starts and logs `addr=:8080`
6. ✅ `curl http://localhost:8080/health` returns `200 OK` with `{"status":"ok","version":"dev"}`
7. ✅ `sqlc generate` succeeds and produces generated Go files

---

## Phase 2: Plaid Integration + Admin Auth ✅

Connect to Plaid, implement the admin dashboard authentication, and build the setup wizard.

**Status:** Complete. Checkpoint 2 verified.

### 2.1 Plaid Client Initialization ✅

- [x] Initialize Plaid Go SDK client from config (client ID, secret, environment)
- [x] Implement access token encryption/decryption (AES-256-GCM) (`plaid-integration.md` Section 7)
- **Ref:** `plaid-integration.md` Sections 1, 7

### 2.2 Plaid Provider Implementation — Link Flow ✅

- [x] Implement `CreateLinkSession`: call Plaid `/link/token/create` with products, webhook URL, user ID (`plaid-integration.md` Section 2.1)
- [x] Implement `ExchangeToken`: call `/item/public_token/exchange`, return Connection + Accounts (`plaid-integration.md` Section 2.2)
- [x] Implement `CreateReauthSession`: call `/link/token/create` in update mode with `access_token` (`plaid-integration.md` Section 2.3)
- [x] Implement `RemoveConnection`: call `/item/remove` (`plaid-integration.md` Section 6.4)
- **Ref:** `plaid-integration.md` Sections 2, 6
- **Note:** Plaid Go SDK v29 only exposes `Sandbox` and `Production` environments (no `Development` constant). The "development" environment maps to Sandbox.

### 2.3 Admin Authentication ✅

- [x] Implement admin account creation (bcrypt hashed passwords, min 8 chars) (`admin-dashboard.md` Section 2)
- [x] Set up session management with `alexedwards/scs` + `pgxstore` (`architecture.md` Section 5.2)
- [x] Implement login/logout routes (`admin-dashboard.md` Section 3)
- [x] Session cookies: `HttpOnly; SameSite=Lax; Secure` (`admin-dashboard.md` Section 3.1)
- [x] CSRF protection middleware for admin POST routes (`architecture.md` Section 1.4)
- [x] Setup detection middleware: redirect to wizard if `setup_complete` is not `true` (`admin-dashboard.md` Section 2.1)
- [x] `GET /admin/api/setup/status` — unauthenticated setup check endpoint (`rest-api.md` Section 8)
- **Ref:** `admin-dashboard.md` Sections 2–3, `architecture.md` Sections 1.4, 5.2

### 2.4 Setup Wizard ✅

- [x] Step 1: Create admin account (`admin-dashboard.md` Section 2.2)
- [x] Step 2: Enter Plaid credentials + validate with test API call (`admin-dashboard.md` Section 2.3–2.4)
- [x] Step 3: Configure sync interval (`admin-dashboard.md` Section 2.4)
- [x] Step 4: Optional webhook URL (`admin-dashboard.md` Section 2.5)
- [x] Step 5: Confirmation + set `setup_complete = true` (`admin-dashboard.md` Section 2.6)
- [x] Programmatic setup endpoint: `POST /admin/api/setup` (`rest-api.md` Section 6.11)
- **Ref:** `admin-dashboard.md` Section 2, `rest-api.md` Section 6.11

### 2.5 Dashboard — Template System & Shared Components ✅

- [x] Template system with Go `html/template` + Pico CSS, embedded via `go:embed` (`admin-dashboard.md` Section 14)
- [x] Navigation sidebar (`admin-dashboard.md` Section 13)
- [x] Flash message system stored in sessions (`admin-dashboard.md` Section 14.4)
- [x] Error pages (404, 500) and empty states (`admin-dashboard.md` Section 18)
- **Ref:** `admin-dashboard.md` Sections 13–14, 18

### 2.6 Dashboard — Connection Management Pages ✅

- [x] Dashboard home page (`admin-dashboard.md` Section 4)
- [x] Connections list page with sync/remove form actions (`admin-dashboard.md` Section 5)
- [x] New connection page with Plaid Link JS integration (`admin-dashboard.md` Section 6)
  - `POST /admin/api/link-token` (`rest-api.md` Section 6.7)
  - `POST /admin/api/exchange-token` (`rest-api.md` Section 6.8)
- [x] Connection detail page (`admin-dashboard.md` Section 7)
- [x] Re-authentication page — update mode, NO token exchange (`admin-dashboard.md` Section 8)
  - `POST /admin/api/connections/:id/reauth` (link token for update mode)
  - `POST /admin/api/connections/:id/reauth-complete` (status update only, no exchange)
- **Ref:** `admin-dashboard.md` Sections 4–8

### 2.7 Dashboard — Family Members Page ✅

- [x] Family members list, create, edit (`admin-dashboard.md` Section 9)
- [x] `POST /admin/api/users` and `PUT /admin/api/users/:id` admin endpoints (`rest-api.md` Section 8)
- [x] No delete in MVP (`admin-dashboard.md` Section 9)
- **Ref:** `admin-dashboard.md` Section 9, `rest-api.md` Section 8

### Checkpoint 2 ✅

Complete the setup wizard, log in, connect a sandbox bank, and see it on the dashboard.

1. ✅ Start `breadbox serve` with `BREADBOX_ENCRYPTION_KEY` set
2. ✅ Open `http://localhost:8080/admin/` — redirects to `/admin/setup/step/1`
3. ✅ Complete all 5 wizard steps with Plaid sandbox credentials (environment = sandbox)
4. ✅ Log in with the admin credentials you created
5. ✅ Dashboard home page loads showing 0 accounts, 0 transactions
6. ✅ Navigate to Connections → "Connect New Bank" → select a family member
7. ✅ In Plaid Link, use sandbox credentials (`user_good` / `pass_good`) → complete link
8. ✅ Connection detail page shows the sandbox institution with status "Active"
9. ✅ Family Members page shows the member you created
10. ✅ Log out → confirm redirect to login page, `/admin/` is inaccessible

---

## Phase 3: Transaction Sync Engine ✅

Build the core sync loop that fetches and stores bank data.

**Status:** Complete. Checkpoint 3 ready for verification.

### 3.1 Service Layer (Deferred to Phase 4)

- Service layer will be built alongside its REST API consumers in Phase 4
- Phase 3 provides the sync engine which Phase 4's service layer will wrap

### 3.2 Cursor-Based Sync Implementation ✅

- [x] Implement `SyncTransactions` for Plaid provider: call `/transactions/sync` with cursor (`plaid-integration.md` Section 3)
- [x] Handle pagination: loop while `has_more` is true (`plaid-integration.md` Section 3.3)
- [x] Handle `TRANSACTIONS_SYNC_MUTATION_DURING_PAGINATION`: reset cursor, discard buffered writes, retry (`plaid-integration.md` Section 3.4)
- [x] Per-connection sync locking (`sync.Map` of mutexes, `TryLock` skips if busy) (`plaid-integration.md` Section 3.3)
- [x] Map all Plaid fields to Breadbox schema including datetime, categories, confidence (`plaid-integration.md` Section 3.2, `data-model.md` Section 3)
- [x] Rate limiting: exponential backoff on 429 (2s→4s→8s→16s→32s, cap 60s, max 5 retries)
- [x] Typed errors: `ErrMutationDuringPagination`, `ErrItemReauthRequired`
- **Ref:** `plaid-integration.md` Section 3, `data-model.md` Section 3

### 3.3 Transaction Processing ✅

- [x] Buffered writes: all results collected in memory during pagination, flushed to DB only after `HasMore=false`
- [x] Process removed FIRST (soft-delete), then added (upsert), then modified (upsert)
- [x] `UpsertTransaction` with ON CONFLICT DO UPDATE — clears `deleted_at` on re-add
- [x] `SoftDeleteTransactionByExternalID` — sets `deleted_at=NOW()` where null
- [x] Account ID resolution with in-memory cache to avoid repeated lookups
- **Ref:** `plaid-integration.md` Section 3.5, `data-model.md` Section 4.2

### 3.4 Balance Refresh ✅

- [x] Implement `GetBalances`: call Plaid `/accounts/get` after each sync
- [x] `UpdateAccountBalances` query: update current, available, limit, currency, last_balance_update
- **Ref:** `plaid-integration.md` Section 4, `data-model.md` Section 2.4

### 3.5 Sync Orchestration & Logging ✅

- [x] Sync engine (`internal/sync/engine.go`): `Sync` (single connection) and `SyncAll` (bounded concurrency, max 5 workers)
- [x] `CreateSyncLog` + `UpdateSyncLog` queries for tracking trigger, counts, status, errors
- [x] Cursor committed only after `HasMore=false`
- [x] Connection status updated on errors (`pending_reauth` for auth errors, clear errors on success)
- [x] Admin "Sync Now" handler: `POST /admin/api/connections/{id}/sync` → async sync, returns 202
- [x] "Sync Now" button on connection detail page (only when status=active)
- **Ref:** `plaid-integration.md` Sections 3, 6, `data-model.md` Sections 2.3, 2.6

### 3.6 Seed Command ✅

- [x] `breadbox seed` subcommand inserts test data (2 users, 2 connections, 4 accounts, 50 transactions, 2 sync logs)
- [x] Idempotent: all inserts use ON CONFLICT DO NOTHING with fixed UUIDs
- **Ref:** `architecture.md` Section 1.1

### Checkpoint 3

Trigger a sync and verify transactions appear in the database.

1. From the connection detail page, click "Sync Now" — see flash message "Sync triggered"
2. Refresh after a few seconds — Sync History shows a new entry with `status = success` and non-zero `added_count`
3. In `psql`: `SELECT COUNT(*) FROM transactions;` returns a positive number
4. `SELECT name, balance_current, balance_available FROM accounts;` shows non-null values
5. Trigger a second sync — sync log shows `added_count = 0` (incremental cursor working)
6. `breadbox seed` inserts test data without errors

---

## Phase 4: REST API ✅

Expose all financial data through authenticated JSON endpoints.

**Status:** Complete. Checkpoint 4 ready for verification.

### 4.1 API Key Authentication Middleware ✅

- [x] Implement API key auth: `X-API-Key: bb_xxxxx` header (`rest-api.md` Section 1)
- [x] Hash presented key with SHA-256, compare to stored `key_hash` where `revoked_at IS NULL` (`rest-api.md` Section 1.1, `data-model.md` Section 2.7)
- [x] Update `last_used_at` on successful auth (async goroutine)
- [x] Error codes: `MISSING_API_KEY`, `INVALID_API_KEY`, `REVOKED_API_KEY` (`rest-api.md` Section 1.2)
- **Ref:** `rest-api.md` Section 1, `data-model.md` Section 2.7

### 4.2 Error Response Format ✅

- [x] Standardized error envelope: `{ "error": { "code": "UPPER_SNAKE_CASE", "message": "..." } }` (`rest-api.md` Section 7, `architecture.md` Section 9.3)
- [x] `WriteError` helper in `internal/middleware/errors.go`
- **Ref:** `rest-api.md` Section 7, `architecture.md` Section 9

### 4.3 Accounts Endpoints ✅

- [x] `GET /api/v1/accounts` — list accounts, filter by `user_id` (`rest-api.md` Section 5.1)
- [x] `GET /api/v1/accounts/:id` — single account with balance (`rest-api.md` Section 5.2)
- **Ref:** `rest-api.md` Section 5.1–5.2

### 4.4 Transactions Endpoints ✅

- [x] `GET /api/v1/transactions` — list with cursor pagination, all 10 filters (date range, account_id, user_id, category, amount range, pending, text search) (`rest-api.md` Section 5.3)
- [x] `GET /api/v1/transactions/count` — total count of active transactions (`rest-api.md` Section 5.4)
- [x] `GET /api/v1/transactions/:id` — single transaction (`rest-api.md` Section 5.5)
- [x] Dynamic SQL query builder with positional parameters for composable filters
- [x] Cursor-based pagination: base64url-encoded JSON `{"d":"YYYY-MM-DD","i":"uuid"}`
- [x] Exclude soft-deleted transactions (`deleted_at IS NULL`) by default
- **Ref:** `rest-api.md` Sections 5.3–5.5, `data-model.md` Section 4.1

### 4.5 Users Endpoint ✅

- [x] `GET /api/v1/users` — list family members (`rest-api.md` Section 5.6)
- **Ref:** `rest-api.md` Section 5.6

### 4.6 Connections Endpoints ✅

- [x] `GET /api/v1/connections` — list connections with status, filter by `user_id` (`rest-api.md` Section 5.7)
- [x] `GET /api/v1/connections/:id/status` — connection health + last sync log info (`rest-api.md` Section 5.8)
- [x] Access tokens and sync cursors excluded from API responses
- **Ref:** `rest-api.md` Sections 5.7–5.8

### 4.7 Sync Trigger Endpoint ✅

- [x] `POST /api/v1/sync` — trigger sync for all connections, return 202 (`rest-api.md` Section 5.9)
- **Ref:** `rest-api.md` Section 5.9

### 4.8 Admin API Endpoints ✅

- [x] `POST /admin/api/api-keys` — create API key, return plaintext once (`rest-api.md` Section 6.1)
- [x] `GET /admin/api/api-keys` — list keys (prefix only, never full key) (`rest-api.md` Section 6.2)
- [x] `DELETE /admin/api/api-keys/:id` — revoke key (set `revoked_at`) (`rest-api.md` Section 6.3)
- [x] Admin dashboard pages: API key list, create form, one-time key display, revoke
- **Ref:** `rest-api.md` Section 6

### 4.9 Service Layer ✅

- [x] Shared service layer in `internal/service/` (used by REST handlers and future MCP tools)
- [x] Clean JSON output: pgtype → Go primitive converters
- [x] API key generation: `crypto/rand` → base62 → `bb_` prefix → SHA-256 hash
- [x] Service initialized in `App` struct and passed to router

### Checkpoint 4

Create an API key and use it to query all REST endpoints with curl.

1. Navigate to API Keys page → "Create API Key" → copy the key
2. `curl -H 'X-API-Key: bb_...' http://localhost:8080/api/v1/accounts` — JSON array with accounts and balances
3. `curl -H 'X-API-Key: bb_...' 'http://localhost:8080/api/v1/transactions?limit=5'` — paginated transactions
4. `curl -H 'X-API-Key: bb_...' 'http://localhost:8080/api/v1/transactions/count'` — `{"count": N}`
5. `curl -H 'X-API-Key: bb_...' 'http://localhost:8080/api/v1/transactions?search=coffee'` — filtered results
6. `curl -H 'X-API-Key: bb_...' http://localhost:8080/api/v1/users` — family member list
7. `curl -H 'X-API-Key: bb_...' http://localhost:8080/api/v1/connections` — connection with status and `last_synced_at`
8. `curl -X POST -H 'X-API-Key: bb_...' http://localhost:8080/api/v1/sync` — `202 Accepted`
9. Test auth errors: no key → `401 MISSING_API_KEY`; bad key → `401 INVALID_API_KEY`
10. Test pagination: `?limit=2` returns `next_cursor` and `has_more: true`; follow cursor for next page

---

## Phase 5: MCP Server ✅

Wrap the REST API layer as MCP tools for AI agent access.

**Status:** Complete. `go build` and `go vet` pass clean.

### 5.1 MCP Server Setup ✅

- [x] Integrated `github.com/modelcontextprotocol/go-sdk` v1.4.0
- [x] Configured Streamable HTTP transport at `/mcp` on the same chi router (`mcp-server.md` Section 2)
- [x] API key authentication via `X-API-Key` middleware applied to MCP routes (`mcp-server.md` Section 2)
- [x] MCP server package: `internal/mcp/` with `server.go` (setup) and `tools.go` (handlers)
- **Ref:** `mcp-server.md` Sections 1–2

### 5.2 MCP Tools ✅

- [x] `list_accounts` — optional `user_id` filter, calls `svc.ListAccounts` (`mcp-server.md` Section 3)
- [x] `query_transactions` — all 10 filters + cursor pagination, calls `svc.ListTransactions` (`mcp-server.md` Section 3)
- [x] `count_transactions` — same filters minus limit/cursor, calls `svc.CountTransactionsFiltered` (`mcp-server.md` Section 3)
- [x] `list_users` — calls `svc.ListUsers` (`mcp-server.md` Section 3)
- [x] `get_sync_status` — calls `svc.ListConnections` (`mcp-server.md` Section 3)
- [x] `trigger_sync` — optional `connection_id` for single-connection sync, calls `svc.TriggerSync` (`mcp-server.md` Section 3)
- [x] All tools call the service layer directly (no HTTP round-trip) (`mcp-server.md` Section 7)
- [x] Error pattern: `IsError: true` with JSON `{"error": "message"}` text content
- [x] Typed input structs with `jsonschema` tags for auto-generated tool schemas
- **Ref:** `mcp-server.md` Sections 3, 7

### 5.3 Stdio Convenience Mode ✅

- [x] `breadbox mcp-stdio` subcommand: loads config, creates App, runs MCP over stdin/stdout
- [x] Logs to stderr (stdout reserved for MCP JSON-RPC)
- [x] No authentication in stdio mode (trusted local process)
- [x] SIGINT/SIGTERM graceful shutdown
- **Ref:** `mcp-server.md` Section 2, `architecture.md` Section 1.1

### 5.4 Dashboard — API Keys Page ✅ (Completed in Phase 4)

- [x] API keys management page: create, view (prefix only), revoke (`admin-dashboard.md` Section 10)
- [x] Display client config examples for Claude Desktop / Claude Code (`mcp-server.md` Sections 2.1–2.2)
- **Ref:** `admin-dashboard.md` Section 10, `mcp-server.md` Sections 2.1–2.2

### 5.5 Service Layer Enhancements ✅

- [x] `CountTransactionsFiltered(ctx, params)` — dynamic `COUNT(*)` with same filter logic as `ListTransactions`
- [x] `TriggerSync(ctx, connectionID *string)` — optional single-connection sync support
- [x] REST handlers updated: `CountTransactionsHandler` accepts filter params, `TriggerSyncHandler` accepts optional `connection_id` body

### Checkpoint 5

Connect Claude Desktop or Claude Code to Breadbox and run a query.

1. Test MCP endpoint directly:
   ```
   curl -X POST -H 'X-API-Key: bb_...' -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}},"id":1}' \
     http://localhost:8080/mcp
   ```
   Verify JSON-RPC response with server capabilities
2. Test stdio: `echo '<same JSON>' | breadbox mcp-stdio` — response on stdout
3. Configure Claude Desktop with the HTTP config from `mcp-server.md` Section 2.1 — verify Breadbox tools appear
4. Ask Claude: "How many accounts do I have in Breadbox?" — verify it calls `list_accounts`
5. Ask Claude: "Show me my recent transactions" — verify `query_transactions` returns data
6. Check API Keys page — "Last Used" timestamp updated

---

## Phase 6: Automated Sync + Webhooks ✅

Make sync automatic and handle Plaid callbacks.

**Status:** Complete. `go build` and `go vet` pass clean.

### 6.1 Cron Scheduling ✅

- [x] Integrated `robfig/cron/v3` for periodic sync (`architecture.md` Section 2)
- [x] Reads interval from `sync_interval_hours` config (default 12h) (`data-model.md` Section 2.8)
- [x] Syncs all active connections on each cron tick via `SyncAll` with `SyncTriggerCron`
- [x] Startup sync: on server start, syncs any connections whose `last_synced_at` is older than the configured interval (`plaid-integration.md` Section 7.4)
- [x] Scheduler struct in `internal/sync/scheduler.go` with `Start`, `Stop`, `RunStartupSync` methods
- **Ref:** `plaid-integration.md` Sections 5.2, 7.4, `architecture.md` Section 2

### 6.2 Webhook Handler ✅

- [x] Implemented `POST /webhooks/{provider}` route (`architecture.md` Section 2)
- [x] Plaid webhook JWT/ES256 verification with JWKS (in-memory cache by `kid`) in `internal/provider/plaid/webhook.go` (`plaid-integration.md` Sections 5.1–5.2)
- [x] Verification: JWT header parsing, ES256 algorithm check, Plaid API JWK fetch, signature verification, `iat` freshness (5 min), SHA-256 body hash (constant-time compare)
- [x] HTTP handler in `internal/webhook/handler.go` dispatches events:
  - `SYNC_UPDATES_AVAILABLE` → fire-and-forget `engine.Sync` with `SyncTriggerWebhook`
  - `ITEM/ERROR` → `UpdateBankConnectionStatus` with `pending_reauth` for re-auth error codes, `error` otherwise
  - `ITEM/PENDING_EXPIRATION` → `UpdateConnectionConsentExpiration`
  - `ITEM/NEW_ACCOUNTS_AVAILABLE` → `UpdateConnectionNewAccounts`
  - Unknown types → log and acknowledge with 200
- [x] Re-auth error codes: `ITEM_LOGIN_REQUIRED`, `INSUFFICIENT_CREDENTIALS`, `INVALID_CREDENTIALS`, `MFA_NOT_SUPPORTED`, `NO_ACCOUNTS`, `USER_SETUP_REQUIRED`
- [x] Added `golang-jwt/jwt/v5` dependency
- **Ref:** `plaid-integration.md` Section 5, `architecture.md` Section 2

### 6.3 Connection Health Monitoring ✅

- [x] Sync engine sets `pending_reauth` on auth errors and clears errors on success (Phase 3)
- [x] Webhook handler updates connection status on `ITEM_ERROR` / `PENDING_EXPIRATION` events
- [x] New queries: `UpdateConnectionNewAccounts`, `UpdateConnectionConsentExpiration`
- [x] Status transitions: `active` ↔ `error`, `active` → `pending_reauth`, `* → disconnected` (`plaid-integration.md` Section 6.1)
- **Ref:** `plaid-integration.md` Section 6

### 6.4 Dashboard — Remaining Pages ✅

- [x] Sync Logs page (`/admin/sync-logs`) with offset-based pagination (25 rows/page), filter by connection and status (`admin-dashboard.md` Section 11)
- [x] Settings page (`/admin/settings`) — sync interval (4/8/12/24h), webhook URL (HTTPS validation), Plaid creds (env-aware: read-only if from env, editable with API validation if from app_config), "Re-run Setup Wizard" link (`admin-dashboard.md` Section 12)
- [x] Service layer: `ListSyncLogsPaginated`, `CountSyncLogsFiltered` dynamic SQL with optional filters
- [x] Navigation updated: 6 items (Dashboard, Connections, Family Members, API Keys, Sync Logs, Settings)
- **Ref:** `admin-dashboard.md` Sections 11–12

### 6.5 Graceful Shutdown ✅

- [x] HTTP server shutdown (30s timeout) — existing
- [x] Cron scheduler `Stop()` — waits for running jobs
- [x] Context cancellation propagates to sync goroutines
- [x] Ordered sequence: HTTP shutdown → scheduler stop → context cancel → DB close
- **Ref:** `architecture.md` Section 1

### Checkpoint 6

Verify cron syncs fire automatically and the remaining dashboard pages work.

1. Set `sync_interval_hours` to a short interval for testing. Restart `breadbox serve`
2. Wait for cron tick — Sync Logs page (`/admin/sync-logs`) shows a new entry with `trigger = cron`
3. Navigate to Settings page → change sync interval → save → flash message "Settings saved"
4. Test graceful shutdown: `Ctrl+C` during a sync — process exits cleanly within 30 seconds
5. (Webhook testing requires a publicly accessible URL or Plaid sandbox webhook endpoints)

---

## Phase 7: Docker Deployment ✅

Package everything for single-command self-hosted deployment.

**Status:** Complete. Docker image builds and runs. `go build` and `go vet` pass clean.

### 7.1 Dockerfile ✅

- [x] Multi-stage build: `golang:1.24-alpine` builder → `alpine:3.21` runtime (`architecture.md` Section 7)
- [x] Single binary output with stripped symbols
- [x] Build-time `VERSION` ARG injected via `-ldflags`
- [x] CA certificates + tzdata in runtime image
- **Ref:** `architecture.md` Section 7

### 7.2 Docker Compose ✅

- [x] Two services: `app` (breadbox) + `db` (postgres:16-alpine) (`architecture.md` Section 7)
- [x] Named volume `postgres_data` for PostgreSQL data persistence
- [x] Auto-run migrations on startup: `breadbox migrate && breadbox serve`
- [x] Environment variable configuration via `.docker.env` + `ENVIRONMENT=docker` override
- [x] Health checks on both services
- **Ref:** `architecture.md` Section 7

### 7.3 Production Polish ✅

- [x] Startup banner with version, port, environment, Plaid status, sync interval, webhook URL, DB pool config
- [x] Connection pool tuning: `DB_MAX_CONNS` (default 25), `DB_MIN_CONNS` (default 2), `DB_MAX_CONN_LIFETIME_MINUTES` (default 60)
- [x] HTTP server timeouts: `HTTP_READ_TIMEOUT_SECONDS` (default 30), `HTTP_WRITE_TIMEOUT_SECONDS` (default 60), `HTTP_IDLE_TIMEOUT_SECONDS` (default 120)
- [x] `.env.example` updated with new config vars
- **Ref:** `architecture.md` Sections 1, 7, 8

### Checkpoint 7 (Final)

Run `docker compose up` from scratch and verify the entire system works end-to-end.

1. `docker compose up -d` from project root
2. `docker compose logs breadbox` — startup banner shows version, port, migration output
3. `curl http://localhost:8080/health` — `200 OK`
4. Open `http://localhost:8080/admin/` — redirects to setup wizard (fresh DB)
5. Complete full flow: setup wizard → connect sandbox bank → trigger sync → create API key → query REST API → connect Claude via MCP
6. `docker compose down && docker compose up -d` — verify data persists across restarts (named volume)
7. Verify migrations ran automatically: `docker compose logs breadbox | grep migrat`

---

## Phase 8: Multi-Provider Refactoring ✅

Decouple the codebase from Plaid-specific assumptions to support multiple bank data providers.
All existing Plaid functionality must continue working identically after this phase.

**Status:** Complete. `go build` and `go vet` pass clean.

### 8.1 Database Schema Migration ✅

- [x] Add `external_id TEXT NULL` and `encrypted_credentials BYTEA NULL` columns to `bank_connections`
- [x] Migrate data: copy `plaid_item_id` → `external_id`, `plaid_access_token` → `encrypted_credentials`
- [x] Drop `plaid_item_id` and `plaid_access_token` columns
- [x] Add `UNIQUE(provider, external_id)` constraint; drop old `plaid_item_id` index
- **Ref:** `data-model.md` Section 2.3, `teller-integration.md` Section 1
- **Files:** `internal/db/migrations/00013_generic_connection_columns.sql`

### 8.2 Update sqlc Queries ✅

- [x] Rewrite all `bank_connections` queries for generic column names
- [x] Rename `GetBankConnectionByPlaidItemID` → `GetBankConnectionByExternalID(provider, external_id)`
- [x] Update `GetBankConnectionForSync`, `CreateBankConnection`, `DeleteBankConnection`
- [x] Run `sqlc generate` to regenerate Go types
- **Files:** `internal/db/queries/bank_connections.sql`, `internal/db/*.sql.go` (generated)

### 8.3 Move Encryption to Shared Package ✅

- [x] Create `internal/crypto/encrypt.go` with `Encrypt()` and `Decrypt()` (moved from `internal/provider/plaid/encrypt.go`)
- [x] Update all callers: Plaid sync, exchange, balances, admin connections
- [x] Delete `internal/provider/plaid/encrypt.go`
- **Files:** `internal/crypto/encrypt.go` (new), `internal/provider/plaid/*.go`, `internal/admin/connections.go`

### 8.4 Provider-Level Error Sentinels ✅

- [x] Create `internal/provider/errors.go` with `ErrReauthRequired` and `ErrSyncRetryable`
- [x] Update `internal/provider/plaid/errors.go` to wrap shared errors
- [x] Remove Plaid-specific error imports from sync engine
- **Files:** `internal/provider/errors.go` (new), `internal/provider/plaid/errors.go`, `internal/sync/engine.go`

### 8.5 Refactor Sync Engine ✅

- [x] Use `conn.ExternalID` and `conn.EncryptedCredentials` (from updated sqlc types)
- [x] Check `provider.ErrSyncRetryable` and `provider.ErrReauthRequired` instead of Plaid-specific errors
- [x] Remove `plaidprovider` import entirely
- **Files:** `internal/sync/engine.go`

### 8.6 Refactor Webhook Handler ✅

- [x] Extract all HTTP headers generically (not just `Plaid-Verification`)
- [x] Use `GetBankConnectionByExternalID(provider, externalID)` for connection lookup
- [x] Add `NeedsReauth bool` to `WebhookEvent`; remove `reauthErrorCodes` map from handler
- [x] Update Plaid webhook implementation to set `NeedsReauth` based on error codes
- **Files:** `internal/provider/provider.go`, `internal/provider/plaid/webhook.go`, `internal/webhook/handler.go`

### 8.7 Refactor Admin Connection Handlers ✅

- [x] Accept `provider` field in link-token and exchange-token requests
- [x] Change `CreateReauthSession(ctx, connectionID)` → `CreateReauthSession(ctx, Connection)`
- [x] Change `RemoveConnection(ctx, connectionID)` → `RemoveConnection(ctx, Connection)`
- [x] Remove all `plaidprovider` type assertions and direct `Decrypt()` calls
- [x] Update Plaid provider implementations for new signatures
- **Ref:** `architecture.md` Section 3 (Provider Interface)
- **Files:** `internal/provider/provider.go`, `internal/provider/plaid/reauth.go`, `internal/provider/plaid/remove.go`, `internal/admin/connections.go`

### 8.8 Settings and Setup for Multi-Provider ✅

- [x] Settings page: add Teller section (placeholder, shows "Not configured" until Phase 9)
- [x] Setup wizard step 2: make Plaid credentials optional (allow Teller-only setup later)
- [x] Programmatic setup endpoint: accept optional Teller fields
- **Files:** `internal/admin/setup.go`, `internal/admin/settings.go`, `internal/templates/pages/settings.html`, `internal/templates/pages/setup_step2.html`

### 8.9 Config System: Teller Keys ✅

- [x] Add `TellerAppID`, `TellerCertPath`, `TellerKeyPath`, `TellerEnv`, `TellerWebhookSecret` to Config struct
- [x] Load from env vars in `Load()`, from `app_config` in `LoadWithDB()` where appropriate
- [x] Cert/key paths and webhook secret are env-var-only (not stored in app_config)
- **Files:** `internal/config/config.go`, `internal/config/load.go`

### 8.10 Admin UI Multi-Provider Templates ✅

- [x] `connection_new.html`: add provider selector dropdown, conditionally load Plaid/Teller JS
- [x] `connection_reauth.html`: detect provider from connection, load correct JS SDK
- [x] `connection_detail.html`: show provider name in connection info
- [x] Only show configured providers in selector (check `a.Providers` map)
- **Files:** `internal/templates/pages/connection_new.html`, `internal/templates/pages/connection_reauth.html`, `internal/templates/pages/connection_detail.html`, `internal/admin/connections.go`

### 8.11 App Initialization: Multi-Provider Skeleton ✅

- [x] Add Teller provider credential detection in `app.New()` (log presence, no init yet)
- **Files:** `internal/app/app.go`

### 8.12 Update Seed Data ✅

- [x] Change `plaid_item_id` → `external_id`, `plaid_access_token` → `encrypted_credentials` in seed SQL
- **Files:** `internal/seed/seed.go`

### 8.13 Update .env.example ✅

- [x] Add Teller environment variables: `TELLER_APP_ID`, `TELLER_CERT_PATH`, `TELLER_KEY_PATH`, `TELLER_ENV`, `TELLER_WEBHOOK_SECRET`
- **Files:** `.env.example`

### Checkpoint 8

Verify all existing Plaid functionality works identically after refactoring:

1. `go build ./cmd/breadbox/` compiles cleanly
2. `go vet ./...` passes
3. `breadbox migrate` applies migration 00013 without errors
4. `breadbox seed` inserts test data with new column names
5. Start `breadbox serve` with Plaid credentials configured
6. Connect a Plaid sandbox bank — full flow works: link token, exchange, accounts appear
7. Trigger "Sync Now" — sync completes, transactions appear
8. Settings page shows both Plaid and Teller sections
9. "Connect New Bank" page shows provider selector (only Plaid if Teller not configured)
10. `psql`: `bank_connections` has `external_id`/`encrypted_credentials`, NOT `plaid_item_id`/`plaid_access_token`

---

## Phase 9: Teller Provider Implementation

Implement the Teller bank data provider alongside Plaid, making Breadbox a true multi-provider system.

### 9.1 Teller HTTP Client

- [ ] Create mTLS-configured HTTP client from cert + key file paths
- [ ] Base URL: `https://api.teller.io` (all environments)
- [ ] HTTP Basic Auth helper (access_token as username, empty password)
- [ ] 30s request timeout, exponential backoff on 429
- **Ref:** `teller-integration.md` Section 1
- **Files:** `internal/provider/teller/client.go`

### 9.2 Teller Provider Struct

- [ ] `TellerProvider` struct implementing `provider.Provider`
- [ ] Compile-time interface check: `var _ provider.Provider = (*TellerProvider)(nil)`
- [ ] Constructor: `NewProvider(httpClient, appID, env, webhookSecret, encryptionKey, logger)`
- **Ref:** `teller-integration.md` Section 1
- **Files:** `internal/provider/teller/provider.go`

### 9.3 Teller Link Flow

- [ ] `CreateLinkSession`: return app ID as token (no server-side creation needed)
- [ ] `ExchangeToken`: parse `{access_token, enrollment_id, institution_name}`, encrypt token, call `GET /accounts`, return Connection + Accounts
- [ ] Admin API handles Teller's `onSuccess` payload format
- **Ref:** `teller-integration.md` Section 2
- **Files:** `internal/provider/teller/link.go`

### 9.4 Teller Transaction Sync

- [ ] Date-range polling: fetch from `(last_synced_at - 10 days)` to today
- [ ] Paginate via `from_id` parameter (last transaction ID from previous page)
- [ ] Map fields: negate amount sign, parse signed string to decimal
- [ ] Return all transactions as `Added`; sync engine handles stale pending cleanup
- [ ] Category mapping via `categories.go` mapping table
- **Ref:** `teller-integration.md` Sections 3, 7
- **Files:** `internal/provider/teller/sync.go`

### 9.5 Teller Balance Refresh

- [ ] Per-account balance fetch: `GET /accounts/{id}/balances`
- [ ] Map: `ledger` → `Current`, `available` → `Available`, `Limit` = nil
- [ ] Currency from account record (not balance response)
- **Ref:** `teller-integration.md` Section 4
- **Files:** `internal/provider/teller/balances.go`

### 9.6 Teller Webhook Handler

- [ ] HMAC-SHA256 signature verification from `Teller-Signature` header
- [ ] Replay protection: reject events older than 5 minutes
- [ ] Map events: `enrollment.disconnected` → `connection_error` (NeedsReauth=true), `transactions.processed` → `sync_available`
- **Ref:** `teller-integration.md` Section 5
- **Files:** `internal/provider/teller/webhook.go`

### 9.7 Teller Reconnection

- [ ] `CreateReauthSession`: return enrollment ID as token (client-side reconnection via Teller Connect)
- [ ] On success: update connection status to `active` (no token exchange needed)
- **Ref:** `teller-integration.md` Section 6
- **Files:** `internal/provider/teller/reauth.go`

### 9.8 Teller Connection Removal

- [ ] `RemoveConnection`: decrypt access token, call `DELETE /enrollments/{enrollment_id}`
- [ ] Idempotent: log and continue if token already invalid
- **Ref:** `teller-integration.md` Section 6
- **Files:** `internal/provider/teller/remove.go`

### 9.9 App Initialization

- [ ] Wire Teller provider in `app.New()` when `TellerAppID + TellerCertPath + TellerKeyPath` are configured
- [ ] Load mTLS certificate, create HTTP client, register `providers["teller"]`
- [ ] Log "teller provider initialized" with environment
- **Files:** `internal/app/app.go`

### 9.10 Admin UI: Teller Connect

- [ ] `connection_new.html`: Teller Connect JS integration — `TellerConnect.setup({applicationId, onSuccess})`, POST enrollment data to `/admin/api/exchange-token`
- [ ] `connection_reauth.html`: Teller Connect reconnection — `TellerConnect.setup({enrollmentId})`, POST to `/admin/api/connections/{id}/reauth-complete`
- **Ref:** `teller-integration.md` Section 2
- **Files:** `internal/templates/pages/connection_new.html`, `internal/templates/pages/connection_reauth.html`

### 9.11 Category Mapping

- [ ] Map ~27 Teller categories to Plaid-compatible primary categories
- [ ] Default unmapped categories to `GENERAL_MERCHANDISE`
- **Ref:** `teller-integration.md` Section 7
- **Files:** `internal/provider/teller/categories.go`

### 9.12 Teller Seed Data

- [ ] Add Teller test connection, accounts, and transactions to seed command
- [ ] Provider = `'teller'`, fake enrollment IDs and encrypted tokens
- **Files:** `internal/seed/seed.go`

### 9.13 Settings & Setup: Teller Validation

- [ ] Teller credential validation (attempt mTLS handshake to verify cert/key)
- [ ] Settings page: editable `teller_app_id`, `teller_env`; display cert/key paths (read-only, env-var-only)
- [ ] Setup wizard: optional Teller configuration alongside Plaid
- **Files:** `internal/provider/teller/validate.go`, `internal/admin/settings.go`, `internal/admin/setup.go`, `internal/templates/pages/settings.html`

### Sync Engine: Stale Pending Cleanup

- [ ] After Teller sync completes, soft-delete pending transactions in the date window not returned by the API
- [ ] Only pending transactions — posted transactions are never auto-deleted
- [ ] Conditioned on `provider = 'teller'` (Plaid handles removals via its own cursor signals)
- **Ref:** `teller-integration.md` Section 3.5
- **Files:** `internal/sync/engine.go`

### Task Dependencies

```
9.1 (HTTP client) ──> 9.2 (provider struct) ──> 9.3 (link flow)
                                             ──> 9.4 (sync)
                                             ──> 9.5 (balances)
                                             ──> 9.6 (webhook)
                                             ──> 9.7 (reauth)
                                             ──> 9.8 (remove)

9.9 (app init) depends on 9.2

9.10 (UI) depends on 9.3 and 9.7

9.11 (categories) independent, used by 9.4

9.12 (seed) independent

9.13 (settings) depends on 9.1 (for validation)
```

### Checkpoint 9

Verify Teller works end-to-end alongside Plaid:

1. `go build ./cmd/breadbox/` compiles cleanly
2. `go vet ./...` passes
3. Configure Teller sandbox credentials in `.local.env`, start server — log shows both providers initialized
4. "Connect New Bank" page shows provider selector with Plaid and Teller options
5. Select Teller, choose a family member — Teller Connect opens
6. Complete Teller sandbox enrollment (`username`/`password`) — connection appears with provider "teller"
7. Trigger "Sync Now" — Teller transactions sync into database
8. Verify categories: Teller categories mapped to primary categories (e.g., `dining` → `FOOD_AND_DRINK`)
9. REST API: `GET /api/v1/transactions` returns transactions from both Plaid and Teller connections
10. Trigger a second Teller sync — no duplicate transactions (upsert working)
11. Test Teller reconnection: set connection to `pending_reauth`, complete Teller Connect reauth
12. Settings page shows functional Teller configuration section
13. `breadbox seed` inserts both Plaid and Teller test data

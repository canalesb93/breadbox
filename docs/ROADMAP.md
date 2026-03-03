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

## Phase 9: Teller Provider Implementation ✅

Implement the Teller bank data provider alongside Plaid, making Breadbox a true multi-provider system.

**Status:** Complete. All Plaid functionality continues working identically.

### 9.1 Teller HTTP Client ✅

- [x] Create mTLS-configured HTTP client from cert + key file paths
- [x] Base URL: `https://api.teller.io` (all environments)
- [x] HTTP Basic Auth helper (access_token as username, empty password)
- [x] 30s request timeout, exponential backoff on 429
- **Ref:** `teller-integration.md` Section 1
- **Files:** `internal/provider/teller/client.go`

### 9.2 Teller Provider Struct ✅

- [x] `TellerProvider` struct implementing `provider.Provider`
- [x] Compile-time interface check: `var _ provider.Provider = (*TellerProvider)(nil)`
- [x] Constructor: `NewProvider(httpClient, appID, env, webhookSecret, encryptionKey, logger)`
- **Ref:** `teller-integration.md` Section 1
- **Files:** `internal/provider/teller/provider.go`

### 9.3 Teller Link Flow ✅

- [x] `CreateLinkSession`: return app ID as token (no server-side creation needed)
- [x] `ExchangeToken`: parse `{access_token, enrollment_id, institution_name}`, encrypt token, call `GET /accounts`, return Connection + Accounts
- [x] Admin API handles Teller's `onSuccess` payload format
- **Ref:** `teller-integration.md` Section 2
- **Files:** `internal/provider/teller/link.go`

### 9.4 Teller Transaction Sync ✅

- [x] Date-range polling: fetch from `(last_synced_at - 10 days)` to today
- [x] Paginate via `from_id` parameter (last transaction ID from previous page)
- [x] Map fields: negate amount sign, parse signed string to decimal
- [x] Return all transactions as `Added`; sync engine handles stale pending cleanup
- [x] Category mapping via `categories.go` mapping table
- **Ref:** `teller-integration.md` Sections 3, 7
- **Files:** `internal/provider/teller/sync.go`

### 9.5 Teller Balance Refresh ✅

- [x] Per-account balance fetch: `GET /accounts/{id}/balances`
- [x] Map: `ledger` → `Current`, `available` → `Available`, `Limit` = nil
- [x] Currency from account record (not balance response)
- **Ref:** `teller-integration.md` Section 4
- **Files:** `internal/provider/teller/balances.go`

### 9.6 Teller Webhook Handler ✅

- [x] HMAC-SHA256 signature verification from `Teller-Signature` header
- [x] Replay protection: reject events older than 5 minutes
- [x] Map events: `enrollment.disconnected` → `connection_error` (NeedsReauth=true), `transactions.processed` → `sync_available`
- **Ref:** `teller-integration.md` Section 5
- **Files:** `internal/provider/teller/webhook.go`

### 9.7 Teller Reconnection ✅

- [x] `CreateReauthSession`: return enrollment ID as token (client-side reconnection via Teller Connect)
- [x] On success: update connection status to `active` (no token exchange needed)
- **Ref:** `teller-integration.md` Section 6
- **Files:** `internal/provider/teller/reauth.go`

### 9.8 Teller Connection Removal ✅

- [x] `RemoveConnection`: decrypt access token, call `DELETE /enrollments/{enrollment_id}`
- [x] Idempotent: log and continue if token already invalid
- **Ref:** `teller-integration.md` Section 6
- **Files:** `internal/provider/teller/remove.go`

### 9.9 App Initialization ✅

- [x] Wire Teller provider in `app.New()` when `TellerAppID + TellerCertPath + TellerKeyPath` are configured
- [x] Load mTLS certificate, create HTTP client, register `providers["teller"]`
- [x] Log "teller provider initialized" with environment
- **Files:** `internal/app/app.go`

### 9.10 Admin UI: Teller Connect ✅

- [x] `connection_new.html`: Provider dropdown (Plaid/Teller), Teller Connect JS integration — `TellerConnect.setup({applicationId, onSuccess})`, POST enrollment data to `/admin/api/exchange-token`
- [x] `connection_reauth.html`: Conditional SDK loading, Teller Connect reconnection — `TellerConnect.setup({enrollmentId})`, POST to `/admin/api/connections/{id}/reauth-complete`
- **Ref:** `teller-integration.md` Section 2
- **Files:** `internal/templates/pages/connection_new.html`, `internal/templates/pages/connection_reauth.html`

### 9.11 Category Mapping ✅

- [x] Map 27 Teller categories to Plaid-compatible primary categories
- [x] Default unmapped categories to `GENERAL_MERCHANDISE`
- **Ref:** `teller-integration.md` Section 7
- **Files:** `internal/provider/teller/categories.go`

### 9.12 Teller Seed Data ✅

- [x] Add Teller test connection (Alice/Wells Fargo), 2 accounts, 6 transactions to seed command
- [x] Provider = `'teller'`, enrollment ID and encrypted token placeholders
- **Files:** `internal/seed/seed.go`

### 9.13 Settings & Setup: Teller Validation ✅

- [x] Teller credential validation via `tls.LoadX509KeyPair` check
- [x] Settings page: Teller cert/webhook status display when configured from env
- [x] Admin handlers pass `HasPlaid`, `HasTeller`, `TellerEnv`, `TellerAppID` to templates
- **Files:** `internal/provider/teller/validate.go`, `internal/admin/settings.go`, `internal/admin/connections.go`, `internal/templates/pages/settings.html`

### Sync Engine: Stale Pending Cleanup ✅

- [x] After Teller sync completes, soft-delete pending transactions in the date window not returned by the API
- [x] Only pending transactions — posted transactions are never auto-deleted
- [x] Conditioned on `provider = 'teller'` (Plaid handles removals via its own cursor signals)
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

---

## Phase 10: Enhanced Settings & Connection Management ✅

Per-account controls, connection pausing, per-connection sync intervals, and provider credential testing.

**Depends on:** Phases 8–9 (generic columns, both providers functional)

**Status:** Complete. Checkpoint 10 ready for verification.

### 10.1 Migration: Account Settings ✅

- [x] Add `display_name TEXT NULL` and `excluded BOOLEAN NOT NULL DEFAULT FALSE` to `accounts`
- [x] `display_name NULL` means "use bank name" — templates use `COALESCE(display_name, name)`
- [x] `excluded` only affects transaction upserts (balances still refresh for reporting)
- **Ref:** `data-model.md` Section 2.4
- **Files:** `internal/db/migrations/00014_account_settings.sql`

### 10.2 Migration: Connection Pause & Interval ✅

- [x] Add `paused BOOLEAN NOT NULL DEFAULT FALSE` to `bank_connections`
- [x] Add `sync_interval_override_minutes INTEGER NULL` to `bank_connections`
- [x] `paused` is orthogonal to `status` — a connection can be `error` + `paused`
- [x] Manual "Sync Now" bypasses pause (only cron respects it)
- **Ref:** `data-model.md` Section 2.3
- **Files:** `internal/db/migrations/00015_connection_pause.sql`

### 10.3 sqlc Queries ✅

- [x] `UpdateAccountDisplayName(ctx, id, display_name)` — nullable text
- [x] `UpdateAccountExcluded(ctx, id, excluded)` — boolean
- [x] `ListExcludedAccountIDsByConnection(ctx, connection_id)` — returns UUIDs
- [x] `UpdateConnectionPaused(ctx, id, paused)` — boolean
- [x] `UpdateConnectionSyncInterval(ctx, id, override_minutes)` — nullable int
- [x] `ListActiveUnpausedConnections(ctx)` — WHERE status='active' AND paused=false
- [x] Update existing account/connection queries to include new columns in SELECT
- **Files:** `internal/db/queries/accounts.sql`, `internal/db/queries/bank_connections.sql`

### 10.4 Sync Engine: Excluded Account Filtering ✅

- [x] Before upserting transactions, fetch excluded account IDs for the connection
- [x] Skip transactions whose account is in the excluded set
- [x] Log skipped count at debug level
- **Ref:** `architecture.md` Section 3
- **Files:** `internal/sync/engine.go`

### 10.5 Scheduler: Pause & Per-Connection Intervals ✅

- [x] Replace `ListActiveConnections` with `ListActiveUnpausedConnections` for cron
- [x] Cron fires at the minimum interval (e.g., every 15 minutes)
- [x] For each connection: compute effective interval = `COALESCE(sync_interval_override_minutes, global_interval)`
- [x] Skip if `last_synced_at + effective_interval > now`
- [x] Startup sync also respects pause and per-connection intervals
- **Files:** `internal/sync/scheduler.go`

### 10.6 Admin Handlers: Account Settings ✅

- [x] `POST /admin/api/accounts/{id}/excluded` — toggle `excluded` (JSON body: `{"excluded": true}`)
- [x] `POST /admin/api/accounts/{id}/display-name` — set display name (JSON body: `{"display_name": "My Checking"}`)
- [x] Both return updated account as JSON
- **Files:** `internal/admin/connections.go`

### 10.7 Admin Handlers: Connection Pause & Interval ✅

- [x] `POST /admin/api/connections/{id}/paused` — toggle pause (JSON body: `{"paused": true}`)
- [x] `POST /admin/api/connections/{id}/sync-interval` — set override (JSON body: `{"minutes": 30}`, null to clear)
- [x] Both return updated connection as JSON
- **Files:** `internal/admin/connections.go`

### 10.8 Templates: Account & Connection Controls ✅

- [x] Connection detail page: account rows with exclude toggle and display name inline edit
- [x] Connection detail page: pause/resume button, per-connection interval dropdown (15m, 30m, 1h, 2h, 4h, 12h, 24h, "Use global")
- [x] Connections list: "Paused" badge next to connection name when paused
- [x] All controls use `fetch()` POST calls (no full page reload)
- **Files:** `internal/templates/pages/connection_detail.html`, `internal/templates/pages/connections.html`

### 10.9 Settings: Test Connection Button ✅

- [x] "Test Connection" button per configured provider on settings page
- [x] Plaid: call existing `ValidateCredentials` (API handshake)
- [x] Teller: validate mTLS certificate key pair
- [x] Display result inline: "Connection successful" or error message
- **Files:** `internal/admin/settings.go`, `internal/templates/pages/settings.html`

### Task Dependencies

```
10.1 (account migration) ─┐
                           ├──> 10.3 (queries) ──> 10.4 (sync engine)
10.2 (connection migration)┘                   ──> 10.5 (scheduler)
                                               ──> 10.6 (account handlers)
                                               ──> 10.7 (connection handlers)
                                                        │
                                                        v
                                                   10.8 (templates)

10.9 (test connection) — independent
```

### Checkpoint 10

1. `breadbox migrate` applies migrations 00014 and 00015 cleanly
2. `psql`: `\d accounts` shows `display_name` and `excluded` columns; `\d bank_connections` shows `paused` and `sync_interval_override_minutes`
3. Connection detail page: toggle "Exclude" on an account → excluded accounts show strike-through or "Excluded" badge
4. Trigger sync → excluded account's transactions are skipped (check sync log counts)
5. Set display name on an account → name appears in connection detail and API responses
6. Pause a connection → "Paused" badge appears; cron skips it (check sync logs after a cron tick)
7. Click "Sync Now" on a paused connection → sync runs anyway (manual bypasses pause)
8. Set per-connection interval to 15 minutes → that connection syncs more frequently than global interval
9. Settings page: "Test Connection" for Plaid shows success with valid credentials

---

## Phase 11: CSV Import Provider ✅

Upload bank CSV exports, map columns, and import transactions with hash-based deduplication.

**Status:** Complete. Checkpoint 11 verified.

**Depends on:** Phase 8 (generic columns, `csv` in provider_type enum)

### 11.1 CSV Parser ✅

- [x] Read file into memory (max 10MB)
- [x] Auto-detect delimiter: try comma, tab, semicolon, pipe — pick the one that produces consistent column counts
- [x] Strip BOM (UTF-8 `\xEF\xBB\xBF`, UTF-16 LE/BE)
- [x] Return headers (first row) and data rows
- [x] Reject files with < 2 rows or > 50,000 rows
- [x] 10,000 row limit for preview (return first 10 rows to UI, full data for import)
- **Ref:** `csv-import.md` Sections 2, 3
- **Files:** `internal/provider/csv/parser.go` (new)

### 11.2 Column Mapping & Templates ✅

- [x] Pre-built templates: Chase (credit + checking), Bank of America, Wells Fargo, Capital One, Amex
- [x] Each template: header patterns, column mappings, sign convention, date format hint
- [x] Auto-detect: compare parsed headers against all templates, return best match
- [x] Sign convention toggle: "Positive = debit" (default) vs "Positive = credit"
- **Ref:** `csv-import.md` Sections 3, 4
- **Files:** `internal/provider/csv/templates.go` (new)

### 11.3 Import Logic ✅

- [x] Apply column mapping to each row → (date, amount, description, category?, merchant?)
- [x] Parse dates using auto-detection strategy (try formats against first 20 values, pick best)
- [x] Parse amounts: strip currency symbols, handle commas, parenthetical negatives, split debit/credit columns
- [x] Normalize sign per selected convention (positive = debit in storage)
- [x] Generate `external_transaction_id = SHA-256(account_id|date|amount|description)`
- [x] Return list of parsed transactions + list of skipped rows with reasons
- **Ref:** `csv-import.md` Sections 5, 6, 7
- **Files:** `internal/provider/csv/dateparser.go`, `internal/provider/csv/amount.go`, `internal/provider/csv/dedup.go` (new)

### 11.4 CSV Provider Stub ✅

- [x] Implement `Provider` interface — all methods return `provider.ErrNotSupported` except `RemoveConnection` (returns nil)
- [x] Add `var _ provider.Provider = (*CSVProvider)(nil)` compile-time check
- [x] Constructor: `NewProvider()` (no config needed)
- **Ref:** `csv-import.md` Section 9
- **Files:** `internal/provider/csv/provider.go` (new)

### 11.5 Import Service ✅

- [x] `ImportCSV(ctx, params)` — orchestrates the full import flow
- [x] Create or reuse CSV connection + account for the selected member
- [x] Call import logic to parse rows
- [x] Upsert transactions via existing `UpsertTransaction` (reuses ON CONFLICT)
- [x] Create `sync_logs` entry with `trigger = manual`, `provider = csv`
- [x] Return import result: total, inserted, updated, skipped counts
- **Ref:** `csv-import.md` Sections 7, 8
- **Files:** `internal/service/csv_import.go` (new)

### 11.6 Admin Handlers ✅

- [x] `GET /admin/connections/import-csv` — render import wizard page
- [x] `POST /admin/api/csv/upload` — multipart upload (10MB limit), parse file, return headers + preview rows + auto-detected template
- [x] `POST /admin/api/csv/preview` — apply column mapping to uploaded data, return first 10 parsed rows with validation
- [x] `POST /admin/api/csv/import` — execute full import with confirmed mapping, return results
- [x] Upload stored in memory (not on disk) for the duration of the wizard session
- **Files:** `internal/admin/csv_import.go` (new)

### 11.7 Template: Import Wizard ✅

- [x] Multi-step wizard UI (fetch-driven, no full page reloads):
  - Step 1: Select family member + file upload
  - Step 2: Column mapping dropdowns + sign toggle + template auto-select + preview table
  - Step 3: Confirm summary (row count, date range, account name)
  - Step 4: Results (counts + link to connection detail)
- [x] Uses base layout consistent with admin dashboard
- **Ref:** `csv-import.md` Section 2
- **Files:** `internal/templates/pages/csv_import.html` (new)

### 11.8 Re-Import: Connection Detail Integration ✅

- [x] CSV connections show "Import More" button on connection detail page
- [x] Button links to `/admin/connections/import-csv?connection_id={id}`
- [x] Wizard pre-fills member and account name from existing connection
- **Ref:** `csv-import.md` Section 8
- **Files:** `internal/templates/pages/connection_detail.html`

### 11.9 App Init & Spec Doc ✅

- [x] Register CSV provider in `app.New()` — always available (no config/credentials needed)
- [x] Log "csv provider registered" at startup
- [x] `docs/csv-import.md` spec is consistent with implementation
- [x] CSV option added to connection_new.html provider dropdown
- **Files:** `internal/app/app.go`, `internal/templates/pages/connection_new.html`

### Checkpoint 11

1. `go build ./cmd/breadbox/` compiles cleanly
2. Start server — log shows "csv provider registered"
3. Navigate to "Import CSV" page (linked from connections page or nav)
4. Upload a Chase credit card CSV → template auto-detected, columns pre-mapped
5. Preview shows correct dates, amounts (negative = charges), descriptions
6. Confirm import → "Imported N transactions" result page
7. Connection detail shows CSV connection with "Import More" button
8. Upload the same CSV again → all transactions count as "updated" (dedup working)
9. Upload a Bank of America CSV for a different member → separate connection created
10. REST API: `GET /api/v1/transactions?user_id={id}` includes CSV-imported transactions
11. Try uploading a malformed CSV (wrong encoding, < 2 rows) → clear error messages

---

## Phase 12A: Admin UI Foundation ✅

Modernize the admin template system, add Alpine.js interactivity, and prepare for new pages.

**Status:** Complete. Checkpoint 12A verified. All 7 tasks done.

### 12A.1 Pico CSS: Classless → Class-Based ✅

- [x] Switch from `pico.classless.min.css` to `pico.min.css` (class-based variant)
- [x] Update both `base.html` and `wizard.html` layouts
- [x] Wizard uses `.bb-wizard` class for centering instead of classless body override
- **Files:** `internal/templates/layout/base.html`, `internal/templates/layout/wizard.html`

### 12A.2 Add Alpine.js ✅

- [x] Add Alpine.js v3 via CDN `<script>` tag in base layout
- [x] Replace all `alert()` calls with Alpine toast component (`connection_detail.html`)
- [x] Replace `confirm()` with inline Alpine two-step pattern (`connection_detail.html`, `api_keys.html`)
- [x] Replace `alert('Copied!')` with Alpine inline feedback (`api_key_created.html`)
- [x] Added `[x-cloak]` CSS rule for hiding Alpine elements before init
- **Files:** `internal/templates/layout/base.html`, `internal/templates/pages/connection_detail.html`, `internal/templates/pages/api_keys.html`, `internal/templates/pages/api_key_created.html`

### 12A.3 Dark Mode ✅

- [x] Remove `data-theme="light"` from `<html>` tag (lets Pico respect `prefers-color-scheme`)
- [x] Replace all hardcoded hex colors in badge/flash styles with `color-mix()` + Pico CSS custom properties
- [x] Updated both `base.html` and `wizard.html` flash styles
- **Files:** `internal/templates/layout/base.html`, `internal/templates/layout/wizard.html`

### 12A.4 Badge Template Functions ✅

- [x] Add `statusBadge(status string)` template function — returns HTML for connection status badges
- [x] Add `syncBadge(status string)` template function — returns HTML for sync status badges
- [x] Replace 5 copy-pasted if-chains across templates with function calls
- [x] Badge colors use CSS custom properties (dark-mode compatible)
- **Files:** `internal/admin/templates.go`, `connections.html`, `connection_detail.html`, `dashboard.html`, `sync_logs.html`

### 12A.5 Common Template Data Helper ✅

- [x] Create `BaseTemplateData(r, sm, currentPage, pageTitle)` helper returning `map[string]any`
- [x] Auto-injects: `CSRFToken`, `Flash` messages, `CurrentPage`, `PageTitle`
- [x] Handler migration deferred to future cleanup — function definition added
- **Files:** `internal/admin/templates.go`

### 12A.6 Navigation Restructure ✅

- [x] Group nav items into Data (Dashboard, Connections, Members) and System (API Keys, Sync Logs, Settings)
- [x] Add visual divider between sections (`.bb-nav-divider`)
- [x] Alpine-powered hamburger menu for mobile (collapses on small screens)
- [x] Current page highlighting via `CurrentPage` (existing pattern)
- **Files:** `internal/templates/partials/nav.html`, `internal/templates/layout/base.html`

### 12A.7 CSS Spacing Tokens ✅

- [x] Define custom properties: `--bb-gap-xs` through `--bb-gap-xl`
- [x] Replace common inline spacing in base layout CSS and page templates with token references
- [x] Incremental — most common patterns replaced, one-off values left as-is
- **Files:** `internal/templates/layout/base.html`, page templates

### Task Dependencies (12A)

```
12A.1 (class-based Pico) ──> 12A.3 (dark mode)
12A.2 (Alpine.js) ──> 12A.6 (nav restructure)
12A.4 (badge functions) — independent
12A.5 (template data helper) — independent
12A.7 (spacing tokens) — independent
```

### Checkpoint 12A

1. All existing pages render correctly with class-based Pico CSS
2. Dark mode: toggle OS/browser dark mode → admin UI switches theme automatically
3. Badge colors are visible in both light and dark themes
4. Confirm dialogs use inline Alpine patterns (no browser `alert()`/`confirm()`)
5. Nav shows grouped sections with divider; hamburger collapses on narrow viewport
6. No inline `style` attributes remain for spacing (replaced with tokens/classes)

---

## Phase 12B: Admin Transaction Pages ✅

Transaction list, account detail, and cross-linking throughout the admin UI.

**Status:** Complete. Checkpoint 12B verified.

**Depends on:** Phase 12A (UI foundation), existing service layer

### 12B.1 Service: Admin Transaction List ✅

- [x] `ListTransactionsAdmin(ctx, params)` — offset-based pagination (consistent with sync logs page)
- [x] JOIN account name, connection institution name, user name for display
- [x] Support all 10 filters: date range, account_id, user_id, category, amount min/max, pending, text search, connection_id, sort order
- [x] Return total count for pagination controls
- [x] `ListDistinctCategories(ctx)` — for category filter dropdown
- **Ref:** `rest-api.md` Section 5.3 (filter spec), `admin-dashboard.md` Section 11 (pagination pattern)
- **Files:** `internal/service/transactions.go`, `internal/service/types.go`

### 12B.2 Handler: Transaction List Page ✅

- [x] `GET /admin/transactions` — parse filter query params from URL
- [x] Load dropdown data: accounts (with connection/user context), users, distinct categories
- [x] Render page with filters applied, preserve filter state in form
- **Files:** `internal/admin/transactions.go` (new)

### 12B.3 Template: Transaction List ✅

- [x] Filter form: date range (start/end date inputs), account dropdown, user dropdown, category dropdown, amount range (min/max), pending toggle, text search input
- [x] Table columns: Date, Description, Amount, Account, Category, Status (pending/posted)
- [x] Alpine expandable row: click row to see full transaction detail (merchant, external ID, timestamps)
- [x] Offset-based pagination (page numbers, prev/next) consistent with sync logs
- [x] Amount formatting: color-coded (red for negative amounts), currency code
- **Files:** `internal/templates/pages/transactions.html` (new)

### 12B.4 Service: Account Detail ✅

- [x] `GetAccountDetail(ctx, id)` — extends `AccountResponse` with connection institution name, provider, user name
- [x] Uses GetAccount + GetBankConnection for provider and user name (option 2 from plan)
- **Files:** `internal/service/accounts.go`, `internal/service/types.go`

### 12B.5 Handler: Account Detail Page ✅

- [x] `GET /admin/accounts/{id}` — load account detail + filtered transaction list
- [x] Reuses transaction list logic from 12B.1 with `account_id` pre-set
- **Files:** `internal/admin/transactions.go`

### 12B.6 Template: Account Detail ✅

- [x] Info card: account name (display_name or bank name), type, subtype, mask, current/available balance, connection link, user name, provider
- [x] Transaction table: pre-filtered by account, same columns/pagination as 12B.3
- [x] Edit controls for display name and excluded status with toast feedback
- **Files:** `internal/templates/pages/account_detail.html` (new)

### 12B.7 Routes & Cross-Links ✅

- [x] Register routes: `GET /admin/transactions`, `GET /admin/accounts/{id}`
- [x] Add "Transactions" link to nav (Data section, after Connections)
- [x] Connection detail: account names link to `/admin/accounts/{id}`
- [x] Transaction list: account names link to account detail
- [x] Dashboard: transaction count links to transaction list, "View All Sync Logs" button
- **Files:** `internal/admin/router.go`, `internal/templates/pages/connection_detail.html`, `internal/templates/partials/nav.html`, `internal/templates/pages/dashboard.html`

### Task Dependencies (12B)

```
12B.1 (service) ──> 12B.2 (handler) ──> 12B.3 (template) ──> 12B.7 (routes + cross-links)
12B.4 (service) ──> 12B.5 (handler) ──> 12B.6 (template) ──> 12B.7 (routes + cross-links)
```

### Checkpoint 12B

1. Navigate to Transactions page from nav → transaction list loads with all synced transactions
2. Apply filters: date range → table updates; search "coffee" → filtered results; select an account → scoped list
3. Pagination: click through pages, verify correct counts
4. Click a transaction row → expands to show full detail (merchant, external ID, timestamps)
5. Click an account name in the transaction list → navigates to account detail page
6. Account detail: info card shows correct balances, type, connection link
7. Account detail: transaction table is pre-filtered to that account
8. Connection detail: account names are now clickable links
9. Dashboard: transaction count is accurate, "View All" links to transaction list

---

## Phase 13A: Bug Fixes & Dashboard UX

Fix confirmed bugs in the setup wizard, improve dashboard navigation, and polish the onboarding experience.

**Depends on:** None (can be done immediately on the current codebase, independent of Phases 10–12)

### 13A.1 Bug Fix: setup_complete Written on GET ✅

- [x] Move `setup_complete = true` from `SetupStep5Handler` GET to a dedicated POST handler
- [x] Step 5 GET only renders the summary page; a "Confirm & Finish" button POSTs to finalize
- [x] Prevents accidental wizard completion from page reloads or direct URL navigation

### 13A.2 Bug Fix: Programmatic Setup Skips Plaid Validation ✅

- [x] Add `plaidprovider.ValidateCredentials(ctx, clientID, secret, environment)` call in `ProgrammaticSetupHandler` when both Plaid creds are provided
- [x] Match the validation behavior of the interactive `SetupStep2Handler` (which already validates)
- [x] Return validation error in the API response if credentials fail

### 13A.3 Bug Fix: Broken "Re-run Setup Wizard" Link ✅

- [x] Remove `<a href="/admin/setup/step/1">Re-run Setup Wizard</a>` from settings page
- [x] Replace with explanatory text: "To reconfigure providers, update the settings above."

### 13A.4 Fix: Sync Interval Unit Mismatch ✅

- [x] Update wizard step 3 to write `sync_interval_minutes` instead of `sync_interval_hours`
- [x] Offer the same option set as the settings page: 15m, 30m, 1h, 4h, 8h, 12h, 24h
- [x] Update step 3 template to show minute-based options
- [x] Update step 5 to display human-readable interval from minutes
- [x] Update programmatic handler: field renamed to `SyncIntervalMinutes`, config key to `sync_interval_minutes`

### 13A.5 Dashboard: Clickable Stats & Alert Banner ✅

- [x] Wrap "Needs Attention" stat card in `<a href="/admin/connections">` when count > 0
- [x] Add broken-connections alert banner (`<div role="alert">`) above stat cards when any connection needs attention
- [x] Banner text: "{N} connection(s) need attention" with link to connections page

### 13A.6 Dashboard: Institution Name Links in Sync Activity ✅

- [x] Make institution names in the Recent Sync Activity table link to `/admin/connections/{id}`
- [x] `ConnectionID` field already present in `ListRecentSyncLogsRow` — no query changes needed

### 13A.7 Human-Readable Error Messages ✅

- [x] Add `errorMessage(code string) string` template function in `templates.go`
- [x] Map known error codes (ITEM_LOGIN_REQUIRED, INSUFFICIENT_CREDENTIALS, INVALID_CREDENTIALS, MFA_NOT_SUPPORTED, NO_ACCOUNTS, enrollment.disconnected) to user-friendly messages
- [x] Use `ErrorCode` field (not `ErrorMessage`) for lookup in connection detail error display

### 13A.8 Connection Detail: Breadcrumb Navigation ✅

- [x] Replace `← Connections` back-link with semantic breadcrumb using `<nav aria-label="breadcrumb">`
- [x] Connection detail: `Connections / {institution name}`
- [x] Reauth page: `Connections / {institution name} / Re-authenticate`

### 13A.9 Wizard Step 5: Provider Status & CTA ✅

- [x] Add provider configuration status: Plaid (from app_config), Teller (from env var), CSV (always available)
- [x] Add "Connect Your First Bank →" CTA button when providers are configured
- [x] Show warning when no providers are configured

### 13A.10 Wizard Step 4: Reframe Webhook as Optional ✅

- [x] Lead with "Webhooks are optional" framing
- [x] Cloudflare Tunnel link in collapsible details section
- [x] Skip button more prominent: "Skip — I'll Sync on a Schedule"
- [x] Clarified that URL is the base URL where Breadbox is hosted

### Task Dependencies (13A)

```
13A.1 (setup_complete bug) — independent, do first
13A.2 (programmatic validation bug) — independent, do first
13A.3 (re-run wizard bug) — independent, do first
13A.4 (interval mismatch) — independent, do first

13A.5 (dashboard stats) ─┐
13A.6 (dashboard links)  ┘ dashboard group

13A.7 (error messages)  ─┐
13A.8 (breadcrumbs)      ┘ connection detail group

13A.9 (step 5 CTA)     ─┐
13A.10 (step 4 reframe)  ┘ wizard polish group
```

### Checkpoint 13A

1. Navigate to `/admin/setup/step/5` directly via URL bar — setup is NOT marked complete (GET no longer writes the flag)
2. Complete the wizard normally through "Confirm & Finish" button — setup IS marked complete
3. `POST /admin/api/setup` with invalid Plaid credentials → returns validation error (not silent save)
4. Settings page: no "Re-run Setup Wizard" link; shows "Change Password" or replacement text
5. Wizard step 3 shows minute-based intervals (15m, 30m, 1h, etc.); saved value matches settings page
6. Dashboard: "Needs Attention" stat is a clickable link when count > 0; alert banner appears for broken connections
7. Dashboard: institution names in Recent Sync Activity are links to connection detail
8. Connection detail: error shows "Your bank login has changed" instead of `ITEM_LOGIN_REQUIRED`
9. Connection detail: breadcrumb shows `Connections / Chase Checking`
10. Wizard step 5: shows provider status and "Connect Your First Bank →" CTA
11. Wizard step 4: leads with "Webhooks are optional" and has clear skip path

---

## Phase 13B: Setup & Settings Overhaul

Restructure the wizard for multi-provider onboarding, add missing settings features, and improve the family members page.

**Depends on:** Phase 13A (bug fixes land first). Some tasks benefit from Phase 12A (Alpine.js for confirmation dialogs) but can use vanilla JS fallback.

### 13B.1 Wizard Step 2: Multi-Provider Selection

- [ ] Rename step 2 from "Configure Plaid" to "Configure Bank Providers"
- [ ] Add provider selection: Plaid / Teller / Both / Skip All
- [ ] Based on selection: show Plaid credential form, Teller env-var guidance card, or both
- [ ] Teller section is informational (cert/key are env-var-only) with copy-ready env var snippet
- [ ] "Skip All" goes directly to step 3 with a note that providers can be configured later in Settings
- **Files:** `internal/admin/setup.go` (step 2 handler), `internal/templates/pages/setup_step2.html`

### 13B.2 Wizard: Optional Family Member Step

- [ ] Add new step between current step 3 (sync interval) and step 4 (webhook)
- [ ] Collects: name (required), email (optional) — same fields as `/admin/users/new`
- [ ] "Skip — I'll add members later" button proceeds without creating a member
- [ ] Renumber subsequent steps (or insert as step 3b to avoid renumbering)
- [ ] Prevents the empty family-member dropdown dead-end when connecting the first bank
- **Files:** `internal/admin/setup.go` (new step handler), `internal/templates/pages/setup_step_member.html` (new)

### 13B.3 Settings: Change Admin Password

- [ ] Add "Security" section to settings page with current-password / new-password / confirm-new-password form
- [ ] New sqlc query: `UpdateAdminPassword(ctx, id, new_hashed_password)`
- [ ] Validate current password before accepting change
- [ ] Minimum 8 characters (same as initial setup)
- [ ] Flash: "Password updated successfully"
- [ ] This is the target for the "Change Password" link added in 13A.3
- **Files:** `internal/admin/settings.go`, `internal/templates/pages/settings.html`, `internal/db/queries/admin_accounts.sql`

### 13B.4 Settings: System Information Section

- [ ] Add collapsible "System" section at bottom of settings page
- [ ] Display: Breadbox version (from build-time `-ldflags -X`), Go runtime version (`runtime.Version()`), PostgreSQL version (`SELECT version()`), server uptime (`time.Since(startTime)`), configured providers count
- [ ] Read-only, informational — primarily for operator debugging
- [ ] Requires passing `startTime` from app init to the settings handler
- **Files:** `internal/admin/settings.go`, `internal/templates/pages/settings.html`

### 13B.5 Settings: Config Source Badges

- [ ] Add `ConfigSources map[string]string` (key → "env" / "db" / "default") populated during `LoadWithDB`
- [ ] Pass to settings template alongside `Config` values
- [ ] Render muted badge next to each setting: "(from env)", "(from database)", "(default)"
- [ ] Makes the config precedence model (env → DB → default) visible and debuggable
- **Files:** `internal/config/config.go`, `internal/config/load.go`, `internal/admin/settings.go`, `internal/templates/pages/settings.html`

### 13B.6 Settings: Teller Configuration Guidance

- [ ] "Not configured" state: add `<details>` block with copy-ready env var snippet and Docker Compose example
- [ ] When `teller_app_id` and `teller_env` are NOT set via env vars, make them editable in the settings form (they already have DB fallback paths in `LoadWithDB`)
- [ ] Cert/key paths remain read-only (env-var-only, file paths on host)
- [ ] Show current Teller status clearly: "Active (env vars)" / "Partially configured (app_id from DB, certs from env)" / "Not configured"
- **Files:** `internal/templates/pages/settings.html`, `internal/admin/settings.go`

### 13B.7 Settings: Safety & Status Indicators

- [ ] Confirmation dialog when changing `plaid_env` value (warn about breaking live connections)
- [ ] Encryption key status line: "Encryption: Configured" or "Encryption: NOT SET — access tokens cannot be stored" (never show the key itself)
- [ ] Use Alpine.js `x-on:submit` for the confirmation if available, vanilla `confirm()` as fallback
- **Files:** `internal/templates/pages/settings.html`

### 13B.8 Family Members: Connection Count & Post-Create CTA

- [ ] Add "Connections" column to members list table showing count of active connections per member
- [ ] New sqlc query: `CountConnectionsByUserID(ctx)` or use `LEFT JOIN` + `COUNT` in existing list query
- [ ] Zero-connection members show "0" (not blank) — makes it obvious who has no banks connected
- [ ] After creating a new member, flash message includes: "Connect a bank for {name} →" with link to `/admin/connections/new`
- **Files:** `internal/admin/members.go`, `internal/templates/pages/users.html`, `internal/db/queries/bank_connections.sql`

### Task Dependencies (13B)

```
13B.1 (multi-provider wizard) ─┐
13B.2 (family member step)     ┘ wizard group (independent of settings)

13B.3 (password change)     ─┐
13B.4 (system info)          │
13B.5 (config sources)       ├─ settings group (all independent of each other)
13B.6 (Teller guidance)      │
13B.7 (safety indicators)   ─┘

13B.8 (members connection count) — independent
```

### Checkpoint 13B

1. Wizard step 2 shows "Configure Bank Providers" with provider selection options
2. Select "Teller" → shows env-var guidance card (not Plaid form); select "Skip All" → proceeds to next step
3. New family member step appears after sync interval; skip works; created member appears in dropdown on connection page
4. Settings: "Change Password" section visible; change succeeds with correct current password; fails with wrong current password
5. Settings: "System" section shows version, Go version, PostgreSQL version, uptime
6. Settings: each setting shows "(from env)" or "(from database)" or "(default)" badge
7. Settings: Teller "not configured" has copy-ready env var snippet; `teller_app_id` is editable when not from env
8. Settings: changing Plaid environment triggers confirmation dialog; encryption key shows "Configured" status
9. Family Members: "Connections" column shows correct counts; new member flash has "Connect a bank →" link

---

## Phase 14: Deployment Readiness & Reliability

Independent of Phases 10-13. Addresses confirmed deployment blockers and data pipeline reliability gaps identified by PM audit.

### 14.1 README & Installation Guide

- [ ] Create `README.md` at project root (none currently exists)
- [ ] Project overview: what Breadbox does, who it's for, tech stack summary
- [ ] Prerequisites: Go 1.24+, PostgreSQL 16+, Docker (optional)
- [ ] Docker quickstart: `docker compose up` → setup wizard → first bank connection (5-minute path)
- [ ] Manual install: build from source, configure PostgreSQL, set env vars, run migrations, start server
- [ ] Configuration reference: all env vars with descriptions, defaults, and required/optional status
- [ ] First-run walkthrough: setup wizard steps, adding a family member, connecting a bank
- [ ] Link to `docs/` for detailed architecture and specs
- **Ref:** No existing README — `docs/architecture.md` sections 7-8 have some deployment info to consolidate
- **Files:** `README.md` (new)

### 14.2 ENCRYPTION_KEY Startup Validation

- [ ] Fail fast on `breadbox serve` if `ENCRYPTION_KEY` is empty/unset and Plaid or Teller providers are configured
- [ ] Currently: app starts successfully but crashes at runtime when `crypto.Encrypt` is called with nil key (e.g., when storing a Plaid access token after link)
- [ ] Check: if `cfg.EncryptionKey == nil` AND (`cfg.PlaidClientID != ""` OR `cfg.TellerAppID != ""`), return startup error
- [ ] Allow startup without key if only CSV provider is used (CSV doesn't encrypt anything)
- [ ] Clear error message: "ENCRYPTION_KEY is required when Plaid or Teller providers are configured. Generate one with: openssl rand -hex 32"
- **Ref:** `internal/config/load.go` lines 53-63 (validation exists for format but not presence)
- **Files:** `internal/config/load.go`, `cmd/breadbox/main.go`

### 14.3 Docker Compose Hardening

- [ ] Change `ports: "5432:5432"` to `expose: ["5432"]` so PostgreSQL is only reachable from other containers, not the host network
- [ ] Add warning comment in `.env.example` above `POSTGRES_PASSWORD`: "# IMPORTANT: Change this in production! Generate with: openssl rand -base64 32"
- [ ] Add `.env.example` comment about `sslmode=disable` being appropriate only for Docker internal networking
- [ ] Keep `ports` mapping for the app container (8080) since that needs host access
- **Ref:** `docker-compose.yml` line 26, `.env.example` lines 55-57
- **Files:** `docker-compose.yml`, `.env.example`

### 14.4 Deep Health Check

- [ ] Split into two endpoints: `GET /health/live` (basic, current behavior) and `GET /health/ready` (deep)
- [ ] `/health/live`: returns `{"status":"ok","version":"..."}` — same as current `/health`
- [ ] `/health/ready`: verifies DB connectivity (`pool.Ping`), checks scheduler is running, returns structured JSON: `{"status":"ok","db":"ok","scheduler":"running","version":"..."}`
- [ ] If DB ping fails: `{"status":"degraded","db":"error","db_error":"..."}`
- [ ] Keep `/health` as alias for `/health/live` (backwards compatible)
- [ ] Response time target: <100ms for `/health/ready`
- [ ] Docker Compose healthcheck should switch to `/health/ready`
- **Ref:** `internal/api/health.go` lines 14-22 (current basic check)
- **Files:** `internal/api/health.go`, `internal/api/router.go`, `docker-compose.yml`

### 14.5 Transactional Sync Writes

- [ ] Wrap the flush sequence in `engine.go` in a single DB transaction using `pool.Begin(ctx)`
- [ ] Current behavior: soft-deletes (lines 178-182), added transaction upserts (lines 187-201), and modified transaction upserts (lines 204-218) are individual SQL statements
- [ ] Risk: if upsert #50 of 100 fails, the first 49 are already committed — inconsistent state
- [ ] Use `pgx.Tx` to get a `*db.Queries` scoped to the transaction: `tx.Begin()` → `db.New(tx)` → flush all → `tx.Commit()`
- [ ] On error: `tx.Rollback()`, mark sync log as error
- [ ] Balance updates (if any) should also be inside the same transaction
- **Ref:** `internal/sync/engine.go` lines 176-218
- **Files:** `internal/sync/engine.go`

### 14.6 Orphaned Sync Log Cleanup

- [ ] On app startup (before scheduler starts), query for any sync logs with `status = 'in_progress'`
- [ ] Mark them as `status = 'error'` with `error_message = 'interrupted by server restart'` and `completed_at = NOW()`
- [ ] New sqlc query: `CleanupOrphanedSyncLogs(ctx)` — `UPDATE sync_logs SET status='error', error_message='...', completed_at=NOW() WHERE status='in_progress'`
- [ ] Log count of cleaned-up logs at INFO level on startup
- [ ] Currently: orphaned `in_progress` logs remain forever after a crash
- **Ref:** `internal/sync/engine.go` lines 52-60 (sync log creation)
- **Files:** `internal/sync/engine.go` or `internal/app/app.go`, `internal/db/queries/sync_logs.sql`

### 14.7 Per-Sync Timeout

- [ ] Add `SYNC_TIMEOUT_SECONDS` config (default: 300 = 5 minutes)
- [ ] Replace `context.Background()` in scheduler with `context.WithTimeout(ctx, syncTimeout)`
- [ ] Currently: `context.Background()` with no deadline — a hung provider API call blocks the goroutine indefinitely
- [ ] On timeout: sync engine marks sync log as error with "sync timed out after X seconds"
- [ ] Timeout applies per-connection (each connection sync gets its own deadline)
- **Ref:** `internal/sync/scheduler.go` lines 34-35
- **Files:** `internal/sync/scheduler.go`, `internal/sync/engine.go`, `internal/config/config.go`

### 14.8 Admin Password Reset CLI

- [ ] Add `breadbox reset-password` cobra subcommand
- [ ] Prompts for new password interactively (with confirmation), or accepts `--password` flag for scripted use
- [ ] Connects directly to DB (uses `DATABASE_URL` env var), updates the admin account's password hash
- [ ] Validates minimum 8 characters (same rule as setup wizard)
- [ ] Currently: only way to reset is direct SQL or deleting admin accounts and re-running setup
- [ ] Prints success message with admin username
- **Ref:** `cmd/breadbox/main.go` lines 33-71 (existing cobra commands)
- **Files:** `cmd/breadbox/main.go`, new file `cmd/breadbox/reset_password.go`

### 14.9 Configurable Log Level

- [ ] Add `LOG_LEVEL` env var: `debug`, `info`, `warn`, `error` (case-insensitive)
- [ ] Default: `info` for docker environment, `debug` for local/development
- [ ] `LOG_LEVEL` takes precedence over environment-based defaults when set
- [ ] Currently: log level is hardcoded — docker=info, everything else=debug (lines 73-81)
- [ ] Parse and validate on startup; warn if invalid value provided (fall back to default)
- **Ref:** `cmd/breadbox/main.go` lines 73-81
- **Files:** `cmd/breadbox/main.go`, `internal/config/config.go`

### 14.10 Startup Validation Summary

- [ ] Extend the existing boot banner (lines 141-158) to include:
  - Teller provider status: "configured (sandbox)" / "not configured"
  - ENCRYPTION_KEY: "configured" / "NOT SET"
  - Admin account: "exists" / "none (setup wizard will run)"
  - Setup status: "complete" / "pending"
- [ ] Warn (log at WARN level, don't fail) for non-critical gaps: missing Teller config, no admin account
- [ ] Fail (log at ERROR and exit) for critical gaps: missing ENCRYPTION_KEY when providers need it (see 14.2)
- [ ] Helps operators verify configuration at a glance after deployment
- **Ref:** `cmd/breadbox/main.go` lines 141-158 (existing banner)
- **Files:** `cmd/breadbox/main.go`

### 14.11 Backup & Restore Documentation

- [ ] Create `docs/backup.md` with:
  - `pg_dump` / `pg_restore` command examples for the Breadbox database
  - Docker volume backup approach (for Docker Compose deployments)
  - Cron-based automated backup script example
  - Restore verification steps (check row counts, test login, verify sync status)
  - Note about ENCRYPTION_KEY: must be preserved — without it, encrypted access tokens are unrecoverable
- [ ] Link from README (14.1)
- **Files:** `docs/backup.md` (new)

### Task Dependencies (Phase 14)

```
14.1 (README)          — independent, do first
14.2 (encryption key)  — independent
14.3 (docker hardening)— independent
14.4 (health check)    — independent
14.5 (tx sync writes)  ─┐
14.6 (orphaned logs)    ├─ sync engine group (14.5 first, provides tx pattern for 14.6)
14.7 (sync timeout)    ─┘
14.8 (password reset)  — independent
14.9 (log level)       — independent
14.10 (startup banner) — after 14.2 (uses same validation logic)
14.11 (backup docs)    — independent
```

### Checkpoint 14

1. `README.md` exists with Docker quickstart that works end-to-end
2. App refuses to start with `ENCRYPTION_KEY` unset when Plaid creds are configured
3. `docker-compose.yml` no longer exposes port 5432 to host
4. `GET /health/ready` returns DB and scheduler status; `GET /health/live` returns basic 200
5. Kill server mid-sync → restart → orphaned sync logs marked as error
6. Sync with a mock slow provider → times out after configured duration
7. `breadbox reset-password` successfully changes admin password
8. `LOG_LEVEL=warn` suppresses info/debug output
9. Startup banner shows Teller status and encryption key status
10. `docs/backup.md` has working pg_dump/pg_restore examples

---

## Phase 15: Agent-Optimized API

Benefits from Phase 14 reliability fixes but not blocked by them. Improves the REST API and MCP tools for AI agent consumption.

### 15.1 Account Name + User Name on Transactions

- [ ] Add `account_name` and `user_name` fields to `TransactionResponse` struct
- [ ] Currently only `account_id` is returned — agents must make separate `list_accounts` and `list_users` calls to resolve names
- [ ] Modify the dynamic SQL query builder to JOIN `accounts` and `users` tables
- [ ] `account_name`: `COALESCE(a.display_name, a.name)` (respects display_name override from Phase 10)
- [ ] `user_name`: `u.name` from `users` table via `accounts.user_id`
- [ ] Update both REST API response and MCP tool response (both use `TransactionResponse`)
- **Ref:** `internal/service/types.go` lines 24-42, `internal/service/transactions.go`
- **Files:** `internal/service/types.go`, `internal/service/transactions.go`

### 15.2 Category_Detailed Filter

- [ ] Add `CategoryDetailed *string` to `TransactionListParams`
- [ ] Wire into dynamic SQL WHERE clause: `AND t.personal_finance_category_detailed = $N`
- [ ] Currently only `category_primary` is filterable (lines 65-69)
- [ ] Add to both REST API query params and MCP `query_transactions` input struct
- [ ] Also add to `count_transactions` filter for consistency
- **Ref:** `internal/service/transactions.go` lines 65-69, `internal/mcp/tools.go`
- **Files:** `internal/service/transactions.go`, `internal/api/transactions.go`, `internal/mcp/tools.go`

### 15.3 List Categories Endpoint

- [ ] New endpoint: `GET /api/v1/categories` returning distinct `(category_primary, category_detailed)` pairs from transactions table
- [ ] Query: `SELECT DISTINCT personal_finance_category_primary, personal_finance_category_detailed FROM transactions WHERE deleted_at IS NULL ORDER BY 1, 2`
- [ ] Response: `{"categories": [{"primary": "FOOD_AND_DRINK", "detailed": "RESTAURANTS"}, ...]}`
- [ ] New MCP tool: `list_categories` — returns same data, helps agents discover valid filter values
- [ ] Useful for agents to know what categories exist before filtering
- **Ref:** `internal/api/router.go` lines 30-41 (no category endpoint exists)
- **Files:** `internal/api/router.go`, new handler in `internal/api/categories.go`, `internal/service/transactions.go`, `internal/mcp/tools.go`

### 15.4 Enrich MCP Tool Descriptions

- [ ] Rewrite all 6 MCP tool descriptions with domain-specific context
- [ ] Current descriptions are generic (e.g., "List all bank accounts. Optionally filter by user_id.")
- [ ] New descriptions should explain:
  - What the data represents (bank accounts synced from Plaid/Teller/CSV)
  - Available filters and their formats (dates: YYYY-MM-DD, amounts: positive=debit)
  - Pagination behavior (cursor-based, default limit)
  - Amount sign convention (positive = money out / debit, negative = money in / credit)
- [ ] Keep descriptions concise but informative — agents read these to decide which tool to use
- **Ref:** `internal/mcp/tools.go` lines 17-42
- **Files:** `internal/mcp/tools.go`

### 15.5 Fix Min/Max Amount Zero-Value Bug

- [ ] Change `MinAmount` and `MaxAmount` in MCP `queryTransactionsInput` from `float64` to `*float64`
- [ ] Replace `if input.MinAmount != 0` with `if input.MinAmount != nil` (nil-check)
- [ ] Currently: passing `min_amount: 0` is silently ignored (treated as "not set") because `float64` zero-value is 0.0
- [ ] REST API handler already does this correctly with `*float64` — match that pattern
- [ ] Same fix for `countTransactionsInput`
- **Ref:** `internal/mcp/tools.go` lines 58-59 (struct), lines 129-133 (handler)
- **Files:** `internal/mcp/tools.go`

### 15.6 Teller Category_Detailed Mapping

- [ ] Map Teller's `Details.Category` to both `CategoryPrimary` (existing) and `CategoryDetailed`
- [ ] Currently: `CategoryDetailed` is always nil for Teller transactions (only primary is mapped, lines 180-183)
- [ ] Teller categories are single-level (e.g., "accommodation", "advertising", "food_and_drink")
- [ ] Strategy: map to Plaid-compatible primary categories; for detailed, use Teller category as-is when it provides sub-level granularity, or leave null when it doesn't
- [ ] Document the mapping table in code comments
- **Ref:** `internal/provider/teller/sync.go` lines 154-192 (`mapTellerTransaction`)
- **Files:** `internal/provider/teller/sync.go`

### 15.7 Transaction Sort Options

- [ ] Add `sort_by` param: `date` (default), `amount`, `name`
- [ ] Add `sort_order` param: `desc` (default), `asc`
- [ ] Currently hardcoded: `ORDER BY t.date DESC, t.id DESC` (line 109)
- [ ] Validate `sort_by` against allowlist to prevent SQL injection
- [ ] Wire into both REST API query params and MCP tool input struct
- [ ] Keep `t.id` as tiebreaker for stable cursor pagination
- **Ref:** `internal/service/transactions.go` line 109
- **Files:** `internal/service/transactions.go`, `internal/api/transactions.go`, `internal/mcp/tools.go`

### 15.8 Enrich MCP Server Instructions

- [ ] Replace generic instructions string with domain-rich overview
- [ ] Current: "Breadbox is a financial data aggregation server. Use the available tools to query accounts, transactions, users, and sync status."
- [ ] New: explain Breadbox data model (connections → accounts → transactions), available tools summary, amount convention (positive=debit), recommended query patterns (list accounts first, then filter transactions)
- [ ] Instructions are shown to AI agents when they connect — this is the "onboarding" text
- **Ref:** `internal/mcp/server.go` line 25
- **Files:** `internal/mcp/server.go`

### 15.9 MCP Overview Resource

- [ ] Add a static MCP resource at URI `breadbox://overview`
- [ ] Returns: data model summary, number of accounts/connections/users, date range of transactions, list of available categories
- [ ] Gives agents useful context before they start querying — reduces round-trips
- [ ] Uses service layer to fetch live stats (account count, transaction date range, etc.)
- [ ] Register via `server.AddResource` in MCP server setup
- **Ref:** `internal/mcp/server.go` (no resources currently registered)
- **Files:** `internal/mcp/server.go`, `internal/mcp/resources.go` (new)

### Task Dependencies (Phase 15)

```
15.1 (account/user names) — independent, high-impact, do first
15.2 (category_detailed)  ─┐
15.3 (list categories)     ├─ category group
15.6 (Teller mapping)     ─┘
15.4 (tool descriptions)  ─┐
15.5 (min/max bug fix)     ├─ MCP quality group
15.8 (MCP instructions)    │
15.9 (MCP overview)       ─┘
15.7 (sort options)        — independent
```

### Checkpoint 15

1. `GET /api/v1/transactions` responses include `account_name` and `user_name` fields
2. MCP `query_transactions` tool accepts `category_detailed` filter and returns results
3. `GET /api/v1/categories` returns distinct category pairs; `list_categories` MCP tool works
4. MCP tool descriptions explain domain concepts, filters, and conventions
5. MCP `query_transactions` with `min_amount: 0` correctly filters (not ignored)
6. Teller transactions have `category_detailed` populated where possible
7. `sort_by=amount&sort_order=asc` returns transactions sorted by amount ascending
8. MCP `Instructions` field provides useful onboarding context for AI agents
9. `breadbox://overview` resource returns data model summary with live stats

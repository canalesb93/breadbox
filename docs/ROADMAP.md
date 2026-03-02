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

---

## Phase 10: Enhanced Settings & Connection Management

Per-account controls, connection pausing, per-connection sync intervals, and provider credential testing.

**Depends on:** Phases 8–9 (generic columns, both providers functional)

### 10.1 Migration: Account Settings

- [ ] Add `display_name TEXT NULL` and `excluded BOOLEAN NOT NULL DEFAULT FALSE` to `accounts`
- [ ] `display_name NULL` means "use bank name" — templates use `COALESCE(display_name, name)`
- [ ] `excluded` only affects transaction upserts (balances still refresh for reporting)
- **Ref:** `data-model.md` Section 2.4
- **Files:** `internal/db/migrations/00014_account_settings.sql`

### 10.2 Migration: Connection Pause & Interval

- [ ] Add `paused BOOLEAN NOT NULL DEFAULT FALSE` to `bank_connections`
- [ ] Add `sync_interval_override_minutes INTEGER NULL` to `bank_connections`
- [ ] `paused` is orthogonal to `status` — a connection can be `error` + `paused`
- [ ] Manual "Sync Now" bypasses pause (only cron respects it)
- **Ref:** `data-model.md` Section 2.3
- **Files:** `internal/db/migrations/00015_connection_pause.sql`

### 10.3 sqlc Queries

- [ ] `UpdateAccountDisplayName(ctx, id, display_name)` — nullable text
- [ ] `UpdateAccountExcluded(ctx, id, excluded)` — boolean
- [ ] `ListExcludedAccountIDsByConnection(ctx, connection_id)` — returns UUIDs
- [ ] `UpdateConnectionPaused(ctx, id, paused)` — boolean
- [ ] `UpdateConnectionSyncInterval(ctx, id, override_minutes)` — nullable int
- [ ] `ListActiveUnpausedConnections(ctx)` — WHERE status='active' AND paused=false
- [ ] Update existing account/connection queries to include new columns in SELECT
- **Files:** `internal/db/queries/accounts.sql`, `internal/db/queries/bank_connections.sql`

### 10.4 Sync Engine: Excluded Account Filtering

- [ ] Before upserting transactions, fetch excluded account IDs for the connection
- [ ] Skip transactions whose account is in the excluded set
- [ ] Log skipped count at debug level
- **Ref:** `architecture.md` Section 3
- **Files:** `internal/sync/engine.go`

### 10.5 Scheduler: Pause & Per-Connection Intervals

- [ ] Replace `ListActiveConnections` with `ListActiveUnpausedConnections` for cron
- [ ] Cron fires at the minimum interval (e.g., every 15 minutes)
- [ ] For each connection: compute effective interval = `COALESCE(sync_interval_override_minutes, global_interval)`
- [ ] Skip if `last_synced_at + effective_interval > now`
- [ ] Startup sync also respects pause and per-connection intervals
- **Files:** `internal/sync/scheduler.go`

### 10.6 Admin Handlers: Account Settings

- [ ] `POST /admin/api/accounts/{id}/excluded` — toggle `excluded` (JSON body: `{"excluded": true}`)
- [ ] `POST /admin/api/accounts/{id}/display-name` — set display name (JSON body: `{"display_name": "My Checking"}`)
- [ ] Both return updated account as JSON
- **Files:** `internal/admin/connections.go`

### 10.7 Admin Handlers: Connection Pause & Interval

- [ ] `POST /admin/api/connections/{id}/paused` — toggle pause (JSON body: `{"paused": true}`)
- [ ] `POST /admin/api/connections/{id}/sync-interval` — set override (JSON body: `{"minutes": 30}`, null to clear)
- [ ] Both return updated connection as JSON
- **Files:** `internal/admin/connections.go`

### 10.8 Templates: Account & Connection Controls

- [ ] Connection detail page: account rows with exclude toggle and display name inline edit
- [ ] Connection detail page: pause/resume button, per-connection interval dropdown (15m, 30m, 1h, 2h, 4h, 12h, 24h, "Use global")
- [ ] Connections list: "Paused" badge next to connection name when paused
- [ ] All controls use `fetch()` POST calls (no full page reload)
- **Files:** `internal/templates/pages/connection_detail.html`, `internal/templates/pages/connections.html`

### 10.9 Settings: Test Connection Button

- [ ] "Test Connection" button per configured provider on settings page
- [ ] Plaid: call existing `ValidateCredentials` (API handshake)
- [ ] Teller: attempt mTLS handshake to `https://api.teller.io/health` (or similar)
- [ ] Display result inline: "Connection successful" or error message
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

## Phase 11: CSV Import Provider

Upload bank CSV exports, map columns, and import transactions with hash-based deduplication.

**Depends on:** Phase 8 (generic columns, `csv` in provider_type enum)

### 11.1 CSV Parser

- [ ] Read file into memory (max 10MB)
- [ ] Auto-detect delimiter: try comma, tab, semicolon, pipe — pick the one that produces consistent column counts
- [ ] Strip BOM (UTF-8 `\xEF\xBB\xBF`, UTF-16 LE/BE)
- [ ] Return headers (first row) and data rows
- [ ] Reject files with < 2 rows or > 50,000 rows
- [ ] 10,000 row limit for preview (return first 10 rows to UI, full data for import)
- **Ref:** `csv-import.md` Sections 2, 3
- **Files:** `internal/provider/csv/parser.go` (new)

### 11.2 Column Mapping & Templates

- [ ] Pre-built templates: Chase (credit + checking), Bank of America, Wells Fargo, Capital One, Amex
- [ ] Each template: header patterns, column mappings, sign convention, date format hint
- [ ] Auto-detect: compare parsed headers against all templates, return best match
- [ ] Sign convention toggle: "Positive = debit" (default) vs "Positive = credit"
- **Ref:** `csv-import.md` Sections 3, 4
- **Files:** `internal/provider/csv/templates.go` (new)

### 11.3 Import Logic

- [ ] Apply column mapping to each row → (date, amount, description, category?, merchant?)
- [ ] Parse dates using auto-detection strategy (try formats against first 20 values, pick best)
- [ ] Parse amounts: strip currency symbols, handle commas, parenthetical negatives, split debit/credit columns
- [ ] Normalize sign per selected convention (positive = debit in storage)
- [ ] Generate `external_transaction_id = SHA-256(account_id|date|amount|description)`
- [ ] Return list of parsed transactions + list of skipped rows with reasons
- **Ref:** `csv-import.md` Sections 5, 6, 7
- **Files:** `internal/provider/csv/import.go` (new)

### 11.4 CSV Provider Stub

- [ ] Implement `Provider` interface — all methods return `provider.ErrNotSupported` except `RemoveConnection` (returns nil)
- [ ] Add `var _ provider.Provider = (*CSVProvider)(nil)` compile-time check
- [ ] Constructor: `NewProvider(logger)` (no config needed)
- **Ref:** `csv-import.md` Section 9
- **Files:** `internal/provider/csv/provider.go` (new)

### 11.5 Import Service

- [ ] `ImportCSV(ctx, params)` — orchestrates the full import flow
- [ ] Create or reuse CSV connection + account for the selected member
- [ ] Call import logic to parse rows
- [ ] Upsert transactions via existing `UpsertTransaction` (reuses ON CONFLICT)
- [ ] Create `sync_logs` entry with `trigger = manual`, `provider = csv`
- [ ] Return import result: total, inserted, updated, skipped counts
- **Ref:** `csv-import.md` Sections 7, 8
- **Files:** `internal/service/csv.go` (new)

### 11.6 Admin Handlers

- [ ] `GET /admin/connections/import-csv` — render import wizard page
- [ ] `POST /admin/api/csv/upload` — multipart upload (10MB limit), parse file, return headers + preview rows + auto-detected template
- [ ] `POST /admin/api/csv/preview` — apply column mapping to uploaded data, return first 10 parsed rows with validation
- [ ] `POST /admin/api/csv/import` — execute full import with confirmed mapping, return results
- [ ] Upload stored in memory (not on disk) for the duration of the wizard session
- **Files:** `internal/admin/csv_import.go` (new)

### 11.7 Template: Import Wizard

- [ ] Multi-step wizard UI (fetch-driven, no full page reloads):
  - Step 1: Select family member + file upload
  - Step 2: Column mapping dropdowns + sign toggle + template auto-select + preview table
  - Step 3: Confirm summary (row count, date range, account name)
  - Step 4: Results (counts + link to connection detail)
- [ ] Use wizard layout consistent with setup wizard
- **Ref:** `csv-import.md` Section 2
- **Files:** `internal/templates/pages/csv_import.html` (new)

### 11.8 Re-Import: Connection Detail Integration

- [ ] CSV connections show "Import More" button on connection detail page
- [ ] Button links to `/admin/connections/import-csv?connection_id={id}`
- [ ] Wizard pre-fills member and account name from existing connection
- **Ref:** `csv-import.md` Section 8
- **Files:** `internal/templates/pages/connection_detail.html`

### 11.9 App Init & Spec Doc

- [ ] Register CSV provider in `app.New()` — always available (no config/credentials needed)
- [ ] Log "csv provider registered" at startup
- [ ] Verify `docs/csv-import.md` spec is complete and consistent with implementation
- **Files:** `internal/app/app.go`, `docs/csv-import.md`

### Task Dependencies

```
11.1 (parser) ──┐
                 ├──> 11.3 (import logic) ──> 11.5 (service) ──> 11.6 (handlers) ──> 11.7 (wizard template)
11.2 (templates)┘                                                                ──> 11.8 (re-import)

11.4 (provider stub) ──> 11.9 (app init) — independent of import flow
```

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

## Phase 12A: Admin UI Foundation

Modernize the admin template system, add Alpine.js interactivity, and prepare for new pages.

**Depends on:** None (independent of all other phases, can be done in parallel)

### 12A.1 Pico CSS: Classless → Class-Based

- [ ] Switch from `pico.classless.min.css` to `pico.min.css` (class-based variant)
- [ ] Add required Pico classes to elements: `<table class="striped">`, `<button class="secondary">`, `<article>`, etc.
- [ ] Update both `base.html` and `wizard.html` layouts
- [ ] Verify all existing pages render correctly after switch
- **Files:** `internal/templates/layout/base.html`, `internal/templates/layout/wizard.html`

### 12A.2 Add Alpine.js

- [ ] Add Alpine.js v3 via CDN `<script>` tag in base layout
- [ ] Replace all `alert()` / `confirm()` calls with inline Alpine patterns (e.g., `x-data="{ confirming: false }"`)
- [ ] Target: connection delete confirm, API key revoke confirm, sync trigger confirm
- **Files:** `internal/templates/layout/base.html`, `internal/templates/pages/connection_detail.html`, `internal/templates/pages/api_keys.html`

### 12A.3 Dark Mode

- [ ] Remove `data-theme="light"` from `<html>` tag (lets Pico respect `prefers-color-scheme`)
- [ ] Replace all hardcoded hex colors in badge/flash styles with Pico CSS custom properties (`--pico-color-green-500`, `--pico-color-red-500`, etc.)
- [ ] Test both light and dark themes render correctly
- **Depends on:** 12A.1 (needs class-based Pico)
- **Files:** `internal/templates/layout/base.html`, `internal/templates/layout/wizard.html`

### 12A.4 Badge Template Functions

- [ ] Add `statusBadge(status string)` template function — returns HTML for connection status badges (`active`=green, `error`=red, `pending_reauth`=yellow, `disconnected`=gray)
- [ ] Add `syncBadge(status string)` template function — returns HTML for sync status badges (`success`=green, `error`=red, `in_progress`=blue)
- [ ] Replace 4+ copy-pasted if-chains across templates with function calls
- [ ] Badge colors use CSS custom properties (dark-mode compatible)
- **Files:** `internal/admin/templates.go`, all pages with badges

### 12A.5 Common Template Data Helper

- [ ] Create `BaseTemplateData(r *http.Request, sm *scs.SessionManager, currentPage string)` helper
- [ ] Auto-injects: `CSRFToken`, `Flash` messages, `CurrentPage` (for nav highlighting), `PageTitle`
- [ ] Reduce boilerplate in every handler (currently each handler manually assembles these fields)
- **Files:** `internal/admin/templates.go`

### 12A.6 Navigation Restructure

- [ ] Group nav items into two sections:
  - **Data:** Dashboard, Connections, Members, Transactions
  - **System:** API Keys, Sync Logs, Settings
- [ ] Add visual divider between sections
- [ ] Alpine-powered hamburger menu for mobile (collapses on small screens)
- [ ] Current page highlighting via `CurrentPage` from 12A.5
- **Depends on:** 12A.2 (needs Alpine.js for hamburger)
- **Files:** `internal/templates/partials/nav.html`

### 12A.7 CSS Spacing Tokens

- [ ] Define custom properties: `--bb-gap-xs` (0.25rem), `--bb-gap-sm` (0.5rem), `--bb-gap-md` (1rem), `--bb-gap-lg` (1.5rem), `--bb-gap-xl` (2rem)
- [ ] Replace inline `style="margin-top: 1rem"` etc. with utility classes or token references
- [ ] Add to base layout `<style>` block
- **Files:** `internal/templates/layout/base.html`

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

## Phase 12B: Admin Transaction Pages

Transaction list, account detail, and cross-linking throughout the admin UI.

**Depends on:** Phase 12A (UI foundation), existing service layer

### 12B.1 Service: Admin Transaction List

- [ ] `ListTransactionsPagedAdmin(ctx, params)` — offset-based pagination (consistent with sync logs page)
- [ ] JOIN account name, connection institution name, user name for display
- [ ] Support all 10 filters: date range, account_id, user_id, category, amount min/max, pending, text search, connection_id, sort order
- [ ] Return total count for pagination controls
- **Ref:** `rest-api.md` Section 5.3 (filter spec), `admin-dashboard.md` Section 11 (pagination pattern)
- **Files:** `internal/service/transactions.go`

### 12B.2 Handler: Transaction List Page

- [ ] `GET /admin/transactions` — parse filter query params from URL
- [ ] Load dropdown data: accounts (with connection/user context), users, distinct categories
- [ ] Render page with filters applied, preserve filter state in form
- **Files:** `internal/admin/transactions.go` (new)

### 12B.3 Template: Transaction List

- [ ] Filter form: date range (start/end date inputs), account dropdown, user dropdown, category dropdown, amount range (min/max), pending toggle, text search input
- [ ] Table columns: Date, Description, Amount, Account, Category, Status (pending/posted)
- [ ] Alpine expandable row: click row to see full transaction detail (merchant, external ID, timestamps)
- [ ] Offset-based pagination (page numbers, prev/next) consistent with sync logs
- [ ] Amount formatting: color-coded (green for credits, default for debits), currency symbol
- **Files:** `internal/templates/pages/transactions.html` (new)

### 12B.4 Service: Account Detail

- [ ] `GetAccountDetail(ctx, id)` — extends `AccountResponse` with connection institution name, provider, user name
- [ ] Returns account info + pre-filtered transaction params for the template
- **Files:** `internal/service/accounts.go`

### 12B.5 Handler: Account Detail Page

- [ ] `GET /admin/accounts/{id}` — load account detail + filtered transaction list
- [ ] Reuses transaction list logic from 12B.1 with `account_id` pre-set
- **Files:** `internal/admin/transactions.go`

### 12B.6 Template: Account Detail

- [ ] Info card: account name (display_name or bank name), type, subtype, mask, current/available balance, last balance update, connection link, user name
- [ ] Transaction table: pre-filtered by account, same columns/pagination as 12B.3
- [ ] "Edit" controls for display name and excluded status (from Phase 10, if available)
- **Files:** `internal/templates/pages/account_detail.html` (new)

### 12B.7 Routes & Cross-Links

- [ ] Register routes: `GET /admin/transactions`, `GET /admin/accounts/{id}`
- [ ] Add "Transactions" link to nav (Data section, after Connections)
- [ ] Connection detail: account names link to `/admin/accounts/{id}`
- [ ] Transaction list: account names link to account detail
- [ ] Dashboard: add transaction count and "View All" link
- **Files:** `internal/admin/router.go`, `internal/templates/pages/connection_detail.html`, `internal/templates/partials/nav.html`

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

### 13A.1 Bug Fix: setup_complete Written on GET

- [ ] Move `setup_complete = true` from `SetupStep5Handler` GET to a dedicated POST handler
- [ ] Step 5 GET only renders the summary page; a "Confirm & Finish" button POSTs to finalize
- [ ] Prevents accidental wizard completion from page reloads or direct URL navigation
- **Bug location:** `setup.go` lines 312–321 — unconditionally writes on every GET request
- **Files:** `internal/admin/setup.go`

### 13A.2 Bug Fix: Programmatic Setup Skips Plaid Validation

- [ ] Add `plaidprovider.ValidateCredentials(ctx, clientID, secret, environment)` call in `ProgrammaticSetupHandler` when both Plaid creds are provided
- [ ] Match the validation behavior of the interactive `SetupStep2Handler` (which already validates)
- [ ] Return validation error in the API response if credentials fail
- **Bug location:** `setup.go` lines 437–450 — saves credentials without testing them
- **Files:** `internal/admin/setup.go`

### 13A.3 Bug Fix: Broken "Re-run Setup Wizard" Link

- [ ] Remove `<a href="/admin/setup/step/1">Re-run Setup Wizard</a>` from settings page
- [ ] Replace with "Change Admin Password" link (target implemented in 13B.3)
- [ ] Until 13B.3 is done, replace with explanatory text: "To reconfigure providers, update settings below"
- **Bug location:** `settings.html` line 76 links to step 1, but `SetupStep1Handler` (setup.go lines 22–27) redirects away if any admin account exists
- **Files:** `internal/templates/pages/settings.html`

### 13A.4 Fix: Sync Interval Unit Mismatch

- [ ] Update wizard step 3 to write `sync_interval_minutes` instead of `sync_interval_hours`
- [ ] Offer the same option set as the settings page: 15m, 30m, 1h, 4h, 8h, 12h, 24h
- [ ] Keep legacy `sync_interval_hours` fallback in config loader (`load.go` lines 137–154) for backwards compatibility
- [ ] Update step 3 template to show minute-based options
- **Bug location:** `setup.go` lines 225–243 writes `sync_interval_hours`; `settings.go` lines 47–68 writes `sync_interval_minutes`
- **Files:** `internal/admin/setup.go`, `internal/templates/pages/setup_step3.html`

### 13A.5 Dashboard: Clickable Stats & Alert Banner

- [ ] Wrap "Needs Attention" stat card in `<a href="/admin/connections">` when count > 0
- [ ] Add broken-connections alert banner (`<div role="alert">`) above stat cards when any connection is in `error` or `pending_reauth` status
- [ ] Banner text: "{N} connection(s) need attention" with link to connections page
- [ ] Pass `BrokenCount` from dashboard handler to template
- **Files:** `internal/templates/pages/dashboard.html`, `internal/admin/dashboard.go`

### 13A.6 Dashboard: Institution Name Links in Sync Activity

- [ ] Make institution names in the Recent Sync Activity table link to `/admin/connections/{id}`
- [ ] Add `ConnectionID` field to the recent logs struct/query result
- [ ] Template: wrap `{{.InstitutionName}}` in `<a href="/admin/connections/{{.ConnectionID}}">`
- **Current state:** `dashboard.html` line 42 — `{{.InstitutionName}}` is plain text
- **Files:** `internal/admin/dashboard.go`, `internal/templates/pages/dashboard.html`, query/service layer for sync logs

### 13A.7 Human-Readable Error Messages

- [ ] Add `errorMessage(code string) string` template function in `templates.go`
- [ ] Map known error codes to user-friendly messages:
  - `ITEM_LOGIN_REQUIRED` → "Your bank login has changed. Please re-authenticate."
  - `INSUFFICIENT_CREDENTIALS` → "Additional credentials are needed. Please re-authenticate."
  - `INVALID_CREDENTIALS` → "Your bank credentials are incorrect. Please re-authenticate."
  - `MFA_NOT_SUPPORTED` → "This connection requires MFA which is not supported. Please reconnect."
  - `NO_ACCOUNTS` → "No accounts found for this connection."
  - `enrollment.disconnected` (Teller) → "This bank connection has been disconnected."
  - Unknown codes → show raw message as fallback
- [ ] Use in connection detail error display (lines 7, 105)
- **Files:** `internal/admin/templates.go`, `internal/templates/pages/connection_detail.html`

### 13A.8 Connection Detail: Breadcrumb Navigation

- [ ] Replace `← Connections` back-link with semantic breadcrumb: `Connections / {institution name}`
- [ ] Use `<nav aria-label="breadcrumb">` with two-element structure
- [ ] Apply same pattern to reauth page: `Connections / {institution name} / Re-authenticate`
- [ ] Phase 12B can extend to three levels: `Connections / {institution} / {account name}`
- **Current state:** `connection_detail.html` line 2 — `<a href="/admin/connections">← Connections</a>`
- **Files:** `internal/templates/pages/connection_detail.html`, `internal/templates/pages/connection_reauth.html`

### 13A.9 Wizard Step 5: Provider Status & CTA

- [ ] Add provider configuration status to step 5 summary:
  - "Plaid: Configured ✓" / "Plaid: Not configured"
  - "Teller: Configured ✓" / "Teller: Not configured (set env vars)"
  - "CSV Import: Always available"
- [ ] Add prominent CTA button: "Connect Your First Bank →" linking to `/admin/connections/new`
- [ ] If no providers are configured, show warning: "No bank data provider configured. Go to Settings to add one."
- [ ] Pass provider availability from `app.Providers` map to template data
- **Files:** `internal/admin/setup.go` (step 5 data), `internal/templates/pages/setup_step5.html`

### 13A.10 Wizard Step 4: Reframe Webhook as Optional

- [ ] Lead with: "Webhooks are optional. Without them, Breadbox will sync on its configured schedule."
- [ ] Two clear paths: "I have a public URL" (shows the URL form) vs "Skip — I'll set this up later"
- [ ] Make Cloudflare Tunnel documentation link more prominent for the local/self-hosted case
- [ ] Clarify that the URL entered here is what Breadbox listens at — the user must also configure it in their Plaid/Teller dashboard
- **Files:** `internal/templates/pages/setup_step4.html`

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

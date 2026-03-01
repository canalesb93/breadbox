# Breadbox — System Architecture and Deployment Specification

**Version:** MVP
**Last Updated:** 2026-03-01

---

## Table of Contents

1. [System Architecture](#1-system-architecture)
2. [Project Directory Structure](#2-project-directory-structure)
3. [Provider Interface](#3-provider-interface)
4. [Configuration](#4-configuration)
5. [Security](#5-security)
6. [Logging](#6-logging)
7. [Docker Deployment](#7-docker-deployment)
8. [Development Setup](#8-development-setup)
9. [Error Handling Patterns](#9-error-handling-patterns)
10. [Testing Strategy](#10-testing-strategy)

---

## 1. System Architecture

### 1.1 Binary and Subcommands

Breadbox compiles to a single Go binary. All runtime components are started by
a single subcommand. The binary exposes five subcommands:

```
breadbox serve        Start HTTP server with all components
breadbox mcp-stdio    Start MCP server on stdio (local agent dev only)
breadbox migrate      Run pending database migrations
breadbox seed         Insert sandbox test data (development only)
breadbox version      Print build version and exit
```

`breadbox serve` is the production entry point. It initializes every component
— HTTP router, sync engine, cron scheduler, webhook handler — and runs them
in the same process. There is no microservice split at MVP.

`breadbox mcp-stdio` is a convenience mode for local agent development. It
starts only the MCP server and reads/writes the MCP protocol on stdin/stdout.
It shares the same database and configuration as `breadbox serve` but does not
bind an HTTP port.

### 1.2 Component Topology

```
                         ┌──────────────────────────────────────────────────┐
                         │                breadbox serve                    │
                         │                                                  │
   HTTP :8080            │   ┌──────────────────────────────────────────┐   │
   ─────────────────────►│   │              chi Router                  │   │
                         │   │                                          │   │
                         │   │  /api/v1/*     REST API handlers         │   │
                         │   │  /mcp          MCP Streamable HTTP       │   │
                         │   │  /admin/*      Admin dashboard handlers  │   │
                         │   │  /webhooks/*   Webhook handlers          │   │
                         │   │  /health       Health check (no auth)    │   │
                         │   └──────────────────────┬───────────────────┘   │
                         │                          │                       │
                         │   ┌──────────────────────▼───────────────────┐   │
                         │   │              App (dependency container)   │   │
                         │   │                                          │   │
                         │   │  Config      Logger      pgx Pool        │   │
                         │   │  Provider    Queries     CronScheduler   │   │
                         │   └──────────────────────────────────────────┘   │
                         │                          │                       │
                         │   ┌──────────────────────▼───────────────────┐   │
                         │   │            Background Workers             │   │
                         │   │                                          │   │
                         │   │  Sync Engine goroutines (per connection)  │   │
                         │   │  Cron Scheduler (robfig/cron)            │   │
                         │   └──────────────────────────────────────────┘   │
                         └──────────────────────────────────────────────────┘
                                                    │
                                    ┌───────────────▼────────────────┐
                                    │          PostgreSQL             │
                                    │   (pgx/v5 connection pool)     │
                                    └────────────────────────────────┘
```

### 1.3 Shared Resources

All components receive shared resources through the `App` struct
(see `internal/app/`). This struct is the single dependency container for the
process. It is constructed once at startup and passed to every handler and
worker.

| Resource | Type | Shared By |
|---|---|---|
| Database pool | `*pgxpool.Pool` | All handlers, sync engine, cron jobs |
| Queries | `*db.Queries` (sqlc generated) | All handlers, sync engine |
| Config | `*config.Config` | All components |
| Logger | `*slog.Logger` | All components |
| Provider | `provider.Provider` | Sync engine, webhook handler, admin handlers |
| Cron Scheduler | `*cron.Cron` | Sync engine setup; stopped on shutdown |

### 1.4 Request Lifecycle

A request entering the HTTP server follows this path:

```
HTTP Request
    │
    ▼
chi Router (path + method dispatch)
    │
    ▼
Middleware Chain
    │  - Request ID injection
    │  - Structured request logging (method, path, status, duration)
    │  - Auth check (API key OR session cookie, depending on route group)
    │  - CSRF check (admin POST routes)
    │
    ▼
Handler (internal/api/, internal/admin/, internal/webhook/)
    │  - Parse and validate request parameters
    │  - Call one or more service functions
    │  - Write JSON or HTML response
    │
    ▼
Service / Business Logic
    │  - Enforce domain rules
    │  - Orchestrate multiple queries or provider calls
    │  - Return typed results or typed errors
    │
    ▼
Repository / sqlc Queries (internal/db/queries.sql.go)
    │  - Execute parameterized SQL via pgx
    │  - Return typed row structs
    │
    ▼
PostgreSQL
```

The MCP server wraps REST API service functions directly — it does not make
internal HTTP calls. MCP tools call the same service layer that REST handlers
call.

### 1.5 Background Goroutines

Two background subsystems run for the lifetime of `breadbox serve`:

**Sync Engine Workers**

When a sync is triggered (by cron or webhook), the sync engine spawns a
goroutine per connection. Each goroutine:

1. Acquires a database connection from the pool.
2. Calls `provider.SyncTransactions` in a loop until the cursor is exhausted.
3. Upserts added/modified transactions and soft-deletes removed ones.
4. Writes a sync log entry with counts and status.
5. Releases the database connection.

The number of concurrent sync workers is bounded to prevent pool exhaustion.

**Cron Scheduler**

`robfig/cron/v3` runs a scheduled job at the interval stored in `app_config`
(default 12 hours). The job enqueues a sync for every active connection. The
scheduler is started after the HTTP server is ready.

### 1.6 Graceful Shutdown

On `SIGINT` or `SIGTERM`, the process performs an ordered shutdown:

```
1. Stop accepting new HTTP connections (http.Server.Shutdown with deadline)
2. Stop the cron scheduler (cron.Stop — waits for running jobs to complete)
3. Cancel the root context (signals all sync goroutines to stop)
4. Wait for all sync goroutines to finish (sync.WaitGroup)
5. Close the pgx connection pool
6. Exit
```

The shutdown deadline is 30 seconds. Any component that exceeds the deadline
is forcibly terminated.

---

## 2. Project Directory Structure

The project follows Go standard project layout conventions. Private application
code lives under `internal/` and is not importable by external packages.

```
breadbox/
├── cmd/
│   └── breadbox/
│       └── main.go              Entry point; parses subcommands, builds App, runs
│
├── internal/
│   ├── app/
│   │   └── app.go               App struct (dependency container); constructor wires
│   │                            all shared resources and returns a ready-to-run App
│   │
│   ├── config/
│   │   ├── config.go            Config struct definition
│   │   └── load.go              Loads env vars; merges with app_config table values
│   │
│   ├── provider/
│   │   ├── provider.go          Provider interface definition + shared types
│   │   │                        (Connection, AccountBalance, WebhookEvent, etc.)
│   │   └── plaid/
│   │       ├── client.go        Plaid SDK wrapper; holds *plaid.APIClient
│   │       ├── link.go          CreateLinkSession, ExchangeToken, CreateReauthSession
│   │       ├── sync.go          SyncTransactions (cursor loop, pagination retry)
│   │       ├── balances.go      GetBalances (/accounts/get)
│   │       ├── webhook.go       HandleWebhook (parse + verify Plaid webhook payload)
│   │       ├── remove.go        RemoveConnection (/item/remove)
│   │       └── encrypt.go       AES-256-GCM encrypt/decrypt for access tokens
│   │
│   ├── sync/
│   │   ├── engine.go            SyncEngine struct; Sync(ctx, connectionID) method
│   │   ├── worker.go            Worker pool; bounds concurrency
│   │   └── scheduler.go        Cron job setup; calls engine.SyncAll on schedule
│   │
│   ├── api/
│   │   ├── router.go            Mounts all /api/v1/ routes on chi sub-router
│   │   ├── accounts.go          GET /api/v1/accounts, GET /api/v1/accounts/:id
│   │   ├── transactions.go      GET /api/v1/transactions, GET /api/v1/transactions/:id
│   │   ├── users.go             GET /api/v1/users
│   │   ├── connections.go       GET /api/v1/connections, GET /api/v1/connections/:id/status
│   │   ├── sync.go              POST /api/v1/sync
│   │   └── health.go            GET /health
│   │
│   ├── mcp/
│   │   ├── server.go            MCP server construction; registers tools; mounts on router
│   │   └── tools.go             Tool handler functions (list_accounts, query_transactions, etc.)
│   │
│   ├── admin/
│   │   ├── router.go            Mounts all /admin/ routes; session auth middleware
│   │   ├── setup.go             First-run wizard handlers (GET+POST)
│   │   ├── login.go             GET+POST /admin/login, POST /admin/logout
│   │   ├── dashboard.go         GET /admin/ (overview)
│   │   ├── connections.go       Connection list, add, re-auth, remove handlers
│   │   ├── accounts.go          Account list, user assignment handlers
│   │   ├── transactions.go      Transaction browse handler
│   │   └── users.go             Family member CRUD handlers
│   │
│   ├── webhook/
│   │   └── handler.go           POST /webhooks/:provider; dispatches to the correct
│   │                            provider implementation based on the :provider path
│   │                            parameter (e.g., "plaid"), then routes events to
│   │                            the sync engine or connection updater
│   │
│   ├── middleware/
│   │   ├── apikey.go            X-API-Key authentication middleware
│   │   ├── session.go           Session cookie authentication middleware
│   │   ├── logging.go           Structured request logging middleware
│   │   ├── requestid.go         Injects X-Request-ID into context
│   │   └── csrf.go              CSRF token generation and validation
│   │
│   └── db/
│       ├── migrations/
│       │   ├── 001_initial_schema.sql
│       │   ├── 002_add_sync_logs.sql
│       │   └── ...              goose SQL migration files (sequential)
│       ├── queries/
│       │   ├── accounts.sql     sqlc annotated SQL queries for accounts
│       │   ├── transactions.sql sqlc annotated SQL queries for transactions
│       │   ├── connections.sql  sqlc annotated SQL queries for bank_connections
│       │   ├── users.sql        sqlc annotated SQL queries for users
│       │   ├── sync_logs.sql    sqlc annotated SQL queries for sync_logs
│       │   ├── admin.sql        sqlc annotated SQL queries for admin_accounts
│       │   └── app_config.sql   sqlc annotated SQL queries for app_config
│       ├── db.go                sqlc generated: New(db) constructor
│       ├── models.go            sqlc generated: row struct types
│       ├── queries.sql.go       sqlc generated: query method implementations
│       └── schema.sql           Canonical schema (documentation; migrations are authoritative)
│
├── web/
│   ├── templates/
│   │   ├── base.html            Base layout (nav, head, Pico CSS link)
│   │   ├── setup/
│   │   │   └── wizard.html      First-run setup wizard pages
│   │   ├── admin/
│   │   │   ├── dashboard.html
│   │   │   ├── connections.html
│   │   │   ├── accounts.html
│   │   │   ├── transactions.html
│   │   │   └── users.html
│   │   └── login.html
│   └── static/
│       └── css/
│           └── app.css          Minimal overrides on top of Pico CSS
│
├── go.mod
├── go.sum
├── Makefile
├── Dockerfile
├── docker-compose.yml
├── sqlc.yml
├── .env.example                 Template for all required and optional env vars
├── .local.env                   (gitignored) Local development env values
└── .docker.env                  (gitignored) Docker Compose env values
```

### Key File Notes

- `cmd/breadbox/main.go` is the only file outside `internal/`. It contains the
  cobra (or flag-based) subcommand dispatch and the top-level startup sequence.
- `internal/app/app.go` is the composition root. Every other package receives
  its dependencies through the `App` struct; no package reaches into another
  package's internals.
- `internal/db/migrations/` contains goose-format SQL files. The authoritative
  schema is defined by running all migrations in sequence, not by `schema.sql`.
  `schema.sql` is a convenience reference only.
- `web/templates/` and `web/static/` are embedded into the binary at compile
  time via `go:embed`. No separate asset server is required.

---

## 3. Provider Interface

### 3.1 Interface Definition

The provider interface lives in `internal/provider/provider.go`. All bank data
provider implementations must satisfy this interface. The MVP ships with a
single implementation (`internal/provider/plaid/`). Teller and CSV
implementations will be added post-MVP without modifying the interface or any
caller.

```go
// Provider is the abstraction over bank data providers (Plaid, Teller, CSV).
// All methods take a context for cancellation and timeout propagation.
type Provider interface {
    // CreateLinkSession starts a new account connection flow.
    // Returns a provider-specific token or URL that the frontend uses to
    // initialize the provider's Link widget.
    CreateLinkSession(ctx context.Context, userID string) (LinkSession, error)

    // ExchangeToken completes the connection flow after the user authenticates
    // in the Link widget. publicToken is the short-lived token returned by the
    // frontend. Returns a Connection (ready to persist) and the initial set of
    // Accounts discovered for this connection.
    ExchangeToken(ctx context.Context, publicToken string) (Connection, []Account, error)

    // SyncTransactions fetches incremental transaction changes for a connection.
    // cursor is the last persisted sync cursor (empty string for initial sync).
    // Returns slices of added, modified, and removed transactions, the next
    // cursor to persist, and whether more pages are available (hasMore).
    SyncTransactions(ctx context.Context, conn Connection, cursor string) (SyncResult, error)

    // GetBalances fetches current account balances for a connection.
    // Used for balance refresh independent of transaction sync.
    GetBalances(ctx context.Context, conn Connection) ([]AccountBalance, error)

    // HandleWebhook parses and validates an inbound webhook payload from the
    // provider. Returns a structured WebhookEvent the application can act on.
    // Returns an error if the payload is invalid or the signature fails.
    HandleWebhook(ctx context.Context, payload WebhookPayload) (WebhookEvent, error)

    // CreateReauthSession starts a re-authentication flow for an existing
    // connection that has broken (e.g., password changed, MFA required).
    // Returns a provider-specific token or URL for the Link widget update mode.
    CreateReauthSession(ctx context.Context, connectionID string) (LinkSession, error)

    // RemoveConnection revokes the provider's access and marks the connection
    // as removed. The application must delete or deactivate the connection
    // record after this call succeeds.
    RemoveConnection(ctx context.Context, connectionID string) error
}
```

### 3.2 Shared Types

These types are defined in `internal/provider/provider.go` and used by all
provider implementations and callers.

```go
type LinkSession struct {
    // Token is the provider-specific value passed to the Link widget.
    // For Plaid this is a link_token. For Teller it may be a URL.
    Token string
    // Expiry indicates when this session token becomes invalid.
    Expiry time.Time
}

type Connection struct {
    // ProviderName identifies the provider: "plaid", "teller", "csv".
    ProviderName string
    // ExternalID is the provider's identifier for this connection.
    // For Plaid this is the item_id.
    ExternalID string
    // EncryptedCredentials is an AES-256-GCM encrypted JSON blob containing
    // all provider-specific secrets needed to make API calls on behalf of this
    // connection (e.g., Plaid access_token).
    EncryptedCredentials []byte
    // InstitutionName is the human-readable name of the financial institution.
    InstitutionName string
}

type Account struct {
    ExternalID      string
    Name            string
    OfficialName    string
    Type            string // depository, credit, loan, investment
    Subtype         string // checking, savings, credit card, etc.
    Mask            string // last 4 digits
    ISOCurrencyCode string
}

type AccountBalance struct {
    AccountExternalID string
    Current           decimal.Decimal
    Available         *decimal.Decimal // nil if not provided
    Limit             *decimal.Decimal // nil if not applicable
    ISOCurrencyCode   string
}

type SyncResult struct {
    Added    []Transaction
    Modified []Transaction
    Removed  []string // external transaction IDs
    Cursor   string
    HasMore  bool
}

type Transaction struct {
    ExternalID            string
    PendingExternalID     *string // set when this transaction posts a pending one
    AccountExternalID     string
    Amount                decimal.Decimal // positive = debit, negative = credit
    Date                  time.Time
    AuthorizedDate        *time.Time
    Name                  string
    MerchantName          *string
    CategoryPrimary       *string
    CategoryDetailed      *string
    PaymentChannel        string // online, in store, other
    Pending               bool
    ISOCurrencyCode       string
}

type WebhookPayload struct {
    // RawBody is the full request body, preserved for signature verification.
    RawBody []byte
    // Headers contains the HTTP headers from the webhook request.
    // Providers use headers for webhook signatures.
    Headers map[string]string
}

type WebhookEvent struct {
    // Type classifies the event for routing.
    // Values: "sync_available", "connection_error", "connection_removed", "unknown"
    Type         string
    ConnectionID string // internal connection ID, resolved by provider
    ErrorCode    *string
}
```

### 3.3 Provider-Specific Data Storage

Provider-specific secrets (e.g., the Plaid `access_token` and `item_id`) are
stored in the `bank_connections` table using explicit typed columns rather than
a generic encrypted JSON blob. This matches the canonical schema defined in
`data-model.md`.

- Column: `plaid_item_id TEXT` — the Plaid item identifier
- Column: `plaid_access_token BYTEA` — the Plaid access token, encrypted with
  AES-256-GCM using the `ENCRYPTION_KEY` env var (nonce prepended to ciphertext)
- On read: the provider decrypts `plaid_access_token` before making API calls.

> **Note:** Provider-specific fields are explicit columns (not a JSON blob) for
> type safety and query simplicity in MVP. This may be revisited when adding
> Teller support.

```
bank_connections
├── id                    UUID primary key
├── user_id               UUID references users
├── provider              TEXT ("plaid", "teller", "csv")
├── plaid_item_id         TEXT nullable (Plaid item_id)
├── plaid_access_token    BYTEA nullable (AES-256-GCM encrypted)
├── institution_name      TEXT
├── sync_cursor           TEXT (last successfully persisted sync cursor)
├── status                TEXT ("active", "error", "pending_reauth", "disconnected")
├── error_code            TEXT nullable
├── error_message         TEXT nullable
├── last_synced_at        TIMESTAMPTZ nullable
└── created_at            TIMESTAMPTZ
```

Connections are soft-deleted: when a user disconnects a bank, the `status` is
set to `disconnected` rather than deleting the row. Accounts and transactions
linked to the connection are preserved for historical queries. The
`bank_connections` row is retained with `status = 'disconnected'`. There is no
CASCADE DELETE on `bank_connections`; downstream records are preserved
intentionally.

### 3.4 Provider Selection

The `provider` column on `bank_connections` determines which `Provider`
implementation handles a given connection. The `App` struct holds a map of
initialized providers keyed by provider name:

```go
type App struct {
    // ...
    Providers map[string]provider.Provider
}
```

When the sync engine or webhook handler needs to act on a connection, it reads
the `provider` field from the `bank_connections` row and looks up the
corresponding implementation in `App.Providers`. If no implementation is
registered for a given provider name, the operation returns an error.

At MVP, only `"plaid"` is registered. The structure is in place for `"teller"`
and `"csv"` to be added without modifying the sync engine or webhook handler.

---

## 4. Configuration

### 4.1 Configuration Sources

Breadbox has two configuration sources that serve different purposes:

| Source | Purpose | When Applied |
|---|---|---|
| Environment variables | Secrets, database credentials, deployment settings | Startup; cannot change without restart |
| `app_config` table | Runtime settings from setup wizard (Plaid keys, sync interval, webhook URL) | Loaded at startup; can be updated without restart via admin UI |

### 4.2 Config Precedence

When a setting exists in both sources, the environment variable takes
precedence. This allows deployment environments (Docker, CI) to override
database settings without modifying the database.

```
Environment Variable  →  overrides  →  app_config table  →  default
```

### 4.3 Required Environment Variables

These must be present at startup. Missing values cause the process to exit with
a descriptive error before binding any port.

| Variable | Description |
|---|---|
| `DATABASE_URL` | Full PostgreSQL connection string. If absent, the four individual vars below are used. |
| `DB_HOST` | PostgreSQL host (used if `DATABASE_URL` is absent) |
| `DB_PORT` | PostgreSQL port (default: `5432`) |
| `DB_NAME` | Database name |
| `DB_USER` | Database user |
| `DB_PASSWORD` | Database password |
| `ENCRYPTION_KEY` | 32-byte key as a 64-character hex string. Used for AES-256-GCM encryption of provider credentials. Generate with: `openssl rand -hex 32` |

### 4.4 Optional Environment Variables

These variables are optional. If set, they override the corresponding
`app_config` table values.

| Variable | Default | Description |
|---|---|---|
| `SERVER_PORT` | `8080` | HTTP listen port |
| `ENVIRONMENT` | `local` | `local` or `docker`; controls log format and env file selection |
| `PLAID_CLIENT_ID` | (from app_config) | Plaid client ID |
| `PLAID_SECRET` | (from app_config) | Plaid secret |
| `PLAID_ENV` | (from app_config) | `sandbox`, `development`, or `production` |

### 4.5 App Config Table Keys

Settings managed through the setup wizard are stored in the `app_config` table
as key-value pairs.

| Key | Description | Default |
|---|---|---|
| `plaid_client_id` | Plaid client ID | (none; required before connecting banks) |
| `plaid_secret` | Plaid secret | (none; required before connecting banks) |
| `plaid_env` | Plaid environment | `sandbox` |
| `sync_interval_hours` | How often to poll for new transactions | `12` |
| `webhook_url` | Public URL for Plaid to send webhooks | (none; polling-only without it) |
| `setup_complete` | Whether the setup wizard has been completed | `false` |

### 4.6 Env File Strategy

Breadbox does not use a single `.env` file. The `ENVIRONMENT` variable
controls which file is loaded:

| `ENVIRONMENT` value | File loaded |
|---|---|
| `local` (default) | `.local.env` |
| `docker` | `.docker.env` |

Both files are gitignored. `.env.example` in the repository root documents
every variable with placeholder values and is the canonical reference for
operators.

The env file is loaded at startup before any other initialization. Variables
already set in the process environment (e.g., passed by `docker-compose`)
are not overridden by the env file.

### 4.7 Config Struct

```go
// internal/config/config.go
type Config struct {
    // Derived from environment
    DatabaseURL     string
    EncryptionKey   []byte // 32 bytes, decoded from hex
    ServerPort      string
    Environment     string

    // May come from env (overrides app_config) or app_config table
    PlaidClientID   string
    PlaidSecret     string
    PlaidEnv        string // "sandbox" | "development" | "production"

    // From app_config table only
    SyncIntervalHours int
    WebhookURL        string
    SetupComplete     bool
}
```

---

## 5. Security

### 5.1 Credential Encryption

Plaid access tokens and any other provider secrets stored in the database are
encrypted with AES-256-GCM before being written and decrypted on read.

- **Algorithm:** AES-256-GCM
- **Key:** 32 bytes derived from `ENCRYPTION_KEY` (64-char hex)
- **Nonce:** 12 bytes, randomly generated per encryption operation
- **Storage format:** nonce prepended to ciphertext, stored as `BYTEA`
- **Implementation:** `internal/provider/plaid/encrypt.go`

The encryption key must be rotated by decrypting all credentials with the old
key, re-encrypting with the new key, and updating `ENCRYPTION_KEY`. No
automated rotation is provided at MVP.

### 5.2 Admin Authentication

Admin dashboard authentication uses bcrypt-hashed passwords.

- **Algorithm:** bcrypt
- **Cost factor:** 12
- **Storage:** `hashed_password` column in `admin_accounts` table
- **Session:** HttpOnly cookie, signed with a server-side secret
  - `Secure` flag: set only when `ENVIRONMENT` is not `local`
  - `SameSite`: `Lax` (canonical value; allows top-level navigations without
    breaking external links to the dashboard)
  - Session store: database-backed using `alexedwards/scs` with `pgxstore`.
    The `pgxstore` adapter creates its own session table automatically on first
    use; no manual migration is required for session storage.

### 5.3 API Key Authentication

REST API clients authenticate with an `X-API-Key` header.

- **Format:** `bb_` prefix followed by a random string, e.g., `bb_abc123...`
- **Storage:** SHA-256 hash of the key is stored; the plaintext is shown once
  at generation time and never stored
- **Verification:** inbound key is hashed and compared to stored hashes

### 5.4 CSRF Protection

All admin dashboard forms that mutate state use CSRF tokens.

- Tokens are generated per-session and stored in the session.
- Each POST form includes a hidden `_csrf` field.
- The middleware validates the field on every state-changing admin route.
- API routes (`/api/v1/`) are exempt (they use API key auth, not cookies).

### 5.5 Sensitive Data in Logs

The logging layer must never emit:

- Plaid access tokens or any raw provider credentials
- Bcrypt hashes or plaintext passwords
- Full API keys (only the `bb_` prefix + first 4 chars for tracing)
- Full transaction records at INFO or above

Structured log fields are reviewed at code review time. The `slog` logger does
not serialize the `Config` struct directly for this reason.

### 5.6 Database Connection Security

In production (`ENVIRONMENT=docker` or when `DATABASE_URL` contains `sslmode`),
connections to PostgreSQL must use TLS. The pgx pool is configured to require
TLS unless explicitly set to `sslmode=disable` for local development.

---

## 6. Logging

### 6.1 Logger

Breadbox uses Go's standard `log/slog` package for structured logging.

- **Development (`ENVIRONMENT=local`):** `slog.TextHandler` writing to stdout
- **Production (`ENVIRONMENT=docker`):** `slog.JSONHandler` writing to stdout
  (consumed by Docker log driver)

The logger is constructed once in `cmd/breadbox/main.go` and stored in `App`.
All components receive the logger through the `App` struct or as a parameter;
no package-level logger variables are used.

### 6.2 Log Levels

| Level | Used For |
|---|---|
| `DEBUG` | Enabled only when `ENVIRONMENT=local`. Detailed sync step output, SQL query timing. |
| `INFO` | Normal operational events: server started, sync completed, connection added. |
| `WARN` | Recoverable issues: sync retry due to mutation, webhook with unknown type, connection entering error state. |
| `ERROR` | Failures requiring operator attention: database error, provider API error, migration failure. |

### 6.3 Standard Log Fields

Every log entry includes:

| Field | Description |
|---|---|
| `time` | RFC3339 timestamp |
| `level` | Log level |
| `msg` | Human-readable message |
| `request_id` | Present on request-scoped logs; injected by `requestid` middleware |

Additional context fields are added by each subsystem.

### 6.4 What to Log

| Subsystem | Event | Level | Key Fields |
|---|---|---|---|
| HTTP | Request completed | INFO | `method`, `path`, `status`, `duration_ms` |
| HTTP | Auth failure | WARN | `method`, `path`, `reason` |
| Sync Engine | Sync started | INFO | `connection_id`, `trigger` (cron/webhook/manual) |
| Sync Engine | Sync page fetched | DEBUG | `connection_id`, `added`, `modified`, `removed` |
| Sync Engine | Sync completed | INFO | `connection_id`, `added`, `modified`, `removed`, `duration_ms` |
| Sync Engine | Sync failed | ERROR | `connection_id`, `error` |
| Sync Engine | Pagination retry | WARN | `connection_id`, `attempt` |
| Provider | API call | DEBUG | `provider`, `operation`, `duration_ms` |
| Connection | Status change | INFO | `connection_id`, `old_status`, `new_status` |
| Webhook | Received | INFO | `provider`, `type` |
| Webhook | Verification failed | WARN | `provider`, `reason` |
| Admin | Login success | INFO | `username` |
| Admin | Login failure | WARN | `username`, `reason` |

### 6.5 What Not to Log

- `access_token` or any value from `encrypted_credentials`
- Plaintext or hashed passwords
- Full API key values
- Raw webhook payloads that may contain tokens
- Full transaction records at INFO level (amounts and merchants are PII-adjacent)

---

## 7. Docker Deployment

### 7.1 Dockerfile

The Dockerfile uses a two-stage build to minimize the final image size while
retaining CA certificates and timezone data required for production operation.

```dockerfile
# Stage 1: Build
FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /breadbox ./cmd/breadbox

# Stage 2: Runtime
FROM alpine:3.21

# CA certificates: required for TLS connections to Plaid API and PostgreSQL
# tzdata: required for cron schedule timezone handling
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=builder /breadbox /app/breadbox

EXPOSE 8080
ENTRYPOINT ["/app/breadbox"]
```

`scratch` is not used as the base because:
- The Plaid API requires HTTPS; CA certificates must be present.
- `robfig/cron` supports timezone-aware schedules; timezone data must be present.

### 7.2 docker-compose.yml

```yaml
services:
  app:
    build: .
    restart: unless-stopped
    ports:
      - "8080:8080"
    env_file:
      - .docker.env
    environment:
      ENVIRONMENT: docker
    command: ["sh", "-c", "/app/breadbox migrate && /app/breadbox serve"]
    depends_on:
      db:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8080/health"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s

  db:
    image: postgres:16-alpine
    restart: unless-stopped
    env_file:
      - .docker.env
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U $${POSTGRES_USER} -d $${POSTGRES_DB}"]
      interval: 10s
      timeout: 5s
      retries: 5

volumes:
  postgres_data:
```

### 7.3 Startup Sequence

The `command` field in the `app` service runs migrations before starting the
server. This ensures the schema is always current after an image upgrade without
a separate migration step.

```
docker compose up
  ├── db service starts → PostgreSQL initializes → health check passes
  └── app service starts
        ├── breadbox migrate → applies any pending goose migrations → exits 0
        └── breadbox serve  → initializes App, binds :8080, starts cron
```

If `breadbox migrate` fails (e.g., bad migration SQL), the container exits with
a non-zero code and does not start the HTTP server.

### 7.4 Volumes and Persistence

| Volume | Service | Path | Contents |
|---|---|---|---|
| `postgres_data` | `db` | `/var/lib/postgresql/data` | All PostgreSQL data files |

No other persistent volumes are required. Application state lives entirely in
PostgreSQL.

### 7.5 Port Exposure

| Port | Service | Exposed to Host | Purpose |
|---|---|---|---|
| `8080` | `app` | Yes (always) | HTTP server |
| `5432` | `db` | No (default) | PostgreSQL — expose only for local dev inspection |

To expose PostgreSQL for local inspection during development, add a ports entry
to the `db` service in a `docker-compose.override.yml`. Do not expose 5432 in
production.

### 7.6 Health Check

`GET /health` returns HTTP 200 with a JSON body when the application is running
and the database connection pool is healthy. The response body is:

```json
{"status": "ok", "version": "0.1.0"}
```

This endpoint requires no authentication. Docker and external health monitors
use it to determine readiness.

---

## 8. Development Setup

### 8.1 Prerequisites

| Tool | Version | Purpose |
|---|---|---|
| Go | 1.24+ | Build and run the application |
| PostgreSQL | 16+ | Local database (or use Docker) |
| sqlc | latest | Generate type-safe Go code from SQL queries |
| goose | latest | Run database migrations |
| Docker + Docker Compose | current stable | Optional: run PostgreSQL and the full stack in containers |

Install sqlc and goose:

```sh
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
go install github.com/pressly/goose/v3/cmd/goose@latest
```

### 8.2 First-Time Setup

```sh
# 1. Clone the repository
git clone https://github.com/your-org/breadbox.git
cd breadbox

# 2. Copy the env template and fill in required values
cp .env.example .local.env
# Edit .local.env: set DATABASE_URL (or DB_* vars), ENCRYPTION_KEY

# 3. Start PostgreSQL
#    Option A: Docker (no local PostgreSQL required)
docker compose up db -d
#    Option B: Local PostgreSQL — create the database manually
createdb breadbox

# 4. Run migrations
make migrate-up

# 5. Generate sqlc code (required after any .sql file change)
make sqlc

# 6. Run the development server
make dev
# Server binds to http://localhost:8080

# 7. Open the setup wizard
open http://localhost:8080/admin/setup
```

On first launch the setup wizard runs automatically if no admin account exists.
Complete it before using any other feature.

### 8.3 Makefile Targets

| Target | Command | Description |
|---|---|---|
| `dev` | `go run ./cmd/breadbox serve` | Run server without building; auto-detects `.local.env` |
| `build` | `go build -o breadbox ./cmd/breadbox` | Compile binary to project root |
| `test` | `go test ./...` | Run all tests |
| `migrate-up` | `goose -dir internal/db/migrations postgres $DATABASE_URL up` | Apply all pending migrations |
| `migrate-down` | `goose -dir internal/db/migrations postgres $DATABASE_URL down` | Roll back the last migration |
| `migrate-create` | `goose -dir internal/db/migrations create $(NAME) sql` | Create a new empty migration file |
| `sqlc` | `sqlc generate` | Regenerate `internal/db/*.go` from SQL query files |
| `seed` | `go run ./cmd/breadbox seed` | Insert sandbox test data (dev only) |
| `docker-up` | `docker compose up --build -d` | Build image and start all services |
| `docker-down` | `docker compose down` | Stop all services (preserves volumes) |

### 8.4 ENCRYPTION_KEY Generation

```sh
openssl rand -hex 32
```

Paste the output as the value of `ENCRYPTION_KEY` in `.local.env` or
`.docker.env`. This key must never change after connections have been created;
doing so renders all stored credentials unreadable.

### 8.5 Plaid Sandbox Configuration

During development, set `PLAID_ENV=sandbox` and use sandbox credentials from
the Plaid dashboard. Sandbox provides test institutions with pre-populated
transaction history and does not require production access approval.

---

## 9. Error Handling Patterns

### 9.1 Service Layer Errors

Service functions return typed errors that callers can inspect for routing
decisions. A common pattern:

```go
// Sentinel errors in the service package
var (
    ErrNotFound         = errors.New("not found")
    ErrConflict         = errors.New("conflict")
    ErrProviderError    = errors.New("provider error")
    ErrInvalidInput     = errors.New("invalid input")
    ErrConnectionBroken = errors.New("connection requires re-authentication")
)
```

Service functions wrap lower-level errors with context:

```go
return nil, fmt.Errorf("sync transactions for connection %s: %w", connID, ErrProviderError)
```

Callers use `errors.Is` to check for specific conditions without string
matching.

### 9.2 HTTP Status Code Mapping

HTTP handlers translate service errors to appropriate status codes:

| Error | HTTP Status |
|---|---|
| `ErrNotFound` | 404 Not Found |
| `ErrConflict` | 409 Conflict |
| `ErrInvalidInput` | 400 Bad Request |
| `ErrProviderError` | 502 Bad Gateway |
| `ErrConnectionBroken` | 422 Unprocessable Entity |
| Auth failure (no key/session) | 401 Unauthorized |
| Auth failure (invalid key) | 403 Forbidden |
| Any other error | 500 Internal Server Error |

### 9.3 JSON Error Response Format

All REST API error responses use a consistent JSON envelope:

```json
{
  "error": {
    "code": "NOT_FOUND",
    "message": "account not found"
  }
}
```

`code` is a machine-readable `UPPER_SNAKE_CASE` string matching the error codes
defined in the REST API spec (e.g., `NOT_FOUND`, `INVALID_PARAMETER`,
`MISSING_API_KEY`). `message` is a human-readable description safe to display.
Internal error details (stack traces, SQL errors) are never included in the
response body; they are logged server-side.

Admin dashboard error responses follow the same envelope when responding to
AJAX requests. For full-page requests, errors are rendered in the HTML template.

### 9.4 Plaid API Error Handling

Plaid API errors are unwrapped from the Plaid SDK response and mapped to
application errors:

| Plaid Error Type | Application Error | Action |
|---|---|---|
| `ITEM_ERROR` / `ITEM_LOGIN_REQUIRED` | `ErrConnectionBroken` | Set connection status to `error`, surface in admin UI |
| `INVALID_ACCESS_TOKEN` | `ErrConnectionBroken` | Same as above |
| `TRANSACTIONS_SYNC_MUTATION_DURING_PAGINATION` | Retry | Retry from the beginning of the sync cursor; log as WARN |
| `RATE_LIMIT_EXCEEDED` | Retry with backoff | Exponential backoff; log as WARN |
| Any other `PLAID_ERROR` | `ErrProviderError` | Log as ERROR; fail the sync |

### 9.5 Database Error Handling

pgx errors are inspected for PostgreSQL error codes:

| Condition | PostgreSQL Code | Handling |
|---|---|---|
| Unique constraint violation | `23505` | Return `ErrConflict` |
| No rows returned | pgx `ErrNoRows` | Return `ErrNotFound` |
| Connection error | pgx network error | Return `ErrProviderError`; log as ERROR |
| All other errors | — | Wrap with context; return as internal error |

---

## 10. Testing Strategy

### 10.1 Scope for MVP

Testing at MVP focuses on the two highest-risk subsystems: the sync engine
(complex state machine with edge cases) and the REST API (external-facing
contract). There is no minimum coverage percentage requirement, but these two
subsystems must have test coverage before the MVP ships.

### 10.2 Unit Tests — Sync Engine

Location: `internal/sync/*_test.go`

The `Provider` interface is mocked in tests. The mock is defined in
`internal/provider/mock.go` and implements the full `Provider` interface with
configurable return values.

Key scenarios tested:

- Initial sync (empty cursor): correct upsert of all returned transactions
- Incremental sync: added, modified, and removed transactions handled correctly
- Multi-page sync (`HasMore=true`): cursor advances correctly across pages
- `TRANSACTIONS_SYNC_MUTATION_DURING_PAGINATION`: retry resets to previous cursor
- Pending-to-posted transition: `pending_transaction_id` linkage preserved
- Provider returns error: sync log records failure; connection status updated
- Context cancellation mid-sync: goroutine exits cleanly

### 10.3 Integration Tests — REST API

Location: `internal/api/*_test.go`

Integration tests run against a real PostgreSQL database. The test setup:

1. Creates a dedicated test database (or uses a transaction per test with
   rollback to isolate state).
2. Runs all migrations against the test database.
3. Constructs a real `App` with a real database pool and a mock `Provider`.
4. Runs an `httptest.Server` with the chi router.
5. Makes HTTP requests and asserts response status codes and JSON bodies.

Key scenarios tested:

- `GET /api/v1/accounts`: returns accounts for authenticated key; 401 without key
- `GET /api/v1/transactions`: pagination cursor; date range and amount filters; text search
- `GET /api/v1/connections/:id/status`: correct status and last sync time
- `POST /api/v1/sync`: triggers sync; returns sync log ID
- `GET /health`: always returns 200, no auth required

### 10.4 End-to-End Testing — Plaid Sandbox

Plaid provides a sandbox environment with deterministic test institutions,
simulated transactions, and webhook delivery.

For manual end-to-end verification:

1. Run `breadbox serve` with `PLAID_ENV=sandbox`.
2. Use the admin dashboard to connect a test institution via Plaid Link.
3. Verify transactions appear in `GET /api/v1/transactions`.
4. Trigger a sync via `POST /api/v1/sync` and verify the sync log.
5. Use Plaid's sandbox `/sandbox/item/fire_webhook` to test webhook delivery.

Sandbox end-to-end tests are not automated at MVP. They are run manually before
releases.

### 10.5 Test Utilities

| Utility | Location | Purpose |
|---|---|---|
| Mock provider | `internal/provider/mock.go` | Implements `Provider`; configurable returns |
| Test DB setup | `internal/testutil/db.go` | Creates/migrates test DB; returns pool |
| Request builder | `internal/testutil/http.go` | Builds authenticated test HTTP requests |

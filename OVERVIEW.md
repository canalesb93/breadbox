# Breadbox

Self-hosted financial data aggregation for families. Syncs bank data from
multiple providers (Plaid, Teller, CSV), stores it locally, exposes it to AI
agents via MCP and to other services via REST API.

## Problem

Banks silo your financial data behind their own apps. AI agents can help with
budgeting, spending analysis, and anomaly detection — but they need structured
access to your data without touching your bank credentials. As transaction
history grows beyond what fits in an LLM context window, raw file dumps stop
working. You need queryable, filterable, always-current financial data exposed
through a structured interface.

## Solution

Breadbox is a service you host yourself that:
1. Connects to your banks and syncs transactions + account balances
2. Stores everything in a local PostgreSQL database you own
3. Exposes raw data via REST API and MCP — agents, dashboards, and other services query it

## Goals (MVP)

- [ ] Connect bank accounts via Plaid Link (primary provider)
- [x] Abstract bank data provider behind an interface (Plaid, Teller, and CSV all implemented)
- [ ] Sync transactions and account balances automatically (configurable interval, default 12h)
- [ ] Track account ownership per family member (label/filter, not access control)
- [ ] REST API exposing raw financial data (accounts, transactions, balances)
- [ ] MCP server over Streamable HTTP wrapping the same data as REST
- [ ] Admin web dashboard (consumes the REST API) to manage connections and view data
- [ ] First-run setup wizard for initial configuration (admin account, Plaid keys, sync interval)
- [ ] Handle connection health: detect broken links, support re-authentication flows
- [ ] Docker Compose deployment (single `docker compose up`)

## Non-Goals (MVP)

- Investment accounts / holdings
- Agent write-back (tags, budgets, category overrides — v2)
- Spending summaries / aggregation endpoints (agents compute these from raw data)
- Mobile app
- Multi-tenant / multi-family
- Real-time streaming
- Merchant name normalization
- Teller and CSV import are now implemented (see Phases 9, 11)

## Architecture

```
┌─────────────┐     ┌───────────────────────────────────┐     ┌────────────┐
│ Bank Data    │     │          Breadbox (Go)             │     │ PostgreSQL │
│ Providers    │◄───►│                                    │◄───►│            │
│ (Plaid,      │     │  ┌──────────┐  ┌───────────────┐  │     └────────────┘
│  Teller,     │     │  │  Sync    │  │  REST API     │  │
│  CSV)        │     │  │  Engine  │  │  /api/v1/...  │  │
└─────────────┘     │  └──────────┘  └───────┬───────┘  │
                    │  ┌──────────┐          │          │
                    │  │ Webhook  │  ┌───────┴───────┐  │
                    │  │ Handler  │  │               │  │
                    │  └──────────┘  ▼               ▼  │
                    │         ┌───────────┐ ┌──────────┐│
                    │         │   Admin   │ │   MCP    ││
                    │         │ Dashboard │ │  Server  ││
                    │         │  (HTML)   │ │  (HTTP)  ││
                    │         └───────────┘ └──────────┘│
                    └───────────────────────────────────┘
```

Single Go binary, single `breadbox serve` command. All components share one
HTTP server (chi router):

- **REST API** (`/api/v1/...`): Core data layer. JSON endpoints for accounts,
  transactions, balances, users, sync status. Used by the dashboard, MCP tools,
  and any external service. API key authenticated.
- **MCP Server**: Streamable HTTP transport on the same router. MCP tools wrap
  the REST API layer. Agents connect remotely over HTTP. Stdio available as
  optional dev convenience (`breadbox mcp-stdio`).
- **Admin Dashboard**: Server-rendered HTML (Go templates + DaisyUI 5/Tailwind CSS v4 + Alpine.js v3 + Plaid Link JS).
  Consumes the service layer directly. Manages connections, family members, connection health,
  re-auth flows. Session authenticated.
- **Sync Engine**: Cron + webhook-triggered bank data sync (background goroutines).
- **Webhook Handler**: Receives Plaid/provider callbacks for sync triggers and
  connection status changes.

## REST API (MVP)

Raw data access. All endpoints return JSON, paginated where applicable.

```
GET  /api/v1/accounts                 List accounts (filter by user_id)
GET  /api/v1/accounts/:id             Single account with balance
GET  /api/v1/transactions             List transactions (cursor pagination)
                                      Filters: start_date, end_date, account_id,
                                      user_id, category, min_amount, max_amount,
                                      pending, search (text)
GET  /api/v1/transactions/:id         Single transaction
GET  /api/v1/users                    List family members
GET  /api/v1/connections              List bank connections with status
GET  /api/v1/connections/:id/status   Connection health + last sync info
GET  /api/v1/categories               List distinct category pairs (primary + detailed)
POST /api/v1/sync                     Trigger sync (all or specific connection)
GET  /health/live                     Basic health check (unauthenticated)
GET  /health/ready                    Deep health check: DB + scheduler (unauthenticated)
```

Auth: `X-API-Key: bb_xxxxx` header. Admin dashboard uses session cookies instead.

## MCP Tools (MVP)

Thin wrappers around the REST API. Focus on raw data retrieval.

| Tool | Description |
|------|-------------|
| `list_accounts` | List all accounts with current balances. Filter by user. |
| `query_transactions` | Search/filter transactions with all REST API filters. Cursor-paginated, default 100 per page. |
| `count_transactions` | Count matching transactions (helps agents decide whether to paginate or narrow filters) |
| `list_users` | List family members |
| `get_sync_status` | Connection health, last sync times, items needing re-auth |
| `trigger_sync` | Manually trigger a data sync |
| `list_categories` | List distinct category pairs (primary + detailed) |

**MCP Resource:** `breadbox://overview` — returns live stats (account count, transaction count, date range, provider summary).

Aggregations (spending summaries, income breakdowns, net worth) are deferred —
agents can compute these from raw transaction data.

## Data Model

| Entity | Key Fields |
|--------|------------|
| Users | name, email (label for account ownership, not login) |
| Bank Connections | provider (plaid/teller/csv), institution, encrypted credentials, sync_cursor, status, error info |
| Accounts | connection_id, type, subtype, mask, balances (current/available/limit), iso_currency_code |
| Transactions | external_transaction_id (unique per provider), pending_transaction_id (links pending→posted), account_id, amount (NUMERIC(12,2)), date, authorized_date, merchant_name, name, category_primary, category_detailed, payment_channel, pending, iso_currency_code, deleted_at (soft delete) |
| Sync Logs | connection_id, trigger type, added/modified/removed counts, status, error |
| Admin Accounts | username, bcrypt hashed_password (dashboard login) |
| App Config | key-value store for setup wizard settings (sync interval, Plaid env, etc.) |

Key design decisions:
- **Pending→posted**: When a pending transaction posts, Plaid removes the old ID and creates a new one. `pending_transaction_id` links them for continuity.
- **Soft deletes**: Removed transactions get `deleted_at` set, not hard-deleted. API excludes them by default.
- **Provider abstraction**: `Bank Connections` is provider-agnostic. Provider-specific fields (plaid item_id, access_token) stored as encrypted JSON or in dedicated columns.
- **Currency per transaction**: Never silently sum across currencies.
- **App Config table**: Stores settings from the setup wizard. Avoids requiring restart for config changes.

## Tech Stack

| Component | Choice | Why |
|-----------|--------|-----|
| Language | Go 1.24+ | Single binary, native concurrency, mature MCP + Plaid SDKs |
| MCP SDK | github.com/modelcontextprotocol/go-sdk | Official, production-ready (v1.4.0), Streamable HTTP support |
| Plaid SDK | github.com/plaid/plaid-go | Official, auto-generated from OpenAPI |
| Database | PostgreSQL | ACID, NUMERIC precision for money, robust indexing, pg_trgm for text search |
| DB access | sqlc + pgx/v5 | Type-safe generated queries (dynamic filters via sqlc.narg pattern) |
| Migrations | goose | Simple SQL migrations |
| HTTP | chi/v5 | Lightweight, composable middleware |
| Scheduling | robfig/cron/v3 | Background sync scheduling |
| Admin UI | Go html/template + DaisyUI 5/Tailwind v4 + Alpine.js v3 | CSS pre-compiled via standalone CLI (no Node.js). Lucide icons via CDN. |
| Deployment | Docker Compose | PostgreSQL + app, single command |

## Bank Data Providers

Breadbox abstracts the bank data provider behind a Go interface. MVP ships with
Plaid only. The interface ensures adding Teller or CSV import later doesn't
require restructuring.

### Plaid (MVP)

- **Sync**: Cursor-based `/transactions/sync`
- **Triggers**: Cron (configurable via setup wizard, default 12h) + `SYNC_UPDATES_AVAILABLE` webhook
- **Products**: Transactions (includes basic account balances via `/accounts/get`)
- **Costs**: ~$1.50/item/month in production (subscription-priced). 200 one-time testing credits in Limited Production. NOT a free monthly tier.
- **Production access**: Requires company profile, security questionnaire, and OAuth registration. OAuth for major banks (Chase, Capital One) can take up to 6 weeks. Plaid supports hobbyist use on "Pay as you go" plan.
- **Webhooks**: Require a publicly accessible URL (Cloudflare Tunnel recommended for self-hosted). Without webhooks, polling-only.
- **Security**: Access tokens AES-256-GCM encrypted at rest
- **Sync edge cases**: Pending→posted ID changes, `TRANSACTIONS_SYNC_MUTATION_DURING_PAGINATION` retry, `days_requested` for >90 days history at link time
- **Connection maintenance**: Banks break connections regularly. Must support Plaid Link update mode for re-authentication.
- **API reference**: https://plaid.com/docs/llms-full.txt

### Teller (Implemented — Phase 9)

- 100 free live connections for indie developers
- Simpler API, no screen scraping
- US-only, fewer institutions than Plaid
- mTLS auth (app-level cert/key) + per-connection access token
- Date-range polling with 10-day overlap (no cursor)
- Category mapping to Plaid-compatible primary + detailed categories

### CSV Import (Implemented — Phase 11)

- Zero cost, zero API dependency
- Import-only: stub Provider interface (no sync/webhook/reauth)
- Dedup via SHA-256 hash of `account_id|date|amount|description`
- Admin UI upload page at `/admin/connections/import-csv`

## First-Run Setup

On first launch, if no admin account exists, Breadbox shows a setup wizard:

1. Create admin account (username + password)
2. Configure bank providers (Plaid / Teller / Both / Skip)
3. Optional: add a family member (name + email)
4. Set sync interval (15m, 30m, 1h, 4h, 8h, 12h, 24h)
5. Optional: webhook URL (with guidance on Cloudflare Tunnel setup)
6. Review configuration and finish

Settings stored in the App Config table. Subsequent launches skip the wizard.

## Implementation Phases

1. **Foundation** — Project skeleton, DB schema, config, migrations, provider interface, health endpoint, app config
2. **Plaid Integration + Admin Auth** — Plaid client, Link flow (connect + re-auth), admin login, setup wizard, connection management
3. **Transaction Sync Engine** — Cursor-based sync loop, pending→posted handling, balance refresh, error recovery, sync logs
4. **REST API** — All data endpoints, API key auth, cursor pagination, dynamic filters
5. **MCP Server** — Streamable HTTP transport, MCP tools wrapping REST layer, stdio convenience mode
6. **Automated Sync + Webhooks** — Cron scheduling, webhook handler, connection health monitoring, graceful shutdown
7. **Docker Deployment** — Multi-stage Dockerfile, Compose, persistent volumes, production polish
8. **Multi-Provider Refactoring** — Provider interface abstraction, generic DB columns, shared crypto
9. **Teller Provider** — mTLS auth, date-range sync, category mapping, webhook support
10. **Enhanced Settings & Connection Management** — Display names, excluded accounts, pause/resume, per-connection sync intervals
11. **CSV Import Provider** — Upload page, column mapping, dedup via SHA-256, stub Provider interface
12A. **Admin UI Foundation** — Class-based Pico CSS, Alpine.js, badge functions, nav restructure
12B. **Admin Transaction Pages** — Transaction list with filters, account detail, cross-linking
13A. **Bug Fixes & Dashboard UX** — Setup wizard bugs, dashboard navigation, error messages, breadcrumbs
13B. **Setup & Settings Overhaul** — Multi-provider wizard, password change, system info, config source badges
14. **Deployment Readiness & Reliability** — README, encryption key validation, health check split, transactional sync, CLI tools
15. **Agent-Optimized API** — Account/user names on transactions, categories endpoint, sort options, MCP enrichment
16A. **Design System Foundation** — DaisyUI 5 + Tailwind CSS v4, drawer sidebar, icons, component classes
16B. **Page Redesign** — Per-page visual upgrades using DaisyUI foundation

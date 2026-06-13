# Breadbox v1 — Schema & API Hardening Proposal

> **Status:** Proposal for review (not yet implemented).
> **Goal:** Prepare the database and public API for the v1 open-source launch so that
> (a) self-hosters can grow their database confidently, and (b) a third-party developer
> would *want* to build a standalone product on top of the API.
> **Stance:** Bold. This is the last cheap moment to make breaking changes. After v1 we
> owe users a stability pledge, so we spend the breakage budget now.

---

## 1. Executive summary

The schema is fundamentally healthy. Past sprints already retired the worst cruft
(`admin_accounts`, `audit_log`, `category_mappings`, `review_queue`,
`transaction_comments`, `member_accounts` are all dropped). What remains is **drift, not
rot**: half-finished renames, reserved-word footguns, provider-vocabulary leakage,
TEXT-as-enum sprawl, and two experimental subsystems (OAuth, device-codes) sitting in the
schema with zero production rows.

The API is RESTful and well-documented, but it blurs **two audiences into one surface**:
the operator (who manages providers, users, workflows) and the integrator (who reads
financial data). v1 should split those cleanly and make the integrator surface a
first-class, stable, fully-paginated contract.

Seven bold strokes define this proposal:

1. **Finish the agent→workflow rename** down to the FK columns and query files. Stop shipping a subsystem that is half "agent", half "workflow".
2. **`short_id` becomes *the* public ID.** The API stops leaking internal UUIDs. One ID, everywhere, base62, stable.
3. **Promote canonical enums to real Postgres `ENUM` types.** We already document 14 canonical enums in `CLAUDE.md`; the schema should enforce them, not approximate them with `TEXT + CHECK`.
4. **Rename `annotations` → `transaction_events`** and treat it as the first-class, partition-ready event log it already is.
5. **Split the surface: public `/api/v1` (token-auth, OpenAPI, stable) vs. operator `/admin` (session-auth).** Workflows, provider credentials, users/logins, and settings move to operator-only.
6. **Uniform cursor pagination + field selection on every list.** Kill offset pagination and bare-array responses.
7. **Defer OAuth and device-codes out of the advertised v1.** Ship API-keys as the single, coherent front door. Add the missing retention/cleanup jobs for everything we *do* ship.

> ⚠️ **Scope note (verified):** `csv_import_*`, `dev_reports`, and `transactions.content_hash`
> appear in the shared dev database but are **NOT in `main`** — they leaked from sibling
> worktree branches (`csv-import-v2`, devmode-reporter). This proposal targets the real
> `main` schema (34 live tables). Their design feedback is captured in §6 for when those
> branches land, but they are not part of the v1 cleanup.

---

## 2. Target table layout (what we should AIM for)

34 tables today → **30 in the v1 target** (after renames, drops, and deferrals). Grouped
by domain. Renamed columns are shown as `old → new`. New columns marked **+**.

### 2.1 Identity & access

| Table | Notable target shape | Changes |
|---|---|---|
| `users` | household members (the *people*) | + `deleted_at` (soft-delete preserves connection history) |
| `auth_accounts` | login credentials, FK→users | `role TEXT → role_type ENUM(admin, editor, viewer)` |
| `api_keys` | the v1 front door | `agent_definition_id → workflow_id`; keep `scope ENUM(full_access, read_only)` |
| `sessions` | dashboard cookies (scs-managed) | unchanged — first-party admin only, never an API token |

### 2.2 Connections & accounts

| Table | Notable target shape | Changes |
|---|---|---|
| `bank_connections` | one row per provider linkage | `external_id → provider_connection_id`; + index `(user_id, status)`; promote `(provider, provider_connection_id)` partial unique index → real `UNIQUE` constraint |
| `accounts` | financial accounts under a connection | `iso_currency_code` → `NOT NULL DEFAULT 'USD'`; document `is_dependent_linked`/`excluded` as API-visible flags |
| `account_links` | multi-account reconciliation links | `match_strategy TEXT → match_strategy ENUM(date_amount_name)` |
| `transaction_matches` | matched txn pairs across linked accounts | keep (feature-complete, 0 rows is expected for fresh installs) |

### 2.3 Transactions & enrichment

| Table | Notable target shape | Changes |
|---|---|---|
| `transactions` | the core ledger | **drop** `unofficial_currency_code` (dead Plaid legacy); `provider_pending_transaction_id → replaced_pending_provider_id`; `category_override → category_source` (ENUM none/rule/agent/user); evaluate moving `provider_raw` off the hot row (see §4); + index `(attributed_user_id)` |
| `categories` | hierarchical category tree | no change — clean |
| `tags` | reusable labels | `lifecycle TEXT → tag_retention ENUM(persistent, ephemeral)` + document "ephemeral = removal requires a reason" |
| `transaction_tags` | txn↔tag join w/ provenance | no change — clean |
| `transaction_rules` | the rule DSL store | `trigger TEXT → rule_trigger ENUM(on_create, on_change, always)` |
| `transaction_events` ⬅ `annotations` | **renamed.** the activity/audit event log | `kind TEXT+CHECK → event_kind ENUM`; partition-by-month plan documented for scale |
| `recurring_series` | detected subscriptions/bills | `cadence`, `status`, `type`, `detection_source`, `confidence` → ENUM types; document `detection_signals` JSONB shape |
| `series_tags` | series↔tag join (materialized onto txns) | no change — clean |

### 2.4 Sync & ingestion

| Table | Notable target shape | Changes |
|---|---|---|
| `sync_logs` | one row per sync run | `trigger → sync_trigger` (kill reserved-word footgun); keep `duration_ms` (perf denorm) |
| `sync_log_accounts` | per-account breakdown of a run | no change — intentional, not redundant |
| `sync_schedules` | cron-anchored schedules | no change — recently landed, clean |
| `sync_schedule_connections` | schedule↔connection join | no change |
| `webhook_events` | provider webhook audit | **+ retention job** (7-day default, config-keyed); document "payload not persisted, processed transactionally" |
| `hosted_link_sessions` | shareable bank-link surface | no change — clean |

### 2.5 Workflows & agents (the rename frontier)

| Table | Notable target shape | Changes |
|---|---|---|
| `workflows` ⬅ (`agent_definitions`) | scheduled AI run definitions | table already renamed; finish query-file rename |
| `workflow_runs` ⬅ (`agent_runs`) | run history + token/cost metrics | `agent_definition_id → workflow_id`; query-file rename |
| `reports` ⬅ `agent_reports` | AI-authored reports | `agent_run_id → workflow_run_id`; collapse `author` + `created_by_name` → single `author` display contract |
| `connector_library` | global custom-MCP connectors | no change — newly landed, clean |

### 2.6 MCP audit

| Table | Notable target shape | Changes |
|---|---|---|
| `mcp_sessions` | one row per MCP transport session | no change |
| `mcp_tool_calls` | per-tool-call audit | **gate** `request_json`/`response_json` persistence behind a config flag (PII); **+ retention job** (30-day default) |

### 2.7 Configuration

| Table | Notable target shape | Changes |
|---|---|---|
| `app_config` | env→DB→default key/value | remove dead seeded keys `sync_interval_hours`, `setup_complete` |

### 2.8 Deferred out of advertised v1 (kept dormant, unadvertised, or removed)

| Table(s) | Verdict | Rationale |
|---|---|---|
| `oauth_clients`, `oauth_access_tokens`, `oauth_refresh_tokens`, `oauth_authorization_codes` | **DEFER** — not advertised; remove from v1 docs/UI | 0 production rows; expiry-cleanup job never wired; unproven rotation/revocation under load. Ship API-keys as the single front door. Re-introduce as a post-1.0 minor with the cleanup hook. |
| `auth_device_codes` | **FINISH or CUT** | RFC-8628 CLI login is genuinely valuable, but the service layer is not wired (orphaned queries). Either complete `breadbox auth login` for v1 or drop the table until it's real. |

> Net table count: 34 today − 4 OAuth (deferred) − 1 device-codes (if cut) − 1 (`reports`
> rename is not a drop) = **~28–30 advertised tables for v1.**

---

## 3. Target API shape (what we should AIM for)

### 3.1 Two surfaces, cleanly split

```
PUBLIC INTEGRATOR API           OPERATOR / ADMIN SURFACE
/api/v1/*                       /admin/*  (or keep /-/*)
─────────────────────           ─────────────────────────
auth: X-API-Key (bb_…)          auth: session cookie + CSRF
documented in openapi.yaml       not in the public spec
stability pledge applies         free to change
cursor pagination everywhere     —

  transactions   (+ events,        providers / credentials
                  tags, metadata)   users / login-accounts
  accounts                          workflows / runs / reports
  connections (read)                connectors
  categories                        settings / app_config
  rules                             sync-schedules
  series                            hosted-links (create)
  tags
  sync-logs (read)
  webhooks (NEW: out-bound)
```

**Why:** today `/api/v1/workflows`, `/api/v1/settings/providers/plaid`, and
`/api/v1/login-accounts` live under the public umbrella but are operator-only in practice.
A third-party dev shouldn't discover an endpoint, build against it, and *then* learn it
needs admin rights. The split makes the public contract honest.

### 3.2 Conventions the public API commits to

- **One ID.** Every resource exposes a single `id` = the 8-char base62 `short_id`. Internal
  UUIDs are never returned. Path params take the public id. (Today REST returns both `id`
  (uuid) *and* `short_id` — confusing and leaky.)
- **Cursor pagination, uniformly.** `?limit=50&cursor=<opaque>` on every list. `limit` caps
  at 500. Response: `{ "<resource>": [...], "next_cursor": "…"|null, "has_more": bool }`.
  No offset, no bare arrays.
- **Field selection, uniformly.** `?fields=core|full|all` (+ named projections) on every
  read. Documented default per resource.
- **Error envelope (unchanged).** `{ "error": { "code": "UPPER_SNAKE", "message": "…" } }`.
  Codes are stable contracts.
- **Money (unchanged, documented loudly).** `NUMERIC(12,2)`, always paired with
  `iso_currency_code`, positive = money out. Never summed across currencies.
- **Idempotency.** Optional `Idempotency-Key` header on all writes; replays within 24h
  return the cached response.
- **Traceability.** Echo `X-Request-ID` on every response (middleware already generates it).
- **CORS.** `CORS_ALLOWED_ORIGINS` env var so self-hosters can call the API from a SPA.

### 3.3 Stability pledge (published with v1)

> Within `/api/v1`, fields are **additive**: never removed, never renamed, never
> re-typed. New capabilities arrive as new fields or new `/api/v1/{resource}` paths. A
> breaking change means a new `/api/v2` prefix; `/api/v1` keeps running. Deprecated
> endpoints carry a `Sunset` header and live for at least 12 months.

### 3.4 Net-new public capabilities for "build a product on this"

| Capability | Shape |
|---|---|
| **Outbound webhooks** | `POST /api/v1/webhooks` registers a callback URL; Breadbox fires HMAC-signed POSTs on transaction/series/report changes. The single biggest unlock for external products (no more polling). |
| **Bulk transaction update** | consolidate `batch-categorize` + `bulk-recategorize` into one `POST /api/v1/transactions/update` (filter- or id-based). |
| **Request body examples in OpenAPI** | spec defines shapes but lacks example payloads for writes. |

### 3.5 MCP surface tightening

- Standardize `fields=` defaults across all row-returning tools (today transactions/rules
  have it, series doesn't).
- Consolidate the 11 series tools: fold `add_series_tag`/`remove_series_tag` into
  `update_series`; clarify `review_series` (adjudicate) vs `assign_series` (create/backfill).
- Add `get_transaction_rule` (single-fetch mirror; REST has it, MCP doesn't).
- Add reconciliation tools (`list_transaction_matches`, `confirm_match`, `reject_match`) if
  account-linking is advertised for v1.

---

## 4. Open decisions (need a call before implementation)

1. **`transactions.provider_raw` (JSONB, full provider payload per row).** Useful for
   debugging and re-processing, but stored on the hottest table for every row — real bytes
   at scale. **Options:** (a) keep + document as internal; (b) move to a side table
   `transaction_provider_payloads`; (c) make it config-gated (off by default).
   **Recommendation:** (b) or (c) — keep the hot ledger lean.
2. **`auth_device_codes`:** finish `breadbox auth login` for v1, or cut the table?
   **Recommendation:** cut for v1 unless CLI login is a launch story.
3. **`transaction_events` partitioning:** ship v1 with the rename + ENUM only, and add
   `PARTITION BY RANGE (created_at)` as a documented v1.x step — or do it now?
   **Recommendation:** rename + ENUM now; partition when an install crosses ~1M events.
4. **Enum migration ordering:** `TEXT → ENUM` conversions are `ALTER TYPE`-class changes
   that the shared-dev-DB rule flags as destructive. They must be sequenced carefully and
   run as a coordinated cutover, not additive trickle. (Affects the build plan, not the
   target.)

---

## 5. Changelog (concise)

### ✅ ADD
- `users.deleted_at` (soft-delete).
- Indexes: `bank_connections(user_id, status)`, `transactions(attributed_user_id)`.
- `UNIQUE(bank_connections.provider, provider_connection_id)` constraint.
- Retention/cleanup jobs: `webhook_events` (7d), `mcp_tool_calls` (30d), OAuth auth-codes (if OAuth ever ships).
- Config flag gating `mcp_tool_calls` request/response JSON persistence.
- Public API: outbound webhooks, `Idempotency-Key`, `X-Request-ID` echo, CORS config, OpenAPI request examples.

### ✏️ RENAME
- **Finish agent→workflow:** `workflow_runs.agent_definition_id → workflow_id`; `api_keys.agent_definition_id → workflow_id`; `agent_reports → reports` with `agent_run_id → workflow_run_id`; query files `agent_*.sql → workflow_*.sql`.
- `annotations → transaction_events` (+ `kind → event_kind`).
- `sync_logs.trigger → sync_trigger` (reserved word).
- `bank_connections.external_id → provider_connection_id`.
- `transactions.category_override → category_source`; `provider_pending_transaction_id → replaced_pending_provider_id`.
- `tags.lifecycle → tag_retention`.
- **Public API IDs:** responses expose `short_id` as the single canonical `id`; drop UUID exposure.

### 🔁 CONVERT (TEXT+CHECK → Postgres ENUM)
- `auth_accounts.role`, `account_links.match_strategy`, `transaction_rules.trigger`,
  `transactions.category_source`, `transaction_events.event_kind`,
  `recurring_series.{cadence,status,type,detection_source,confidence}`.

### ❌ REMOVE
- `transactions.unofficial_currency_code` (dead).
- `app_config` seeded keys `sync_interval_hours`, `setup_complete` (dead).
- OAuth (4 tables) **from advertised v1** — keep dormant or remove; ship API-keys only.
- `auth_device_codes` if CLI login isn't a v1 story.
- API: flat `/login-accounts` list, redundant `batch-categorize`/`bulk-recategorize` (→ `/transactions/update`).

### ➡️ MOVE (public → operator surface)
- `/workflows`, `/workflow-runs`, `/reports`, `/connectors`, provider credentials, users/login management, settings, sync-schedules → `/admin/*` (session-auth, out of the public spec).

### ⏸️ DEFER (document, don't build for v1)
- Multi-tenant OAuth 2.1 client-credentials, fine-grained scopes beyond read/write,
  `transaction_events` partitioning, account-link MCP tools (unless linking ships).

---

## 6. Notes for unmerged branches (not v1 scope, captured here)
- **csv-import-v2** (`csv_import_sessions/rows/profiles`, `transactions.content_hash`):
  `raw_blob BYTEA` storing whole CSVs in Postgres is an anti-pattern — externalize or
  stream. `csv_import_rows` double-stores `raw` JSON + parsed columns — pick one.
- **devmode-reporter** (`dev_reports`): leaked into dev DB with no `main` migration and no
  DB-write path in the handler ("no token, no persistence"). Gate behind a dev flag or drop
  before it reaches `main`.

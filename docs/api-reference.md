# API Quick Reference

Complete list of all REST API endpoints. All endpoints are prefixed with `/api/v1/` unless noted otherwise.

## Authentication

All API endpoints require an API key via the `X-API-Key` header:

```
X-API-Key: bb_your_api_key_here
```

API keys are created from the admin dashboard under **API Keys**. Keys can be scoped as `full_access` or `read_only`. Write endpoints require `full_access` scope.

## Amount Convention

- **Positive** = money out (debits, purchases, fees)
- **Negative** = money in (credits, deposits, refunds)
- Always check `iso_currency_code` -- never sum across different currencies

---

## Health

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/health` | No | Basic liveness check |
| GET | `/health/live` | No | Basic liveness check |
| GET | `/health/ready` | No | Readiness check (verifies DB + scheduler) |
| GET | `/api/v1/version` | No | Current server version and update availability |

## Accounts

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/accounts` | Read | List all accounts |
| GET | `/accounts/{id}` | Read | Get a single account |

## Transactions

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/transactions` | Read | Query transactions with filters and pagination |
| GET | `/transactions/count` | Read | Count transactions matching filters |
| GET | `/transactions/summary` | Read | Aggregated totals by category, month, week, day |
| GET | `/transactions/merchants` | Read | Merchant-level stats (count, total, avg) |
| GET | `/transactions/{id}` | Read | Get a single transaction |
| GET | `/transactions/{id}/annotations` | Read | List the activity-timeline rows for a transaction (comments, rule applications, tag/category changes). Mirror of MCP `list_annotations`. |
| PATCH | `/transactions/{id}/category` | Write | Set transaction category (override) |
| DELETE | `/transactions/{id}/category` | Write | Reset transaction category to provider default |
| POST | `/transactions/batch-categorize` | Write | Batch categorize multiple transactions (max 500) |
| POST | `/transactions/bulk-recategorize` | Write | Bulk recategorize by filter (server-side UPDATE) |
| POST | `/transactions/update` | Write | Atomic multi-field batch (category + tags + comment per row, max 50 ops) |
| DELETE | `/transactions/{id}` | Write | Soft-delete a transaction (sets `deleted_at`; hidden from all reads). |
| POST | `/transactions/{id}/restore` | Write | Restore a soft-deleted transaction. |

### Transaction Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `account_id` | string | Filter by account |
| `user_id` | string | Filter by user (includes attributed transactions) |
| `category_slug` | string | Filter by category slug |
| `start_date` | string | Start date (YYYY-MM-DD) |
| `end_date` | string | End date (YYYY-MM-DD) |
| `min_amount` | float | Minimum amount |
| `max_amount` | float | Maximum amount |
| `search` | string | Search name/merchant (comma-separated for OR) |
| `exclude_search` | string | Exclude matching name/merchant (min 2 chars) |
| `search_mode` | string | `contains` (default), `words`, `fuzzy` |
| `pending` | bool | Filter by pending status |
| `tags` | string | Comma-separated slugs; result transactions must carry **every** slug (AND) |
| `any_tag` | string | Comma-separated slugs; result transactions must carry **at least one** slug (OR) |
| `sort_by` | string | `date` (default), `amount`, `provider_name` |
| `sort_order` | string | `desc` (default), `asc` |
| `fields` | string | Field selection. Aliases: `minimal`, `core`, `category`, `timestamps` |
| `cursor` | string | Pagination cursor (only with date sort) |
| `limit` | int | Results per page (default 100, max 500) |

## Categories

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/categories` | Read | List all categories (2-level hierarchy) |
| GET | `/categories/export` | Read | Export categories as TSV |
| GET | `/categories/{id}` | Read | Get a single category |
| POST | `/categories` | Write | Create a category |
| PUT | `/categories/{id}` | Write | Update a category |
| DELETE | `/categories/{id}` | Write | Delete a category |
| POST | `/categories/import` | Write | Import categories from TSV |
| POST | `/categories/{id}/merge` | Write | Merge a category into another |

## Users

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/users` | Read | List all family members |

## Connections

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/connections` | Read | List all bank connections |
| GET | `/connections/{id}` | Read | Get full connection detail (status, paused, sync interval, account count) |
| GET | `/connections/{id}/status` | Read | Get connection status and last sync info |
| POST | `/connections/{id}/sync` | Write | Trigger a sync for this single connection |
| POST | `/connections/{id}/paused` | Write | Pause or resume scheduled syncs for a connection |
| POST | `/connections/{id}/sync-interval` | Write | Set or clear a per-connection sync-interval override |
| DELETE | `/connections/{id}` | Write | Soft-disconnect a connection (clears tokens, hides from list) |
| POST | `/connections/{id}/reauth` | Write | Start the provider re-auth flow — returns a fresh link token |
| POST | `/connections/{id}/reauth-complete` | Write | Mark connection active again after the user finishes the re-auth flow |

`{id}` accepts either the connection's UUID or 8-char short_id.

### GET `/connections/{id}`

Full per-connection detail. Returns `200` with the connection record:

```json
{
  "id": "01J...",
  "short_id": "abc12345",
  "user_id": "u1abc234",
  "user_name": "Alice",
  "provider": "plaid",
  "institution_id": "ins_3",
  "institution_name": "Chase",
  "status": "active",
  "error_code": null,
  "error_message": null,
  "last_synced_at": "2026-05-09T15:00:00Z",
  "created_at": "2026-04-01T12:00:00Z",
  "updated_at": "2026-05-09T15:00:00Z",
  "paused": false,
  "sync_interval_override_minutes": null,
  "consecutive_failures": 0,
  "account_count": 3
}
```

A non-existent id returns `404 NOT_FOUND`. Disconnected connections are still returned (with `status: "disconnected"`) — only the `/connections` list filters them out.

### POST `/connections/{id}/sync`

Per-connection variant of `POST /sync` (body-less). The handler returns `202 Accepted` and runs the sync in the background.

```json
{ "status": "sync_triggered" }
```

A non-existent or `disconnected` connection returns `404 NOT_FOUND` (the sync resolver hides disconnected rows).

### POST `/connections/{id}/paused`

Toggle the `paused` flag — paused connections are skipped by the cron scheduler but can still be synced manually via `POST /connections/{id}/sync`.

**Body**

| Field | Type | Description |
|-------|------|-------------|
| `paused` | bool | Required. `true` to pause, `false` to resume. |

Returns `200` with the full connection detail (same shape as `GET /connections/{id}`). `404 NOT_FOUND` if the connection is missing.

### POST `/connections/{id}/sync-interval`

Set or clear the per-connection sync-interval override (minutes). When cleared, the connection falls back to the global default.

**Body**

| Field | Type | Description |
|-------|------|-------------|
| `interval_minutes` | int / null | Minutes between scheduled syncs. Pass `null` (or omit) to clear the override and revert to the global default. Values `<= 0` are treated as a clear. |

Returns `200` with the full connection detail. `404 NOT_FOUND` if the connection is missing.

### DELETE `/connections/{id}`

Soft-disconnect: flips status to `disconnected`, wipes the encrypted access token, and soft-deletes related transactions in a single DB transaction. The row is preserved (FK policy is `SET NULL` for accounts and transactions, `CASCADE` for `sync_logs`) so historical data stays linked.

Returns `204 No Content`. Calling on a missing or already-disconnected connection returns `404 NOT_FOUND` (idempotent at the API surface).

Provider-side credential revocation (e.g. Plaid's `/item/remove`) is **not** performed by the REST endpoint — it is admin-handler-only because the public service layer doesn't carry the provider registry. The connection is unusable from Breadbox's side regardless.

### POST `/connections/{id}/reauth`

Starts a provider re-auth flow for a connection in `pending_reauth` (or any
broken) state. Calls the provider for a short-lived link token; the client
hands the token to the provider's UI (Plaid Link, Teller Connect, etc.) and
calls `/reauth-complete` once the user finishes.

Body: none.

Returns `200`:

```json
{
  "link_token": "link-sandbox-...",
  "expiration": "2026-05-09T16:30:00Z"
}
```

Errors:

- `404 NOT_FOUND` — connection doesn't exist.
- `400 INVALID_PARAMETER` — connection's provider isn't configured on this server.
- `502 PROVIDER_ERROR` — upstream provider call failed.

### POST `/connections/{id}/reauth-complete`

Marks a previously broken connection active again and clears `error_code` /
`error_message`. Call this after the user has completed the provider re-auth
UI started by `/reauth`. Body is ignored — Plaid's OAuth redirect path
exchanges the public token out-of-band, so no payload is required today.

Returns `200`:

```json
{ "status": "active" }
```

Errors:

- `404 NOT_FOUND` — connection doesn't exist.

## Sync

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| POST | `/sync` | Write | Trigger manual sync — all active connections, or one connection if `connection_id` is given |

### Request body

The body is optional. Omit it (or send an empty body) to enqueue **every active** connection for sync. Pass `connection_id` to scope the sync to a single connection.

| Field | Type | Description |
|-------|------|-------------|
| `connection_id` | string | Optional. Connection short_id or UUID. When provided, only that connection is synced. |

```json
{ "connection_id": "abc12345" }
```

The handler returns `202 Accepted` immediately and runs the sync asynchronously; observe progress through `/sync/logs` (when shipped) or by polling `/connections/{id}/status`. A non-existent `connection_id` returns `404 NOT_FOUND`. A connection whose status is `disconnected` is treated as not-found by the sync resolver and likewise returns `404 NOT_FOUND`.

```json
{ "status": "sync_triggered" }
```

### Sync visibility

`POST /sync` is fire-and-forget — these GET endpoints close the loop. They wrap the same data the admin dashboard uses, so REST clients can poll progress, audit prior runs, and read provider health without scraping HTML. All read-scope. Pair them with the connection-management endpoints under `/connections/{id}` for per-connection drilldowns.

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/sync/logs` | Read | Paginated history with filters |
| GET | `/sync/logs/{id}` | Read | Single log + per-account rows |
| GET | `/sync/health` | Read | Aggregate sync health (last 24h) |
| GET | `/sync/health/providers` | Read | Per-provider health summary |
| GET | `/sync/stats` | Read | Aggregate stats matching the same filter set as `/sync/logs` |

#### `GET /sync/logs`

| Query param | Type | Description |
|-------------|------|-------------|
| `connection_id` | string | UUID or short_id. Restrict to one connection. |
| `status` | string | One of `in_progress`, `success`, `error`. |
| `trigger` | string | One of `cron`, `webhook`, `manual`, `initial`. |
| `from` | string (RFC3339) | Lower bound on `started_at` (inclusive). |
| `to` | string (RFC3339) | Upper bound on `started_at` (exclusive). Must be after `from`. |
| `limit` | int | Default 50, max 200. |
| `cursor` | string | Opaque cursor returned by a prior page. Treat as a black box. |

Response (`200 OK`):

```json
{
  "sync_logs": [
    {
      "id": "01J...",
      "connection_id": "01J...",
      "institution_name": "Chase",
      "trigger": "manual",
      "status": "success",
      "added_count": 12,
      "modified_count": 1,
      "removed_count": 0,
      "unchanged_count": 47,
      "started_at": "2026-04-26T12:00:00Z",
      "completed_at": "2026-04-26T12:00:02Z",
      "duration": "2.041s",
      "duration_ms": 2041,
      "accounts_affected": 3
    }
  ],
  "next_cursor": "eyJwIjoyfQ",
  "has_more": true,
  "limit": 50,
  "total": 173
}
```

`error_message`, `friendly_error_message`, and `warning_message` are present only when populated. The list omits the per-rule breakdown — fetch a single log to see `rule_hits`. `400 INVALID_PARAMETER` is returned for malformed filter values; `400 INVALID_CURSOR` for a cursor that fails to decode.

#### `GET /sync/logs/{id}`

Path id must be the sync log UUID (sync logs do not expose a short_id alias on the REST surface). Response embeds the per-account breakdown plus the per-rule hit counts:

```json
{
  "id": "01J...",
  "connection_id": "01J...",
  "institution_name": "Chase",
  "provider": "plaid",
  "trigger": "manual",
  "status": "success",
  "added_count": 12,
  "modified_count": 1,
  "removed_count": 0,
  "unchanged_count": 47,
  "started_at": "2026-04-26T12:00:00Z",
  "completed_at": "2026-04-26T12:00:02Z",
  "duration": "2.041s",
  "accounts_affected": 3,
  "rule_hits": [
    { "rule_id": "01J...", "rule_name": "Coffee → Food & Drink", "count": 4 }
  ],
  "total_rule_hits": 4,
  "accounts": [
    {
      "id": "01J...",
      "sync_log_id": "01J...",
      "account_id": "01J...",
      "account_name": "Checking",
      "added_count": 8,
      "modified_count": 1,
      "removed_count": 0,
      "unchanged_count": 30
    }
  ]
}
```

`404 NOT_FOUND` when the id doesn't resolve.

#### `GET /sync/health`

Aggregate over the last 24h, plus the most recent sync's status and the overall verdict (`healthy`, `degraded`, `unhealthy`). Useful as a single-shot dashboard probe.

```json
{
  "overall_health": "healthy",
  "last_sync_time": "5 minutes ago",
  "last_sync_status": "success",
  "recent_sync_count": 12,
  "recent_success_rate": 100.0,
  "recent_error_count": 0,
  "connection_errors": 0,
  "next_sync_time": ""
}
```

`last_sync_time` is a human-readable relative timestamp ("5 minutes ago"), not RFC3339 — it mirrors what the admin dashboard renders. Pair with `/sync/logs?limit=1` if you need an absolute timestamp.

#### `GET /sync/health/providers`

Per-provider snapshot. Keyed by provider type (`plaid`, `teller`, `csv`):

```json
{
  "providers": {
    "plaid": {
      "provider": "plaid",
      "connection_count": 3,
      "account_count": 7,
      "last_sync_status": "success",
      "last_sync_time": "5 minutes ago"
    },
    "teller": {
      "provider": "teller",
      "connection_count": 1,
      "account_count": 2,
      "last_sync_status": "error",
      "last_sync_time": "1 hour ago",
      "last_sync_error": "401 unauthorized"
    }
  }
}
```

Disconnected connections are excluded from the connection / account counts (they don't sync).

#### `GET /sync/stats`

Same filter set as `/sync/logs` (`connection_id`, `status`, `trigger`, `from`, `to`). Returns aggregate counters for the matching slice — useful for "out of N runs matching this filter, X succeeded" UIs without paginating the full list.

```json
{
  "total_syncs": 173,
  "success_count": 168,
  "error_count": 5,
  "warning_count": 2,
  "success_rate": 97.11,
  "avg_duration_ms": 1842.5,
  "total_added": 4203,
  "total_modified": 87,
  "total_removed": 12,
  "total_unchanged": 18230
}
```

### POST `/transactions/update`

Atomic multi-field batch. Each operation can set a category (or clear an override), add/remove tags, and attach a comment — all atomic per transaction. REST sibling of the MCP `update_transactions` tool.

**Body**

| Field | Type | Description |
|-------|------|-------------|
| `operations` | array | Required. Up to 50 ops. |
| `on_error` | string | `"continue"` (default — each op runs in its own DB tx, partial failures don't undo successful items) or `"abort"` (whole batch is one DB tx, rolls back on first error). |

Each operation:

| Field | Type | Description |
|-------|------|-------------|
| `transaction_id` | string | Required. UUID or short_id. |
| `category_slug` | string | Optional. Sets `category_override=true`. Mutually exclusive with `reset_category`. |
| `reset_category` | bool | Optional. Clears the override and drops the transaction back to `uncategorized` so rules can re-categorize. |
| `tags_to_add` | array | `[{"slug":"..."}]`. Auto-creates the tag if the slug is not yet registered. |
| `tags_to_remove` | array | `[{"slug":"..."}]`. Unknown slugs are a no-op. |
| `comment` | string | Optional annotation, attributed to your API key. Max 10000 chars. |

**Response** `200 OK`

```json
{
  "results": [
    {"transaction_id": "k7Xm9pQ2", "status": "ok"},
    {"transaction_id": "x4Lz1mNa", "status": "error", "error": {"code": "NOT_FOUND", "message": "..."}}
  ],
  "succeeded": 1,
  "failed": 1
}
```

Per-op errors are reported inside `results[]`; the top-level response is still `200`. The whole call returns `400 INVALID_PARAMETER` only on malformed input (empty `operations`, more than 50, bad `on_error`). In `abort` mode a partial-batch failure rolls back the DB transaction and the response includes `aborted: true` plus the partial per-op outcomes.

### DELETE `/transactions/{id}` and POST `/transactions/{id}/restore`

Soft-delete and undo. `DELETE /transactions/{id}` sets the row's `deleted_at` timestamp; every read endpoint (list, get, summary, merchants, count, …) filters on `deleted_at IS NULL`, so a deleted transaction immediately disappears from all responses. The DB row is preserved so `POST /transactions/{id}/restore` can clear `deleted_at` and bring it back.

Both endpoints return `204 No Content` on success and write a `transaction_deleted` / `transaction_restored` annotation on the activity timeline attributed to the calling API key.

Both are idempotent at the API surface — a no-op returns `404 NOT_FOUND`:

- `DELETE` on a transaction that doesn't exist or is already soft-deleted → `404`.
- `POST /restore` on a transaction that doesn't exist or isn't currently soft-deleted → `404`.

Path id accepts either a UUID or short_id for live (non-deleted) rows. Restore on a soft-deleted row must use the UUID — the short_id resolver itself filters on `deleted_at IS NULL` and won't find a deleted row by its short id.

## Transaction Comments

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/transactions/{id}/comments` | Read | List comments on a transaction |
| POST | `/transactions/{id}/comments` | Write | Add a comment to a transaction |
| PUT | `/transactions/{id}/comments/{comment_id}` | Write | Update a comment |
| DELETE | `/transactions/{id}/comments/{comment_id}` | Write | Delete a comment |

## Transaction Annotations

`GET /transactions/{id}/annotations` returns the activity-timeline rows for a single transaction — comments, rule applications, tag adds/removes, and category sets. Same payload as the MCP `list_annotations` tool, wrapped in a `{ "annotations": [...] }` envelope. The rendering contract (dedup, soft-delete tombstones, system-event kinds) is documented in `docs/activity-timeline.md`.

Path id accepts either a UUID or short_id (the same resolver as the rest of the transaction endpoints).

| Query param | Type | Description |
|-------------|------|-------------|
| `kind` | string (repeatable, comma-separated) | Filter by raw DB kind: `comment`, `rule_applied`, `tag_added`, `tag_removed`, `category_set`, `sync_started`, `sync_updated`. Both `?kind=comment&kind=rule_applied` and `?kind=comment,rule_applied` are accepted. |
| `actor_type` | string (repeatable, comma-separated) | Filter by actor type: `user`, `agent`, `system`. Same repeatable / comma-separated handling as `kind`. |
| `since` | string (RFC3339) | Return only rows created strictly after this timestamp. Pair with `limit` to bound a delta read. |
| `limit` | int | Cap the returned rows to the most recent N (timeline tail), still ordered ASC. `0` (default) returns the full timeline; the server caps at `200`. Negative values are rejected. |
| `raw` | bool | When `true`, bypass enrichment and dedup. Returns the unmodified DB view — rule-source duplicates and same-actor adjacent comment-vs-tag-note pairs survive, and the derived `summary` / `action` / `subject` fields are empty. |

Example:

```bash
curl -H "X-API-Key: $BB_API_KEY" \
  "https://breadbox.example.com/api/v1/transactions/abc12345/annotations?kind=comment&limit=5"
```

```json
{
  "annotations": [
    {
      "id": "01J...",
      "short_id": "ann7zk2x",
      "transaction_id": "01J...",
      "kind": "comment",
      "actor_type": "user",
      "actor_name": "Alice",
      "content": "needs receipt",
      "created_at": "2026-04-26T12:00:00Z"
    }
  ]
}
```

A 400 `INVALID_PARAMETER` is returned for malformed `since` timestamps or non-numeric / negative `limit` values; 404 `NOT_FOUND` is returned when the path id can't be resolved (the same MCP-shared limitation applies — a syntactically valid but unknown UUID returns an empty list rather than 404).

## Tags & Reviews

The review queue is a tag. Transactions carrying the seeded `needs-review` tag (or any operator-defined trigger tag) are the backlog. A seeded `on_create` system rule auto-attaches `needs-review` to every newly-synced transaction; disable that rule to opt out. When removing a tag, passing a rationale `note` is optional — if provided, it's recorded on the `tag_removed` annotation.

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/tags` | Read | List all registered tags |
| POST | `/tags` | Write | Create a tag. Body: `{"slug":"...","display_name":"...","description":"...","color":"#abc123","icon":"tag"}`. Slug must match `^[a-z0-9][a-z0-9\-:]*[a-z0-9]$`. Returns `409 SLUG_CONFLICT` if the slug is already registered. |
| GET | `/tags/{slug}` | Read | Get a single tag by UUID, short_id, or slug |
| PATCH | `/tags/{slug}` | Write | Partial update — every field optional: `{"display_name":"...","description":"...","color":"...","icon":"...","lifecycle":"persistent\|ephemeral"}`. Slug is immutable. |
| DELETE | `/tags/{slug}` | Write | Delete a tag. Cascades to `transaction_tags`; annotations referencing the tag retain `tag_id=NULL`. Returns `204 No Content`. |
| POST | `/transactions/{id}/tags` | Write | Attach a tag to a transaction (body: `{"slug":"...","note":"..."}`). Auto-creates the tag if the slug is not yet registered. Idempotent — returns `already_present: true` on repeat calls. |
| DELETE | `/transactions/{id}/tags/{slug}` | Write | Detach a tag from a transaction. Optional `?note=...` or JSON body `{"note":"..."}` recorded on the `tag_removed` annotation. Idempotent — returns `already_absent: true` when the tag isn't attached. |

For filtering, pass `tags=slug1,slug2` (AND) or `any_tag=slug1,slug2` (OR) to `/transactions` and `/transactions/count`.

Additional tag-touching operations exposed via MCP: `list_tags`, `add_transaction_tag`, `remove_transaction_tag`, `create_tag`, `update_tag`, `delete_tag`, `update_transactions`, `list_annotations`. The admin dashboard covers the same ground at `/tags`, `/transactions/:id/edit`, and bulk actions on `/transactions`.

## Transaction Rules

Rules auto-categorize transactions during sync by matching conditions on transaction fields.

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/rules` | Read | List all rules with filters |
| GET | `/rules/{id}` | Read | Get a single rule |
| POST | `/rules` | Write | Create a rule |
| PUT | `/rules/{id}` | Write | Update a rule |
| DELETE | `/rules/{id}` | Write | Delete a rule |
| POST | `/rules/{id}/apply` | Write | Apply a single rule retroactively |
| POST | `/rules/apply-all` | Write | Apply all active rules retroactively |
| POST | `/rules/preview` | Write | Dry-run a condition against existing transactions |

### Rule Condition Structure

Rules use a recursive JSON condition tree supporting AND/OR/NOT logic:

```json
{
  "type": "and",
  "conditions": [
    { "field": "provider_name", "operator": "contains", "value": "AMAZON" },
    { "field": "amount", "operator": "gt", "value": 50 }
  ]
}
```

**Available fields:** `provider_name`, `provider_merchant_name`, `amount`, `provider_category_primary`, `provider_category_detailed`, `pending`, `provider`, `account_id`, `user_id`, `user_name`

**String operators:** `eq`, `neq`, `contains`, `not_contains`, `matches` (regex), `in`

**Numeric operators:** `eq`, `neq`, `gt`, `gte`, `lt`, `lte`

**Boolean operators:** `eq`, `neq`

### Pipeline stage (priority)

`POST /rules` and `PUT /rules/{id}` accept either `stage` (semantic) or `priority` (raw integer) on the request body:

| Field | Type | Description |
|-------|------|-------------|
| `stage` | string | `baseline` / `standard` / `refinement` / `override`. Resolves to priority `0 / 10 / 50 / 100`. |
| `priority` | int | Raw pipeline-stage integer, `0..1000`. Lower runs first. |

If both are supplied, `priority` wins. If neither is supplied on create, the rule defaults to `standard` (priority `10`). Stage names are case-insensitive; unknown stage strings return `400 VALIDATION_ERROR`. See [`docs/rule-dsl.md`](rule-dsl.md) for the full priority-as-pipeline-stage model.

## Account Links

Account links connect dependent (authorized user) accounts to primary (cardholder) accounts for cross-connection transaction deduplication.

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/account-links` | Read | List all account links |
| GET | `/account-links/{id}` | Read | Get a single link with match stats |
| GET | `/account-links/{id}/matches` | Read | List matched transaction pairs |
| POST | `/account-links` | Write | Create a link (auto-runs initial reconciliation) |
| PUT | `/account-links/{id}` | Write | Update a link |
| DELETE | `/account-links/{id}` | Write | Delete a link |
| POST | `/account-links/{id}/reconcile` | Write | Re-run matching for a link |
| POST | `/transaction-matches/{id}/confirm` | Write | Confirm a matched pair |
| POST | `/transaction-matches/{id}/reject` | Write | Reject a matched pair |
| POST | `/transaction-matches/manual` | Write | Manually match two transactions |

## Agent Reports

AI agents can submit summaries and flag transactions for human review.

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/reports` | Read | List all reports (bare array, bounded) |
| GET | `/reports/unread-count` | Read | Count of unread reports |
| POST | `/reports` | Write | Submit a report |
| PATCH | `/reports/{id}/read` | Write | Mark a report as read |

### `POST /reports` body

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `title` | string | Yes | Short headline shown in the reports list. |
| `body` | string | Yes | Markdown body. May include transaction references, tables, etc. |
| `priority` | string | No | `low`, `normal` (default), `high`. Surfaced in the admin list view. |
| `tags` | string[] | No | Free-form labels for filtering (e.g., `["monthly-summary", "groceries"]`). |
| `author` | string | No | Override display name for the report author. Defaults to the authenticated actor (API key name). |

### `POST /rules/apply-all` response

```json
{
  "rules_applied": { "<rule_id>": 17, "<rule_id>": 4 },
  "total_affected": 21
}
```

`rules_applied` is an object keyed by rule ID, mapping to the number of transactions updated by that rule in this retroactive pass. Order is not guaranteed. `total_affected` is the sum across all rules.

## OAuth / MCP Auth

OAuth 2.1 endpoints for MCP client authentication:

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/.well-known/oauth-authorization-server` | No | OAuth server metadata |
| GET | `/.well-known/oauth-protected-resource` | No | Protected resource metadata |
| POST | `/oauth/token` | No | Token exchange |
| POST | `/oauth/register` | No | Dynamic client registration |

## List Response Envelopes

Two shapes, picked per-resource:

- **Bounded resources** (small by design: accounts, users, connections, categories, reports) return a **bare JSON array**:

  ```json
  [ { "id": "...", ... }, { "id": "...", ... } ]
  ```

- **Paginated resources** (transactions, rules) return a **resource-keyed envelope** with cursor pagination:

  ```json
  { "transactions": [...], "next_cursor": "eyJ...", "has_more": true, "limit": 100 }
  ```

  ```json
  { "rules": [...], "next_cursor": "eyJ...", "has_more": true, "total": 248 }
  ```

The list key matches the resource name. Pass the `next_cursor` value as the `cursor` query parameter to fetch the next page. Cursor pagination only works with the default date sort on transactions.

## Error Format

All errors return a JSON envelope:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Human-readable description"
  }
}
```

Error codes use `UPPER_SNAKE_CASE`. Common codes: `VALIDATION_ERROR`, `NOT_FOUND`, `UNAUTHORIZED`, `FORBIDDEN`, `RATE_LIMITED`, `INTERNAL_ERROR`.

## Rate limiting

All `/api/v1/*` endpoints are rate-limited per API key using a token bucket. `/health/*` and `/api/v1/version` are exempt (used by load balancers and monitoring).

Defaults: **120 requests/minute, burst 60**. Override with the `API_RATE_LIMIT_RPM` and `API_RATE_LIMIT_BURST` environment variables at server startup. Unauthenticated requests (no valid `X-API-Key` or `Authorization: Bearer`) fall back to bucketing by client IP.

Every response includes:

| Header | Meaning |
|--------|---------|
| `X-RateLimit-Limit` | Bucket capacity (burst). |
| `X-RateLimit-Remaining` | Tokens left after this request. |
| `X-RateLimit-Reset` | Epoch seconds when the bucket fully refills. |

Over-limit requests return `429 Too Many Requests` with `code: "RATE_LIMITED"` and a `Retry-After` header (seconds to wait before retrying):

```json
{
  "error": {
    "code": "RATE_LIMITED",
    "message": "Rate limit exceeded; retry after 1s"
  }
}
```

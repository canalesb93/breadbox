---
paths:
  - "internal/api/**"
  - "internal/admin/**"
  - "internal/middleware/**"
---

# REST API & admin handlers

## Error envelope

All error responses use:

```json
{ "error": { "code": "UPPER_SNAKE_CASE", "message": "Human-readable description" } }
```

- Codes are **stable contracts** — treat them as API surface. Don't rename on a whim.
- REST handlers use `internal/api/response.go` helpers (`writeError`, `writeJSON`). Admin handlers use `internal/admin/response.go` equivalents. Don't hand-roll either.
- 4xx for client errors, 5xx only for actual server bugs. Validation errors on the public REST API return **`400`** with either `VALIDATION_ERROR` or a more specific code like `INVALID_PARAMETER` / `INVALID_CURSOR` / `INVALID_DATE_RANGE`. Admin form submissions under `/admin/api/*` return `422 VALIDATION_ERROR` instead (classic browser-form convention) — don't conflate the two.

## Auth

### API keys (REST `/api/v1/*`)

- Header: `X-API-Key: bb_xxxxx`.
- Format: `bb_` prefix + base62 body (32 random bytes).
- Stored as SHA-256 hex hash. Prefix stored separately for UI display. `revoked_at` for soft-revoke.
- `scope` column: `full_access` or `read_only`. `middleware.RequireWriteScope()` blocks read-only keys from write endpoints.
- Full key record exposed to handlers via `middleware.SetAPIKey()` / `GetAPIKey()` for actor attribution.

### Admin sessions (`/` and `/-/*`)

- `alexedwards/scs` session manager with `pgxstore`.
- Cookies: `HttpOnly; SameSite=Lax; Secure`.
- CSRF token via `internal/admin/csrf.go` on state-changing forms.
- First-run: `CountAdminAccounts == 0` → redirect to `/setup`. No separate flag.

## Field selection

`?fields=` query param on list and detail endpoints. Supports individual field names and aliases:
- `minimal`, `core`, `category`, `timestamps`, `triage`, `review_core`, `transaction_core`.
- `id` and `short_id` **always** included.

Filtering happens in handlers via `service.FilterFields(response, parsedFields)` — the service method returns the full struct.

## Dynamic filters

Query endpoints (`/api/v1/transactions`, `/rules`, `/reviews`, `/merchants`) share a filter vocabulary built on positional `$N` SQL. See `internal/service/` for the composable filter builders. Handlers just parse query params and pass them in.

## List response envelopes

Two shapes, picked per-resource:

- **Bounded resources** (households are small: accounts, users, connections, categories, agent reports) return a **bare JSON array**. No pagination metadata, no wrapper.

  ```json
  [ { "id": "...", ... }, { "id": "...", ... } ]
  ```

- **Paginated resources** (transactions, rules, reviews, merchants, sync logs) return a **resource-keyed object**:

  ```json
  { "transactions": [...], "next_cursor": "opaque-string-or-null", "has_more": false, "limit": 50 }
  ```

  The list key matches the resource (`transactions`, `rules`, `reviews`, etc.). Pagination metadata fields (`next_cursor`, `has_more`, `total`, `limit`) are included when meaningful for that endpoint.

There is **no** `{ "data": [...] }` envelope. Old drafts of `docs/rest-api.md` showed one; reality is resource-keyed. When adding a new paginated list endpoint, pick a resource key and follow the paginated shape above.

## Pagination

Cursor pagination (not OFFSET) for the public REST API. Pass `cursor=...` from a previous response's `next_cursor` to fetch the next page. Only works with the default sort (date DESC) for transactions.

`TransactionRuleListResult` additionally carries offset fields (`page`, `page_size`, `total_pages`) — these are populated **only** when the admin UI calls the service layer with `Page > 0` and are consumed by the admin template, not emitted by the REST handler. The public `/api/v1/rules` endpoint uses cursor pagination exclusively; the offset fields stay zero-valued and are elided via `omitempty`.

## Search modes

`search_mode` param on `query_transactions`, `count_transactions`, `merchant_summary`, `list_transaction_rules`:
- `contains` (default) — substring `ILIKE`.
- `words` — split on spaces, AND all words. Handles "Century Link" vs "CenturyLink".
- `fuzzy` — pg_trgm similarity. Typo-tolerant.

Comma-separated values in `search` are auto-ORed in all modes. Shared impl in `internal/service/search.go`.

`exclude_search` (minimum 2 chars) adds `NOT ILIKE` on name and merchant_name. Useful for hunting unknown charges.

## Actor pattern

Write endpoints attribute changes to an actor (user, agent, system). `middleware.Actor(r)` returns `{Type, ID, Name}` pulled from the session or API key. Pass it to service methods that create rules, submit reports, etc.

## Health endpoints

Split for orchestrators:
- `GET /health/live` — basic HTTP 200, no dependencies.
- `GET /health/ready` — DB ping + scheduler status.

## Admin-only endpoints

Prefixed `/-/` to disambiguate from user-facing routes. Examples: `POST /-/reports/{id}/read`, `POST /-/reports/{id}/unread`, `POST /-/reports/read-all`. CSRF + session required.

## Webhooks

- Path: `/webhooks/:provider`.
- No auth middleware — each provider handler verifies HMAC / signature internally.
- Enqueue to `webhook_events` table; sync engine consumes. Don't block the HTTP response on the actual sync.

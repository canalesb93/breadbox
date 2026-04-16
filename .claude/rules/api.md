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
- Use `internal/api/response.go` helpers (`writeError`, `writeJSON`) — don't hand-roll.
- 4xx for client errors, 5xx only for actual server bugs. Validation errors are 400 with a specific code like `INVALID_DATE_RANGE`.

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

## Pagination

Cursor pagination (not OFFSET) for transactions, rules, reviews. Response envelope:

```json
{ "data": [...], "next_cursor": "opaque-string-or-null" }
```

Pass `cursor=...` on the next request. Only works with default sort (date DESC) for transactions.

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

Prefixed `/-/` to disambiguate from user-facing routes. Examples: `POST /-/reports/{id}/read`, `POST /-/reports/read-all`. CSRF + session required.

## Webhooks

- Path: `/webhooks/:provider`.
- No auth middleware — each provider handler verifies HMAC / signature internally.
- Enqueue to `webhook_events` table; sync engine consumes. Don't block the HTTP response on the actual sync.

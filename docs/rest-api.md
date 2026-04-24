# Breadbox REST API Specification

**Version:** 0.1.0

> **Note:** This document covers the core API endpoints (health, accounts, transactions, users, connections, sync). For a complete list of all endpoints including comments, rules, account links, reports, and batch operations, see the [API Quick Reference](api-reference.md).

---

## Table of Contents

1. [API Overview](#1-api-overview)
2. [Authentication](#2-authentication)
3. [Pagination](#3-pagination)
4. [Error Handling](#4-error-handling)
5. [Endpoint Specifications](#5-endpoint-specifications)
   - [Health](#51-health)
   - [Accounts](#52-accounts)
   - [Transactions](#53-transactions)
   - [Users](#54-users)
   - [Connections](#55-connections)
   - [Sync](#56-sync)
6. [Filtering and Search](#6-filtering-and-search)
7. [Response Schemas](#7-response-schemas)
8. [Admin API](#8-admin-api)

---

## 1. API Overview

Breadbox is a self-hosted financial data aggregation service. The REST API provides structured, queryable access to bank accounts, transactions, balances, and connection status. It is the canonical data layer consumed by the admin dashboard, the MCP server, and any external clients (AI agents, scripts, dashboards).

### Base URL

```
/api/v1/
```

All REST API endpoints are prefixed with `/api/v1/` except the health check (`/health`) and the admin API (`/admin/api/`).

### Content Type

All requests and responses use `application/json`. Clients must include the header:

```
Content-Type: application/json
```

on any request with a body (e.g., `POST /api/v1/sync`). Requests without a body do not need this header.

### Amount Convention

Amounts follow the **Plaid convention**:

- **Positive amount** = money flowing out of the account (debit, purchase, fee)
- **Negative amount** = money flowing into the account (credit, deposit, refund)

This convention is preserved as-is from the data source. Clients should not assume amounts have been normalized. All amounts are represented as decimal numbers with up to two decimal places (e.g., `42.50`, `-1500.00`).

Currency is always provided alongside an amount via the `iso_currency_code` field (e.g., `"USD"`). Clients must never silently aggregate amounts across different currencies.

### Versioning

The current API version is `v1`. Breaking changes will increment this version. Non-breaking additions (new optional fields, new optional query parameters) may be made without a version increment.

---

## 2. Authentication

### API Key Authentication

All `/api/v1/` endpoints require authentication via an API key passed in the `X-API-Key` request header.

```
X-API-Key: bb_4xK9mZ2nQpR7sT1vW3yA5bC8dE6fG0h
```

#### Key Format

API keys have the format:

```
bb_<base62-encoded-32-random-bytes>
```

- Prefix: `bb_` (identifies the token type)
- Body: 32 cryptographically random bytes, base62-encoded (characters `0-9`, `A-Z`, `a-z`)
- Total length: approximately 46 characters (prefix + ~43 base62 characters)

Example: `bb_4xK9mZ2nQpR7sT1vW3yA5bC8dE6fG0hJqLnPuVw`

#### Key Storage

API keys are never stored in plaintext. The database stores:

| Field | Description |
|---|---|
| `id` | UUID, primary key |
| `prefix` | The `bb_` prefix plus the first 8 characters of the key body (e.g., `bb_4xK9mZ2n`). Used to identify a key in the admin UI without exposing the full key. |
| `key_hash` | SHA-256 hash of the full key string (hex-encoded). Used for constant-time comparison on each request. |
| `name` | Human-readable label (e.g., `"Home Assistant"`) |
| `created_at` | ISO 8601 timestamp |
| `last_used_at` | ISO 8601 timestamp, nullable |
| `revoked_at` | ISO 8601 timestamp, nullable. Non-null means the key is inactive. |

#### Key Lifecycle

1. A key is created via `POST /admin/api/api-keys`.
2. The **full plaintext key is returned exactly once** in the creation response. It cannot be retrieved again.
3. All subsequent API requests include the full plaintext key in the `X-API-Key` header. Breadbox hashes the incoming key and compares it to the stored hash. The auth middleware validates: `WHERE key_hash = $hash AND revoked_at IS NULL`.
4. A key is revoked via `DELETE /admin/api/api-keys/:id`. Revocation sets the `revoked_at` timestamp to the current time. Revoked keys are rejected immediately.

#### Rate Limiting

The MVP does not enforce rate limits on API key-authenticated endpoints. This may be revisited post-MVP. Clients should implement reasonable request rates and use pagination rather than issuing many small requests.

#### Authentication Errors

| Scenario | HTTP Status | Error Code |
|---|---|---|
| `X-API-Key` header is missing | `401 Unauthorized` | `MISSING_API_KEY` |
| Key format is invalid (does not start with `bb_`) | `401 Unauthorized` | `INVALID_API_KEY` |
| Key does not match any stored hash | `401 Unauthorized` | `INVALID_API_KEY` |
| Key exists but has been revoked | `401 Unauthorized` | `REVOKED_API_KEY` |

---

## 3. Pagination

List endpoints that can return large result sets use **cursor-based pagination**. Offset-based pagination (e.g., `page=2`) is not supported.

### Why Cursor-Based

Cursor pagination is stable: inserting or deleting records between pages does not cause items to be skipped or duplicated. This is important for transaction data, which is frequently modified by background syncs.

### Request Parameters

| Parameter | Type | Required | Default | Max | Description |
|---|---|---|---|---|---|
| `cursor` | string | No | — | — | Opaque pagination cursor from a previous response's `next_cursor` field. Omit to fetch the first page. |
| `limit` | integer | No | `100` | `500` | Number of records to return per page. |

### Cursor Format

The cursor is an **opaque, base64url-encoded string**. Clients must treat it as opaque — do not attempt to parse or construct cursors manually. Internally, a cursor encodes a `(timestamp, id)` pair that allows the database query to resume from the correct position using a keyset condition:

```sql
WHERE (date, id) < ($cursor_date, $cursor_id)
ORDER BY date DESC, id DESC
```

Cursors are not guaranteed to be stable across schema changes or server restarts, though in practice they will remain valid as long as the referenced row exists. If a cursor becomes invalid, the server returns a `400` error with code `INVALID_CURSOR`.

### Response Shape

Paginated list responses use a **resource-keyed** envelope. The list key matches the resource name:

```json
{
  "transactions": [ ... ],
  "next_cursor": "eyJkYXRlIjoiMjAyNS0xMi0xNSIsImlkIjoiYWJjZGVmIn0",
  "has_more": true,
  "limit": 50
}
```

Rules use `"rules"`:

```json
{ "rules": [ ... ], "next_cursor": "...", "has_more": true, "total": 248 }
```

| Field | Type | Description |
|---|---|---|
| `<resource>` | array | The page of results. May be empty. Key name matches the resource (`transactions`, `rules`, etc.). |
| `next_cursor` | string \| null | Cursor to pass as `cursor` in the next request. `null` or omitted when there are no more results. |
| `has_more` | boolean | `true` if additional pages exist. Equivalent to `next_cursor != null`. |
| `limit` | integer | Echoed back so clients can detect when their requested limit was clamped (transactions only). |
| `total` | integer | Total count across all pages (rules only). Not populated for high-cardinality resources where counting is expensive. |

**Bounded resources** — `GET /api/v1/accounts`, `/users`, `/connections`, `/categories`, `/reports` — are not paginated and return a **bare JSON array** rather than an envelope. See each endpoint's example in [Section 5](#5-endpoint-specifications).

### Fetching All Pages

To retrieve all results, keep fetching with the returned `next_cursor` until `has_more` is `false`:

```
GET /api/v1/transactions?limit=500
GET /api/v1/transactions?limit=500&cursor=<next_cursor from page 1>
GET /api/v1/transactions?limit=500&cursor=<next_cursor from page 2>
... (until has_more is false)
```

---

## 4. Error Handling

All errors return a consistent JSON body with an HTTP status code in the 4xx or 5xx range.

### Error Response Format

```json
{
  "error": {
    "code": "MACHINE_READABLE_CODE",
    "message": "A human-readable description of what went wrong."
  }
}
```

| Field | Type | Description |
|---|---|---|
| `error.code` | string | Stable, machine-readable error code in `UPPER_SNAKE_CASE`. Safe to match in client code. |
| `error.message` | string | Human-readable description. May change without notice; do not parse this field programmatically. |

### Standard Error Codes

| HTTP Status | Code | Description |
|---|---|---|
| `400 Bad Request` | `INVALID_PARAMETER` | A query parameter or request body field has an invalid value (wrong type, out of range, bad format). The `message` field identifies which parameter. |
| `400 Bad Request` | `INVALID_BODY` | The request body is not valid JSON. |
| `400 Bad Request` | `INVALID_CURSOR` | The provided `cursor` value is malformed or no longer valid. |
| `400 Bad Request` | `VALIDATION_ERROR` | A required field is missing, empty, or fails semantic validation (e.g., unknown category slug, missing rule name). The `message` field describes the issue. |
| `401 Unauthorized` | `MISSING_API_KEY` | `X-API-Key` header was not provided. |
| `401 Unauthorized` | `INVALID_API_KEY` | The provided key is malformed or does not match any active key. |
| `401 Unauthorized` | `REVOKED_API_KEY` | The key exists but has been revoked. |
| `403 Forbidden` | `INSUFFICIENT_SCOPE` | Read-only API key attempted a write endpoint. |
| `404 Not Found` | `NOT_FOUND` | The requested resource does not exist. |
| `409 Conflict` | `SYNC_IN_PROGRESS` | A sync is already running for the requested connection. |
| `500 Internal Server Error` | `INTERNAL_ERROR` | An unexpected server-side error. Check server logs. |
| `503 Service Unavailable` | `DATABASE_UNAVAILABLE` | The database connection is not healthy. |

> **Note:** Validation failures on the public `/api/v1/*` API return `400`, not `422`. Admin form submissions under `/admin/api/*` return `422` for form-validation failures (classic browser-form convention) — see [Section 8](#8-admin-api).

### Example Error Response

```json
{
  "error": {
    "code": "INVALID_PARAMETER",
    "message": "Query parameter 'limit' must be an integer between 1 and 500, got: 'two-hundred'."
  }
}
```

---

## 5. Endpoint Specifications

---

### 5.1 Health

Three health check endpoints are available. All are **unauthenticated**.

#### `GET /health` (alias: `GET /health/live`)

Basic liveness probe. Returns 200 if the HTTP server is running.

**Response — 200 OK**

```json
{
  "status": "ok",
  "version": "0.1.0"
}
```

#### `GET /health/ready`

Deep readiness probe. Verifies DB connectivity and scheduler status. Suitable for Docker/Kubernetes readiness checks.

**Response — 200 OK**

```json
{
  "status": "ok",
  "db": "ok",
  "scheduler": "running",
  "version": "0.1.0"
}
```

**Response — 503 Service Unavailable**

If the database is not reachable or scheduler is not running:

```json
{
  "status": "degraded",
  "db": "error",
  "db_error": "connection refused",
  "scheduler": "running",
  "version": "0.1.0"
}
```

| Field | Type | Description |
|---|---|---|
| `status` | string | `"ok"` when all checks pass, `"degraded"` otherwise. |
| `db` | string | `"ok"` or `"error"`. Only on `/health/ready`. |
| `db_error` | string | Error details when `db` is `"error"`. Only on `/health/ready`. |
| `scheduler` | string | `"running"` or `"stopped"`. Only on `/health/ready`. |
| `version` | string | Semver string of the running Breadbox binary. |

---

### 5.2 Accounts

#### `GET /api/v1/accounts`

Returns a list of all bank accounts known to Breadbox, including their current balances. Accounts are returned in alphabetical order by institution name, then by account name.

This endpoint returns **all accounts** without pagination — the total number of accounts for a household is expected to be small (typically under 20). Soft-deleted accounts are excluded.

**Authentication:** `X-API-Key` required.

**Query Parameters**

| Parameter | Type | Required | Default | Description |
|---|---|---|---|---|
| `user_id` | UUID | No | — | Filter to accounts assigned to a specific family member. If omitted, all accounts are returned. |

**Response — 200 OK**

Returns a bare JSON array — this endpoint does not paginate.

```json
[
  {
    "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "connection_id": "c9d8e7f6-a5b4-3210-fedc-ba0987654321",
    "user_id": "u1a2b3c4-d5e6-f789-0abc-def123456789",
    "name": "Platinum Checking",
    "official_name": "Chase Total Checking®",
    "type": "depository",
    "subtype": "checking",
    "mask": "4821",
    "iso_currency_code": "USD",
    "balance_current": 3241.87,
    "balance_available": 3191.87,
    "balance_limit": null,
    "institution_name": "Chase",
    "created_at": "2025-11-01T09:00:00Z",
    "updated_at": "2026-02-28T14:30:00Z"
  },
  {
    "id": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
    "connection_id": "c9d8e7f6-a5b4-3210-fedc-ba0987654321",
    "user_id": "u1a2b3c4-d5e6-f789-0abc-def123456789",
    "name": "Sapphire Reserve",
    "official_name": "Chase Sapphire Reserve®",
    "type": "credit",
    "subtype": "credit card",
    "mask": "7743",
    "iso_currency_code": "USD",
    "balance_current": 1842.50,
    "balance_available": 18157.50,
    "balance_limit": 20000.00,
    "institution_name": "Chase",
    "created_at": "2025-11-01T09:00:00Z",
    "updated_at": "2026-02-28T14:30:00Z"
  }
]
```

**Error Cases**

| Condition | Status | Code |
|---|---|---|
| Missing API key | `401` | `MISSING_API_KEY` |
| Invalid API key | `401` | `INVALID_API_KEY` |
| `user_id` is not a valid UUID | `400` | `INVALID_PARAMETER` |
| `user_id` does not match any user (returns empty, not error) | `200` | — |

---

#### `GET /api/v1/accounts/:id`

Returns a single account by its Breadbox UUID, including full balance details.

**Authentication:** `X-API-Key` required.

**Path Parameters**

| Parameter | Type | Description |
|---|---|---|
| `id` | UUID | The Breadbox account UUID. |

**Response — 200 OK**

```json
{
  "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "connection_id": "c9d8e7f6-a5b4-3210-fedc-ba0987654321",
  "user_id": "u1a2b3c4-d5e6-f789-0abc-def123456789",
  "name": "Platinum Checking",
  "official_name": "Chase Total Checking®",
  "type": "depository",
  "subtype": "checking",
  "mask": "4821",
  "iso_currency_code": "USD",
  "balance_current": 3241.87,
  "balance_available": 3191.87,
  "balance_limit": null,
  "institution_name": "Chase",
  "created_at": "2025-11-01T09:00:00Z",
  "updated_at": "2026-02-28T14:30:00Z"
}
```

**Error Cases**

| Condition | Status | Code |
|---|---|---|
| Missing API key | `401` | `MISSING_API_KEY` |
| Invalid API key | `401` | `INVALID_API_KEY` |
| `id` is not a valid UUID | `400` | `INVALID_PARAMETER` |
| Account not found | `404` | `NOT_FOUND` |

**Example**

```
GET /api/v1/accounts/a1b2c3d4-e5f6-7890-abcd-ef1234567890 HTTP/1.1
Host: breadbox.local:8080
X-API-Key: bb_4xK9mZ2nQpR7sT1vW3yA5bC8dE6fG0hJqLnPuVw
```

---

### 5.3 Transactions

#### `GET /api/v1/transactions`

Returns a paginated list of transactions. Default sort: `date` descending, then by `id` descending (for stable ordering of same-day transactions). Supports rich filtering and configurable sort order.

Soft-deleted transactions (where `deleted_at` is non-null) are excluded from all results.

**Authentication:** `X-API-Key` required.

**Query Parameters**

| Parameter | Type | Required | Default | Description |
|---|---|---|---|---|
| `cursor` | string | No | — | Opaque pagination cursor. Omit for first page. |
| `limit` | integer | No | `100` | Results per page. Min: `1`. Max: `500`. |
| `start_date` | string (ISO 8601 date) | No | — | Inclusive lower bound on `date`. Format: `YYYY-MM-DD`. |
| `end_date` | string (ISO 8601 date) | No | — | Exclusive upper bound on `date`. Format: `YYYY-MM-DD`. A transaction with `date == end_date` is excluded. |
| `account_id` | UUID | No | — | Filter to a single account. |
| `user_id` | UUID | No | — | Filter to all accounts belonging to a specific family member. |
| `category` | string | No | — | Exact match on `provider_category_primary` (e.g., `"FOOD_AND_DRINK"`, `"TRANSPORTATION"`). Case-sensitive. |
| `category_detailed` | string | No | — | Exact match on `provider_category_detailed` (e.g., `"FOOD_AND_DRINK_GROCERIES"`). Case-sensitive. |
| `min_amount` | number | No | — | Inclusive lower bound on `amount`. Uses Plaid sign convention (positive = debit). |
| `max_amount` | number | No | — | Inclusive upper bound on `amount`. |
| `pending` | boolean | No | — | If `true`, return only pending transactions. If `false`, return only posted transactions. If omitted, return both. |
| `search` | string | No | — | Text search applied to `provider_name` and `provider_merchant_name`. Case-insensitive. Uses PostgreSQL `pg_trgm` similarity or `ILIKE`. Min length: 2 characters. |
| `sort_by` | string | No | `date` | Sort field. Valid values: `date`, `amount`, `provider_name`. |
| `sort_order` | string | No | `desc` | Sort direction. Valid values: `asc`, `desc`. Cursor pagination only works with `date` sort. |

All filters are combined with `AND` logic. See [Section 6](#6-filtering-and-search) for detailed filter semantics.

**Response — 200 OK**

```json
{
  "transactions": [
    {
      "id": "t1a2b3c4-d5e6-f789-0abc-def123456789",
      "account_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "provider_transaction_id": "lPNZkR5o3Vh1qWxYuEj2dM7fKtS9gA",
      "provider_pending_transaction_id": null,
      "amount": 67.43,
      "iso_currency_code": "USD",
      "unofficial_currency_code": null,
      "date": "2026-02-27",
      "datetime": "2026-02-27T10:23:00Z",
      "authorized_date": "2026-02-26",
      "authorized_datetime": "2026-02-26T19:45:00Z",
      "provider_name": "WHOLE FOODS MARKET #10452",
      "provider_merchant_name": "Whole Foods Market",
      "provider_category_primary": "FOOD_AND_DRINK",
      "provider_category_detailed": "FOOD_AND_DRINK_GROCERIES",
      "provider_payment_channel": "in store",
      "pending": false,
      "created_at": "2026-02-27T08:15:00Z",
      "updated_at": "2026-02-27T08:15:00Z"
    },
    {
      "id": "t2b3c4d5-e6f7-890a-bcde-f12345678901",
      "account_id": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
      "provider_transaction_id": "mQoZjS4p2Wg0rVyXtFk3eN8hLuT1bB",
      "provider_pending_transaction_id": null,
      "amount": 14.00,
      "iso_currency_code": "USD",
      "unofficial_currency_code": null,
      "date": "2026-02-26",
      "datetime": null,
      "authorized_date": "2026-02-26",
      "authorized_datetime": null,
      "provider_name": "NETFLIX.COM",
      "provider_merchant_name": "Netflix",
      "provider_category_primary": "ENTERTAINMENT",
      "provider_category_detailed": "ENTERTAINMENT_STREAMING_SERVICES",
      "provider_payment_channel": "online",
      "pending": false,
      "created_at": "2026-02-26T22:00:00Z",
      "updated_at": "2026-02-26T22:00:00Z"
    },
    {
      "id": "t3c4d5e6-f7a8-901b-cdef-123456789012",
      "account_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "provider_transaction_id": "nRpAkT5q3Xh1sWzYvGl4fO9iMvU2cC",
      "provider_pending_transaction_id": null,
      "amount": -2500.00,
      "iso_currency_code": "USD",
      "unofficial_currency_code": null,
      "date": "2026-02-25",
      "datetime": null,
      "authorized_date": null,
      "authorized_datetime": null,
      "provider_name": "EMPLOYER DIRECT DEPOSIT",
      "provider_merchant_name": null,
      "provider_category_primary": "INCOME",
      "provider_category_detailed": "INCOME_WAGES",
      "provider_payment_channel": "other",
      "pending": false,
      "created_at": "2026-02-25T06:00:00Z",
      "updated_at": "2026-02-25T06:00:00Z"
    }
  ],
  "next_cursor": "eyJkYXRlIjoiMjAyNi0wMi0yNSIsImlkIjoidDNjNGQ1ZTYtZjdhOC05MDFiLWNkZWYtMTIzNDU2Nzg5MDEyIn0",
  "has_more": true,
  "limit": 50
}
```

**Error Cases**

| Condition | Status | Code |
|---|---|---|
| Missing API key | `401` | `MISSING_API_KEY` |
| Invalid API key | `401` | `INVALID_API_KEY` |
| `limit` out of range or non-integer | `400` | `INVALID_PARAMETER` |
| `start_date` or `end_date` not in `YYYY-MM-DD` format | `400` | `INVALID_PARAMETER` |
| `start_date` is after `end_date` | `400` | `INVALID_PARAMETER` |
| `account_id` or `user_id` not a valid UUID | `400` | `INVALID_PARAMETER` |
| `min_amount` or `max_amount` not a valid number | `400` | `INVALID_PARAMETER` |
| `min_amount` is greater than `max_amount` | `400` | `INVALID_PARAMETER` |
| `pending` is not `true` or `false` | `400` | `INVALID_PARAMETER` |
| `search` is shorter than 2 characters | `400` | `INVALID_PARAMETER` |
| `cursor` is malformed or expired | `400` | `INVALID_CURSOR` |

**Example — Filtered Request**

Fetch all posted grocery transactions for a specific user in February 2026, up to 50 per page:

```
GET /api/v1/transactions?user_id=u1a2b3c4-d5e6-f789-0abc-def123456789&category=FOOD_AND_DRINK&start_date=2026-02-01&end_date=2026-03-01&pending=false&limit=50 HTTP/1.1
Host: breadbox.local:8080
X-API-Key: bb_4xK9mZ2nQpR7sT1vW3yA5bC8dE6fG0hJqLnPuVw
```

---

#### `GET /api/v1/transactions/count`

Returns the total count of transactions matching the given filters. Accepts the same filter parameters as `GET /api/v1/transactions` except `cursor` and `limit` (which are pagination-only). This endpoint is used by the MCP `count_transactions` tool.

**Authentication:** `X-API-Key` required.

**Query Parameters**

| Parameter | Type | Required | Default | Description |
|---|---|---|---|---|
| `start_date` | string (ISO 8601 date) | No | — | Inclusive lower bound on `date`. Format: `YYYY-MM-DD`. |
| `end_date` | string (ISO 8601 date) | No | — | Exclusive upper bound on `date`. Format: `YYYY-MM-DD`. |
| `account_id` | UUID | No | — | Filter to a single account. |
| `user_id` | UUID | No | — | Filter to all accounts belonging to a specific family member. |
| `category` | string | No | — | Exact match on `provider_category_primary`. Case-sensitive. |
| `min_amount` | number | No | — | Inclusive lower bound on `amount`. |
| `max_amount` | number | No | — | Inclusive upper bound on `amount`. |
| `pending` | boolean | No | — | If `true`, count only pending transactions. If `false`, count only posted. If omitted, count both. |
| `search` | string | No | — | Text search applied to `provider_name` and `provider_merchant_name`. Min length: 2 characters. |

All filters are combined with `AND` logic. Soft-deleted transactions are excluded.

**Response — 200 OK**

```json
{
  "count": 1523
}
```

| Field | Type | Description |
|---|---|---|
| `count` | integer | Total number of transactions matching the applied filters. |

**Error Cases**

| Condition | Status | Code |
|---|---|---|
| Missing API key | `401` | `MISSING_API_KEY` |
| Invalid API key | `401` | `INVALID_API_KEY` |
| `start_date` or `end_date` not in `YYYY-MM-DD` format | `400` | `INVALID_PARAMETER` |
| `start_date` is after `end_date` | `400` | `INVALID_PARAMETER` |
| `account_id` or `user_id` not a valid UUID | `400` | `INVALID_PARAMETER` |
| `min_amount` or `max_amount` not a valid number | `400` | `INVALID_PARAMETER` |
| `pending` is not `true` or `false` | `400` | `INVALID_PARAMETER` |
| `search` is shorter than 2 characters | `400` | `INVALID_PARAMETER` |

---

#### `GET /api/v1/transactions/:id`

Returns a single transaction by its Breadbox UUID.

**Authentication:** `X-API-Key` required.

**Path Parameters**

| Parameter | Type | Description |
|---|---|---|
| `id` | UUID | The Breadbox transaction UUID. |

**Response — 200 OK**

```json
{
  "id": "t1a2b3c4-d5e6-f789-0abc-def123456789",
  "account_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "provider_transaction_id": "lPNZkR5o3Vh1qWxYuEj2dM7fKtS9gA",
  "provider_pending_transaction_id": null,
  "amount": 67.43,
  "iso_currency_code": "USD",
  "unofficial_currency_code": null,
  "date": "2026-02-27",
  "datetime": "2026-02-27T10:23:00Z",
  "authorized_date": "2026-02-26",
  "authorized_datetime": "2026-02-26T19:45:00Z",
  "provider_name": "WHOLE FOODS MARKET #10452",
  "provider_merchant_name": "Whole Foods Market",
  "provider_category_primary": "FOOD_AND_DRINK",
  "provider_category_detailed": "FOOD_AND_DRINK_GROCERIES",
  "provider_payment_channel": "in store",
  "pending": false,
  "created_at": "2026-02-27T08:15:00Z",
  "updated_at": "2026-02-27T08:15:00Z"
}
```

**Error Cases**

| Condition | Status | Code |
|---|---|---|
| Missing API key | `401` | `MISSING_API_KEY` |
| Invalid API key | `401` | `INVALID_API_KEY` |
| `id` is not a valid UUID | `400` | `INVALID_PARAMETER` |
| Transaction not found (or soft-deleted) | `404` | `NOT_FOUND` |

---

### 5.4 Users

#### `GET /api/v1/users`

Returns a list of all family members configured in Breadbox. Users are not login accounts — they are labels used to assign accounts and filter transactions by household member.

This endpoint returns all users without pagination — the number of family members is small by design.

**Authentication:** `X-API-Key` required.

**Query Parameters**

None.

**Response — 200 OK**

Returns a bare JSON array — this endpoint does not paginate.

```json
[
  {
    "id": "u1a2b3c4-d5e6-f789-0abc-def123456789",
    "name": "Alex Canales",
    "email": "alex@example.com",
    "created_at": "2025-11-01T09:00:00Z",
    "updated_at": "2025-11-01T09:00:00Z"
  },
  {
    "id": "u2b3c4d5-e6f7-890a-bcde-f12345678901",
    "name": "Jordan Canales",
    "email": null,
    "created_at": "2025-11-15T12:00:00Z",
    "updated_at": "2025-11-15T12:00:00Z"
  }
]
```

**Error Cases**

| Condition | Status | Code |
|---|---|---|
| Missing API key | `401` | `MISSING_API_KEY` |
| Invalid API key | `401` | `INVALID_API_KEY` |

---

### 5.5 Connections

#### `GET /api/v1/connections`

Returns a list of all bank connections (Plaid Items), including their current status and last sync time. Connections are returned in the order they were created.

**Authentication:** `X-API-Key` required.

**Query Parameters**

| Parameter | Type | Required | Default | Description |
|---|---|---|---|---|
| `user_id` | UUID | No | — | Filter to connections belonging to a specific family member. If omitted, all connections are returned. |

**Response — 200 OK**

Returns a bare JSON array — this endpoint does not paginate.

```json
[
  {
    "id": "c9d8e7f6-a5b4-3210-fedc-ba0987654321",
    "institution_id": "ins_3",
    "institution_name": "Chase",
    "provider": "plaid",
    "status": "active",
    "error_code": null,
    "error_message": null,
    "last_synced_at": "2026-02-28T14:30:00Z",
    "last_attempted_sync_at": "2026-02-28T14:30:00Z",
    "user_id": "u1a2b3c4-d5e6-f789-0abc-def123456789",
    "user_name": "Alex Canales",
    "created_at": "2025-11-01T09:00:00Z",
    "updated_at": "2026-02-28T14:30:00Z"
  },
  {
    "id": "d8e9f0a1-b2c3-4567-efab-cd9012345678",
    "institution_id": "ins_11",
    "institution_name": "Bank of America",
    "provider": "plaid",
    "status": "error",
    "error_code": "ITEM_LOGIN_REQUIRED",
    "error_message": "the login details of this item have changed",
    "last_synced_at": "2026-01-15T08:00:00Z",
    "last_attempted_sync_at": "2026-02-28T14:31:00Z",
    "user_id": "u2b3c4d5-e6f7-890a-bcde-f12345678901",
    "user_name": "Jordan Canales",
    "created_at": "2025-12-01T11:00:00Z",
    "updated_at": "2026-02-28T14:31:00Z"
  }
]
```

**Connection Status Values**

| Value | Description |
|---|---|
| `active` | Connection is healthy and syncing normally. |
| `error` | Last sync encountered an error. See `error_code` and `error_message` for details. |
| `pending_reauth` | The user must re-authenticate via Plaid Link update mode. Typically triggered by `ITEM_LOGIN_REQUIRED`. |
| `disconnected` | The connection has been manually disconnected and is no longer active. |

Note: Whether a sync is currently in progress can be determined from `sync_logs` (most recent entry with `status = "in_progress"`) or from a `sync_in_progress` boolean if the client needs it without querying logs.

**Error Cases**

| Condition | Status | Code |
|---|---|---|
| Missing API key | `401` | `MISSING_API_KEY` |
| Invalid API key | `401` | `INVALID_API_KEY` |
| `user_id` is not a valid UUID | `400` | `INVALID_PARAMETER` |

---

#### `GET /api/v1/connections/:id/status`

Returns detailed health and sync status for a single connection, including the most recent sync log entry.

**Authentication:** `X-API-Key` required.

**Path Parameters**

| Parameter | Type | Description |
|---|---|---|
| `id` | UUID | The Breadbox connection UUID. |

**Response — 200 OK**

```json
{
  "id": "c9d8e7f6-a5b4-3210-fedc-ba0987654321",
  "institution_name": "Chase",
  "provider": "plaid",
  "status": "active",
  "error_code": null,
  "error_message": null,
  "last_synced_at": "2026-02-28T14:30:00Z",
  "last_attempted_sync_at": "2026-02-28T14:30:00Z",
  "recent_sync": {
    "id": "s1a2b3c4-d5e6-f789-0abc-def123456789",
    "trigger": "cron",
    "status": "success",
    "added_count": 12,
    "modified_count": 2,
    "removed_count": 0,
    "error": null,
    "started_at": "2026-02-28T14:29:45Z",
    "completed_at": "2026-02-28T14:30:00Z"
  }
}
```

**`recent_sync` Object**

| Field | Type | Description |
|---|---|---|
| `id` | UUID | Sync log entry UUID. |
| `trigger` | string | What triggered the sync: `"cron"`, `"webhook"`, `"manual"`, or `"initial"`. |
| `status` | string | `"in_progress"`, `"success"`, or `"error"`. Matches the `sync_status` DB enum. |
| `added_count` | integer | Number of new transactions added. |
| `modified_count` | integer | Number of existing transactions updated. |
| `removed_count` | integer | Number of transactions soft-deleted. |
| `error` | string \| null | Error message if `status` is `"error"`. |
| `started_at` | string (ISO 8601) | When the sync began. |
| `completed_at` | string \| null (ISO 8601) | When the sync finished. `null` if still in progress. |

If no sync has ever been run for this connection, `recent_sync` is `null`.

**Error Cases**

| Condition | Status | Code |
|---|---|---|
| Missing API key | `401` | `MISSING_API_KEY` |
| Invalid API key | `401` | `INVALID_API_KEY` |
| `id` is not a valid UUID | `400` | `INVALID_PARAMETER` |
| Connection not found | `404` | `NOT_FOUND` |

---

### 5.6 Sync

#### `POST /api/v1/sync`

Triggers a manual data sync. If `connection_id` is provided, syncs only that connection. If omitted, syncs all active connections sequentially.

The sync runs asynchronously in the background. This endpoint returns immediately with a `202 Accepted` response; it does not wait for the sync to complete. Use `GET /api/v1/connections/:id/status` to poll for results.

**Authentication:** `X-API-Key` required.

**Request Body**

```json
{
  "connection_id": "c9d8e7f6-a5b4-3210-fedc-ba0987654321"
}
```

The body is optional. If provided, it must be valid JSON.

| Field | Type | Required | Description |
|---|---|---|---|
| `connection_id` | UUID | No | The connection to sync. If omitted, all active connections are synced. |

**Response — 202 Accepted**

```json
{
  "message": "Sync started.",
  "connection_id": "c9d8e7f6-a5b4-3210-fedc-ba0987654321"
}
```

If syncing all connections, `connection_id` is `null`:

```json
{
  "message": "Sync started for all active connections.",
  "connection_id": null
}
```

**Error Cases**

| Condition | Status | Code |
|---|---|---|
| Missing API key | `401` | `MISSING_API_KEY` |
| Invalid API key | `401` | `INVALID_API_KEY` |
| `connection_id` is not a valid UUID | `400` | `INVALID_PARAMETER` |
| `connection_id` does not match any connection | `404` | `NOT_FOUND` |
| A sync is already in progress for the specified connection | `409` | `SYNC_IN_PROGRESS` |
| Request body is present but not valid JSON | `400` | `INVALID_PARAMETER` |

**Example**

```
POST /api/v1/sync HTTP/1.1
Host: breadbox.local:8080
X-API-Key: bb_4xK9mZ2nQpR7sT1vW3yA5bC8dE6fG0hJqLnPuVw
Content-Type: application/json

{
  "connection_id": "c9d8e7f6-a5b4-3210-fedc-ba0987654321"
}
```

```json
HTTP/1.1 202 Accepted
Content-Type: application/json

{
  "message": "Sync started.",
  "connection_id": "c9d8e7f6-a5b4-3210-fedc-ba0987654321"
}
```

---

### 5.7 Categories

#### `GET /api/v1/categories`

Returns a list of distinct category pairs (primary + detailed) across all transactions. Useful for populating filter dropdowns and for AI agents to understand the available category taxonomy.

**Authentication:** `X-API-Key` required.

**Request**

No parameters.

**Response — 200 OK**

Returns a bare JSON array — this endpoint does not paginate.

```json
[
  {
    "primary": "FOOD_AND_DRINK",
    "detailed": "FOOD_AND_DRINK_GROCERIES"
  },
  {
    "primary": "FOOD_AND_DRINK",
    "detailed": "FOOD_AND_DRINK_RESTAURANTS"
  },
  {
    "primary": "TRANSPORTATION",
    "detailed": "TRANSPORTATION_GAS"
  }
]
```

| Field | Type | Description |
|---|---|---|
| `primary` | string | Primary Plaid category (e.g., `"FOOD_AND_DRINK"`). |
| `detailed` | string \| null | Detailed Plaid subcategory (e.g., `"FOOD_AND_DRINK_GROCERIES"`). May be `null`. |

---

## 6. Filtering and Search

All filters on `GET /api/v1/transactions` are combined with `AND` logic. A transaction must satisfy every provided filter to be included in the results.

### Date Range Filtering

- The `date` field on a transaction is the **posted date** (or the pending date if the transaction is pending), in `YYYY-MM-DD` format. It does not include a time component.
- `start_date` is **inclusive**: transactions with `date >= start_date` are included.
- `end_date` is **exclusive**: transactions with `date < end_date` are included. A transaction with `date == end_date` is excluded.
- This convention makes it easy to express calendar months without overlap: `start_date=2026-02-01&end_date=2026-03-01` returns all transactions in February 2026.
- Both `start_date` and `end_date` are independent — you may specify either, both, or neither.
- Dates must be in `YYYY-MM-DD` format (ISO 8601 date). Timestamps with time components are not accepted.

```
# All of February 2026
start_date=2026-02-01&end_date=2026-03-01

# From January 1, 2026 onward (no upper bound)
start_date=2026-01-01

# Through the end of 2025 (no lower bound)
end_date=2026-01-01
```

The SQL equivalent:

```sql
AND date >= $start_date   -- if start_date provided
AND date <  $end_date     -- if end_date provided
```

### Text Search

The `search` parameter performs a **case-insensitive substring search** across two fields:

- `name` — The raw transaction name as returned by the bank (e.g., `"WHOLE FOODS MARKET #10452"`)
- `merchant_name` — The cleaned merchant name as classified by Plaid (e.g., `"Whole Foods Market"`), if available

The search uses PostgreSQL `ILIKE` with a `%search%` pattern, or `pg_trgm` similarity if trigram indexes are present. The same query matches either field:

```sql
AND (
  name ILIKE '%' || $search || '%'
  OR merchant_name ILIKE '%' || $search || '%'
)
```

Minimum search length is **2 characters**. Single-character searches return a `400` error.

The `search` parameter does not search on category, amount, date, or any other field.

### Amount Range Filtering

- `min_amount` is **inclusive**: `amount >= min_amount`
- `max_amount` is **inclusive**: `amount <= max_amount`
- Amounts use the Plaid sign convention: **positive = debit (money out), negative = credit (money in)**.
- To find all debits over $100: `min_amount=100`
- To find all credits (deposits): `max_amount=-0.01` (any negative amount)
- To find all debits between $10 and $50: `min_amount=10&max_amount=50`

```sql
AND amount >= $min_amount  -- if min_amount provided
AND amount <= $max_amount  -- if max_amount provided
```

Both parameters accept decimal values with up to two decimal places (e.g., `42.50`, `100`, `-1500.00`).

### Category Filtering

The `category` parameter performs an **exact, case-sensitive match** on the `category_primary` field. Plaid's category taxonomy uses `UPPER_SNAKE_CASE` strings.

Common values include:

| `category_primary` | Description |
|---|---|
| `FOOD_AND_DRINK` | Restaurants, groceries, bars |
| `TRANSPORTATION` | Gas, public transit, ride-sharing |
| `ENTERTAINMENT` | Streaming, movies, games |
| `SHOPPING` | Retail, online shopping |
| `UTILITIES` | Electric, water, internet |
| `INCOME` | Direct deposits, wages |
| `TRANSFER_IN` | Transfers into the account |
| `TRANSFER_OUT` | Transfers out of the account |
| `MEDICAL` | Healthcare, pharmacy |
| `HOME` | Rent, mortgage, home improvement |
| `TRAVEL` | Hotels, flights, car rental |
| `LOAN_PAYMENTS` | Student loans, car loans |

The full taxonomy is defined by Plaid and may expand over time. Breadbox stores and returns values as-is.

### Combining Filters

All filters compose with `AND`. For example:

```
GET /api/v1/transactions
  ?user_id=u1a2b3c4-d5e6-f789-0abc-def123456789
  &start_date=2026-01-01
  &end_date=2026-03-01
  &category=FOOD_AND_DRINK
  &min_amount=0.01
  &pending=false
  &search=whole foods
```

Returns only posted (non-pending) grocery transactions at Whole Foods for a specific family member, with a positive amount (debits only), from January 1 through February 28, 2026.

---

## 7. Response Schemas

This section defines the canonical shape of each object type returned by the API. Fields marked **[Plaid]** are sourced directly from Plaid's API response without transformation. Fields marked **[Internal]** are computed or assigned by Breadbox.

### Account

Represents a bank account linked via a connection.

| Field | Type | Source | Description |
|---|---|---|---|
| `id` | UUID (string) | Internal | Breadbox-assigned UUID. Stable identifier for this account. |
| `connection_id` | UUID (string) | Internal | UUID of the parent bank connection (Plaid Item). |
| `user_id` | UUID \| null (string) | Internal | UUID of the family member this account is assigned to. `null` if unassigned. |
| `name` | string | Plaid | Short account name as provided by the institution (e.g., `"Platinum Checking"`). |
| `official_name` | string \| null | Plaid | Full official product name (e.g., `"Chase Total Checking®"`). May be null. |
| `type` | string | Plaid | Plaid account type: `"depository"`, `"credit"`, `"loan"`, `"investment"`, `"other"`. |
| `subtype` | string \| null | Plaid | Plaid account subtype: `"checking"`, `"savings"`, `"credit card"`, `"mortgage"`, etc. |
| `mask` | string \| null | Plaid | Last 4 digits of the account number (e.g., `"4821"`). |
| `iso_currency_code` | string \| null | Plaid | ISO 4217 currency code (e.g., `"USD"`). `null` if the account uses an unofficial currency code. |
| `balance_current` | number \| null | Plaid | Current balance. For credit accounts: amount owed. For depository: current funds. May be `null` if Plaid did not return a balance. |
| `balance_available` | number \| null | Plaid | Available balance after pending transactions. `null` for credit accounts where this is not reported. |
| `balance_limit` | number \| null | Plaid | Credit limit for credit accounts. `null` for non-credit accounts. |
| `institution_name` | string | Internal | Institution name resolved from the connection (e.g., `"Chase"`). |
| `created_at` | string (ISO 8601) | Internal | Timestamp when this account record was first created in Breadbox. |
| `updated_at` | string (ISO 8601) | Internal | Timestamp when this account record was last updated (e.g., balance refresh). |

### Transaction

Represents a single financial transaction.

| Field | Type | Source | Description |
|---|---|---|---|
| `id` | UUID (string) | Internal | Breadbox-assigned UUID. |
| `account_id` | UUID (string) | Internal | UUID of the account this transaction belongs to. |
| `external_transaction_id` | string | Plaid | The transaction ID assigned by Plaid (e.g., `"lPNZkR5o3Vh1qWxYuEj2dM7fKtS9gA"`). Unique within a Plaid Item. |
| `pending_transaction_id` | string \| null | Plaid | If this is a posted transaction that replaced a pending one, this field contains the `external_transaction_id` of the original pending transaction. `null` otherwise. Use this to link posted→pending records. |
| `amount` | number | Plaid | Transaction amount. Positive = debit (money out). Negative = credit (money in). Stored as `NUMERIC(12,2)` in the database. |
| `iso_currency_code` | string \| null | Plaid | ISO 4217 currency code (e.g., `"USD"`). `null` if the account uses an unofficial currency code. |
| `unofficial_currency_code` | string \| null | Plaid | Unofficial currency code for crypto or other non-ISO currencies. `null` for standard currencies. Mutually exclusive with a non-null `iso_currency_code`. |
| `date` | string (YYYY-MM-DD) | Plaid | Posted date for posted transactions. Pending date for pending transactions. |
| `datetime` | string \| null (ISO 8601) | Plaid | Date and time of the transaction with timezone offset, if available. `null` when the institution only provides a date (no time). Stored as `TIMESTAMPTZ` in the database. |
| `authorized_date` | string \| null (YYYY-MM-DD) | Plaid | The date the transaction was authorized (may differ from posted date). `null` for pending transactions or when not provided by the institution. |
| `authorized_datetime` | string \| null (ISO 8601) | Plaid | Date and time the transaction was authorized, if available. `null` when not provided by the institution. Stored as `TIMESTAMPTZ` in the database. |
| `name` | string | Plaid | Raw transaction name from the institution (e.g., `"WHOLE FOODS MARKET #10452"`). |
| `merchant_name` | string \| null | Plaid | Cleaned merchant name as classified by Plaid (e.g., `"Whole Foods Market"`). `null` if Plaid could not classify. |
| `category_primary` | string \| null | Plaid | Primary Plaid category in `UPPER_SNAKE_CASE` (e.g., `"FOOD_AND_DRINK"`). `null` if unclassified. |
| `category_detailed` | string \| null | Plaid | Detailed Plaid subcategory (e.g., `"FOOD_AND_DRINK_GROCERIES"`). `null` if unclassified. |
| `payment_channel` | string \| null | Plaid | How the transaction was made: `"in store"`, `"online"`, `"other"`. |
| `pending` | boolean | Plaid | `true` if the transaction has not yet posted. Pending transactions may be updated or replaced by Plaid. |
| `created_at` | string (ISO 8601) | Internal | Timestamp when this transaction record was first created in Breadbox. |
| `updated_at` | string (ISO 8601) | Internal | Timestamp when this transaction record was last modified. |

Note: Soft-deleted transactions (with a non-null `deleted_at`) are never returned by the API. They are retained in the database for historical reference only.

### User

Represents a family member. Users are organizational labels, not login accounts.

| Field | Type | Source | Description |
|---|---|---|---|
| `id` | UUID (string) | Internal | Breadbox-assigned UUID. |
| `name` | string | Internal | Display name (e.g., `"Alex Canales"`). |
| `email` | string \| null | Internal | Optional email address. Not used for authentication. |
| `created_at` | string (ISO 8601) | Internal | Timestamp when this user was created. |
| `updated_at` | string (ISO 8601) | Internal | Timestamp when this user was last updated. |

### Connection

Represents a bank connection (a Plaid Item linking to one institution).

| Field | Type | Source | Description |
|---|---|---|---|
| `id` | UUID (string) | Internal | Breadbox-assigned UUID. |
| `institution_id` | string | Plaid | Plaid institution identifier (e.g., `"ins_3"` for Chase). |
| `institution_name` | string | Plaid | Human-readable institution name (e.g., `"Chase"`). |
| `provider` | string | Internal | Data provider for this connection. Currently always `"plaid"`. Future values: `"teller"`, `"csv"`. |
| `status` | string | Internal | Connection health status: `"active"`, `"error"`, `"pending_reauth"`, `"disconnected"`. |
| `error_code` | string \| null | Internal / Plaid | Machine-readable error code from the last failed sync (e.g., `"ITEM_LOGIN_REQUIRED"`). `null` if no error. |
| `error_message` | string \| null | Internal / Plaid | Human-readable error description. `null` if no error. |
| `last_synced_at` | string \| null (ISO 8601) | Internal | Timestamp of the most recently completed successful sync. Matches the `last_synced_at` column on `bank_connections`. `null` if the connection has never synced successfully. |
| `last_attempted_sync_at` | string \| null (ISO 8601) | Internal | Timestamp of the most recently attempted sync (successful or not). Computed from the most recent `sync_logs` entry for this connection. `null` if never attempted. |
| `user_id` | UUID (string) | Internal | UUID of the family member who owns this connection. Sourced from `bank_connections.user_id`. |
| `user_name` | string | Internal | Display name of the owning family member. Resolved via JOIN to the `users` table. |
| `created_at` | string (ISO 8601) | Internal | Timestamp when this connection was first added to Breadbox. |
| `updated_at` | string (ISO 8601) | Internal | Timestamp when this connection record was last updated. |

Note: `access_token` and other Plaid credentials are **never** included in API responses. They are stored encrypted in the database and are only accessed internally by the sync engine.

### SyncStatus (Connection Detail)

Returned by `GET /api/v1/connections/:id/status` as the top-level object, extending the Connection schema with a `recent_sync` field.

| Field | Type | Description |
|---|---|---|
| *(all Connection fields)* | — | All fields from the Connection schema. |
| `recent_sync` | object \| null | The most recent sync log entry. `null` if no sync has ever been run. |
| `recent_sync.id` | UUID (string) | Sync log entry UUID. |
| `recent_sync.trigger` | string | What triggered the sync: `"cron"`, `"webhook"`, `"manual"`, or `"initial"`. |
| `recent_sync.status` | string | `"in_progress"`, `"success"`, or `"error"`. Matches the `sync_status` DB enum. |
| `recent_sync.added_count` | integer | Number of transactions added. |
| `recent_sync.modified_count` | integer | Number of transactions modified. |
| `recent_sync.removed_count` | integer | Number of transactions soft-deleted. |
| `recent_sync.error` | string \| null | Error message if `status` is `"error"`. `null` otherwise. |
| `recent_sync.started_at` | string (ISO 8601) | When the sync began. |
| `recent_sync.completed_at` | string \| null (ISO 8601) | When the sync completed. `null` if still in progress. |

---

## 8. Admin API

The Admin API is used exclusively by the Breadbox admin dashboard. It uses **session cookie authentication**, not API key authentication, and is not intended for external programmatic access.

### Authentication

Admin sessions are established via the admin login form. The server issues an HTTP-only session cookie on successful login. All `/admin/api/` endpoints require a valid session cookie; requests without one receive a `302` redirect to the login page.

Admin credentials (username + bcrypt-hashed password) are configured during the first-run setup wizard.

---

### Setup

#### `GET /admin/api/setup/status`

Returns whether the first-run setup wizard has been completed. This endpoint is **unauthenticated** — it must be accessible before any admin account exists.

**Response — 200 OK**

```json
{
  "setup_complete": false
}
```

| Field | Type | Description |
|---|---|---|
| `setup_complete` | boolean | `true` if the setup wizard has been completed and an admin account exists. `false` if setup is still needed. |

---

#### `POST /admin/api/setup`

Completes the first-run setup wizard. Creates the admin account, stores Plaid credentials and sync configuration in the App Config table, and marks setup as complete. This endpoint is only accepted if setup has not yet been completed; subsequent calls return `409 Conflict`.

**Request Body**

```json
{
  "admin_username": "admin",
  "admin_password": "correct horse battery staple",
  "plaid_client_id": "5f3c8a2b1e9d4607c3b5a1f2",
  "plaid_secret": "3e8b1a5f9c2d4607e3b5a1f2c8d4a7b9",
  "plaid_environment": "sandbox",
  "sync_interval_hours": 12,
  "webhook_url": "https://breadbox.example.com/webhooks/plaid"
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `admin_username` | string | Yes | Username for the admin account. Must be non-empty. |
| `admin_password` | string | Yes | Password for the admin account. Min length: 8 characters. Stored as a bcrypt hash. |
| `plaid_client_id` | string | Yes | Plaid API client ID. |
| `plaid_secret` | string | Yes | Plaid API secret. |
| `plaid_environment` | string | Yes | Plaid environment: `"sandbox"`, `"development"`, or `"production"`. |
| `sync_interval_hours` | integer | No | How often to automatically sync all connections. Default: `12`. Min: `1`. Max: `168` (1 week). |
| `webhook_url` | string | No | Public URL for Plaid webhooks (e.g., `"https://breadbox.example.com/webhooks/plaid"`). If omitted, webhooks are not configured and Breadbox operates in polling-only mode. |

**Response — 201 Created**

```json
{
  "message": "Setup complete."
}
```

**Error Cases**

| Condition | Status | Description |
|---|---|---|
| Setup already completed | `409 Conflict` | Returns `{ "error": { "code": "SETUP_ALREADY_COMPLETE", "message": "..." } }` |
| Missing required field | `422 Unprocessable Entity` | |
| Password too short | `422 Unprocessable Entity` | |
| Invalid `plaid_environment` value | `422 Unprocessable Entity` | |

---

### Plaid Link

#### `POST /admin/api/link-token`

Requests a short-lived Plaid Link token from the Plaid API. The frontend uses this token to initialize the Plaid Link drop-in UI, which walks the user through selecting their institution and authenticating.

**Request Body**

```json
{
  "user_id": "u1a2b3c4-d5e6-f789-0abc-def123456789"
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `user_id` | string (UUID) | Yes | The Breadbox user UUID for the family member linking the account. Used as the `client_user_id` when calling Plaid's `/link/token/create`. |

**Response — 200 OK**

```json
{
  "link_token": "link-sandbox-3e8b1a5f-9c2d-4607-e3b5-a1f2c8d4a7b9",
  "expiration": "2026-03-01T16:00:00Z"
}
```

| Field | Type | Description |
|---|---|---|
| `link_token` | string | The Plaid Link token. Pass this to `Plaid.create({ token: link_token })` in the frontend. Valid for 30 minutes. |
| `expiration` | string (ISO 8601) | When the link token expires. |

---

#### `POST /admin/api/exchange-token`

Exchanges a Plaid public token (returned by the Plaid Link UI on successful connection) for a long-lived access token, then stores the access token encrypted in the database and creates a Connection record.

This endpoint triggers an initial sync for the new connection after exchange.

**Request Body**

```json
{
  "public_token": "public-sandbox-3e8b1a5f-9c2d-4607-e3b5-a1f2c8d4a7b9",
  "user_id": "u1a2b3c4-d5e6-f789-0abc-def123456789",
  "institution_id": "ins_3",
  "institution_name": "Chase",
  "accounts": [
    { "id": "acc_plaid_id_1", "name": "Platinum Checking", "type": "depository", "subtype": "checking" }
  ]
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `public_token` | string | Yes | The `public_token` from the Plaid Link `onSuccess` callback. |
| `user_id` | string (UUID) | Yes | The Breadbox user UUID for the family member who completed Plaid Link. |
| `institution_id` | string | Yes | The Plaid institution ID from the `onSuccess` metadata (e.g., `"ins_3"`). |
| `institution_name` | string | Yes | The institution name from the `onSuccess` metadata (e.g., `"Chase"`). |
| `accounts` | array | Yes | The selected accounts array from the Plaid Link `onSuccess` metadata. Each element has `id`, `name`, `type`, and `subtype`. |

**Response — 201 Created**

```json
{
  "connection_id": "c9d8e7f6-a5b4-3210-fedc-ba0987654321",
  "institution_name": "Chase",
  "status": "active"
}
```

**Error Cases**

| Condition | Status | Description |
|---|---|---|
| Plaid rejects the public token | `502 Bad Gateway` | Returns Plaid error details in `message`. |
| Missing required field | `422 Unprocessable Entity` | |

---

#### `POST /admin/api/connections/:id/reauth`

Generates a new Plaid Link token for re-authentication (Plaid Link update mode). Used when a connection enters `pending_reauth` status. The frontend passes this token to Plaid Link, which handles credential refresh. After the user completes re-auth, the existing access token is updated automatically by Plaid; no further exchange is needed.

**Path Parameters**

| Parameter | Type | Description |
|---|---|---|
| `id` | UUID | Connection UUID. |

**Request Body**

None.

**Response — 200 OK**

```json
{
  "link_token": "link-sandbox-7c9d2e1f-4a6b-8031-d5e2-b3f4c7e9a0b1",
  "expiration": "2026-03-01T16:30:00Z"
}
```

**Error Cases**

| Condition | Status | Description |
|---|---|---|
| Connection not found | `404 Not Found` | |
| Plaid API error | `502 Bad Gateway` | |

---

#### `DELETE /admin/api/connections/:id`

Removes a bank connection from Breadbox. This action:

1. Calls Plaid's `/item/remove` endpoint to revoke the access token on Plaid's side.
2. Sets the connection status to `disconnected` and clears credentials.
3. Associated accounts and transactions are **preserved** for historical queries — `accounts.connection_id` is set to `NULL` (not deleted).

To restore access to the same institution, the user must go through Plaid Link again.

**Path Parameters**

| Parameter | Type | Description |
|---|---|---|
| `id` | UUID | Connection UUID. |

**Request Body**

None.

**Response — 204 No Content**

Empty body.

**Error Cases**

| Condition | Status | Description |
|---|---|---|
| Connection not found | `404 Not Found` | |
| Plaid API error during item removal | `502 Bad Gateway` | Breadbox still marks the connection as removed locally even if the Plaid call fails. |

---

### Family Members

#### `POST /admin/api/users`

Creates a new family member.

**Request Body**

```json
{
  "name": "Jordan Canales",
  "email": "jordan@example.com"
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | Yes | Display name. Must be non-empty. |
| `email` | string | No | Optional email address. Must be valid email format if provided. |

**Response — 201 Created**

```json
{
  "id": "u2b3c4d5-e6f7-890a-bcde-f12345678901",
  "name": "Jordan Canales",
  "email": "jordan@example.com",
  "created_at": "2026-03-01T10:00:00Z",
  "updated_at": "2026-03-01T10:00:00Z"
}
```

**Error Cases**

| Condition | Status | Description |
|---|---|---|
| Missing `name` | `422 Unprocessable Entity` | |
| Invalid email format | `422 Unprocessable Entity` | |

---

#### `PUT /admin/api/users/:id`

Updates a family member's name or email. All fields in the request body replace the current values. Omitted optional fields are not changed.

**Path Parameters**

| Parameter | Type | Description |
|---|---|---|
| `id` | UUID | User UUID. |

**Request Body**

```json
{
  "name": "Jordan M. Canales",
  "email": null
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | No | New display name. If provided, must be non-empty. |
| `email` | string \| null | No | New email. Pass `null` to clear the email. |

**Response — 200 OK**

Returns the updated User object (same schema as `POST /admin/api/users`).

**Error Cases**

| Condition | Status | Description |
|---|---|---|
| User not found | `404 Not Found` | |
| `name` is an empty string | `422 Unprocessable Entity` | |
| Invalid email format | `422 Unprocessable Entity` | |

---

### API Key Management

#### `POST /admin/api/api-keys`

Creates a new API key. The full plaintext key is returned in this response only and cannot be retrieved again.

**Request Body**

```json
{
  "name": "Home Assistant"
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | Yes | A human-readable label for this key (e.g., `"Claude Desktop"`, `"Home Assistant"`). |

**Response — 201 Created**

```json
{
  "id": "k1a2b3c4-d5e6-f789-0abc-def123456789",
  "name": "Home Assistant",
  "key": "bb_4xK9mZ2nQpR7sT1vW3yA5bC8dE6fG0hJqLnPuVw",
  "prefix": "bb_4xK9mZ2n",
  "created_at": "2026-03-01T10:00:00Z"
}
```

| Field | Type | Description |
|---|---|---|
| `id` | UUID | Key record UUID. Use this to revoke the key later. |
| `name` | string | The label provided at creation. |
| `key` | string | **The full plaintext API key. Store this securely. It will never be shown again.** |
| `prefix` | string | The shortened prefix used to identify the key in the admin UI without exposing the full key. |
| `created_at` | string (ISO 8601) | When the key was created. |

**Error Cases**

| Condition | Status | Description |
|---|---|---|
| Missing `name` | `422 Unprocessable Entity` | |

---

#### `DELETE /admin/api/api-keys/:id`

Revokes an API key. The key's `revoked_at` timestamp is set to the current time. Any in-flight requests using this key will fail immediately on their next validation check.

**Path Parameters**

| Parameter | Type | Description |
|---|---|---|
| `id` | UUID | Key record UUID (the `id` returned at creation). |

**Request Body**

None.

**Response — 204 No Content**

Empty body.

**Error Cases**

| Condition | Status | Description |
|---|---|---|
| Key not found | `404 Not Found` | |
| Key is already revoked | `409 Conflict` | Returns `{ "error": { "code": "ALREADY_REVOKED", "message": "..." } }` |

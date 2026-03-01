# Plaid Integration Specification

**Project:** Breadbox
**Version:** MVP
**Last Updated:** 2026-03-01

---

## Table of Contents

1. [Plaid Client Configuration](#1-plaid-client-configuration)
2. [Plaid Link Flow (Bank Connection)](#2-plaid-link-flow-bank-connection)
3. [Transaction Sync Engine](#3-transaction-sync-engine)
4. [Balance Refresh](#4-balance-refresh)
5. [Webhook Handling](#5-webhook-handling)
6. [Connection Lifecycle](#6-connection-lifecycle)
7. [API Call Budget](#7-api-call-budget)
8. [Provider Interface](#8-provider-interface)

---

## 1. Plaid Client Configuration

### 1.1 SDK and Initialization

Breadbox uses the official `github.com/plaid/plaid-go` Go SDK. The Plaid API client is initialized once at application startup and shared across all components (sync engine, webhook handler, admin dashboard handlers) via dependency injection.

The client requires three configuration values:

| Parameter | Description |
|-----------|-------------|
| `client_id` | Plaid-issued API client identifier |
| `secret` | Plaid-issued API secret (environment-specific) |
| `environment` | One of: `sandbox`, `development`, `production` |

### 1.2 Environment Selection

Three Plaid environments are supported. The environment determines which Plaid API base URL is used:

| Environment | Base URL | Purpose |
|-------------|----------|---------|
| `sandbox` | `https://sandbox.plaid.com` | Local development and testing; test credentials only |
| `development` | `https://development.plaid.com` | Up to 100 live Items; free but limited |
| `production` | `https://production.plaid.com` | Full production access; billed at ~$1.50/Item/month |

The Go SDK selects the base URL automatically based on the `plaid.Environment` constant passed to `plaid.NewAPIClient(cfg)`.

### 1.3 Credential Sourcing

Credentials are sourced in this priority order:

1. **Environment variables** (primary): `PLAID_CLIENT_ID`, `PLAID_SECRET`, `PLAID_ENV`. When set, environment variables take precedence over all other sources. This follows the standard containerized app convention: `Environment Variable → overrides → app_config table → default`.
2. **App Config table** (fallback): Values set via the first-run setup wizard are stored in the `app_config` table under keys `plaid_client_id`, `plaid_secret`, and `plaid_env`. These are read at startup and on re-initialization if credentials change, but are overridden by any environment variable that is set.

The setup wizard writes to the App Config table; environment variables are read-only and never written back. If neither source provides credentials, Breadbox refuses to start and logs a clear error.

The Plaid `secret` is sensitive. It is never stored in plaintext beyond the App Config table row and is not logged. The `access_token` values stored per connection are AES-256-GCM encrypted at rest (see Section 6).

### 1.4 Go SDK Initialization Pattern

```go
cfg := plaid.NewConfiguration()
cfg.AddDefaultHeader("PLAID-CLIENT-ID", clientID)
cfg.AddDefaultHeader("PLAID-SECRET", secret)
cfg.UseEnvironment(plaid.Sandbox) // or Production, Development
client := plaid.NewAPIClient(cfg)
```

The resulting `*plaid.APIClient` is the single shared instance injected into all components.

---

## 2. Plaid Link Flow (Bank Connection)

### 2.1 New Connection Flow

The flow to connect a new bank account involves four stages: link token creation (backend), Plaid Link UI (frontend), public token exchange (backend), and account discovery + initial sync trigger (backend).

#### Stage 1: Link Token Creation

The backend calls `/link/token/create` before rendering the "Add Connection" page. The link token is short-lived (30 minutes) and must be passed to the frontend to initialize Plaid Link JS.

**Endpoint:** `POST /link/token/create`

**Request fields:**

| Field | Value | Notes |
|-------|-------|-------|
| `client_name` | `"Breadbox"` | Displayed to user in Link UI; max 30 chars |
| `language` | `"en"` | ISO 639-1 language code |
| `country_codes` | `["US"]` | ISO-3166-1 alpha-2; extend as needed |
| `user.client_user_id` | Internal user UUID | Must be stable per user; must not be PII (no email/phone) |
| `products` | `["transactions"]` | MVP uses Transactions only; includes basic balance data |
| `transactions.days_requested` | `730` | Request maximum 2 years of history on first link |
| `webhook` | Configured webhook URL | Set if webhook URL is configured in App Config; omit otherwise |

**Why `transactions.days_requested: 730`:** Plaid defaults to 90 days of history. For a personal finance aggregator, users expect full history. Setting this to the maximum (730 days) at link time enables the initial historical backfill without re-linking. The default without this field is 90 days; the maximum is 730 days. Note: requesting extended history increases the time for `HISTORICAL_UPDATE_COMPLETE` to fire.

**Response fields used:**

| Field | Description |
|-------|-------------|
| `link_token` | Opaque token passed to Plaid Link JS; expires in 30 minutes |

#### Stage 2: Plaid Link JS (Frontend)

Plaid Link is initialized in the admin dashboard via the `@plaid/link` script loaded from Plaid's CDN. No npm build step; loaded via `<script>` tag.

```html
<script src="https://cdn.plaid.com/link/v2/stable/link-initialize.js"></script>
```

Initialization and callback handling (Go template pseudocode):

```javascript
const handler = Plaid.create({
  token: "{{ .LinkToken }}",  // injected by Go template
  onSuccess: function(publicToken, metadata) {
    // POST publicToken + metadata to /admin/connections/exchange
    fetch('/admin/connections/exchange', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({
        public_token: publicToken,
        institution_id: metadata.institution.institution_id,
        institution_name: metadata.institution.name,
        accounts: metadata.accounts  // [{id, name, mask, type, subtype}]
      })
    }).then(() => window.location.href = '/admin/connections');
  },
  onExit: function(err, metadata) {
    // Log and display error if err is non-null
  },
  onEvent: function(eventName, metadata) {
    // Optional: log Link events for debugging
  }
});
handler.open();
```

The `metadata.accounts` array from `onSuccess` contains preliminary account information (id, name, mask, type, subtype) that is used to pre-populate accounts without an extra API call. However, these are confirmed against the `/accounts/get` response after exchange (see Stage 4).

#### Stage 3: Public Token Exchange

The admin dashboard backend endpoint `/admin/connections/exchange` receives the POST from the frontend and performs the exchange.

**Endpoint:** `POST /item/public_token/exchange`

**Request fields:**

| Field | Value |
|-------|-------|
| `public_token` | The ephemeral token from `onSuccess`; valid for 30 minutes |
| `client_id` | From App Config |
| `secret` | From App Config |

**Response fields:**

| Field | Description |
|-------|-------------|
| `access_token` | Long-lived token for subsequent API calls; encrypted and stored in `bank_connections` |
| `item_id` | Plaid's stable identifier for this institution connection; stored in `bank_connections` |
| `request_id` | For troubleshooting only; not stored |

**Storage after exchange:**

A new row is inserted into `bank_connections` with:

| Column | Value |
|--------|-------|
| `provider` | `"plaid"` |
| `institution_id` | From `onSuccess` metadata |
| `institution_name` | From `onSuccess` metadata |
| `plaid_item_id` | `item_id` from exchange response |
| `plaid_access_token` | `access_token` encrypted with AES-256-GCM |
| `status` | `"active"` |
| `sync_cursor` | NULL (no sync has occurred) |
| `user_id` | The family member assigned at link time |

#### Stage 4: Account Discovery

After storing the connection, call `/accounts/get` with the new `access_token` to get the authoritative account list.

**Endpoint:** `POST /accounts/get`

**Request fields:** `access_token`, `client_id`, `secret`

**Response — `accounts` array fields used:**

| Plaid Field | Maps To | Description |
|-------------|---------|-------------|
| `account_id` | `external_account_id` | Stable Plaid account identifier |
| `name` | `name` | Account display name |
| `official_name` | `official_name` | Full official name (may be null) |
| `mask` | `mask` | Last 4 digits |
| `type` | `type` | `depository`, `credit`, `loan`, `investment` |
| `subtype` | `subtype` | `checking`, `savings`, `credit card`, etc. |
| `balances.current` | `balance_current` | Current balance |
| `balances.available` | `balance_available` | Available balance (null for credit) |
| `balances.limit` | `balance_limit` | Credit limit (null for depository) |
| `balances.iso_currency_code` | `iso_currency_code` | 3-letter ISO 4217 code |

Upsert accounts into the `accounts` table keyed on `external_account_id`.

#### Stage 5: Initial Sync Trigger

After account discovery, enqueue an asynchronous sync job for the new connection. The sync engine (Section 3) handles the actual work. The admin dashboard redirects the user to the connections list; sync progress is visible via the connection status endpoint.

### 2.2 Re-Authentication Flow (Link Update Mode)

#### When Re-Authentication Is Needed

Re-authentication is required when Plaid returns item-level errors that prevent data access. These are surfaced either through:

- A webhook with `webhook_type: "ITEM"` and `webhook_code: "ERROR"` containing an `ITEM_LOGIN_REQUIRED` (or related) error
- A `PENDING_EXPIRATION` webhook (7 days before OAuth consent expires in UK/EU)
- A failed sync attempt returning an error with `error_type: "ITEM_ERROR"` and `error_code: "ITEM_LOGIN_REQUIRED"`

Error codes that require re-authentication:

| Error Code | Cause |
|------------|-------|
| `ITEM_LOGIN_REQUIRED` | Password changed, MFA expired, OAuth consent revoked |
| `INSUFFICIENT_CREDENTIALS` | User abandoned OAuth flow |
| `INVALID_CREDENTIALS` | Institution rejected credentials |
| `MFA_NOT_SUPPORTED` | User's MFA type unsupported |
| `NO_ACCOUNTS` | All accounts closed or access revoked |
| `USER_SETUP_REQUIRED` | User must complete setup at institution website |

When any of these are detected, the connection status is set to `pending_reauth` (see Section 6).

#### Link Token Creation for Update Mode

Update mode uses the same `/link/token/create` endpoint but with the `access_token` of the existing Item.

**Endpoint:** `POST /link/token/create`

**Request fields that differ from new connection:**

| Field | Value | Notes |
|-------|-------|-------|
| `access_token` | Decrypted access token of the existing Item | Presence of this field activates update mode |
| `products` | Omit | Products are already attached to the Item |
| `transactions.days_requested` | Omit | Not applicable in update mode |

All other fields (`client_name`, `language`, `country_codes`, `user.client_user_id`, `webhook`) are identical to the new connection flow.

#### No Token Exchange After Update Mode

Critically: update mode does **not** produce a new `access_token`. The existing `access_token` remains valid after the user re-authenticates. The `onSuccess` callback receives a `public_token` but **it must not be exchanged**. The connection row is updated: `status` is set back to `active`, `error_code` and `error_message` are cleared, and the next sync will use the existing cursor.

#### Admin Dashboard UX

- The connections list shows a "Re-authenticate" button for connections in `pending_reauth` or `error` status.
- Clicking this button calls a backend endpoint that creates an update-mode link token and renders a page that opens Plaid Link.
- On `onSuccess`, the frontend POSTs to `/admin/connections/:id/reauth-complete`.
- The backend updates the connection status to `active` and triggers an immediate sync.

---

## 3. Transaction Sync Engine

### 3.1 Endpoint Overview

All transaction syncing uses `/transactions/sync`. This endpoint uses a cursor to track the position in the transaction update stream, returning only changes (added, modified, removed) since the last sync.

**Endpoint:** `POST /transactions/sync`

**Request fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `access_token` | string | Yes | Item access token |
| `client_id` | string | Yes | From config |
| `secret` | string | Yes | From config |
| `cursor` | string | No | Omit on first sync; use stored cursor on subsequent syncs |
| `count` | integer | No | Transactions per page; default 100, max 500. Use 500 for efficiency. |

> **Note:** `original_description` is excluded; the `name` field covers this use case.

**Response fields:**

| Field | Type | Description |
|-------|------|-------------|
| `added` | array | New transactions since cursor |
| `modified` | array | Changed transactions since cursor |
| `removed` | array | Objects with `transaction_id` of deleted transactions |
| `next_cursor` | string | Store this; use as `cursor` on next call |
| `has_more` | boolean | If true, call again immediately with `next_cursor` |
| `accounts` | array | Updated account data (balances); use to refresh account balances |
| `transactions_update_status` | string | `NOT_READY`, `INITIAL_UPDATE_COMPLETE`, or `HISTORICAL_UPDATE_COMPLETE` |

### 3.2 Transaction Object Fields

Each object in the `added` and `modified` arrays contains:

| Plaid Field | Breadbox Column | Type | Notes |
|-------------|-----------------|------|-------|
| `transaction_id` | `external_transaction_id` | string | Stable Plaid ID; used as upsert key |
| `pending_transaction_id` | `pending_transaction_id` | string | Non-null when this posted tx replaces a pending tx |
| `account_id` | (join to `accounts.external_account_id`) | string | Link to account |
| `amount` | `amount` | NUMERIC(12,2) | Positive = debit (money out); negative = credit (money in) |
| `iso_currency_code` | `iso_currency_code` | string | ISO 4217; may be null if `unofficial_currency_code` is set |
| `date` | `date` | date | Posted date (or expected posted date for pending) |
| `authorized_date` | `authorized_date` | date | Date transaction was authorized; may be null |
| `name` | `name` | string | Plaid-cleaned merchant/payee name |
| `merchant_name` | `merchant_name` | string | Further-normalized merchant name; may be null |
| `personal_finance_category.primary` | `category_primary` | string | e.g., `FOOD_AND_DRINK`, `TRANSPORTATION` |
| `personal_finance_category.detailed` | `category_detailed` | string | e.g., `FOOD_AND_DRINK_RESTAURANTS` |
| `payment_channel` | `payment_channel` | string | `online`, `in store`, `other` |
| `pending` | `pending` | boolean | True if transaction has not posted |
| `transaction_type` | — | string | Deprecated; use `personal_finance_category` instead |

### 3.3 Sync Algorithm

The sync algorithm runs per-connection. All connections may be synced concurrently (separate goroutines), but a single connection is only synced by one goroutine at a time (mutex per connection or per-connection sync lock in DB).

```
function syncConnection(connectionID):
    connection = loadConnection(connectionID)
    decryptedToken = decrypt(connection.plaid_access_token)
    cursor = connection.sync_cursor  // null on first sync

    previousCursor = cursor  // preserved for mutation-restart

    loop:
        response = callPlaid("/transactions/sync", {
            access_token: decryptedToken,
            cursor: cursor,        // omit if null (first sync)
            count: 500
        })

        if error is TRANSACTIONS_SYNC_MUTATION_DURING_PAGINATION:
            cursor = previousCursor  // restart from last committed cursor
            continue

        if error is item-level (ITEM_LOGIN_REQUIRED, etc.):
            markConnectionError(connectionID, error)
            return

        if error is rate limit:
            backoffAndRetry()
            continue

        if network/transient error:
            retryWithExponentialBackoff()
            continue

        processAdded(response.added)
        processModified(response.modified)
        processRemoved(response.removed)
        updateAccountBalances(response.accounts)

        if not response.has_more:
            commitCursor(connectionID, response.next_cursor)
            previousCursor = response.next_cursor
            break
        else:
            cursor = response.next_cursor
            // do NOT commit cursor yet; has_more=true means page is incomplete

    writeSyncLog(connectionID, added_count, modified_count, removed_count, status="success")
```

**Key invariant:** The stored `sync_cursor` is only updated after `has_more` is `false`. Committing a mid-pagination cursor would cause data loss on restart.

### 3.4 Processing Added Transactions

For each object in `response.added`:

1. Look up the `account_id` in `accounts` table by `external_account_id`.
2. Upsert into `transactions` on conflict `(external_transaction_id)`:
   - On insert: populate all columns from the transaction object.
   - On conflict: update all mutable fields (name, amount, date, category, pending, etc.).
   - Never update `created_at`; always update `updated_at`.
3. If `pending_transaction_id` is non-null, check whether the referenced pending transaction exists in the database. If it does and has `deleted_at` set (it will have been soft-deleted by `processRemoved` in this same batch or a prior sync), record the link for historical continuity. The posted transaction carries the lineage via its `pending_transaction_id` column.

### 3.5 Processing Modified Transactions

For each object in `response.modified`: update the existing row in `transactions` by `external_transaction_id`. Apply the same field updates as the upsert above. If the row does not exist (possible in edge cases), insert it as in Section 3.4.

### 3.6 Processing Removed Transactions

For each object in `response.removed` (each contains only `transaction_id`):

1. Set `deleted_at = NOW()` on the matching `transactions` row (soft delete).
2. The REST API excludes rows with `deleted_at IS NOT NULL` by default.
3. Do not hard-delete; the `pending_transaction_id` on posted transactions may reference these removed pending IDs.

### 3.7 Pending to Posted Transition

When a pending transaction posts at the institution:

1. Plaid adds the pending transaction's ID to `response.removed`.
2. Plaid adds a new posted transaction to `response.added` with:
   - A new, different `transaction_id`
   - `pending: false`
   - `pending_transaction_id` set to the old pending `transaction_id`

Breadbox handling:
1. `processRemoved` soft-deletes the pending transaction row (sets `deleted_at`).
2. `processAdded` inserts the new posted transaction with `pending_transaction_id` referencing the old ID.
3. The old pending row remains queryable with `include_deleted=true` for audit purposes.
4. The REST API transaction list returns only the posted transaction by default.

**Order of operations within a batch:** Always process `removed` before `added` within a single response page so that the soft-delete of the pending row happens before the posted row is inserted. This prevents foreign-key-style confusion if any query runs between the two operations.

### 3.8 Cursor Storage

The cursor is stored in `bank_connections.sync_cursor`. It is updated atomically with the sync log write in the same database transaction. On success:

```sql
UPDATE bank_connections
SET sync_cursor = $1, last_synced_at = NOW()
WHERE id = $2;
```

If the process crashes mid-sync before committing the cursor, the next sync resumes from the last committed cursor, which may result in re-processing already-seen transactions. The upsert logic (Section 3.4) makes this safe.

### 3.9 Error Handling

| Error | Action |
|-------|--------|
| `TRANSACTIONS_SYNC_MUTATION_DURING_PAGINATION` | Restart loop from `previousCursor` (last committed cursor). Plaid modifies its transaction stream while pagination is in flight; this signals that the cursor state changed mid-page. |
| `RATE_LIMIT_EXCEEDED` | Exponential backoff: wait 2s, 4s, 8s, 16s (cap at 60s). Log each retry. Give up and write error to sync log after 5 attempts. |
| `ITEM_LOGIN_REQUIRED` (or any re-auth error) | Stop sync, set connection `status = "error"`, store `error_code` and `error_message` in `bank_connections`. Do not retry until re-authentication completes. |
| Network timeout / 5xx from Plaid | Retry up to 3 times with exponential backoff (1s, 2s, 4s). If all fail, write failure to sync log; scheduler will retry at next interval. |
| Unknown / unexpected error | Log full error context, write failure to sync log, do not update cursor. |

**Rate limits:**
- Production: 50 requests per Item per minute; 2,500 requests per client per minute
- Sandbox: 50 requests per Item per minute; 1,000 requests per client per minute

With a default sync interval of 12 hours and `count=500`, a normal incremental sync for a single Item requires 1-3 API calls. Rate limits are only a concern during bulk historical backfill of many Items simultaneously; implement a semaphore or staggered schedule for initial sync of a large number of connections.

### 3.10 `days_requested` Parameter

`days_requested` is set at **link token creation time** (see Section 2.1), not at sync time. It controls how much historical data Plaid retrieves during the initial backfill after linking.

- Default: 90 days
- Maximum: 730 days (approximately 2 years)
- Minimum: 1 day (30 days in production per Plaid's rules)
- Breadbox sets this to `730` for maximum history on new connections

Setting `days_requested` to 730 means the initial `HISTORICAL_UPDATE_COMPLETE` webhook will fire later (up to several minutes after linking for most institutions). The `transactions_update_status` field in sync responses reflects this progress.

---

## 4. Balance Refresh

### 4.1 `/accounts/get` (Cached Balances)

**Endpoint:** `POST /accounts/get`

- **Cost:** Included in Transactions product subscription; no additional per-call billing
- **Data freshness:** Returns cached balances from Plaid's last successful Item refresh. For Items with Transactions enabled, this typically updates approximately once per day.
- **Use when:** Displaying current balances in the dashboard or API responses. Sufficient for nearly all Breadbox use cases.

The `accounts` array returned by `/transactions/sync` contains the same cached balance data as `/accounts/get`. Breadbox refreshes balances from the `accounts` array in every sync response rather than making a separate `/accounts/get` call.

### 4.2 `/accounts/balance/get` (Real-Time Balances)

**Endpoint:** `POST /accounts/balance/get`

- **Cost:** Billed per request as an add-on; not included in base Transactions subscription
- **Data freshness:** Real-time; Plaid contacts the institution synchronously
- **Latency:** Higher; institution contact may take several seconds
- **Use when:** Never in MVP. This endpoint is relevant for payment risk assessment (ACH funding decisions), which is out of scope.

Breadbox does **not** call `/accounts/balance/get` in MVP. All balance data comes from the `accounts` array in `/transactions/sync` responses.

### 4.3 Balance Data Mapping

Balances are refreshed in the `accounts` table after every sync. Fields sourced from the `accounts` array in `/transactions/sync` (or from `/accounts/get` during initial account discovery):

| Plaid Field | Breadbox Column | Notes |
|-------------|-----------------|-------|
| `balances.current` | `balance_current` | Ledger balance; does not net pending |
| `balances.available` | `balance_available` | Current minus pending; null for credit accounts |
| `balances.limit` | `balance_limit` | Credit limit; null for depository |
| `balances.iso_currency_code` | `iso_currency_code` | Prefer over unofficial_currency_code |
| `balances.last_updated_datetime` | `last_balance_update` | When Plaid last fetched this balance |

---

## 5. Webhook Handling

### 5.1 Webhook Endpoint

Breadbox exposes a single webhook endpoint for all Plaid events:

```
POST /webhooks/plaid
```

This endpoint is:
- Unauthenticated (no API key required; authenticity is verified via Plaid's JWT mechanism)
- Publicly accessible (requires a public URL; Cloudflare Tunnel recommended for self-hosted setups)
- Configured in Plaid via the `webhook` field in `/link/token/create`

The webhook URL is stored in App Config under `webhook_url`. It can be configured or updated in the setup wizard and takes effect on the next link token creation or via `/item/webhook/update`.

### 5.2 Webhook Verification

All incoming webhooks must be verified before processing. Plaid signs each webhook with an ES256 JWT in the `Plaid-Verification` HTTP header.

**Verification steps:**

1. **Extract the JWT** from the `Plaid-Verification` request header.

2. **Decode the JWT header** (without verifying signature) to extract `alg` and `kid`.

3. **Validate algorithm:** If `alg` is not `"ES256"`, reject the request with HTTP 400.

4. **Fetch the public key** by calling `/webhook_verification_key/get`:
   - Request: `{ "client_id": "...", "secret": "...", "key_id": "<kid from JWT header>" }`
   - Response: A JWK object with fields `alg`, `crv`, `kty`, `use`, `x`, `y`, `kid`, `created_at`, `expired_at`
   - Cache this key by `kid`; keys rotate but old keys remain valid for existing webhooks
   - Reject the key if its `expired_at` is non-null and in the past

5. **Verify the JWT signature** using the retrieved JWK public key with an ES256 JWT library.

6. **Validate webhook age:** Extract the `iat` (issued-at) claim from the verified JWT payload. Reject if `NOW() - iat > 5 minutes` to prevent replay attacks.

7. **Verify payload integrity:** Extract `request_body_sha256` from the verified JWT payload. Compute SHA-256 of the raw webhook request body (as received; whitespace-sensitive, tab-spacing of 2). Compare using constant-time equality. Reject if mismatch.

8. Only if all checks pass, parse and process the webhook body.

Implementation note: Cache JWKs in memory keyed by `kid`. Re-fetch only on cache miss. This avoids calling `/webhook_verification_key/get` on every webhook.

### 5.3 Webhook Events Handled

All Plaid webhooks include at minimum: `webhook_type`, `webhook_code`, `item_id`.

#### `TRANSACTIONS` / `SYNC_UPDATES_AVAILABLE`

Plaid fires this when new transaction data is available for an Item.

```json
{
  "webhook_type": "TRANSACTIONS",
  "webhook_code": "SYNC_UPDATES_AVAILABLE",
  "item_id": "wz666mbe9q...",
  "initial_update_complete": true,
  "historical_update_complete": false
}
```

**Breadbox action:** Look up the connection by `plaid_item_id = item_id`. Enqueue an incremental sync job. If a sync for this connection is already queued or running, skip (idempotent). Do not block the webhook response waiting for sync completion; return HTTP 200 immediately.

#### `ITEM` / `ERROR`

Plaid fires this when an Item enters an error state.

```json
{
  "webhook_type": "ITEM",
  "webhook_code": "ERROR",
  "item_id": "wz666mbe9q...",
  "error": {
    "error_type": "ITEM_ERROR",
    "error_code": "ITEM_LOGIN_REQUIRED",
    "error_message": "the login details of this item have changed...",
    "display_message": null
  }
}
```

**Breadbox action:** Look up connection by `plaid_item_id`. Set `status = "error"`, store `error_code` and `error_message` in `bank_connections`. If `error_code` is one of the re-auth codes (Section 2.2), set `status = "pending_reauth"`. Cancel any pending sync jobs for this connection.

#### `ITEM` / `PENDING_EXPIRATION`

Fired approximately 7 days before an Item's OAuth consent expires (primarily UK/EU institutions, but may apply to US OAuth items).

```json
{
  "webhook_type": "ITEM",
  "webhook_code": "PENDING_EXPIRATION",
  "item_id": "wz666mbe9q...",
  "consent_expiration_time": "2026-03-08T00:00:00Z"
}
```

**Breadbox action:** Set connection `status = "pending_reauth"`, store `consent_expiration_time` in `bank_connections`. The dashboard will surface this as a warning prompting re-authentication before expiry. Do not stop syncing; the Item is still functional until actual expiration.

#### `ITEM` / `NEW_ACCOUNTS_AVAILABLE`

Plaid detected new accounts at the institution that the user has not yet shared with Breadbox.

```json
{
  "webhook_type": "ITEM",
  "webhook_code": "NEW_ACCOUNTS_AVAILABLE",
  "item_id": "wz666mbe9q..."
}
```

**Breadbox action:** Set a flag on the connection (`new_accounts_available = true`). The admin dashboard surfaces a prompt offering to launch update mode with `update.account_selection_enabled = true`. No automatic account addition — user consent is required.

### 5.4 Idempotency

Plaid may deliver the same webhook more than once (at-least-once delivery). Breadbox handles this:

- **`SYNC_UPDATES_AVAILABLE`:** Syncs are inherently idempotent due to cursor management. A duplicate webhook triggers a sync that finds no new data and returns immediately after calling `/transactions/sync` once and receiving an empty `added`/`modified`/`removed` with `has_more: false`.
- **`ITEM` / `ERROR`:** Setting status on a connection is idempotent; writing the same error code twice has no adverse effect.
- **Deduplication:** Optionally store the `request_id` from the Plaid-Verification JWT payload in a short-lived cache (Redis or in-memory with TTL of 10 minutes) and skip processing if already seen.

### 5.5 Webhook URL Configuration

The webhook URL must be publicly accessible. Breadbox documents the following setup options:

1. **Cloudflare Tunnel (recommended for self-hosted):** Creates a stable public URL (`https://<name>.cfargotunnel.com`) that tunnels to the local Breadbox instance. No port forwarding or static IP required.
2. **Reverse proxy with public IP:** Nginx/Caddy in front of Breadbox with a domain and TLS certificate.
3. **No webhooks (polling-only fallback):** If no webhook URL is configured, Breadbox relies solely on the cron-based sync schedule. Webhook URL field in setup wizard is optional; leaving it blank disables webhook-driven sync.

---

## 6. Connection Lifecycle

### 6.1 Connection States

| Status | Description |
|--------|-------------|
| `active` | Connection is healthy; syncing normally |
| `pending_reauth` | Re-authentication required (PENDING_EXPIRATION, consent near expiry, or preventive prompt) |
| `error` | Item is in an error state; sync is blocked until resolved |
| `disconnected` | User has intentionally removed the connection; Item has been removed from Plaid |

### 6.2 State Transitions

```
[Initial Link]
     |
     v
  active <-----------------------------+
     |                                 |
     | ITEM_ERROR webhook              | Successful re-auth
     | or sync returns ITEM_LOGIN_REQUIRED
     v                                 |
   error ---------> pending_reauth ---+
     |
     | PENDING_EXPIRATION webhook
     v
  pending_reauth
     |
     | User triggers /item/remove
     | or admin disconnects
     v
  disconnected
```

**Transitions in detail:**

| From | To | Trigger |
|------|----|---------|
| `active` | `error` | Sync or webhook returns item-level error requiring re-auth |
| `active` | `pending_reauth` | `PENDING_EXPIRATION` webhook |
| `error` | `pending_reauth` | Error code is a re-auth code; dashboard prompts re-auth |
| `pending_reauth` | `active` | User completes re-auth via Link update mode |
| `error` | `active` | User completes re-auth via Link update mode |
| `active` | `disconnected` | Admin deletes connection |
| `error` | `disconnected` | Admin deletes connection |
| `pending_reauth` | `disconnected` | Admin deletes connection |

### 6.3 Admin Dashboard Connection Health

The connections list (`GET /api/v1/connections`) returns:

| Field | Description |
|-------|-------------|
| `status` | One of the four states above |
| `error_code` | Plaid error code if `status = "error"` |
| `error_message` | Human-readable error description |
| `last_synced_at` | Timestamp of last successful sync completion |
| `next_sync_at` | Estimated next scheduled sync time |
| `new_accounts_available` | Boolean; true if `NEW_ACCOUNTS_AVAILABLE` webhook was received |
| `consent_expiration_time` | Non-null if expiration is approaching |

The dashboard renders:
- Green status indicator for `active`
- Yellow warning for `pending_reauth` with "Re-authenticate" button
- Red error indicator for `error` with error message and "Re-authenticate" button
- Gray for `disconnected`

### 6.4 Item Removal

When an admin deletes a connection from the dashboard:

1. **Call Plaid:** `POST /item/remove` with the decrypted `access_token`. This invalidates the access token and stops Plaid billing for the Item. Provide `reason_code` if the SDK supports it.
2. **Update connection row:** Set `status = "disconnected"`, `plaid_access_token = NULL` (or wipe encrypted value), `deleted_at = NOW()`.
3. **Retain historical data:** Accounts and transactions linked to this connection are **not** deleted. They remain queryable and are marked with their `connection_id` for reference. The REST API can filter by connection status if callers want to exclude disconnected sources.
4. **Soft-delete accounts:** Optionally mark accounts as `active = false` to exclude them from the default account listing while preserving transaction history.

If `/item/remove` fails (e.g., token already invalid), log the error and proceed with local cleanup anyway. Plaid will eventually expire the token.

---

## 7. API Call Budget

### 7.1 Calls Per Operation

| Operation | Endpoint(s) Called | Count |
|-----------|--------------------|-------|
| New connection | `/link/token/create`, `/item/public_token/exchange`, `/accounts/get` | 3 |
| Incremental sync (typical) | `/transactions/sync` × 1-2 pages | 1-2 |
| Incremental sync (large backlog) | `/transactions/sync` × N pages (500 tx/page) | N |
| Re-auth link token creation | `/link/token/create` | 1 |
| Webhook verification (cached JWK) | 0 (JWK in memory) | 0 |
| Webhook verification (cache miss) | `/webhook_verification_key/get` | 1 |
| Item removal | `/item/remove` | 1 |

### 7.2 Sync Frequency Analysis

Default sync interval: 12 hours. With webhooks enabled, most syncs are webhook-triggered within minutes of new transactions.

| Scenario | Calls/Day/Item | Notes |
|----------|----------------|-------|
| Webhook-driven, 2 syncs/day | 2-4 | Each sync: 1-2 pages |
| Polling-only, 12h interval | 2-4 | Same as webhook-driven |
| Initial historical backfill (730 days) | 10-20 (one-time) | More pages for historical volume |

With 10 active Items at 12h polling, the client sees roughly 20-40 `/transactions/sync` calls per day, well under the 2,500/minute client rate limit.

### 7.3 Cost Minimization Strategies

1. **Use `/transactions/sync` exclusively for balance data.** The `accounts` array in the sync response contains cached balances. Do not call `/accounts/get` separately after the initial account discovery.
2. **Never call `/accounts/balance/get`.** This is a paid add-on not needed for Breadbox's use case (no ACH payments).
3. **Webhook-driven sync reduces unnecessary polling.** With `SYNC_UPDATES_AVAILABLE`, Breadbox only calls `/transactions/sync` when Plaid has new data. This is more efficient than strict time-based polling.
4. **Use `count=500`.** Maximizing transactions per page minimizes the number of API calls per sync session, especially during historical backfill.
5. **JWK caching.** Cache webhook verification keys in memory to avoid `/webhook_verification_key/get` on every webhook.
6. **Skip sync if already in progress.** Per-connection sync locks prevent redundant concurrent syncs from duplicate webhook deliveries.
7. **Call `/item/remove` on disconnection.** Required to stop billing. Failing to call this continues Plaid subscription charges even after the user removes the connection in Breadbox.

### 7.4 Recommended Sync Strategy

Use a hybrid approach:

- **Primary trigger:** `SYNC_UPDATES_AVAILABLE` webhook → immediate incremental sync
- **Fallback trigger:** Cron at configured interval (default 12h) → incremental sync for all active connections
- **Startup trigger:** On server start, sync any connections whose `last_synced_at` is older than the configured interval

This ensures no data is missed if webhooks fail to deliver, while keeping API usage efficient when webhooks are working.

---

## 8. Provider Interface

### 8.1 Interface Design

The Plaid integration is implemented behind a `Provider` interface. This allows Teller and CSV import implementations to be added later without restructuring the sync engine, REST API, or webhook handler.

The interface is defined in Go:

```go
// Provider abstracts a bank data source.
// Implementations: PlaidProvider, TellerProvider (post-MVP), CSVProvider (post-MVP).
type Provider interface {
    // CreateLinkSession generates a link session to initialize the provider's link UI.
    // For new connections, userID identifies the user. For re-auth, use CreateReauthSession.
    CreateLinkSession(ctx context.Context, userID string) (LinkSession, error)

    // ExchangeToken completes the link flow and returns a new connection with its accounts.
    // Called after the frontend link UI succeeds.
    ExchangeToken(ctx context.Context, publicToken string) (Connection, []Account, error)

    // SyncTransactions performs an incremental sync for a connection from the given cursor.
    // Pass an empty cursor string on the first sync.
    SyncTransactions(ctx context.Context, conn Connection, cursor string) (SyncResult, error)

    // GetBalances returns current balances for all accounts on a connection.
    GetBalances(ctx context.Context, conn Connection) ([]AccountBalance, error)

    // HandleWebhook processes a verified inbound webhook payload.
    // Webhook signature verification happens in the HTTP handler before calling this method.
    HandleWebhook(ctx context.Context, payload WebhookPayload) (WebhookEvent, error)

    // CreateReauthSession generates a link session for re-authenticating an existing connection.
    CreateReauthSession(ctx context.Context, connectionID string) (LinkSession, error)

    // RemoveConnection invalidates credentials at the provider and marks
    // the connection disconnected. Historical data is retained.
    RemoveConnection(ctx context.Context, connectionID string) error

    // ProviderName returns the string identifier for this provider ("plaid", "teller", "csv").
    ProviderName() string
}
```

> **Note on webhook verification:** Plaid webhook signature verification (JWT/ES256 via `Plaid-Verification` header) is performed in the HTTP handler layer before `HandleWebhook` is called. The handler verifies the signature, validates the payload hash, and only passes a parsed `WebhookPayload` to the provider after all checks pass. This keeps cryptographic verification at the transport boundary and out of the provider interface.

### 8.2 Supporting Types

```go
type LinkMetadata struct {
    InstitutionID   string
    InstitutionName string
    Accounts        []AccountMetadata
}

type AccountMetadata struct {
    ExternalID string
    Name       string
    Mask       string
    Type       string
    Subtype    string
}

type Connection struct {
    ProviderName         string // "plaid", "teller", "csv" — matches architecture.md
    ExternalID           string // provider's identifier (e.g., Plaid item_id)
    EncryptedCredentials []byte // AES-256-GCM encrypted access_token (Plaid) or API key (Teller)
    InstitutionName      string
}

type SyncResult struct {
    Added    []Transaction
    Modified []Transaction
    Removed  []string  // external transaction IDs
    Cursor   string
    HasMore  bool
}

type WebhookResult struct {
    Action     string // "sync_triggered", "status_updated", "ignored"
    Connection string // connection ID affected, if any
}

type Account struct {
    ExternalID      string
    Name            string
    OfficialName    string
    Mask            string
    Type            string
    Subtype         string
    BalanceCurrent  *decimal.Decimal
    BalanceAvailable *decimal.Decimal
    BalanceLimit    *decimal.Decimal
    ISOCurrencyCode string
}
```

### 8.3 Provider Registration

The sync engine and webhook handler hold a `map[string]Provider` keyed by provider name. The Plaid provider is registered at startup. When a connection is loaded from the database, its `provider` column (`"plaid"`) is used to look up the correct provider implementation.

```go
providers := map[string]Provider{
    "plaid": plaid.NewProvider(plaidClient, db, encryptionKey),
    // "teller": teller.NewProvider(...),  // post-MVP
    // "csv":    csv.NewProvider(...),      // post-MVP
}
```

### 8.4 What the Interface Encapsulates

The `Provider` interface fully encapsulates all Plaid-specific concerns:

- Plaid SDK client and API calls
- Cursor management and sync pagination loop
- Pending→posted transaction reconciliation
- Access token encryption/decryption
- Plaid-specific error codes and re-auth detection

The sync engine, REST API, and webhook handler only interact with `Provider`; they have no knowledge of Plaid-specific data structures or error codes. Error types are translated to a provider-agnostic error vocabulary at the interface boundary. Webhook signature verification (Plaid JWT/ES256) is handled at the HTTP handler layer before the provider's `HandleWebhook` method is called.

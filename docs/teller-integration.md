# Teller Integration Specification

**Project:** Breadbox
**Version:** MVP
**Last Updated:** 2026-03-01

---

## Table of Contents

1. [Teller Client Configuration](#1-teller-client-configuration)
2. [Teller Connect Flow](#2-teller-connect-flow)
3. [Transaction Sync Engine](#3-transaction-sync-engine)
4. [Balance Refresh](#4-balance-refresh)
5. [Webhook Handling](#5-webhook-handling)
6. [Connection Lifecycle](#6-connection-lifecycle)
7. [Category Mapping](#7-category-mapping)
8. [Differences from Plaid](#8-differences-from-plaid)

---

## 1. Teller Client Configuration

### 1.1 Authentication Model

Teller uses a **dual-layer authentication** model:

1. **mTLS (Mutual TLS):** Every API request must include a client certificate
   that identifies the application. The certificate and private key are provided
   as files (`cert.pem`, `private-key.pem`) obtained from the Teller Dashboard.
   This is an application-level credential, not per-connection.

2. **HTTP Basic Auth:** The per-connection access token is passed as the
   username in HTTP Basic Auth. The password is always empty.

```
curl --cert cert.pem --key private-key.pem \
     -u token_xxxxx: \
     https://api.teller.io/accounts
```

**Base URL:** `https://api.teller.io/`

### 1.2 Environment Selection

| Environment | Data | Cost | Notes |
|---|---|---|---|
| `sandbox` | Simulated | Free | Unlimited enrollments. Test credentials: `username`/`password` |
| `development` | Real bank data | Free | 100 cumulative enrollments (deleting does not restore count) |
| `production` | Real bank data | Paid | Unlimited |

All environments use the same base URL. The environment is selected during
Teller Connect enrollment, not in the API call itself. Sandbox enrollments
cannot access real data, and vice versa.

**Sandbox test credentials:**

| Username | Password | Behavior |
|---|---|---|
| `username` | `password` | Successful enrollment |
| `otp` | `password` | MFA required (code: `0000`) |
| `challenge` | `password` | KBA challenge (answer: `blue`) |
| `disconnected` | `password` | Simulates disconnection |

### 1.3 Credential Sourcing

| Credential | Source | Storage |
|---|---|---|
| `TELLER_APP_ID` | Environment variable or `app_config` table | Application ID for Teller Connect |
| `TELLER_CERT_PATH` | Environment variable only | File path to mTLS certificate |
| `TELLER_KEY_PATH` | Environment variable only | File path to mTLS private key |
| `TELLER_ENV` | Environment variable or `app_config` table | `sandbox`, `development`, or `production` |
| `TELLER_WEBHOOK_SECRET` | Environment variable only | HMAC-SHA256 signing secret |

Certificate and key paths are environment-variable-only because they reference
filesystem paths. They cannot be stored in the `app_config` database table.
The webhook secret is also env-only since it is a deployment secret.

### 1.4 Go HTTP Client Pattern

```go
// internal/provider/teller/client.go

type Client struct {
    httpClient *http.Client
    baseURL    string
}

func NewClient(certPath, keyPath, env string) (*Client, error) {
    cert, err := tls.LoadX509KeyPair(certPath, keyPath)
    if err != nil {
        return nil, fmt.Errorf("load teller certificate: %w", err)
    }

    transport := &http.Transport{
        TLSClientConfig: &tls.Config{
            Certificates: []tls.Certificate{cert},
        },
    }

    return &Client{
        httpClient: &http.Client{
            Transport: transport,
            Timeout:   30 * time.Second,
        },
        baseURL: "https://api.teller.io",
    }, nil
}
```

Every API call sets Basic Auth on the request:

```go
func (c *Client) newRequest(method, path, accessToken string) (*http.Request, error) {
    req, err := http.NewRequest(method, c.baseURL+path, nil)
    if err != nil {
        return nil, err
    }
    req.SetBasicAuth(accessToken, "")
    return req, nil
}
```

### 1.5 Rate Limiting

Teller does not publish exact rate limits. HTTP `429` responses indicate a
rate limit breach. Implement the same exponential backoff as Plaid:
`2s → 4s → 8s → 16s → 32s`, cap at `60s`, max 5 retries.

---

## 2. Teller Connect Flow

### 2.1 New Connection Flow

Teller Connect is a JavaScript widget that runs entirely client-side. Unlike
Plaid Link, there is **no server-side link token creation**. The application ID
is the only input needed to initialize the widget.

**Frontend (JavaScript):**

```html
<script src="https://cdn.teller.io/connect/connect.js"></script>
<script>
var tellerConnect = TellerConnect.setup({
    applicationId: "app_xxxxxx",
    environment: "sandbox",
    products: ["transactions", "balance"],
    onSuccess: function(enrollment) {
        // POST enrollment data to server
        fetch('/admin/api/exchange-token', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({
                provider: 'teller',
                access_token: enrollment.accessToken,
                enrollment_id: enrollment.enrollment.id,
                institution_name: enrollment.enrollment.institution.name,
                user_id: selectedUserId
            })
        });
    },
    onExit: function() { /* user dismissed */ },
    onFailure: function(failure) { /* failure.type, failure.code */ }
});
tellerConnect.open();
</script>
```

**Teller Connect `onSuccess` payload:**

```json
{
    "accessToken": "token_xxxxxxxxxxxxx",
    "user": { "id": "usr_xxxxxxxxxxxxx" },
    "enrollment": {
        "id": "enr_xxxxxxxxxxxxx",
        "institution": { "name": "Example Bank" }
    },
    "signatures": ["xxxxxxxxxxxxx"]
}
```

### 2.2 Token Handling

There is no exchange step. The access token arrives directly from the Teller
Connect `onSuccess` callback. The server receives it in the POST body and:

1. Encrypts the access token with AES-256-GCM (shared `internal/crypto` package)
2. Stores the encrypted token in `bank_connections.encrypted_credentials`
3. Sets `bank_connections.external_id` to the enrollment ID (`enr_xxxxx`)
4. Sets `bank_connections.provider` to `'teller'`

**Provider interface mapping:**

- `CreateLinkSession(ctx, userID)` → Returns `LinkSession{Token: appID}`.
  The token is the static application ID, not a per-session value. The admin
  UI uses this to call `TellerConnect.setup({applicationId: token})`.

- `ExchangeToken(ctx, publicToken)` → For Teller, the "public token" is
  actually a JSON string containing `{access_token, enrollment_id, institution_name}`.
  The method parses this, encrypts the access token, calls `GET /accounts` to
  discover accounts, and returns a `Connection` + `[]Account`.

### 2.3 Account Discovery

Accounts are discovered by calling the Teller API after enrollment:

```
GET /accounts
Authorization: Basic {access_token}:
```

Response:

```json
[
    {
        "id": "acc_xxxxx",
        "enrollment_id": "enr_xxxxx",
        "type": "depository",
        "subtype": "checking",
        "status": "open",
        "currency": "USD",
        "last_four": "1234",
        "name": "My Checking",
        "institution": { "id": "ins_xxxxx", "name": "Example Bank" }
    }
]
```

**Field mapping to `provider.Account`:**

| Teller Field | Provider Field | Notes |
|---|---|---|
| `id` | `ExternalID` | |
| `name` | `Name` | |
| (not available) | `OfficialName` | Empty string |
| `type` | `Type` | Only `depository` or `credit` (no `loan`/`investment`) |
| `subtype` | `Subtype` | `checking`, `savings`, `money_market`, `certificate_of_deposit`, `credit_card`, etc. |
| `last_four` | `Mask` | |
| `currency` | `ISOCurrencyCode` | Typically `USD` |

### 2.4 Signature Verification (Optional)

If the server generates a `nonce` and passes it to `TellerConnect.setup()`,
the `onSuccess` callback includes `signatures` — ED25519 with SHA-256 digest
signatures of `nonce.accessToken.userId.enrollmentId.environment`. Verify
against the public key from the Teller Dashboard to prevent token injection.

This is recommended for production but not required for MVP.

---

## 3. Transaction Sync Engine

### 3.1 Sync Strategy (Date-Range Polling)

Teller does **not** offer a cursor-based incremental sync endpoint like Plaid's
`/transactions/sync`. Instead, transactions are fetched by date range and
reconciled with the database using upserts.

**Sync algorithm:**

1. Read `sync_cursor` from `bank_connections`. For Teller, this is an ISO
   timestamp string representing the last successful sync time.
2. Calculate the query window:
   - `from_date = sync_cursor - 10 days` (overlap catches pending→posted date shifts)
   - `to_date = today`
   - If `sync_cursor` is empty (initial sync), `from_date` = 2 years ago
3. For each account in the connection, fetch all transactions in the window.
4. Return all fetched transactions as `SyncResult.Added` (the sync engine's
   upsert handles duplicates via `ON CONFLICT`).
5. Set `SyncResult.HasMore = false` (date-range sync completes in one pass).
6. Set `SyncResult.Cursor` to the current timestamp as an ISO string.

**Why a 10-day overlap:** When a pending transaction posts, the posting date
may differ from the original pending date by several days. Fetching a window
that extends 10 days before the last sync ensures no posted transactions are
missed due to date shifts.

### 3.2 Pagination

Teller paginates transactions using the `from_id` query parameter:

```
GET /accounts/{account_id}/transactions?from_date=2026-01-01&to_date=2026-03-01&count=250
```

For subsequent pages:

```
GET /accounts/{account_id}/transactions?from_date=2026-01-01&to_date=2026-03-01&count=250&from_id=txn_last
```

Continue fetching until fewer than `count` transactions are returned.

### 3.3 Transaction Field Mapping

| Teller Field | Breadbox Field | Notes |
|---|---|---|
| `id` | `external_transaction_id` | Stable across most pending→posted transitions |
| `account_id` | `account_external_id` | Resolved to internal account UUID by sync engine |
| `amount` | `amount` | Parse string to decimal, **negate sign** (see 3.4) |
| `date` | `date` | ISO 8601 date string `YYYY-MM-DD` |
| `description` | `name` | Raw bank statement text |
| `status` | `pending` | `"pending"` → `true`, `"posted"` → `false` |
| `details.category` | `category_primary` | Via category mapping table (Section 7) |
| `details.counterparty.name` | `merchant_name` | Nullable |
| (not available) | `authorized_date` | `NULL` |
| (not available) | `datetime` | `NULL` |
| (not available) | `category_detailed` | `NULL` |
| (not available) | `category_confidence` | `NULL` |
| (not available) | `payment_channel` | `"other"` |
| (not available) | `pending_transaction_id` | `NULL` (Teller has no explicit linkage) |
| `currency` | `iso_currency_code` | From parent account's `currency` field |

### 3.4 Amount Sign Convention

Teller and Plaid use **opposite** sign conventions:

| Provider | Positive Amount | Negative Amount |
|---|---|---|
| Plaid | Money out (debit) | Money in (credit) |
| Teller | Money in (credit) | Money out (debit) |

Breadbox stores amounts using Plaid's convention (positive = debit). The Teller
provider **negates** all amounts before returning them in `SyncResult`:

```go
amount, _ := decimal.NewFromString(tellerTx.Amount)
transaction.Amount = amount.Neg()
```

### 3.5 Stale Pending Cleanup

After each Teller sync, the **sync engine** (not the provider) runs cleanup
for stale pending transactions. This handles the case where a pending
transaction posts under a new ID — the old pending record is never returned
by Teller again.

**Rules:**
- Only **pending** transactions are candidates for cleanup
- Only transactions within the sync date window are checked
- **Posted transactions are never auto-deleted** — absence from the API could
  be a query boundary issue or a transient API problem
- Cleanup uses soft-delete (`deleted_at = NOW()`)

```sql
UPDATE transactions
SET deleted_at = NOW()
WHERE account_id IN (... accounts for this connection ...)
  AND date >= $from_date
  AND date <= $to_date
  AND pending = true
  AND external_transaction_id NOT IN (... returned IDs ...)
  AND deleted_at IS NULL;
```

This logic is conditioned on `provider = 'teller'` in the sync engine. Plaid
handles removals through its own cursor-based removal signals.

### 3.6 Error Handling

| HTTP Status | Meaning | Action |
|---|---|---|
| `200` | Success | Process response |
| `403` | Invalid/revoked access token | Return `provider.ErrReauthRequired` |
| `404` with `enrollment.disconnected.*` | Enrollment broken | Return `provider.ErrReauthRequired` |
| `429` | Rate limit exceeded | Exponential backoff (same as Plaid) |
| `502` | Bank unavailable | Retry with backoff |
| `5xx` | Server error | Retry with backoff |

---

## 4. Balance Refresh

### 4.1 Per-Account Balance Fetch

Unlike Plaid (which returns all account balances in one call per item), Teller
requires a separate API call per account:

```
GET /accounts/{account_id}/balances
Authorization: Basic {access_token}:
```

Response:

```json
{
    "account_id": "acc_xxxxx",
    "ledger": "5000.00",
    "available": "4800.00",
    "links": { "self": "...", "account": "..." }
}
```

The `GetBalances` implementation iterates over all accounts for the connection
and makes one balance call per account.

### 4.2 Field Mapping

| Teller Field | Breadbox Field | Notes |
|---|---|---|
| `ledger` | `balance_current` | Total account funds (nullable) |
| `available` | `balance_available` | Funds minus pending (nullable) |
| (not available) | `balance_limit` | `NULL` — Teller does not return credit limits |

At least one of `ledger` or `available` is always present.

### 4.3 Currency Handling

The balance response does **not** include a currency field. The currency must
be taken from the parent account's `currency` field, which is stored in the
`accounts` table during enrollment.

---

## 5. Webhook Handling

### 5.1 Signature Verification

Teller webhooks are signed with **HMAC-SHA256** using the webhook secret
(`TELLER_WEBHOOK_SECRET`). The signature is in the `Teller-Signature` header.

**Header format:**

```
Teller-Signature: t=1688960969,v1=signature1,v1=signature2
```

**Verification steps:**

1. Extract `timestamp` from the `t=` component.
2. Create `signed_message` by joining timestamp and request body with a period:
   `{timestamp}.{json_body}`.
3. Compute HMAC-SHA256 of `signed_message` using the webhook secret as the key.
4. Compare the computed signature against each `v1=` value using
   constant-time comparison.
5. **Replay protection:** Reject events with timestamp older than 3 minutes
   (use 5 minutes to match Plaid's tolerance and handle clock skew).

Multiple `v1=` signatures support key rotation — if any one matches, the
webhook is valid.

### 5.2 Event Types

| Teller Event | `WebhookEvent.Type` | `NeedsReauth` | Action |
|---|---|---|---|
| `enrollment.disconnected` | `connection_error` | `true` | Set connection status to `pending_reauth` |
| `transactions.processed` | `sync_available` | `false` | Trigger sync for the connection |
| `webhook.test` | `unknown` | `false` | Log and acknowledge |
| (other) | `unknown` | `false` | Log and acknowledge |

### 5.3 Webhook Payload Structure

```json
{
    "id": "wh_xxxxx",
    "type": "enrollment.disconnected",
    "timestamp": "2023-07-10T03:49:29Z",
    "payload": {
        "enrollment_id": "enr_xxxxx",
        "reason": "disconnected.credentials_invalid"
    }
}
```

For `transactions.processed`, the payload includes the actual transaction
objects. However, the Breadbox implementation triggers a full sync rather than
processing the embedded transactions, to maintain consistency with the
date-range polling strategy.

### 5.4 Webhook Registration

Webhooks are configured in the Teller Dashboard, not via API. The webhook URL
should be: `https://{your-domain}/webhooks/teller`

The `{provider}` path parameter in the Breadbox webhook route (`/webhooks/{provider}`)
routes the request to the Teller provider's `HandleWebhook` method.

### 5.5 Connection Lookup

The `enrollment_id` in the webhook payload maps to `bank_connections.external_id`
where `provider = 'teller'`. The webhook handler calls
`GetBankConnectionByExternalID('teller', enrollmentID)` to resolve the
internal connection ID.

---

## 6. Connection Lifecycle

### 6.1 Reconnection

Teller reconnection is entirely client-side. When a connection is in
`pending_reauth` status, the admin UI passes the enrollment ID to Teller
Connect:

```javascript
TellerConnect.setup({
    applicationId: "app_xxxxxx",
    enrollmentId: "enr_xxxxx",  // triggers reconnection mode
    onSuccess: function(enrollment) {
        // Same access token is restored
        // POST to /admin/api/connections/{id}/reauth-complete
    }
});
```

**Provider interface mapping:**

`CreateReauthSession(ctx, conn)` returns `LinkSession{Token: conn.ExternalID}`.
The token is the enrollment ID, which the admin UI JS uses as the
`enrollmentId` parameter.

On success, the connection status is set back to `active`. No token exchange
is needed — Teller restores the same access token.

### 6.2 Connection Removal

To revoke Teller's access to a connection:

```
DELETE /enrollments/{enrollment_id}
Authorization: Basic {access_token}:
```

**Provider interface mapping:**

`RemoveConnection(ctx, conn)`:
1. Decrypt the access token from `conn.EncryptedCredentials`
2. Call `DELETE /enrollments/{conn.ExternalID}` with the decrypted token
3. Log and continue if the token is already invalid (idempotent)

After the API call succeeds, the application sets `status = 'disconnected'`
and clears `encrypted_credentials`.

### 6.3 Status Transitions

Teller connections follow the same status transitions as Plaid:

```
active  ←→  error               (API errors, bank issues)
active   →  pending_reauth      (credentials invalid, MFA required)
   *     →  disconnected        (user-initiated removal)
```

**Teller disconnection error codes that map to `pending_reauth`:**

- `enrollment.disconnected.credentials_invalid`
- `enrollment.disconnected.user_action.mfa_required`
- `enrollment.disconnected.user_action.web_login_required`
- `enrollment.disconnected.user_action.captcha_required`
- `enrollment.disconnected.user_action.contact_information_required`
- `enrollment.disconnected.user_action.insufficient_permissions`

**Error codes that map to `error`:**

- `enrollment.disconnected.account_locked`
- `enrollment.disconnected` (generic)

**Error codes that map to `disconnected`:**

- `enrollment.disconnected.enrollment_inactive`

---

## 7. Category Mapping

### 7.1 Teller Categories → Breadbox Primary Categories

Teller provides a single-level category in `details.category`. Breadbox
normalizes these to primary categories compatible with Plaid's taxonomy.

| Teller Category | Breadbox `category_primary` |
|---|---|
| `accommodation` | `TRAVEL` |
| `advertising` | `GENERAL_SERVICES` |
| `bar` | `FOOD_AND_DRINK` |
| `charity` | `GENERAL_SERVICES` |
| `clothing` | `SHOPPING` |
| `dining` | `FOOD_AND_DRINK` |
| `education` | `GENERAL_SERVICES` |
| `electronics` | `SHOPPING` |
| `entertainment` | `ENTERTAINMENT` |
| `fuel` | `TRANSPORTATION` |
| `general` | `GENERAL_MERCHANDISE` |
| `groceries` | `FOOD_AND_DRINK` |
| `health` | `MEDICAL` |
| `home` | `HOME_IMPROVEMENT` |
| `income` | `INCOME` |
| `insurance` | `LOAN_PAYMENTS` |
| `investment` | `TRANSFER_IN` |
| `loan` | `LOAN_PAYMENTS` |
| `office` | `GENERAL_SERVICES` |
| `phone` | `GENERAL_SERVICES` |
| `service` | `GENERAL_SERVICES` |
| `shopping` | `SHOPPING` |
| `software` | `GENERAL_SERVICES` |
| `sport` | `ENTERTAINMENT` |
| `tax` | `GOVERNMENT_AND_NON_PROFIT` |
| `transport` | `TRANSPORTATION` |
| `transportation` | `TRANSPORTATION` |
| `utilities` | `RENT_AND_UTILITIES` |

### 7.2 Unmapped Categories

If a Teller category is not in the mapping table (e.g., a new category added
by Teller), default to `GENERAL_MERCHANDISE`. The `category_detailed` field is
always `NULL` for Teller transactions since Teller has no sub-categories.

---

## 8. Differences from Plaid

### 8.1 Summary Table

| Aspect | Plaid | Teller | Implementation Notes |
|---|---|---|---|
| **API Auth** | API key + secret (headers) | mTLS certificate + access token (Basic Auth) | App-level cert/key files in config |
| **Link flow** | Server creates `link_token`, client uses it | Client-only: `applicationId` only | `CreateLinkSession` returns static app ID |
| **Token exchange** | `public_token` → `access_token` (server call) | Access token delivered directly in `onSuccess` | `ExchangeToken` encrypts + stores directly |
| **Transaction sync** | Cursor-based incremental (`/transactions/sync`) | Date-range polling, no cursor | "Cursor" = last sync timestamp |
| **Pending→Posted** | Explicit `pending_transaction_id` linkage | Stable IDs usually, but no linkage | Stale pending cleanup handles this |
| **Balances** | All accounts in one call per item | One call per account | Loop over accounts in `GetBalances` |
| **Webhooks** | JWT/ES256, `Plaid-Verification` header | HMAC-SHA256, `Teller-Signature` header | Provider-specific verification in `HandleWebhook` |
| **Webhook data** | Notification only (sync separately) | `transactions.processed` includes tx data | Trigger full sync regardless (consistency) |
| **Categories** | Two-level (primary + detailed) | Single-level (~27 categories) | Map to primary, `category_detailed = NULL` |
| **Amount sign** | Positive = debit | Negative = debit | Negate Teller amounts |
| **Authorized date** | Separate `authorized_date` field | Not provided | `NULL` for Teller |
| **Account types** | depository, credit, loan, investment | depository, credit only | Subset, no mapping needed |
| **Reconnection** | New `link_token` in update mode | Pass `enrollmentId` to Teller Connect | Client-side only, no server call |
| **Connection removal** | `/item/remove` | `DELETE /enrollments/{id}` | Both use decrypted access token |

### 8.2 Implications for Shared Code

- `SyncResult.HasMore` is always `false` for Teller (complete in one pass)
- `SyncResult.Cursor` is an ISO timestamp string (not an opaque cursor blob)
- `SyncResult.Modified` is always empty (Teller has no "modified" concept —
  upsert handles updates)
- `SyncResult.Removed` is always empty (stale pending cleanup is handled by
  the sync engine, not the provider)
- `Transaction.PendingExternalID` is always `nil`
- `Transaction.AuthorizedDate` is always `nil`
- `Transaction.PaymentChannel` is always `"other"`
- Amount sign must be negated before returning from the provider
- `AccountBalance.Limit` is always `nil`
- `AccountBalance.ISOCurrencyCode` comes from the account record, not the
  balance API response

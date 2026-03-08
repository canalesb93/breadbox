# Phase 30: Provider Expansion

**Version:** 1.0
**Status:** Draft
**Last Updated:** 2026-03-08

---

## Table of Contents

1. [Overview](#1-overview)
2. [Goals](#2-goals)
3. [Provider Selection](#3-provider-selection)
4. [Finicity Integration](#4-finicity-integration)
5. [Pluggy Integration](#5-pluggy-integration)
6. [Provider Interface Compliance](#6-provider-interface-compliance)
7. [Dashboard Changes](#7-dashboard-changes)
8. [Category Mapping](#8-category-mapping)
9. [Community Contribution Guide](#9-community-contribution-guide)
10. [Implementation Tasks](#10-implementation-tasks)
11. [Dependencies](#11-dependencies)

---

## 1. Overview

Phase 30 extends Breadbox beyond Plaid and Teller by adding two new bank data
providers: **Finicity (Mastercard Open Banking)** for expanded US/Canada
coverage and **Pluggy** for Brazil. Both providers implement the existing
`provider.Provider` interface (7 methods), follow the same encrypted-credential
and sync-engine patterns, and are configurable through the admin dashboard
without requiring code changes.

---

## 2. Goals

1. **Geographic expansion.** Cover US/Canada (Finicity as a Plaid alternative)
   and Brazil (Pluggy) — two markets not currently served or only partially
   served.
2. **Self-serve focus.** Both providers offer sandbox access without a sales
   call. Finicity has pay-as-you-go pricing; Pluggy has 100 free connections
   and transparent per-connection pricing.
3. **Provider parity.** New providers are first-class citizens with the same
   dashboard configuration, connection management, sync engine integration,
   category mapping, and webhook support as Plaid and Teller.
4. **Community template.** Establish patterns and documentation that make it
   straightforward for contributors to add additional providers in the future.

---

## 3. Provider Selection

### Final Recommendation

Based on the research in `docs/provider-research.md`:

| Priority | Provider | Geography | Rationale |
|---|---|---|---|
| **1** | Finicity (Mastercard) | US, Canada | Best self-serve Plaid alternative. Pay-as-you-go pricing. OpenAPI spec available for client generation. OAuth connection flow. |
| **2** | Pluggy | Brazil | Most developer-friendly LatAm option. Truly self-serve with transparent pricing. 100 free connections. REST API suitable for hand-written client. |

### Why Not Others

- **Belvo:** Multi-country LatAm coverage but production requires a sales call.
  Revisit if Belvo opens self-serve production access.
- **Yapily/Tink:** European PSD2 providers but sandbox-only self-serve.
  GoCardless (the only free European option) closed signups in July 2025.
  Monitor Yapily Starter tier.
- **MX/Akoya/Yodlee:** Enterprise-only, pricing starts at $15K+/year.
  Incompatible with self-hosted personal-use model.
- **Mono:** Nigeria-focused, acquired by Flutterwave (Jan 2026), uncertain
  long-term direction.

---

## 4. Finicity Integration

### 4.1 Authentication Model

Finicity uses a **two-layer authentication** model:

1. **Partner-level credentials:** `partnerId` + `partnerSecret` authenticate
   the application. These are used to obtain a short-lived partner access
   token via `POST /aggregation/v2/partners/authentication`.
2. **Customer access tokens:** Per-connection tokens stored in
   `bank_connections.encrypted_credentials`. Used for all data-fetching API
   calls.

The partner access token expires after **2 hours**. The provider must cache it
and refresh transparently before API calls.

**Base URL:** `https://api.finicity.com` (production),
`https://api.finicity.com` (sandbox uses same URL, different credentials)

### 4.2 Credential Sourcing

| Credential | Source | Storage |
|---|---|---|
| `FINICITY_PARTNER_ID` | Environment variable or `app_config` table | Partner ID |
| `FINICITY_PARTNER_SECRET` | Environment variable or `app_config` table | Partner secret |
| `FINICITY_APP_KEY` | Environment variable or `app_config` table | App key (Finicity Dashboard) |
| `FINICITY_ENV` | Environment variable or `app_config` table | `sandbox` or `production` |
| `FINICITY_WEBHOOK_SECRET` | Environment variable or `app_config` table | HMAC signing secret for webhooks |

Pattern follows Plaid: env vars take precedence, `app_config` DB table used
for dashboard-configured values.

### 4.3 API Client Design

**Approach: Hand-written HTTP client** (like Teller, not SDK-generated).

Finicity publishes an OpenAPI spec at
`github.com/Mastercard/open-banking-us-openapi`, but the generated client would
be very large (hundreds of endpoints). Breadbox only needs ~8 endpoints.
A hand-written client keeps the dependency small and follows the Teller
precedent.

```
internal/provider/finicity/
    client.go       — HTTP client with partner auth token caching
    provider.go     — FinicityProvider struct, compile-time interface check
    link.go         — CreateLinkSession (Finicity Connect URL generation)
    exchange.go     — ExchangeToken (customer creation + account discovery)
    sync.go         — SyncTransactions (date-range polling, like Teller)
    balances.go     — GetBalances
    webhook.go      — HandleWebhook (signature verification)
    reauth.go       — CreateReauthSession (Finicity Connect in fix mode)
    remove.go       — RemoveConnection (DELETE customer or revoke access)
    categories.go   — Finicity-to-Breadbox category mapping
    errors.go       — HTTP status → provider error mapping
    validate.go     — ValidateCredentials for dashboard test button
```

**Client struct:**

```go
type Client struct {
    httpClient    *http.Client
    baseURL       string
    partnerID     string
    partnerSecret string
    appKey        string

    // Cached partner access token (2-hour TTL).
    mu           sync.Mutex
    accessToken  string
    tokenExpiry  time.Time
}
```

Every API request includes:
- `Finicity-App-Key` header (static app key)
- `Finicity-App-Token` header (cached partner access token, auto-refreshed)
- `Content-Type: application/json`
- `Accept: application/json`

### 4.4 Connection Flow (Finicity Connect)

Finicity Connect is an OAuth-based flow. Unlike Plaid Link (which is a
JavaScript widget), Finicity Connect generates a URL that opens in a browser
or iframe.

**Flow:**

1. **Create customer** (server-side, if new): `POST /aggregation/v1/customers/active`
   with a username derived from the Breadbox user ID. Finicity requires a
   unique `username` per customer — use `bb_{user_uuid}`.
2. **Generate Connect URL** (server-side): `POST /connect/v2/generate` with
   `customerId`, `partnerId`, webhook URL, and requested products
   (`["transactions", "accountBalance"]`).
3. **User completes flow** in browser. On success, Finicity fires a webhook
   or the client-side redirect includes an `institutionLoginId`.
4. **Discover accounts** (server-side): `GET /aggregation/v1/customers/{customerId}/accounts`
5. **Store connection**: `encrypted_credentials` holds the encrypted customer
   ID + institution login ID JSON blob (not an access token — Finicity uses
   the partner token + customer ID for data access).

**Provider interface mapping:**

- `CreateLinkSession(ctx, userID)` → Creates customer if needed, generates
  Connect URL. Returns `LinkSession{Token: connectURL, Expiry: 30min}`.
- `ExchangeToken(ctx, publicToken)` → The "public token" is a JSON string
  `{customer_id, institution_login_id}` posted back from the Connect
  callback. Fetches accounts and returns `Connection` + `[]Account`.

**Credentials stored in `encrypted_credentials`:**

```json
{
    "customer_id": "123456789",
    "institution_login_id": "987654321"
}
```

Both values are needed for subsequent API calls. Encrypted with AES-256-GCM
via `internal/crypto`.

### 4.5 Transaction Sync

Finicity uses **date-range polling** (same strategy as Teller, no cursor-based
incremental sync).

**Endpoint:** `GET /aggregation/v3/customers/{customerId}/transactions`

**Query parameters:**
- `fromDate` — Unix epoch timestamp
- `toDate` — Unix epoch timestamp
- `start` — Pagination offset (1-based)
- `limit` — Page size (max 1000)
- `includePending` — `true`

**Sync algorithm:**

1. Read `sync_cursor` from `bank_connections` (ISO timestamp of last sync).
2. Calculate window: `from = cursor - 10 days`, `to = now`. Initial sync:
   `from = 2 years ago`.
3. Paginate through all transactions using `start`/`limit` offset pagination.
4. Return all as `SyncResult.Added` (upsert handles dedup).
5. `HasMore = false`, `Cursor = now ISO timestamp`.

### 4.6 Amount Sign Convention

Finicity uses the **same sign convention as Plaid**: positive = money out
(debit), negative = money in (credit). **No sign negation needed.**

### 4.7 Transaction Field Mapping

| Finicity Field | Breadbox Field | Notes |
|---|---|---|
| `id` (string of int64) | `external_transaction_id` | Stable ID |
| `accountId` | `account_external_id` | Resolved to internal UUID |
| `amount` | `amount` | Same sign convention as Plaid (no negation) |
| `postedDate` / `transactionDate` | `date` | Unix epoch → `time.Time` |
| `description` | `name` | Raw description |
| `status` | `pending` | `"active"` → `false`, `"pending"` → `true` |
| `categorization.category` | `category_primary` | Via category mapping |
| `categorization.bestRepresentation` | `merchant_name` | Cleaned merchant name |
| `type` | `payment_channel` | Map: `atm`→`other`, `directDebit`→`online`, `check`→`other`, etc. |
| (not available) | `authorized_date` | `NULL` |
| (not available) | `pending_transaction_id` | `NULL` |

### 4.8 Balance Fetch

**Endpoint:** `GET /aggregation/v1/customers/{customerId}/accounts`

Account balances are embedded in the accounts response (no separate balance
endpoint). Fields:

| Finicity Field | Breadbox Field | Notes |
|---|---|---|
| `balance` | `balance_current` | Current balance |
| `availableBalance` | `balance_available` | Nullable |
| (not available) | `balance_limit` | `NULL` |
| `currency` | `iso_currency_code` | `"USD"` typically |

### 4.9 Webhook Support

Finicity sends webhook notifications when new transactions are available.

**Signature verification:** Finicity webhooks include a signature header.
Verify using the webhook secret (HMAC-SHA256, same pattern as Teller).

**Event mapping:**

| Finicity Event | `WebhookEvent.Type` | `NeedsReauth` | Action |
|---|---|---|---|
| `account` (institutionLogin error) | `connection_error` | `true` | Set to `pending_reauth` |
| `transaction` (new data) | `sync_available` | `false` | Trigger sync |
| `ping` | `unknown` | `false` | Acknowledge |

**Connection lookup:** The webhook payload includes `customerId` and
`institutionLoginId`. Look up via `bank_connections.external_id` where
`provider = 'finicity'`. The `external_id` stores the `institutionLoginId`.

### 4.10 Connection Lifecycle

**Reconnection:** Generate a new Finicity Connect URL in "fix" mode:
`POST /connect/v2/generate/fix` with the existing `institutionLoginId`.

**Removal:** `DELETE /aggregation/v1/customers/{customerId}/institutionLogins/{institutionLoginId}`
revokes access. If the customer has no other connections, optionally delete the
customer too.

### 4.11 Error Handling

| HTTP Status | Meaning | Action |
|---|---|---|
| `200` | Success | Process response |
| `401` | Partner token expired | Refresh token, retry |
| `203` | Partial data (institution issue) | Return `ErrSyncRetryable` |
| `404` | Customer/account not found | Return `ErrReauthRequired` |
| `429` | Rate limit | Exponential backoff |
| `5xx` | Server error | Retry with backoff |

---

## 5. Pluggy Integration

### 5.1 Authentication Model

Pluggy uses a **simple API key + secret** model:

1. **Client credentials:** `clientId` + `clientSecret` used to obtain a JWT
   access token via `POST /auth`. Token expires after **2 hours**.
2. **Per-connection token:** Each connected item has an `itemId`. API calls
   reference the item ID; authentication is via the JWT bearer token.

**Base URL:** `https://api.pluggy.ai`

### 5.2 Credential Sourcing

| Credential | Source | Storage |
|---|---|---|
| `PLUGGY_CLIENT_ID` | Environment variable or `app_config` table | Client ID from Pluggy Dashboard |
| `PLUGGY_CLIENT_SECRET` | Environment variable or `app_config` table | Client secret |
| `PLUGGY_WEBHOOK_URL` | Environment variable or `app_config` table | Webhook callback URL |

No mTLS, no special certificates. Standard API key authentication.

### 5.3 API Client Design

**Approach: Hand-written HTTP client** (same pattern as Teller).

```
internal/provider/pluggy/
    client.go       — HTTP client with JWT token caching
    provider.go     — PluggyProvider struct, compile-time interface check
    link.go         — CreateLinkSession (Connect Token generation)
    exchange.go     — ExchangeToken (item creation + account discovery)
    sync.go         — SyncTransactions (date-range polling)
    balances.go     — GetBalances
    webhook.go      — HandleWebhook
    reauth.go       — CreateReauthSession
    remove.go       — RemoveConnection
    categories.go   — Pluggy-to-Breadbox category mapping
    errors.go       — HTTP status → provider error mapping
    validate.go     — ValidateCredentials for dashboard test
```

**Client struct:**

```go
type Client struct {
    httpClient    *http.Client
    baseURL       string
    clientID      string
    clientSecret  string

    // Cached JWT access token.
    mu           sync.Mutex
    accessToken  string
    tokenExpiry  time.Time
}
```

Every API request includes:
- `Authorization: Bearer {jwt_token}` (auto-refreshed)
- `Content-Type: application/json`

### 5.4 Connection Flow (Pluggy Connect)

Pluggy Connect is a JavaScript widget similar to Plaid Link.

**Flow:**

1. **Create Connect Token** (server-side): `POST /connect_token` with optional
   `webhookUrl` and `clientUserId`.
2. **User completes flow** in Pluggy Connect widget. On success, the callback
   provides an `itemId`.
3. **Discover accounts** (server-side): `GET /items/{itemId}/accounts`
4. **Store connection**: `encrypted_credentials` holds the encrypted `itemId`.

**Provider interface mapping:**

- `CreateLinkSession(ctx, userID)` → Creates a connect token. Returns
  `LinkSession{Token: connectToken, Expiry: 30min}`.
- `ExchangeToken(ctx, publicToken)` → The "public token" is a JSON string
  `{item_id}`. Fetches accounts and returns `Connection` + `[]Account`.

**Credentials stored in `encrypted_credentials`:**

```json
{
    "item_id": "a1b2c3d4-e5f6-..."
}
```

### 5.5 Transaction Sync

Pluggy uses **date-range polling** (same strategy as Teller).

**Endpoint:** `GET /transactions?accountId={accountId}&from={date}&to={date}`

**Pagination:** Offset-based with `page` and `pageSize` parameters.

**Sync algorithm:** Same as Teller — 10-day overlap window, all results
returned as `SyncResult.Added`, upsert handles dedup.

### 5.6 Amount Sign Convention

Pluggy uses the **same sign convention as Plaid**: positive = money out
(debit), negative = money in (credit). **No sign negation needed.**

Verify during implementation — if Pluggy uses opposite convention (like
Teller), negate amounts in the provider before returning.

### 5.7 Transaction Field Mapping

| Pluggy Field | Breadbox Field | Notes |
|---|---|---|
| `id` | `external_transaction_id` | UUID |
| `accountId` | `account_external_id` | Resolved to internal UUID |
| `amount` | `amount` | Verify sign convention |
| `date` | `date` | ISO 8601 |
| `description` | `name` | Raw description |
| `status` | `pending` | `"POSTED"` → `false`, `"PENDING"` → `true` |
| `category` | `category_primary` | Via category mapping |
| `merchantName` | `merchant_name` | Nullable |
| `currencyCode` | `iso_currency_code` | `"BRL"` typically |
| `paymentData.paymentMethod` | `payment_channel` | Map to `online`/`in_store`/`other` |
| (not available) | `authorized_date` | `NULL` |
| (not available) | `pending_transaction_id` | `NULL` |

### 5.8 Balance Fetch

**Endpoint:** `GET /accounts/{accountId}`

| Pluggy Field | Breadbox Field | Notes |
|---|---|---|
| `balance` | `balance_current` | Current balance |
| `availableBalance` | `balance_available` | Nullable |
| `creditData.creditLimit` | `balance_limit` | For credit accounts |
| `currencyCode` | `iso_currency_code` | `"BRL"` |

### 5.9 Webhook Support

Pluggy sends webhooks for item status changes and transaction updates.

**Webhook URL:** Configured per connect token in `POST /connect_token`.

**Event mapping:**

| Pluggy Event | `WebhookEvent.Type` | `NeedsReauth` | Action |
|---|---|---|---|
| `item/updated` (status: `LOGIN_ERROR`) | `connection_error` | `true` | Set to `pending_reauth` |
| `item/updated` (status: `UPDATED`) | `sync_available` | `false` | Trigger sync |
| `item/updated` (status: `OUTDATED`) | `connection_error` | `true` | Set to `pending_reauth` |

**Connection lookup:** Webhook payload includes `itemId` which maps to
`bank_connections.external_id` where `provider = 'pluggy'`.

### 5.10 Connection Lifecycle

**Reconnection:** `POST /items/{itemId}/update_credentials` or generate a new
Connect Token with `updateItem: itemId`. The widget reopens in update mode.

**Removal:** `DELETE /items/{itemId}` removes the connection from Pluggy.

### 5.11 Currency Handling

Pluggy primarily serves Brazilian institutions. Transactions will use `BRL`
as the ISO currency code. Breadbox already stores `iso_currency_code` per
transaction, so no schema changes are needed. The MCP and API instructions
about never summing across currencies apply naturally.

---

## 6. Provider Interface Compliance

Both Finicity and Pluggy implement all 7 methods of the `provider.Provider`
interface defined in `internal/provider/provider.go`.

### 6.1 Finicity Method Mapping

| Method | Implementation | Notes |
|---|---|---|
| `CreateLinkSession` | Generate Finicity Connect URL | Creates customer if needed |
| `ExchangeToken` | Parse callback data, discover accounts | Stores `{customer_id, institution_login_id}` |
| `SyncTransactions` | Date-range polling with 10-day overlap | Same pattern as Teller |
| `GetBalances` | Balances from accounts endpoint | Embedded in account response |
| `HandleWebhook` | HMAC-SHA256 signature verification | Maps events to `WebhookEvent` |
| `CreateReauthSession` | Generate Connect URL in fix mode | Uses existing `institutionLoginId` |
| `RemoveConnection` | DELETE institution login | Cleans up customer if last connection |

No methods return `ErrNotSupported`. All 7 are fully implemented.

### 6.2 Pluggy Method Mapping

| Method | Implementation | Notes |
|---|---|---|
| `CreateLinkSession` | Create connect token via API | Returns token for widget |
| `ExchangeToken` | Parse `itemId`, discover accounts | Stores `{item_id}` |
| `SyncTransactions` | Date-range polling with 10-day overlap | Same pattern as Teller |
| `GetBalances` | Per-account balance fetch | Like Teller (one call per account) |
| `HandleWebhook` | Parse webhook payload | Event-based, maps to `WebhookEvent` |
| `CreateReauthSession` | Create connect token with `updateItem` | Widget opens in update mode |
| `RemoveConnection` | DELETE item | Revokes Pluggy access |

No methods return `ErrNotSupported`. All 7 are fully implemented.

### 6.3 Compile-Time Interface Checks

Both providers include the standard compile-time check, matching the
convention in `internal/provider/plaid/provider.go` and
`internal/provider/teller/provider.go`:

```go
var _ provider.Provider = (*FinicityProvider)(nil)
var _ provider.Provider = (*PluggyProvider)(nil)
```

---

## 7. Dashboard Changes

### 7.1 Provider Enum Migration

Add `finicity` and `pluggy` to the `provider_type` PostgreSQL enum:

```sql
-- +goose Up
ALTER TYPE provider_type ADD VALUE IF NOT EXISTS 'finicity';
ALTER TYPE provider_type ADD VALUE IF NOT EXISTS 'pluggy';

-- +goose Down
-- PostgreSQL does not support removing enum values. No-op.
```

This migration must run before any connections can be created for these
providers.

### 7.2 Provider Cards on `/admin/providers`

The providers page (`internal/admin/providers.go`) already renders equal-weight
cards for Plaid, Teller, and CSV. Add two new cards:

**Finicity card:**
- Fields: Partner ID, Partner Secret (masked), App Key (masked), Environment
  (sandbox/production), Webhook Secret (masked)
- "Test Connection" button calls
  `POST /admin/api/test-provider/finicity`
- Save handler: `POST /admin/providers/finicity`
- Config source badges via `configSource` template function

**Pluggy card:**
- Fields: Client ID, Client Secret (masked), Webhook URL
- "Test Connection" button calls
  `POST /admin/api/test-provider/pluggy`
- Save handler: `POST /admin/providers/pluggy`
- Config source badges

### 7.3 Connection Flow UI

The "Connect New Bank" page (`connection_new.html`) currently conditionally
shows Plaid Link and Teller Connect buttons. Extend with:

**Finicity:** Add a "Connect via Finicity" button. Since Finicity Connect
opens a URL (not an embedded widget), the button triggers an API call to get
the Connect URL, then opens it in a new tab or iframe. On completion, a
redirect back to Breadbox provides the callback data.

**Pluggy:** Add a "Connect via Pluggy" button. Pluggy Connect is a JavaScript
widget loaded from CDN (`https://cdn.pluggy.ai/connect/v2/connect.js`).
Integration pattern mirrors Teller Connect:

```javascript
var pluggyConnect = PluggyConnect.init({
    connectToken: token,
    onSuccess: function(data) {
        fetch('/admin/api/exchange-token', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({
                provider: 'pluggy',
                public_token: JSON.stringify({item_id: data.item.id}),
                user_id: selectedUserId,
                institution_id: data.item.connector.id.toString(),
                institution_name: data.item.connector.name
            })
        });
    },
    onError: function(error) { /* handle */ }
});
pluggyConnect.open();
```

### 7.4 Reauth UI

The reauth page (`connection_reauth.html`) currently handles Plaid and Teller.
Extend the provider-switching logic:

- **Finicity:** Open the fix-mode Connect URL in a new tab.
- **Pluggy:** Open Pluggy Connect widget with the `updateItem` parameter.

### 7.5 Provider Initialization

Add initialization branches in `internal/app/providers.go`:

```go
// initFinicityProvider in internal/app/providers.go
func initFinicityProvider(cfg *config.Config, providers map[string]provider.Provider, logger *slog.Logger) error {
    if cfg.FinicityPartnerID == "" {
        return nil
    }
    client := finicityprovider.NewClient(
        cfg.FinicityPartnerID,
        cfg.FinicityPartnerSecret,
        cfg.FinicityAppKey,
    )
    providers["finicity"] = finicityprovider.NewProvider(
        client, cfg.EncryptionKey, cfg.FinicityWebhookSecret, logger,
    )
    logger.Info("finicity provider initialized")
    return nil
}
```

Extend `ReinitProvider` with `case "finicity":` and `case "pluggy":` branches
following the same pattern as Plaid and Teller.

### 7.6 Config Struct Additions

Add to `internal/config/config.go`:

```go
// Finicity
FinicityPartnerID     string
FinicityPartnerSecret string
FinicityAppKey        string
FinicityEnv           string
FinicityWebhookSecret string

// Pluggy
PluggyClientID     string
PluggyClientSecret string
PluggyWebhookURL   string
```

Environment variable names: `FINICITY_PARTNER_ID`, `FINICITY_PARTNER_SECRET`,
`FINICITY_APP_KEY`, `FINICITY_ENV`, `FINICITY_WEBHOOK_SECRET`,
`PLUGGY_CLIENT_ID`, `PLUGGY_CLIENT_SECRET`, `PLUGGY_WEBHOOK_URL`.

---

## 8. Category Mapping

### 8.1 Strategy

Both Finicity and Pluggy categories map to the existing Breadbox category
system (defined in `docs/category-mapping.md`). The `category_mappings` table
handles this:

```sql
INSERT INTO category_mappings (provider, provider_category, category_id)
VALUES ('finicity', 'ATM Fee', (SELECT id FROM categories WHERE slug = 'bank_fees_atm'));
```

### 8.2 Finicity Categories

Finicity provides a two-level categorization similar to Plaid. The
`categorization` object includes `category` (broad) and `bestRepresentation`
(cleaned merchant). Finicity also returns a `normalizedPayeeName`.

**Mapping approach:** Map Finicity's `category` values to Breadbox category
slugs using the `category_mappings` table. Add rows during seed/migration.

| Finicity Category | Breadbox Slug |
|---|---|
| `ATM Fee` | `bank_fees_atm` |
| `Automotive Expenses` | `transportation` |
| `Charitable Giving` | `government_and_non_profit_donations` |
| `Child/Dependent Expenses` | `general_services` |
| `Clothing/Shoes` | `general_merchandise_clothing` |
| `Education` | `general_services_education` |
| `Entertainment` | `entertainment` |
| `Food/Drink` | `food_and_drink` |
| `Gas/Fuel` | `transportation_gas` |
| `Gifts` | `general_merchandise` |
| `Groceries` | `food_and_drink_groceries` |
| `Healthcare/Medical` | `medical` |
| `Home Improvement` | `home_improvement` |
| `Income` | `income` |
| `Insurance` | `loan_payments_insurance` |
| `Investments` | `transfer_in` |
| `Mortgage` | `rent_and_utilities_mortgage` |
| `Office Expenses` | `general_services` |
| `Personal Care` | `personal_care` |
| `Rent` | `rent_and_utilities_rent` |
| `Restaurants` | `food_and_drink_restaurant` |
| `Taxes` | `government_and_non_profit_tax` |
| `Travel` | `travel` |
| `Utilities` | `rent_and_utilities` |
| (unmapped) | `NULL` (no category) |

The full mapping table will be populated during implementation by cross-
referencing Finicity's published category list with the Breadbox default
taxonomy.

### 8.3 Pluggy Categories

Pluggy uses its own category taxonomy with Portuguese and English labels.
Categories are nested (parent + child).

**Mapping approach:** Same `category_mappings` table with `provider = 'pluggy'`.

| Pluggy Category | Breadbox Slug |
|---|---|
| `Alimentação` / `Food` | `food_and_drink` |
| `Compras` / `Shopping` | `general_merchandise` |
| `Educação` / `Education` | `general_services_education` |
| `Entretenimento` / `Entertainment` | `entertainment` |
| `Moradia` / `Housing` | `rent_and_utilities` |
| `Saúde` / `Health` | `medical` |
| `Transporte` / `Transportation` | `transportation` |
| `Viagem` / `Travel` | `travel` |
| `Investimento` / `Investment` | `transfer_in` |
| `Receita` / `Income` | `income` |
| `Impostos` / `Taxes` | `government_and_non_profit_tax` |
| `Serviços` / `Services` | `general_services` |
| (unmapped) | `NULL` (no category) |

### 8.4 In-Memory Cache

During sync, category lookups use the existing in-memory cache pattern
established in the category mapping system. Each provider's `categories.go`
file contains a static mapping as a fallback, but the `category_mappings`
table is the source of truth.

### 8.5 Raw Category Preservation

Raw provider category strings are always stored in `category_primary` and
`category_detailed` fields on the transaction, matching the existing pattern.
The resolved `category_id` FK points to the Breadbox category. This preserves
auditability per the design principle in `docs/category-mapping.md`.

---

## 9. Community Contribution Guide

### 9.1 Guide Location

`docs/adding-a-provider.md` — a standalone guide for contributors who want to
add new bank data providers to Breadbox.

### 9.2 Guide Outline

1. **Prerequisites**
   - Go 1.24+, PostgreSQL, working Breadbox dev environment
   - Provider API credentials (sandbox access)

2. **Directory Structure**
   - Create `internal/provider/{name}/` with the standard file set:
     `client.go`, `provider.go`, `link.go`, `exchange.go`, `sync.go`,
     `balances.go`, `webhook.go`, `reauth.go`, `remove.go`, `categories.go`,
     `errors.go`, `validate.go`

3. **Implement the Provider Interface**
   - All 7 methods of `provider.Provider` (defined in
     `internal/provider/provider.go`)
   - Methods that don't apply return `provider.ErrNotSupported`
   - Include compile-time interface check:
     `var _ provider.Provider = (*MyProvider)(nil)`

4. **HTTP Client Pattern**
   - Use `net/http` with a custom client struct
   - Include retry logic with exponential backoff for `429` responses
   - Cache any short-lived auth tokens (access tokens, JWTs)
   - 30-second HTTP timeout

5. **Amount Convention**
   - Breadbox stores amounts as Plaid convention: positive = debit (money out)
   - If the provider uses opposite convention, negate amounts before returning
   - Document the provider's convention in a comment

6. **Category Mapping**
   - Create `categories.go` with a static `mapCategory()` function
   - Add rows to `category_mappings` table via a migration
   - Map to existing Breadbox category slugs (see `docs/category-mapping.md`)
   - Unmapped categories → `NULL` (no category assigned)

7. **Credential Storage**
   - All access tokens and secrets encrypted with AES-256-GCM
     (`internal/crypto`)
   - Stored in `bank_connections.encrypted_credentials` as a JSON blob
   - Config values (API keys, app IDs) go in `internal/config/config.go`
     with env var and `app_config` DB support

8. **Registration**
   - Add init function in `internal/app/providers.go`
   - Add `case` to `ReinitProvider` in `internal/app/providers.go`
   - Add enum value to `provider_type` via goose migration
   - Add provider card to `internal/admin/providers.go` and
     `templates/providers.html`

9. **Sync Engine Integration**
   - The sync engine (`internal/sync/engine.go`) calls providers
     generically — no provider-specific code should be needed
   - Exception: if the provider needs stale-pending cleanup (like Teller),
     add a conditional block in `cleanStalePending`
   - Date-range polling providers should set `HasMore = false` and use ISO
     timestamp strings as cursors

10. **Dashboard UI**
    - Add connection widget script to `templates/connection_new.html`
    - Add reauth flow to `templates/connection_reauth.html`
    - Add provider card to `templates/providers.html`

11. **Testing**
    - Unit tests for category mapping, amount conversion, error mapping
    - Integration tests with sandbox credentials (if available)
    - Add provider to `ProvidersTestHandler` in
      `internal/admin/providers.go`

12. **Checklist**
    - [ ] All 7 interface methods implemented
    - [ ] Compile-time interface check
    - [ ] Credentials encrypted at rest
    - [ ] Amount sign convention matches Plaid (or negated)
    - [ ] Category mapping populated
    - [ ] DB migration for enum value
    - [ ] Config struct + env var loading
    - [ ] Dashboard provider card
    - [ ] Connection new/reauth UI
    - [ ] `ReinitProvider` case added
    - [ ] Error sentinels used (`ErrReauthRequired`, `ErrSyncRetryable`)
    - [ ] Exponential backoff on rate limits

---

## 10. Implementation Tasks

### Phase 30A: Finicity Provider (Priority 1)

1. **DB migration:** Add `finicity` to `provider_type` enum
   - File: `internal/db/migrations/000XX_add_finicity_provider.sql`

2. **Config additions:** Add Finicity config fields and env var loading
   - Files: `internal/config/config.go`, `internal/config/load.go`

3. **HTTP client:** Hand-written client with partner token caching
   - File: `internal/provider/finicity/client.go`

4. **Provider implementation:** All 7 interface methods
   - Files: `internal/provider/finicity/provider.go`, `link.go`,
     `exchange.go`, `sync.go`, `balances.go`, `webhook.go`, `reauth.go`,
     `remove.go`

5. **Category mapping:** Static map + DB migration for `category_mappings`
   - File: `internal/provider/finicity/categories.go`
   - Migration: `internal/db/migrations/000XX_finicity_category_mappings.sql`

6. **Error mapping:** HTTP status → provider error sentinels
   - Files: `internal/provider/finicity/errors.go`, `validate.go`

7. **App registration:** Init function, `ReinitProvider` case
   - File: `internal/app/providers.go`

8. **Dashboard provider card:** Config form + test button
   - Files: `internal/admin/providers.go`,
     `templates/providers.html`

9. **Connection UI:** Finicity Connect integration
   - Files: `templates/connection_new.html`,
     `templates/connection_reauth.html`

10. **Sync engine:** Verify generic sync works; add stale-pending cleanup if
    needed (Finicity pending transactions may need same treatment as Teller)
    - File: `internal/sync/engine.go` (conditional block if required)

### Phase 30B: Pluggy Provider (Priority 2)

11. **DB migration:** Add `pluggy` to `provider_type` enum
    - File: `internal/db/migrations/000XX_add_pluggy_provider.sql`

12. **Config additions:** Add Pluggy config fields and env var loading
    - Files: `internal/config/config.go`, `internal/config/load.go`

13. **HTTP client:** Hand-written client with JWT token caching
    - File: `internal/provider/pluggy/client.go`

14. **Provider implementation:** All 7 interface methods
    - Files: `internal/provider/pluggy/provider.go`, `link.go`,
      `exchange.go`, `sync.go`, `balances.go`, `webhook.go`, `reauth.go`,
      `remove.go`

15. **Category mapping:** Static map + DB migration for `category_mappings`
    - File: `internal/provider/pluggy/categories.go`
    - Migration: `internal/db/migrations/000XX_pluggy_category_mappings.sql`

16. **Error mapping:** HTTP status → provider error sentinels
    - Files: `internal/provider/pluggy/errors.go`, `validate.go`

17. **App registration:** Init function, `ReinitProvider` case
    - File: `internal/app/providers.go`

18. **Dashboard provider card:** Config form + test button
    - Files: `internal/admin/providers.go`,
      `templates/providers.html`

19. **Connection UI:** Pluggy Connect widget integration
    - Files: `templates/connection_new.html`,
      `templates/connection_reauth.html`

### Phase 30C: Community Guide (Priority 3)

20. **Write contribution guide:** `docs/adding-a-provider.md`
    - Content: Section 9 outline above, expanded with code examples from
      Finicity and Pluggy implementations

21. **Update CLAUDE.md:** Add Finicity/Pluggy conventions (amount sign,
    auth model, credential storage format)

22. **Update docs/architecture.md:** Add Finicity and Pluggy to provider
    comparison table

---

## 11. Dependencies

### Required Before Phase 30

- **Category system (Phase 20):** The `categories` and `category_mappings`
  tables must exist. Finicity and Pluggy category mappings are inserted into
  `category_mappings` with their respective provider names.
- **Provider interface:** No changes needed to `internal/provider/provider.go`.
  The existing 7-method interface and shared types handle both providers
  without modification.
- **Sync engine:** No structural changes needed to
  `internal/sync/engine.go`. The generic provider loop, upsert logic, and
  balance update flow all work as-is. The only potential addition is a
  stale-pending cleanup conditional for Finicity (if its pending transaction
  behavior matches Teller's).

### No Changes Required

- **Provider interface types** (`Connection`, `Account`, `SyncResult`,
  `Transaction`, `WebhookPayload`, `WebhookEvent`, `AccountBalance`,
  `LinkSession`): All existing types are sufficient for both providers.
- **Error sentinels** (`ErrReauthRequired`, `ErrSyncRetryable`,
  `ErrNotSupported`): Sufficient for both providers.
- **Crypto package** (`internal/crypto/encrypt.go`): AES-256-GCM
  encrypt/decrypt used by all providers.
- **Exchange token handler** (`internal/admin/connections.go`): Already
  provider-agnostic — routes `provider` field to the correct provider
  implementation.

### Database Migrations

1. Add `finicity` to `provider_type` enum
2. Add `pluggy` to `provider_type` enum
3. Seed `category_mappings` rows for Finicity categories
4. Seed `category_mappings` rows for Pluggy categories

These are additive migrations with no data transformation.

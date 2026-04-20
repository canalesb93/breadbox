# Breadbox Data Model Specification

**Version:** 1.0
**Status:** Draft
**Last Updated:** 2026-03-01

This document defines the complete PostgreSQL database schema for the Breadbox MVP. An engineer should be able to write all migration files directly from this document without consulting any other source.

---

## Table of Contents

1. [Schema Overview](#1-schema-overview)
2. [Table Definitions](#2-table-definitions)
   - [users](#21-users)
   - [admin_accounts](#22-admin_accounts)
   - [bank_connections](#23-bank_connections)
   - [accounts](#24-accounts)
   - [transactions](#25-transactions)
   - [sync_logs](#26-sync_logs)
   - [api_keys](#27-api_keys)
   - [app_config](#28-app_config)
   - [tags](#29-tags)
   - [transaction_tags](#210-transaction_tags)
   - [annotations](#211-annotations)
3. [Plaid Field Mapping](#3-plaid-field-mapping)
4. [Key Design Decisions](#4-key-design-decisions)
5. [Indexes](#5-indexes)
6. [Migration Strategy](#6-migration-strategy)

---

## 1. Schema Overview

### 1.1 Tables

| Table | Purpose |
|---|---|
| `users` | Family member labels used to tag account ownership. Not login accounts. |
| `admin_accounts` | Dashboard login credentials. Single-admin for MVP. |
| `bank_connections` | Provider-agnostic records of a linked bank item (Plaid Item, Teller enrollment, etc.). Holds encrypted credentials and sync state. |
| `accounts` | Individual bank accounts (checking, savings, credit cards) within a connection. Mirrors the Plaid Account object. |
| `transactions` | Financial transactions synced from providers. Soft-deleted on removal. |
| `sync_logs` | Immutable audit trail of every sync operation attempt. |
| `api_keys` | Hashed API keys for REST API and MCP access. |
| `app_config` | Key-value store for runtime configuration set during the first-run setup wizard. |
| `tags` | Reusable labels attached to transactions (e.g. `needs-review`). |
| `transaction_tags` | Many-to-many join between transactions and tags with attribution metadata. |
| `annotations` | Unified activity timeline per transaction (comments, tag events, rule applications, category sets). |

### 1.2 Entity Relationship Diagram

```
┌─────────────────┐
│   admin_accounts│
│─────────────────│
│ id (PK)         │
│ username        │
│ hashed_password │
│ created_at      │
└─────────────────┘

┌─────────────────┐
│   app_config    │
│─────────────────│
│ key (PK)        │
│ value           │
│ updated_at      │
└─────────────────┘

┌─────────────────┐
│   api_keys      │
│─────────────────│
│ id (PK)         │
│ name            │
│ key_hash        │
│ key_prefix      │
│ last_used_at    │
│ revoked_at      │
│ created_at      │
└─────────────────┘

┌─────────────────┐         ┌──────────────────────┐
│     users       │         │   bank_connections   │
│─────────────────│         │──────────────────────│
│ id (PK)         │◄────────│ id (PK)              │
│ name            │  user_id│ user_id (FK)         │
│ email           │  (nullable)                    │
│ created_at      │         │ provider             │
│ updated_at      │         │ institution_id        │
└─────────────────┘         │ institution_name      │
                            │ external_id          │
                            │ encrypted_credentials│
                            │ sync_cursor          │
                            │ status               │
                            │ error_code           │
                            │ error_message        │
                            │ new_accounts_available│
                            │ consent_expiration_time│
                            │ last_synced_at       │
                            │ created_at           │
                            │ updated_at           │
                            └──────────┬───────────┘
                                       │ 1
                                       │
                                       │ has many
                                       │ N
                            ┌──────────▼───────────┐         ┌──────────────────────┐
                            │       accounts       │         │      sync_logs       │
                            │──────────────────────│         │──────────────────────│
                            │ id (PK)              │         │ id (PK)              │
                            │ connection_id (FK)   │         │ connection_id (FK)   │
                            │ external_account_id  │         │ trigger              │
                            │ name                 │         │ added_count          │
                            │ official_name        │         │ modified_count       │
                            │ type                 │         │ removed_count        │
                            │ subtype              │         │ status               │
                            │ mask                 │         │ error_message        │
                            │ balance_current      │         │ started_at           │
                            │ balance_available    │         │ completed_at         │
                            │ balance_limit        │         └──────────────────────┘
                            │ iso_currency_code    │
                            │ last_balance_update  │
                            │ created_at           │
                            │ updated_at           │
                            └──────────┬───────────┘
                                       │ 1
                                       │
                                       │ has many
                                       │ N
                            ┌──────────▼───────────┐
                            │     transactions     │
                            │──────────────────────│
                            │ id (PK)              │
                            │ account_id (FK)      │
                            │ external_transaction_id│
                            │ pending_transaction_id│
                            │ amount               │
                            │ iso_currency_code    │
                            │ date                 │
                            │ authorized_date      │
                            │ datetime             │
                            │ authorized_datetime  │
                            │ name                 │
                            │ merchant_name        │
                            │ category_primary     │
                            │ category_detailed    │
                            │ category_confidence  │
                            │ payment_channel      │
                            │ pending              │
                            │ deleted_at           │
                            │ created_at           │
                            │ updated_at           │
                            └──────────────────────┘
```

**Foreign Key Summary:**

```
bank_connections.user_id        → users.id             (SET NULL on delete)
accounts.connection_id          → bank_connections.id  (SET NULL on delete)
transactions.account_id         → accounts.id          (SET NULL on delete)
sync_logs.connection_id         → bank_connections.id  (CASCADE DELETE)
```

> **Data preservation policy:** When a connection is removed, accounts and transactions are preserved for historical queries. `accounts.connection_id` and `transactions.account_id` are set to `NULL` rather than cascade-deleted. The connection status is set to `'disconnected'` rather than deleting the connection row. This allows past transaction history to remain queryable even after a bank is unlinked.

### 1.3 Naming Conventions

- **Table names:** snake_case, singular noun (e.g., `transaction`, not `transactions`) — **Exception:** Breadbox uses plural table names throughout for idiomatic SQL readability (`users`, `accounts`, `transactions`).
- **Column names:** snake_case.
- **Primary keys:** `id`, type `UUID`, generated with `gen_random_uuid()`.
- **Foreign keys:** named `<referenced_table_singular>_id` (e.g., `account_id`, `connection_id`, `user_id`).
- **Timestamps:** always `TIMESTAMPTZ` (UTC with timezone offset stored). Never plain `TIMESTAMP`.
- **Enums:** defined as PostgreSQL `CREATE TYPE ... AS ENUM (...)` types, named with a descriptive suffix (e.g., `provider_type`, `connection_status`, `sync_trigger`, `sync_status`).
- **Boolean columns:** prefixed with `is_` only when the column name would otherwise be ambiguous. `pending` is clear on its own.
- **Soft-delete column:** `deleted_at TIMESTAMPTZ NULL` — `NULL` means active, non-`NULL` means deleted.

---

## 2. Table Definitions

### 2.1 `users`

**Purpose:** Stores family member records used as ownership labels for bank connections and accounts. Users are not login accounts — they are display names for grouping and filtering financial data (e.g., "Alice", "Bob"). Access control is not enforced per-user; the label is purely organizational.

#### Columns

| Column | Type | Nullable | Default | Description |
|---|---|---|---|---|
| `id` | `UUID` | No | `gen_random_uuid()` | Primary key. |
| `name` | `TEXT` | No | — | Display name of the family member (e.g., "Alice"). |
| `email` | `TEXT` | Yes | `NULL` | Optional email address. Used for display only — not for authentication. |
| `created_at` | `TIMESTAMPTZ` | No | `NOW()` | Record creation timestamp. |
| `updated_at` | `TIMESTAMPTZ` | No | `NOW()` | Last modification timestamp. Updated by application logic or a trigger. |

#### Primary Key

```sql
PRIMARY KEY (id)
```

#### Foreign Keys

None.

#### Unique Constraints

```sql
UNIQUE (email)  -- if provided, email must be globally unique
```

Email uniqueness is enforced at the database level but the column is nullable — `NULL` values do not violate the unique constraint (standard SQL behavior). Two users may both have `NULL` email.

#### Indexes

| Index | Columns | Type | Rationale |
|---|---|---|---|
| `users_pkey` | `id` | B-tree (implicit PK) | Primary key lookup. |
| `users_email_idx` | `email` | B-tree | Support login or lookup by email if added in future; enforce unique constraint efficiently. |

---

### 2.2 `admin_accounts`

**Purpose:** Stores the single administrative login credential for the Breadbox web dashboard. Created during the first-run setup wizard. The application enforces a single-admin constraint at the application layer (MVP does not support multiple admins). Dashboard access uses session cookies; this table is never exposed via the REST API.

#### Columns

| Column | Type | Nullable | Default | Description |
|---|---|---|---|---|
| `id` | `UUID` | No | `gen_random_uuid()` | Primary key. |
| `username` | `TEXT` | No | — | Login username. Case-sensitive. |
| `hashed_password` | `BYTEA` | No | — | bcrypt hash of the password. Stored as raw bytes (`bytea`), not a hex string, to avoid encoding ambiguity. |
| `created_at` | `TIMESTAMPTZ` | No | `NOW()` | Record creation timestamp. No `updated_at` — password changes are tracked by replacing the hash value, not a new row. |

#### Primary Key

```sql
PRIMARY KEY (id)
```

#### Foreign Keys

None.

#### Unique Constraints

```sql
UNIQUE (username)
```

#### Indexes

| Index | Columns | Type | Rationale |
|---|---|---|---|
| `admin_accounts_pkey` | `id` | B-tree (implicit PK) | Primary key lookup. |
| `admin_accounts_username_idx` | `username` | B-tree | Username lookup during login. Enforces unique constraint. |

---

### 2.3 `bank_connections`

**Purpose:** Represents a single linked bank "item" from a data provider. For Plaid, one item corresponds to one institution login and may contain multiple accounts. Stores connection status, encrypted provider credentials, and the sync cursor used for incremental transaction retrieval. Designed to be provider-agnostic — provider-specific fields are stored in dedicated nullable columns rather than a polymorphic JSON blob, keeping the schema inspectable and typed.

#### Enum Types (defined before the table)

```sql
CREATE TYPE provider_type AS ENUM ('plaid', 'teller', 'csv');

CREATE TYPE connection_status AS ENUM (
    'active',           -- connection is healthy and syncing normally
    'error',            -- non-recoverable sync error; see error_code and error_message
    'pending_reauth',   -- bank requires user to re-authenticate (Plaid ITEM_LOGIN_REQUIRED)
    'disconnected'      -- manually disconnected or revoked by the institution
);
```

> **Canonical enum values:** The four values above (`active`, `error`, `pending_reauth`, `disconnected`) are the authoritative connection status values for all Breadbox documents. `rest-api.md`, `mcp-server.md`, and `plaid-integration.md` must align to these exact values. `reauth_required`, `syncing`, `pending`, and `disabled` are not valid enum values in this schema.

#### Columns

| Column | Type | Nullable | Default | Description |
|---|---|---|---|---|
| `id` | `UUID` | No | `gen_random_uuid()` | Primary key. |
| `user_id` | `UUID` | Yes | `NULL` | FK to `users.id`. Which family member owns this connection. Nullable — a connection may exist before being assigned to a user. `SET NULL` on user delete (connection survives). |
| `provider` | `provider_type` | No | — | Which data provider manages this connection. |
| `institution_id` | `TEXT` | Yes | `NULL` | Provider-specific institution identifier (e.g., Plaid's `ins_109508`). Used for display and deduplication. |
| `institution_name` | `TEXT` | Yes | `NULL` | Human-readable institution name (e.g., "Chase"). Cached from provider to avoid re-fetching. |
| `external_id` | `TEXT` | Yes | `NULL` | Provider-assigned identifier for this connection. For Plaid: `item_id`. For Teller: enrollment `id`. Used in webhook payloads and for deduplication. Unique per provider (see constraint below). Replaces the original `plaid_item_id` column (migration 00013). |
| `encrypted_credentials` | `BYTEA` | Yes | `NULL` | AES-256-GCM encrypted provider credentials. For Plaid: access token. For Teller: access token. Stored as raw ciphertext bytes. Never logged or returned by the API. Replaces the original `plaid_access_token` column (migration 00013). |
| `sync_cursor` | `TEXT` | Yes | `NULL` | Plaid cursor-based sync position. Passed as `cursor` in `/transactions/sync` requests to retrieve only changes since the last sync. `NULL` on initial sync (fetches full history). |
| `status` | `connection_status` | No | `'active'` | Current health of the connection. |
| `error_code` | `TEXT` | Yes | `NULL` | Provider error code when `status` is `error` or `pending_reauth` (e.g., Plaid's `ITEM_LOGIN_REQUIRED`). `NULL` otherwise. |
| `error_message` | `TEXT` | Yes | `NULL` | Human-readable error description from the provider. Displayed in the dashboard to help the user understand what action is needed. `NULL` when healthy. |
| `new_accounts_available` | `BOOLEAN` | No | `FALSE` | Set to `TRUE` when Plaid reports that new accounts are available to add for this item (from `NEW_ACCOUNTS_AVAILABLE` webhook). Cleared after the user reviews the accounts in the dashboard. |
| `consent_expiration_time` | `TIMESTAMPTZ` | Yes | `NULL` | When the user's consent for this Plaid item expires, as reported by Plaid. `NULL` if the institution does not enforce consent expiration. |
| `paused` | `BOOLEAN` | No | `FALSE` | When `TRUE`, cron-scheduled syncs skip this connection. Manual "Sync Now" still works. Orthogonal to `status`. Added in Phase 10 (migration 00015). |
| `sync_interval_override_minutes` | `INTEGER` | Yes | `NULL` | Per-connection sync interval override. When set, cron checks this connection's staleness individually. `NULL` uses the global `sync_interval_minutes`. Added in Phase 10 (migration 00015). |
| `last_synced_at` | `TIMESTAMPTZ` | Yes | `NULL` | Timestamp of the most recently completed successful sync. `NULL` if never synced. Used to compute "last seen" display and detect stale connections. Note: `last_attempted_sync_at` (the time of the most recent sync attempt regardless of outcome) does not need its own column — it can be derived from `sync_logs` with `SELECT MAX(started_at) FROM sync_logs WHERE connection_id = $1`. |
| `created_at` | `TIMESTAMPTZ` | No | `NOW()` | Record creation timestamp. |
| `updated_at` | `TIMESTAMPTZ` | No | `NOW()` | Last modification timestamp. |

#### Primary Key

```sql
PRIMARY KEY (id)
```

#### Foreign Keys

```sql
FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE SET NULL
```

When a user record is deleted, `user_id` is set to `NULL` rather than cascade-deleting the connection. Financial data is preserved; it simply becomes unassigned. The operator can reassign it.

#### Unique Constraints

```sql
UNIQUE (provider, external_id)  -- each provider connection may only appear once
```

The `(provider, external_id)` unique constraint prevents duplicate connections from being created if the Link/enrollment flow is invoked twice for the same institution login.

#### Indexes

| Index | Columns | Type | Rationale |
|---|---|---|---|
| `bank_connections_pkey` | `id` | B-tree (implicit PK) | Primary key lookup. |
| `bank_connections_user_id_idx` | `user_id` | B-tree | List all connections for a given user. Used by `GET /api/v1/connections?user_id=`. |
| `bank_connections_status_idx` | `status` | B-tree | Filter connections by health status (e.g., find all `pending_reauth` for the dashboard health panel). |
| `bank_connections_provider_external_id_idx` | `(provider, external_id)` | B-tree (unique) | Webhook handler looks up connection by provider + external ID. Enforces unique constraint. |

---

### 2.4 `accounts`

**Purpose:** Stores individual financial accounts (checking, savings, credit cards, etc.) that belong to a bank connection. Each row mirrors a Plaid Account object. Balances are stored here and refreshed on each sync via `/accounts/get`. When a connection is removed, `connection_id` is set to `NULL` (SET NULL) rather than cascade-deleting accounts — historical account and transaction data is preserved. See Section 4.5.

#### Columns

| Column | Type | Nullable | Default | Description |
|---|---|---|---|---|
| `id` | `UUID` | No | `gen_random_uuid()` | Primary key. |
| `connection_id` | `UUID` | Yes | `NULL` | FK to `bank_connections.id`. SET NULL on connection delete. Nullable so accounts are preserved when a connection is removed. |
| `external_account_id` | `TEXT` | No | — | Provider-assigned account identifier. For Plaid: `account_id`. Stable unless Plaid cannot reconcile the account. Used for idempotent upserts during sync. |
| `name` | `TEXT` | No | — | Account name as assigned by the user or institution (e.g., "Plaid Checking"). |
| `official_name` | `TEXT` | Yes | `NULL` | Official name as given by the financial institution (e.g., "Plaid Gold Standard 0% Interest Checking"). Often more verbose than `name`. |
| `type` | `TEXT` | No | — | Plaid account type. One of: `depository`, `credit`, `loan`, `investment`, `other`. Stored as `TEXT` rather than an enum to remain forward-compatible with new Plaid types without a migration. |
| `subtype` | `TEXT` | Yes | `NULL` | Plaid account subtype (e.g., `checking`, `savings`, `credit card`, `mortgage`). Nullable — Plaid may return `null` for some account types. |
| `mask` | `TEXT` | Yes | `NULL` | Last 2–4 alphanumeric characters of the displayed account mask (e.g., "0000"). Used for user-facing display to identify the account without exposing the full number. |
| `balance_current` | `NUMERIC(12,2)` | Yes | `NULL` | Current balance. For depository accounts: total funds in the account. For credit accounts: amount owed. Nullable — Plaid may not always return a value. |
| `balance_available` | `NUMERIC(12,2)` | Yes | `NULL` | Available balance. For depository accounts: funds available for withdrawal (current minus pending debits). For credit accounts: remaining credit. Plaid frequently returns `null` for credit accounts. |
| `balance_limit` | `NUMERIC(12,2)` | Yes | `NULL` | Credit limit for credit accounts, or overdraft limit for some depository accounts. `NULL` for accounts without a limit concept. |
| `iso_currency_code` | `TEXT` | Yes | `NULL` | ISO-4217 currency code for the balances (e.g., `USD`, `EUR`). `NULL` if Plaid returns an unofficial currency. |
| `display_name` | `TEXT` | Yes | `NULL` | Optional user-assigned display name. Templates use `COALESCE(display_name, name)` for rendering. Added in Phase 10. |
| `excluded` | `BOOLEAN` | No | `FALSE` | When `TRUE`, transaction upsert is skipped during sync (balances still refresh). Useful for accounts the user wants to track balances on but not import transactions from. Added in Phase 10. |
| `last_balance_update` | `TIMESTAMPTZ` | Yes | `NULL` | When the balance columns were last refreshed from the provider. Set after each successful `/accounts/get` call. This is the canonical column name; `plaid-integration.md` should be updated to use `last_balance_update` instead of `balance_updated_at`. |
| `created_at` | `TIMESTAMPTZ` | No | `NOW()` | Record creation timestamp. |
| `updated_at` | `TIMESTAMPTZ` | No | `NOW()` | Last modification timestamp. |

#### Primary Key

```sql
PRIMARY KEY (id)
```

#### Foreign Keys

```sql
FOREIGN KEY (connection_id) REFERENCES bank_connections (id) ON DELETE SET NULL
```

#### Unique Constraints

```sql
UNIQUE (external_account_id)
```

Plaid `account_id` values are globally unique across all items and institutions. This constraint enables safe upsert logic (`INSERT ... ON CONFLICT (external_account_id) DO UPDATE`) during sync.

#### Indexes

| Index | Columns | Type | Rationale |
|---|---|---|---|
| `accounts_pkey` | `id` | B-tree (implicit PK) | Primary key lookup. |
| `accounts_connection_id_idx` | `connection_id` | B-tree | List all accounts for a connection. Used when syncing and when populating the dashboard. Supports SET NULL foreign key enforcement. |
| `accounts_external_account_id_idx` | `external_account_id` | B-tree | Upsert lookup during sync and webhook processing. Enforces unique constraint. |

---

### 2.5 `transactions`

**Purpose:** Stores individual financial transactions synced from providers. This is the primary data table queried by REST API consumers and MCP tools. Plaid's cursor-based sync (`/transactions/sync`) returns three arrays: `added`, `modified`, and `removed`. Added and modified transactions are upserted; removed transactions are soft-deleted (their `deleted_at` is set). Soft deletion preserves referential integrity for any analysis or logging that references transaction IDs.

#### Columns

| Column | Type | Nullable | Default | Description |
|---|---|---|---|---|
| `id` | `UUID` | No | `gen_random_uuid()` | Primary key. Internal Breadbox identifier. |
| `account_id` | `UUID` | Yes | `NULL` | FK to `accounts.id`. SET NULL on account delete. Nullable so transactions are preserved for historical queries if the parent account is removed. |
| `external_transaction_id` | `TEXT` | No | — | Provider-assigned transaction identifier. For Plaid: `transaction_id`. Case-sensitive. Stable for posted transactions; pending transactions may receive a new `transaction_id` when they post. Used for upsert during sync. |
| `pending_transaction_id` | `TEXT` | Yes | `NULL` | For a posted transaction that was previously pending, this is the `transaction_id` of that original pending transaction. Plaid sets this field to link the posted record back to the pending record it replaced. The application uses this to locate and soft-delete the superseded pending row. |
| `amount` | `NUMERIC(12,2)` | No | — | Transaction amount in the account's currency. **Sign convention (Plaid passthrough):** Positive values represent money leaving the account (debits, purchases, payments). Negative values represent money entering the account (credits, refunds, deposits). This matches Plaid's native convention exactly — Breadbox does not invert the sign. See Section 4 for full rationale. |
| `iso_currency_code` | `TEXT` | Yes | `NULL` | ISO-4217 currency code for this transaction (e.g., `USD`). `NULL` if Plaid returns an unofficial currency code. Never silently aggregate across currencies. |
| `unofficial_currency_code` | `TEXT` | Yes | `NULL` | Non-standard currency code (e.g., cryptocurrency). Mutually exclusive with `iso_currency_code` — Plaid guarantees at most one is non-null. |
| `date` | `DATE` | No | — | Transaction date. For pending transactions: the date the transaction occurred. For posted transactions: the date it posted. Format: `YYYY-MM-DD`. |
| `authorized_date` | `DATE` | Yes | `NULL` | Date the transaction was authorized by the user (e.g., when a card was swiped). Earlier than or equal to `date` for posted transactions. Not always available. |
| `datetime` | `TIMESTAMPTZ` | Yes | `NULL` | Full timestamp of the posted transaction, if provided by the institution. Only available for select institutions. `NULL` for most transactions. |
| `authorized_datetime` | `TIMESTAMPTZ` | Yes | `NULL` | Full timestamp of authorization, if provided by the institution. `NULL` for most transactions. |
| `name` | `TEXT` | No | — | Merchant name or transaction description as returned by Plaid (sourced from the raw financial institution description). This is the primary human-readable description of the transaction. Plaid marks this field as deprecated in favor of `merchant_name` but it remains populated and is the most reliable description field. |
| `merchant_name` | `TEXT` | Yes | `NULL` | Plaid-enriched merchant name, cleaned and normalized from the `name` field (e.g., "McDonald's" instead of "MCDONALDS 00321 CARD PURCH"). `NULL` when Plaid cannot identify the merchant. |
| `category_primary` | `TEXT` | Yes | `NULL` | High-level personal finance category from Plaid's `personal_finance_category.primary` (e.g., `FOOD_AND_DRINK`, `TRANSPORTATION`, `INCOME`). Part of Plaid's enriched categorization taxonomy (v2). |
| `category_detailed` | `TEXT` | Yes | `NULL` | Granular subcategory from Plaid's `personal_finance_category.detailed` (e.g., `FOOD_AND_DRINK_RESTAURANTS`, `TRANSPORTATION_GAS_AND_FUEL`). Can be used as a stable identifier. |
| `category_confidence` | `TEXT` | Yes | `NULL` | Plaid's confidence in the category assignment. One of: `VERY_HIGH`, `HIGH`, `MEDIUM`, `LOW`, `UNKNOWN`. |
| `payment_channel` | `TEXT` | Yes | `NULL` | Channel used to make the payment. One of: `online`, `in store`, `other`. `NULL` if Plaid does not provide this field. |
| `pending` | `BOOLEAN` | No | `FALSE` | Whether the transaction has settled. `TRUE` = pending (not yet cleared). `FALSE` = posted. Pending transactions may change details or be replaced entirely when they post. |
| `deleted_at` | `TIMESTAMPTZ` | Yes | `NULL` | Soft-delete timestamp. `NULL` means the transaction is active. Set to the current time when Plaid includes this `transaction_id` in the `removed` array of a `/transactions/sync` response. Soft-deleted transactions are excluded from API responses by default but are never hard-deleted. |
| `created_at` | `TIMESTAMPTZ` | No | `NOW()` | Timestamp when this row was first inserted into Breadbox. |
| `updated_at` | `TIMESTAMPTZ` | No | `NOW()` | Timestamp of the last upsert (update from Plaid's `modified` array or first insert). |

#### Primary Key

```sql
PRIMARY KEY (id)
```

#### Foreign Keys

```sql
FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE SET NULL
```

#### Unique Constraints

```sql
UNIQUE (external_transaction_id)
```

Enables `INSERT ... ON CONFLICT (external_transaction_id) DO UPDATE` upsert pattern for all sync operations. Plaid `transaction_id` values are globally unique.

#### Indexes

See Section 5 for full index rationale. Summary:

| Index | Columns | Type | Rationale |
|---|---|---|---|
| `transactions_pkey` | `id` | B-tree (implicit PK) | Primary key lookup. |
| `transactions_external_transaction_id_idx` | `external_transaction_id` | B-tree | Upsert lookup during sync. Enforces unique constraint. |
| `transactions_account_id_date_idx` | `account_id, date DESC` | B-tree | Primary query pattern: all transactions for an account ordered by date. |
| `transactions_account_id_date_active_idx` | `account_id, date DESC` WHERE `deleted_at IS NULL` | Partial B-tree | Same as above but excludes soft-deleted rows. Most API queries use this path. |
| `transactions_date_idx` | `date DESC` | B-tree | Date-range queries spanning multiple accounts (e.g., agent asking "all transactions last 30 days"). |
| `transactions_pending_idx` | `pending` | B-tree | Filter pending/posted transactions. Low cardinality; effective when combined with other predicates. |
| `transactions_category_primary_idx` | `category_primary` | B-tree | Filter by spending category (e.g., all `FOOD_AND_DRINK` transactions). |
| `transactions_name_merchant_gin_idx` | `name, merchant_name` | GIN (pg_trgm) | Full-text trigram search on transaction description and merchant name. Supports `ILIKE '%query%'` efficiently. |
| `transactions_account_id_idx` | `account_id` | B-tree | Join path from accounts to transactions; supports CASCADE DELETE FK. |

---

### 2.6 `sync_logs`

**Purpose:** Immutable audit log of every sync operation. One row is inserted when a sync starts (`status = 'in_progress'`), then updated when it completes or fails. Provides the data for connection health monitoring, the `get_sync_status` MCP tool, and the dashboard's "Last synced" display.

#### Enum Types

```sql
CREATE TYPE sync_trigger AS ENUM (
    'cron',      -- scheduled background sync
    'webhook',   -- triggered by Plaid SYNC_UPDATES_AVAILABLE webhook
    'manual',    -- operator triggered via POST /api/v1/sync
    'initial'    -- first sync after a new connection is established
);

CREATE TYPE sync_status AS ENUM (
    'in_progress',  -- sync started, not yet complete
    'success',      -- sync completed without error
    'error'         -- sync failed; see error_message
);
```

> **Canonical enum values:** The three values above (`in_progress`, `success`, `error`) are the authoritative sync log status values. `rest-api.md` uses `completed` and `failed` in its response schema — those are incorrect and must be updated to match this enum. All other documents should align to these values.

#### Columns

| Column | Type | Nullable | Default | Description |
|---|---|---|---|---|
| `id` | `UUID` | No | `gen_random_uuid()` | Primary key. |
| `connection_id` | `UUID` | No | — | FK to `bank_connections.id`. CASCADE DELETE. Which connection this sync ran against. |
| `trigger` | `sync_trigger` | No | — | What caused this sync to run. |
| `added_count` | `INTEGER` | No | `0` | Number of transactions inserted during this sync (from Plaid's `added` array). |
| `modified_count` | `INTEGER` | No | `0` | Number of transactions updated during this sync (from Plaid's `modified` array). |
| `removed_count` | `INTEGER` | No | `0` | Number of transactions soft-deleted during this sync (from Plaid's `removed` array). |
| `status` | `sync_status` | No | `'in_progress'` | Current status of the sync operation. |
| `error_message` | `TEXT` | Yes | `NULL` | Human-readable error description if `status = 'error'`. Includes Plaid error codes and messages. `NULL` on success. |
| `started_at` | `TIMESTAMPTZ` | No | `NOW()` | Timestamp when the sync operation began. |
| `completed_at` | `TIMESTAMPTZ` | Yes | `NULL` | Timestamp when the sync finished (success or error). `NULL` while `in_progress`. |

#### Primary Key

```sql
PRIMARY KEY (id)
```

#### Foreign Keys

```sql
FOREIGN KEY (connection_id) REFERENCES bank_connections (id) ON DELETE CASCADE
```

#### Unique Constraints

None.

#### Indexes

| Index | Columns | Type | Rationale |
|---|---|---|---|
| `sync_logs_pkey` | `id` | B-tree (implicit PK) | Primary key lookup. |
| `sync_logs_connection_id_started_at_idx` | `connection_id, started_at DESC` | B-tree | Fetch recent sync history for a specific connection (dashboard, `get_sync_status`). |
| `sync_logs_started_at_idx` | `started_at DESC` | B-tree | List all recent syncs across all connections (admin dashboard overview). |

---

### 2.7 `api_keys`

**Purpose:** Stores hashed API keys used to authenticate REST API and MCP requests. The plaintext key is shown once at creation and never stored. Only a SHA-256 hash is persisted. The key prefix (first 8 characters) is stored separately to allow users to identify which key made a request without storing the full key. Authentication works by hashing the presented key and comparing it to stored hashes.

#### Columns

| Column | Type | Nullable | Default | Description |
|---|---|---|---|---|
| `id` | `UUID` | No | `gen_random_uuid()` | Primary key. |
| `name` | `TEXT` | No | — | Human-readable label for this key (e.g., "Claude Desktop", "Home Server Script"). Not required to be unique. |
| `key_hash` | `TEXT` | No | — | SHA-256 hex digest of the full API key. Used for constant-time comparison during authentication. |
| `key_prefix` | `TEXT` | No | — | First 8 characters of the plaintext key (e.g., `bb_xKj2mN`). Displayed in the dashboard to help identify which key is which. Never sufficient to authenticate. |
| `last_used_at` | `TIMESTAMPTZ` | Yes | `NULL` | Timestamp of the most recent successful authentication using this key. `NULL` if never used. Updated on each authenticated request. |
| `revoked_at` | `TIMESTAMPTZ` | Yes | `NULL` | Revocation timestamp. `NULL` means the key is active. Non-`NULL` means the key has been revoked and must be rejected by the authentication middleware, even if the hash matches. Set to the current time by `DELETE /admin/api/api-keys/:id`. |
| `created_at` | `TIMESTAMPTZ` | No | `NOW()` | Record creation timestamp. |

#### Primary Key

```sql
PRIMARY KEY (id)
```

#### Foreign Keys

None.

#### Unique Constraints

```sql
UNIQUE (key_hash)
```

Two keys cannot produce the same hash. This is mathematically guaranteed for SHA-256 with sufficiently random key generation, but the constraint provides an explicit database-level guarantee.

#### Indexes

| Index | Columns | Type | Rationale |
|---|---|---|---|
| `api_keys_pkey` | `id` | B-tree (implicit PK) | Primary key lookup. |
| `api_keys_key_hash_idx` | `key_hash` | B-tree | Authentication path: look up a key by its hash on every authenticated request. Enforces unique constraint. This index must be fast. |

---

### 2.8 `app_config`

**Purpose:** Key-value store for application configuration set during the first-run setup wizard. Stores Plaid credentials (client ID and environment), sync interval, and other runtime settings. Using a database table rather than a config file means settings can be updated through the dashboard without restarting the process. The Plaid secret is stored encrypted.

#### Columns

| Column | Type | Nullable | Default | Description |
|---|---|---|---|---|
| `key` | `VARCHAR(255)` | No | — | Configuration key. Primary key. Uses underscored naming (e.g., `plaid_client_id`, `sync_interval_hours`). |
| `value` | `TEXT` | Yes | `NULL` | Configuration value as a text string. Sensitive values (Plaid secret) are stored AES-256-GCM encrypted and decoded by the application layer. `NULL` indicates the key exists but has no value set. |
| `updated_at` | `TIMESTAMPTZ` | No | `NOW()` | Timestamp of the last write to this row. |

#### Primary Key

```sql
PRIMARY KEY (key)
```

#### Foreign Keys

None.

#### Unique Constraints

The primary key on `key` is itself unique.

#### Indexes

| Index | Columns | Type | Rationale |
|---|---|---|---|
| `app_config_pkey` | `key` | B-tree (implicit PK) | Direct key lookup (the only access pattern). |

#### Known Configuration Keys

The following keys are seeded during initial migration and used by the application. This list is illustrative; additional keys (MCP mode, Teller settings, review guidelines, etc.) are added by features and documented in their own docs.

| Key | Example Value | Description |
|---|---|---|
| `plaid_client_id` | `(empty)` | Plaid API client ID. Set during setup wizard. |
| `plaid_secret` | `(encrypted)` | AES-256-GCM encrypted Plaid API secret. |
| `plaid_env` | `sandbox` | Plaid environment: `sandbox` or `production`. |
| `webhook_url` | `(empty)` | Publicly accessible URL for Plaid webhooks. Optional. |
| `sync_interval_hours` | `12` | How often the cron sync runs, in hours. |
| `setup_complete` | `false` | Whether the first-run setup wizard has been completed. |

---

### 2.9 `tags`

**Purpose:** Reusable labels attached to transactions. Tags are the coordination primitive for the review workflow and for agent-to-agent handoffs. The seeded `needs-review` tag is the queue: transactions carrying it are awaiting assessment. Operators can create additional tags from the `/tags` admin page or via the `create_tag` MCP tool. Defined in migration `20260415075421_tags_and_annotations.sql`.

#### Columns

| Column | Type | Nullable | Default | Description |
|---|---|---|---|---|
| `id` | `UUID` | No | `gen_random_uuid()` | Primary key. |
| `short_id` | `TEXT` | No | trigger-generated | 8-character base62 alias. |
| `slug` | `TEXT` | No | — | Lowercase alphanumerics with optional hyphens/colons (e.g. `needs-review`, `subscription:monthly`). Immutable after creation. Globally unique. |
| `display_name` | `TEXT` | No | — | Human-readable label (e.g. "Needs Review"). Mutable. |
| `description` | `TEXT` | No | `''` | Operator-facing description. |
| `color` | `TEXT` | Yes | `NULL` | CSS color used when rendering the tag chip. |
| `icon` | `TEXT` | Yes | `NULL` | Lucide icon name for chip rendering. |
| `lifecycle` | `TEXT` | No | `'persistent'` | **Deprecated.** Legacy persistent/ephemeral distinction — no longer read or written by the app. Column remains in the schema for backwards compatibility; all new rows default to `persistent`. Notes on tag removal are optional and recorded on the `tag_removed` annotation payload regardless. |
| `created_at` | `TIMESTAMPTZ` | No | `NOW()` | Record creation timestamp. |
| `updated_at` | `TIMESTAMPTZ` | No | `NOW()` | Last modification timestamp. |

#### Primary Key

`id`

#### Unique Constraints

```sql
UNIQUE (slug)
UNIQUE (short_id)
```

#### Check Constraints

```sql
CHECK (lifecycle IN ('persistent', 'ephemeral')) -- deprecated, column no longer used
```

#### Seeds

| Slug | Display Name | Purpose |
|---|---|---|
| `needs-review` | Needs Review | Marks transactions awaiting initial categorization review. The seeded `on_create` system rule auto-attaches this tag during sync. |

A companion seeded system rule on `transaction_rules` fires on every `on_create` event and attaches `needs-review` to newly-synced transactions. To opt out of the default review queue, disable that rule from `/rules` — this is the supported opt-out. System-seeded rules can't be deleted, only disabled.

#### Indexes

| Index | Columns | Type | Rationale |
|---|---|---|---|
| `tags_pkey` | `id` | B-tree (implicit PK) | Primary key lookup. |
| `tags_slug_key` | `slug` | B-tree (unique) | Slug lookup (MCP tools resolve by slug). |
| `tags_short_id_key` | `short_id` | B-tree (unique) | Short ID lookup. |

---

### 2.10 `transaction_tags`

**Purpose:** Many-to-many join between transactions and tags, with attribution metadata. One row means "this tag is currently on this transaction."

#### Columns

| Column | Type | Nullable | Default | Description |
|---|---|---|---|---|
| `transaction_id` | `UUID` | No | — | FK → `transactions(id)`. CASCADE DELETE. |
| `tag_id` | `UUID` | No | — | FK → `tags(id)`. CASCADE DELETE. |
| `added_by_type` | `TEXT` | No | — | `user`, `agent`, `rule`, or `system`. |
| `added_by_id` | `TEXT` | Yes | `NULL` | Attribution ID (admin user ID, API key prefix, rule short_id, etc.). |
| `added_by_name` | `TEXT` | No | `''` | Display name of the attributor. |
| `added_at` | `TIMESTAMPTZ` | No | `NOW()` | When the tag was attached. |

#### Primary Key

```sql
PRIMARY KEY (transaction_id, tag_id)
```

The composite PK enforces one-tag-per-transaction idempotency — re-adding an already-attached tag is a no-op (handled by `ON CONFLICT DO NOTHING` in the service layer).

#### Foreign Keys

| Column | References | On Delete |
|---|---|---|
| `transaction_id` | `transactions(id)` | `CASCADE` |
| `tag_id` | `tags(id)` | `CASCADE` |

#### Check Constraints

```sql
CHECK (added_by_type IN ('user', 'agent', 'rule', 'system'))
```

#### Indexes

| Index | Columns | Type | Rationale |
|---|---|---|---|
| `transaction_tags_pkey` | `(transaction_id, tag_id)` | B-tree (implicit PK) | Idempotent upsert; transaction → tag lookup. |
| `transaction_tags_tag_idx` | `tag_id` | B-tree | Reverse lookup: "all transactions with this tag" (e.g. the `needs-review` backlog query). |
| `transaction_tags_recent_idx` | `(tag_id, added_at DESC)` | B-tree | Most-recently-tagged queries for a specific tag. |

---

### 2.11 `annotations`

**Purpose:** Canonical activity timeline for every transaction. Each row is one event: a free-form comment, a rule firing, a tag add/remove, or a category being set. `list_annotations(transaction_id)` returns the full ordered history.

#### Columns

| Column | Type | Nullable | Default | Description |
|---|---|---|---|---|
| `id` | `UUID` | No | `gen_random_uuid()` | Primary key. |
| `short_id` | `TEXT` | No | trigger-generated | 8-character base62 alias. |
| `transaction_id` | `UUID` | No | — | FK → `transactions(id)`. CASCADE DELETE. |
| `kind` | `TEXT` | No | — | One of `comment`, `rule_applied`, `tag_added`, `tag_removed`, `category_set`. |
| `actor_type` | `TEXT` | No | — | `user`, `agent`, or `system`. |
| `actor_id` | `TEXT` | Yes | `NULL` | Attribution ID (admin user ID, API key prefix, etc.). |
| `actor_name` | `TEXT` | No | — | Display name of the actor. |
| `session_id` | `UUID` | Yes | `NULL` | FK → `mcp_sessions(id)`. Links the event back to the originating MCP session when applicable. |
| `payload` | `JSONB` | No | `'{}'` | Event-specific payload. For `comment`: `{content}`. For `tag_added`/`tag_removed`: `{note, tag_slug}`. For `rule_applied`: `{action_field, action_value, rule_id, rule_name}`. For `category_set`: `{category_slug, previous_category_slug}` (slug references the `categories` table — not detailed here; see `internal/db/migrations/`). |
| `tag_id` | `UUID` | Yes | `NULL` | FK → `tags(id)`. SET NULL on delete. Populated for `tag_added`/`tag_removed` events. |
| `rule_id` | `UUID` | Yes | `NULL` | FK → `transaction_rules(id)`. SET NULL on delete. Populated for `rule_applied` events. |
| `created_at` | `TIMESTAMPTZ` | No | `NOW()` | When the event occurred. Timeline is ordered by this column. |

#### Primary Key

`id`

#### Foreign Keys

| Column | References | On Delete |
|---|---|---|
| `transaction_id` | `transactions(id)` | `CASCADE` |
| `session_id` | `mcp_sessions(id)` — table not detailed here; see `internal/db/migrations/` for schema. | (FK only, no cascade specified) |
| `tag_id` | `tags(id)` | `SET NULL` |
| `rule_id` | `transaction_rules(id)` — table not detailed here; see `internal/db/migrations/` for schema. | `SET NULL` |

#### Unique Constraints

```sql
UNIQUE (short_id)
```

#### Check Constraints

```sql
CHECK (kind IN ('comment', 'rule_applied', 'tag_added', 'tag_removed', 'category_set'))
CHECK (actor_type IN ('user', 'agent', 'system'))
```

#### Indexes

| Index | Columns | Type | Rationale |
|---|---|---|---|
| `annotations_pkey` | `id` | B-tree (implicit PK) | Primary key lookup. |
| `annotations_short_id_key` | `short_id` | B-tree (unique) | Short ID lookup. |
| `annotations_txn_idx` | `(transaction_id, created_at ASC)` | B-tree | Timeline query for a single transaction (ASC ordering matches UI rendering). |
| `annotations_kind_idx` | `(kind, created_at DESC)` | B-tree | Cross-transaction queries by event kind (e.g. "recent rule applications"). |

---

## 3. Plaid Field Mapping

### 3.1 Transaction Fields (`/transactions/sync` → `transactions` table)

The Plaid `/transactions/sync` endpoint returns `TransactionBase` objects in the `added` and `modified` arrays.

| Plaid Field | Plaid Type | Breadbox Column | Breadbox Type | Notes |
|---|---|---|---|---|
| `transaction_id` | string | `external_transaction_id` | `TEXT` | Stable for posted; may change when pending posts. Case-sensitive. |
| `pending_transaction_id` | string \| null | `pending_transaction_id` | `TEXT NULL` | Links posted transaction back to the pending record it replaced. |
| `account_id` | string | — (join) | — | Used to resolve `account_id` FK via `accounts.external_account_id`. Not stored redundantly. |
| `amount` | number | `amount` | `NUMERIC(12,2)` | Passed through with Plaid's sign convention (positive = money out). See Section 4. |
| `iso_currency_code` | string \| null | `iso_currency_code` | `TEXT NULL` | Null when `unofficial_currency_code` is set. |
| `unofficial_currency_code` | string \| null | `unofficial_currency_code` | `TEXT NULL` | Cryptocurrency or non-standard currency. Null when `iso_currency_code` is set. |
| `date` | string (YYYY-MM-DD) | `date` | `DATE` | Pending: occurrence date. Posted: settlement date. |
| `authorized_date` | string \| null (YYYY-MM-DD) | `authorized_date` | `DATE NULL` | When the user authorized the transaction. |
| `datetime` | string \| null (ISO 8601) | `datetime` | `TIMESTAMPTZ NULL` | Full timestamp of posted transaction. Select institutions only. |
| `authorized_datetime` | string \| null (ISO 8601) | `authorized_datetime` | `TIMESTAMPTZ NULL` | Full timestamp of authorization. Select institutions only. |
| `name` | string | `name` | `TEXT` | Raw institution description / merchant name. Always populated. |
| `merchant_name` | string \| null | `merchant_name` | `TEXT NULL` | Plaid-enriched merchant name. Null when Plaid cannot identify merchant. |
| `personal_finance_category.primary` | string \| null | `category_primary` | `TEXT NULL` | High-level category (e.g., `FOOD_AND_DRINK`). |
| `personal_finance_category.detailed` | string \| null | `category_detailed` | `TEXT NULL` | Granular subcategory (e.g., `FOOD_AND_DRINK_RESTAURANTS`). |
| `personal_finance_category.confidence_level` | string \| null | `category_confidence` | `TEXT NULL` | `VERY_HIGH`, `HIGH`, `MEDIUM`, `LOW`, or `UNKNOWN`. |
| `payment_channel` | string \| null | `payment_channel` | `TEXT NULL` | `online`, `in store`, or `other`. |
| `pending` | boolean | `pending` | `BOOLEAN` | `true` if not yet settled. |

**Intentionally excluded Plaid fields:**

| Plaid Field | Reason Excluded |
|---|---|
| `category` | Legacy Plaid category array (e.g., `["Food and Drink", "Restaurants"]`). Superseded by `personal_finance_category`. Excluded to avoid storing both. |
| `category_id` | Companion to legacy `category`. Excluded for same reason. |
| `location` (object) | Contains address, city, region, lat/lon, store_number. Excluded for MVP — adds schema complexity for data rarely used in budgeting queries. Can be added in v2 as a `JSONB` column. |
| `counterparties` | Array of counterparty objects with name, entity_id, type, confidence. Detailed enrichment. Excluded for MVP; `merchant_name` covers the primary use case. |
| `merchant_entity_id` | Plaid's stable merchant entity identifier. Useful for normalizing merchants across transactions. Excluded for MVP (merchant normalization is a non-goal). |
| `logo_url` | URL to merchant logo PNG. Display enhancement; not required for data access. |
| `website` | Merchant website URL. Display enhancement; not required for data access. |
| `check_number` | Only relevant for check transactions. Excluded for MVP. |
| `account_owner` | Sub-account owner for multi-card scenarios. Not relevant for single-family use. |
| `transaction_code` | European institution payment classification. MVP targets US users. |
| `payment_meta` | Inter-bank transfer metadata (reference numbers, payee/payer names). Excluded for MVP. |
| `original_description` | Raw financial institution description (only returned when requested via options). `name` covers this use case. |

### 3.2 Account Fields (`/accounts/get` → `accounts` table)

| Plaid Field | Plaid Type | Breadbox Column | Breadbox Type | Notes |
|---|---|---|---|---|
| `account_id` | string | `external_account_id` | `TEXT` | Stable identifier for the account. |
| `name` | string | `name` | `TEXT` | Account name as displayed to user. |
| `official_name` | string \| null | `official_name` | `TEXT NULL` | Formal institution name. |
| `type` | string | `type` | `TEXT` | `depository`, `credit`, `loan`, `investment`, `other`. |
| `subtype` | string \| null | `subtype` | `TEXT NULL` | e.g., `checking`, `savings`, `credit card`, `mortgage`. |
| `mask` | string \| null | `mask` | `TEXT NULL` | Last 2–4 digits of account number. |
| `balances.current` | number \| null | `balance_current` | `NUMERIC(12,2) NULL` | Total balance. Meaning varies by account type. |
| `balances.available` | number \| null | `balance_available` | `NUMERIC(12,2) NULL` | Available funds / remaining credit. |
| `balances.limit` | number \| null | `balance_limit` | `NUMERIC(12,2) NULL` | Credit limit or overdraft limit. |
| `balances.iso_currency_code` | string \| null | `iso_currency_code` | `TEXT NULL` | Currency of the balance. |

**Intentionally excluded Plaid account fields:**

| Plaid Field | Reason Excluded |
|---|---|
| `balances.unofficial_currency_code` | Non-standard currency. MVP targets USD accounts. Can be added in v2. |
| `verification_status` | Micro-deposit verification state. Relevant during account linking, not stored persistently. |
| `holder_category` | Business vs. personal classification. Not a relevant filter for MVP. |

---

## 4. Key Design Decisions

### 4.1 UUIDs over Auto-Increment Integers

All primary keys use `UUID` (v4, random) generated with PostgreSQL's `gen_random_uuid()`.

**Rationale:**
- **No enumeration:** Integer IDs expose record counts and allow trivial enumeration of all records (`/transactions/1`, `/transactions/2`, ...). UUIDs are opaque.
- **Portability:** UUIDs can be generated in the application layer without a database round-trip, enabling pre-allocation of IDs before insert. Useful for idempotent sync operations.
- **Multi-source merging:** If data is ever merged from multiple database instances (e.g., migrating from one server to another), UUID primary keys avoid collisions that sequential integers would cause.
- **External compatibility:** Plaid and other providers use string identifiers; UUIDs are consistent with that convention.

**Trade-off:** UUIDs are larger (16 bytes vs. 4 bytes for `INT`) and random UUIDs cause index fragmentation over time. For Breadbox's expected data volume (tens of thousands of transactions per household), this is not a concern.

### 4.2 `NUMERIC(12,2)` for Monetary Amounts

All amount and balance columns use `NUMERIC(12,2)` (exact fixed-point arithmetic, 12 total digits, 2 decimal places).

**Rationale:**
- **Floating-point imprecision:** `FLOAT` and `DOUBLE PRECISION` types cannot exactly represent most decimal fractions. `0.1 + 0.2 != 0.3` in IEEE 754. This is unacceptable for financial data.
- **`NUMERIC` is exact:** PostgreSQL's `NUMERIC` type stores the exact decimal value with no rounding error.
- **Precision `(12, 2)`:** Supports balances and amounts up to $9,999,999,999.99 — sufficient for any personal banking scenario. Two decimal places covers cents in USD, EUR, GBP, and most other currencies.
- **`NUMERIC` not `DECIMAL`:** `NUMERIC` and `DECIMAL` are equivalent in PostgreSQL, but `NUMERIC` is the SQL standard name and preferred in Breadbox for clarity.

### 4.3 Soft Deletes on Transactions

Removed transactions (from Plaid's `removed` array) have their `deleted_at` column set to the current timestamp rather than being hard-deleted from the database.

**Rationale:**
- **Plaid's removal semantics:** When Plaid removes a transaction, it may be because the transaction was declined, reversed, or re-issued under a new ID. The original record may have been referenced by the agent in a prior conversation.
- **Agent continuity:** If an AI agent asked "what was that $50 Spotify charge last Tuesday?" and the transaction is hard-deleted because Plaid re-issued it with a new ID, the agent's memory of the conversation becomes inconsistent with the data.
- **Audit trail:** Soft deletes provide a complete history of what was seen and when, which is useful for debugging sync edge cases.
- **API behavior:** All REST API and MCP queries add `WHERE deleted_at IS NULL` by default. Soft-deleted transactions are invisible to consumers unless explicitly requested.
- **Storage cost:** Negligible. Breadbox is a personal finance tool; the volume of removed transactions is tiny.

### 4.4 Provider-Agnostic Connection Fields

The `bank_connections` table uses generic column names (`external_id`, `encrypted_credentials`, `sync_cursor`) rather than provider-specific names. This was refactored in Phase 8 (migration 00013) from the original `plaid_item_id` and `plaid_access_token` columns.

**Rationale:**
- **Multi-provider support:** All three providers (Plaid, Teller, CSV) use the same columns. No provider-specific columns exist.
- **Inspectability:** Named columns are visible in `psql \d bank_connections` and queryable with simple SQL. A `JSONB` blob would require knowing the internal JSON structure.
- **Type safety:** `BYTEA` for the encrypted credentials is a deliberate type choice. It cannot accidentally be treated as text.
- **Unique constraint:** `(provider, external_id)` ensures no duplicate connections per provider.
- **NULL semantics are clear:** `external_id IS NULL` for CSV connections (import-only, no external identifier needed).

### 4.5 Data Preservation on Connection Removal

When a bank connection is removed, accounts and transactions are **preserved**, not deleted. `accounts.connection_id` and `transactions.account_id` use `SET NULL` on delete rather than `CASCADE DELETE`.

**Rationale:**
- **Historical continuity:** Transaction history remains queryable after a bank is unlinked. An agent or user reviewing past spending should not lose records simply because the connection was severed.
- **Connection lifecycle:** Connections are set to `status = 'disconnected'` rather than being hard-deleted from the database. This preserves the sync audit trail in `sync_logs` and keeps the connection visible as a historical record in the dashboard.
- **Recoverable state:** If a connection is accidentally disconnected, the account and transaction history is still intact. Re-linking the same institution re-associates the data via the `external_account_id` upsert.
- **Orphan handling:** `accounts` with a `NULL` `connection_id` and `transactions` with a `NULL` `account_id` are treated as archived data. API queries should filter by `connection_id IS NOT NULL` or `account_id IS NOT NULL` when only active records are needed.
- **Exception:** `sync_logs.connection_id → bank_connections` still uses `CASCADE DELETE` since sync log rows without a connection have no meaning and consume space without value.
- **User label deletion:** `bank_connections.user_id → users` uses `SET NULL`. Deleting a user label does not cascade-delete the connections, accounts, or transactions — that would be catastrophic data loss for a family member removal.

### 4.6 Amount Sign Convention

Breadbox **passes through Plaid's native sign convention without inversion.**

**Plaid's convention:**
- `amount > 0`: Money is leaving the account. Purchases, fees, payments, withdrawals.
- `amount < 0`: Money is entering the account. Refunds, deposits, credits, received transfers.

**Example values:**
- A $12.50 coffee purchase: `amount = 12.50`
- A $1,000.00 paycheck deposit: `amount = -1000.00`
- A $50.00 refund: `amount = -50.00`

**Rationale for passthrough:**
- **Fidelity:** The Plaid API is authoritative. Inverting the sign would risk introducing a bug if Plaid ever changes its convention or if edge cases (certain transfer types) don't follow the expected pattern.
- **Agent compatibility:** Agents querying the REST API should work directly with Plaid documentation. If Breadbox inverted the sign, agents using Plaid documentation as context would produce incorrect analyses.
- **Explicit documentation:** The sign convention is documented here, in the API response schema, and in the MCP tool descriptions. The burden is on the consumer to understand the convention, not on Breadbox to hide it.

**API consumers:** The REST API and MCP tool descriptions explicitly document this convention. Agents should treat positive amounts as expenses and negative amounts as income/credits.

---

## 5. Indexes

This section documents every index in the schema, the query pattern it supports, and the rationale for its existence.

### 5.1 Complete Index Listing

#### `users`

| Index Name | Table | Columns | Type | Predicate | Purpose |
|---|---|---|---|---|---|
| `users_pkey` | `users` | `id` | B-tree | — | PK lookup by UUID. |
| `users_email_idx` | `users` | `email` | B-tree | — | Lookup by email; enforces `UNIQUE (email)`. |

#### `admin_accounts`

| Index Name | Table | Columns | Type | Predicate | Purpose |
|---|---|---|---|---|---|
| `admin_accounts_pkey` | `admin_accounts` | `id` | B-tree | — | PK lookup. |
| `admin_accounts_username_idx` | `admin_accounts` | `username` | B-tree | — | Login lookup; enforces `UNIQUE (username)`. |

#### `bank_connections`

| Index Name | Table | Columns | Type | Predicate | Purpose |
|---|---|---|---|---|---|
| `bank_connections_pkey` | `bank_connections` | `id` | B-tree | — | PK lookup. |
| `bank_connections_user_id_idx` | `bank_connections` | `user_id` | B-tree | — | List connections for a user. |
| `bank_connections_status_idx` | `bank_connections` | `status` | B-tree | — | Filter by status (health dashboard). |
| `bank_connections_provider_external_id_idx` | `bank_connections` | `(provider, external_id)` | B-tree (unique) | — | Webhook lookup by provider + external_id. Enforces unique constraint. |

#### `accounts`

| Index Name | Table | Columns | Type | Predicate | Purpose |
|---|---|---|---|---|---|
| `accounts_pkey` | `accounts` | `id` | B-tree | — | PK lookup. |
| `accounts_connection_id_idx` | `accounts` | `connection_id` | B-tree | — | List accounts for a connection; FK enforcement. |
| `accounts_external_account_id_idx` | `accounts` | `external_account_id` | B-tree | — | Upsert lookup during sync. Enforces unique constraint. |

#### `transactions`

| Index Name | Table | Columns | Type | Predicate | Purpose |
|---|---|---|---|---|---|
| `transactions_pkey` | `transactions` | `id` | B-tree | — | PK lookup. |
| `transactions_external_transaction_id_idx` | `transactions` | `external_transaction_id` | B-tree | — | Upsert during sync. Enforces unique constraint. |
| `transactions_account_id_date_idx` | `transactions` | `account_id, date DESC` | B-tree | — | Primary query pattern: transactions for account, newest first. |
| `transactions_account_id_date_active_idx` | `transactions` | `account_id, date DESC` | Partial B-tree | `WHERE deleted_at IS NULL` | Same as above but excludes soft-deleted rows. Used by default API queries. Smaller and faster than the full index for active data. |
| `transactions_date_idx` | `transactions` | `date DESC` | B-tree | — | Date-range queries across all accounts (agent-style: "all transactions last 30 days"). |
| `transactions_pending_idx` | `transactions` | `pending` | B-tree | — | Filter pending transactions. Low cardinality but useful as a combined condition. |
| `transactions_category_primary_idx` | `transactions` | `category_primary` | B-tree | — | Filter by spending category (e.g., all `FOOD_AND_DRINK`). |
| `transactions_name_merchant_gin_idx` | `transactions` | `name`, `merchant_name` | GIN (pg_trgm) | — | Full-text trigram similarity search on transaction description. Enables `ILIKE '%starbucks%'` at scale. |
| `transactions_account_id_idx` | `transactions` | `account_id` | B-tree | — | FK support for CASCADE DELETE and joins from accounts. |

#### `sync_logs`

| Index Name | Table | Columns | Type | Predicate | Purpose |
|---|---|---|---|---|---|
| `sync_logs_pkey` | `sync_logs` | `id` | B-tree | — | PK lookup. |
| `sync_logs_connection_id_started_at_idx` | `sync_logs` | `connection_id, started_at DESC` | B-tree | — | Recent sync history for a specific connection. |
| `sync_logs_started_at_idx` | `sync_logs` | `started_at DESC` | B-tree | — | All recent syncs across connections (admin overview). |

#### `api_keys`

| Index Name | Table | Columns | Type | Predicate | Purpose |
|---|---|---|---|---|---|
| `api_keys_pkey` | `api_keys` | `id` | B-tree | — | PK lookup. |
| `api_keys_key_hash_idx` | `api_keys` | `key_hash` | B-tree | — | Authentication: hash lookup on every API request. Enforces unique constraint. |

#### `app_config`

| Index Name | Table | Columns | Type | Predicate | Purpose |
|---|---|---|---|---|---|
| `app_config_pkey` | `app_config` | `key` | B-tree | — | Direct key lookup (primary access pattern). |

### 5.2 Transaction Query Patterns and Index Coverage

The following table shows the primary query patterns expected from the REST API and MCP tools, and which index serves each:

| Query Pattern | Example Filter | Index Used |
|---|---|---|
| All transactions for an account, recent first | `account_id = $1 AND deleted_at IS NULL ORDER BY date DESC` | `transactions_account_id_date_active_idx` |
| Date range for an account | `account_id = $1 AND date BETWEEN $2 AND $3 AND deleted_at IS NULL` | `transactions_account_id_date_active_idx` |
| Date range across all accounts (user-level) | `date BETWEEN $1 AND $2 AND deleted_at IS NULL` | `transactions_date_idx` (then filter by account join) |
| Filter by category | `category_primary = $1 AND deleted_at IS NULL` | `transactions_category_primary_idx` |
| Filter pending only | `pending = TRUE AND deleted_at IS NULL` | `transactions_pending_idx` |
| Text search on merchant/description | `(name ILIKE '%starbucks%' OR merchant_name ILIKE '%starbucks%') AND deleted_at IS NULL` | `transactions_name_merchant_gin_idx` |
| Sync upsert | `ON CONFLICT (external_transaction_id)` | `transactions_external_transaction_id_idx` |
| Soft-delete on removal | `UPDATE ... SET deleted_at = NOW() WHERE external_transaction_id = $1` | `transactions_external_transaction_id_idx` |
| Pending→posted: find old pending row | `WHERE external_transaction_id = $pending_transaction_id` | `transactions_external_transaction_id_idx` |

### 5.3 pg_trgm Extension

The GIN trigram index on `transactions` requires the `pg_trgm` extension:

```sql
CREATE EXTENSION IF NOT EXISTS pg_trgm;
```

This must be run before the index is created. It should be the first migration file.

---

## 6. Migration Strategy

### 6.1 Tool

Breadbox uses [goose](https://github.com/pressly/goose) for SQL migrations. Migration files are plain SQL with goose annotation comments. Goose tracks applied migrations in a `goose_db_version` table it manages automatically.

### 6.2 File Organization

Migration files live in `internal/db/migrations/`. Each file is named with a sequential numeric prefix (older) or timestamp prefix (recent) and a descriptive slug:

```
internal/db/migrations/
  00001_extensions.sql
  00002_enums.sql
  00003_users.sql
  00004_admin_accounts.sql
  00005_bank_connections.sql
  00006_accounts.sql
  00007_transactions.sql
  00008_sync_logs.sql
  00009_api_keys.sql
  00010_app_config.sql
  00011_seed_app_config.sql
```

### 6.3 Migration Ordering

Migrations must be applied in numeric order. The ordering respects foreign key dependencies:

1. **Extensions** — `pg_trgm`. Must be first; the transactions index depends on it.
2. **Enums** — All `CREATE TYPE ... AS ENUM` statements. Must precede tables that use them.
3. **`users`** — No dependencies.
4. **`admin_accounts`** — No dependencies.
5. **`bank_connections`** — References `users`.
6. **`accounts`** — References `bank_connections`.
7. **`transactions`** — References `accounts`. GIN index requires `pg_trgm` from step 1.
8. **`sync_logs`** — References `bank_connections`.
9. **`api_keys`** — No dependencies.
10. **`app_config`** — No dependencies.
11. **Seed data** — Inserts default `app_config` rows. Must follow `app_config` table creation.

### 6.4 Sessions Table

There is no manually managed `sessions` table in these migrations. Session storage is handled by the [`alexedwards/scs`](https://github.com/alexedwards/scs) library using its `pgxstore` backend. The `pgxstore` backend creates and manages its own `sessions` table automatically when the application starts. No manual migration is needed for sessions.

The `sessions` table created by `pgxstore` is opaque to the application schema — do not reference it in migrations or application queries. Session lifetime and cleanup are managed by `scs` internally.

### 6.5 Goose File Format

Every migration file must include both `Up` and `Down` sections. The `Down` section must fully reverse the `Up` section.

```sql
-- +goose Up
-- +goose StatementBegin

-- migration SQL here

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- reversal SQL here

-- +goose StatementEnd
```

Use `-- +goose StatementBegin` / `-- +goose StatementEnd` wrappers whenever the migration contains `DO $$ ... $$` blocks, function definitions, or any statement that itself contains semicolons (to prevent goose from splitting on internal semicolons).

### 6.6 Example Migration Files

These examples illustrate the goose file format and the earliest migrations — they may drift from the latest files in `internal/db/migrations/`. Always treat the on-disk migrations as the source of truth.

#### `00001_extensions.sql`

```sql
-- +goose Up
CREATE EXTENSION IF NOT EXISTS "pgcrypto";   -- gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS "pg_trgm";    -- trigram text search

-- +goose Down
DROP EXTENSION IF EXISTS "pg_trgm";
DROP EXTENSION IF EXISTS "pgcrypto";
```

#### `00002_enums.sql`

```sql
-- +goose Up
CREATE TYPE provider_type AS ENUM ('plaid', 'teller', 'csv');

CREATE TYPE connection_status AS ENUM (
    'active',
    'error',
    'pending_reauth',
    'disconnected'
);

CREATE TYPE sync_trigger AS ENUM (
    'cron',
    'webhook',
    'manual',
    'initial'
);

CREATE TYPE sync_status AS ENUM (
    'in_progress',
    'success',
    'error'
);

-- +goose Down
DROP TYPE IF EXISTS sync_status;
DROP TYPE IF EXISTS sync_trigger;
DROP TYPE IF EXISTS connection_status;
DROP TYPE IF EXISTS provider_type;
```

#### `00011_seed_app_config.sql`

```sql
-- +goose Up
INSERT INTO app_config (key, value, updated_at) VALUES
    ('plaid_client_id',    '',         NOW()),
    ('plaid_secret',       '',         NOW()),
    ('plaid_env',          'sandbox',  NOW()),
    ('webhook_url',        '',         NOW()),
    ('sync_interval_hours','12',       NOW()),
    ('setup_complete',     'false',    NOW())
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DELETE FROM app_config WHERE key IN (
    'plaid_client_id',
    'plaid_secret',
    'plaid_env',
    'webhook_url',
    'sync_interval_hours',
    'setup_complete'
);
```

### 6.7 Running Migrations

```bash
# Apply all pending migrations
goose -dir internal/db/migrations postgres "$DATABASE_URL" up

# Roll back the most recent migration
goose -dir internal/db/migrations postgres "$DATABASE_URL" down

# Check migration status
goose -dir internal/db/migrations postgres "$DATABASE_URL" status
```

In the Docker Compose deployment, the Breadbox binary runs `goose up` automatically at startup before starting the HTTP server. This ensures the schema is always up to date when the container starts, with no separate migration step required.

### 6.8 Adding Future Migrations

When adding a new migration:
1. Use the next sequential number as the prefix.
2. Never edit an existing migration file — always add a new one.
3. If a column needs a default added after the fact, write a new `ALTER TABLE` migration.
4. If an enum value needs to be added, use `ALTER TYPE ... ADD VALUE` in a new migration. Note: PostgreSQL does not support removing enum values; redesign the enum as a `TEXT` column if removal is needed.

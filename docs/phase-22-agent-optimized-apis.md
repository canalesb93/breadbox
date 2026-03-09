# Phase 22: Agent-Optimized APIs & Token Efficiency

**Version:** 1.0
**Status:** Draft
**Last Updated:** 2026-03-08

---

## Table of Contents

1. [Overview](#1-overview)
2. [Goals](#2-goals)
3. [Field Selection](#3-field-selection)
4. [Category Mapping MCP Tools](#4-category-mapping-mcp-tools)
5. [Transaction Summary Tool](#5-transaction-summary-tool)
6. [Overview Resource Enhancement](#6-overview-resource-enhancement)
7. [Service Layer Changes](#7-service-layer-changes)
8. [REST API Changes](#8-rest-api-changes)
9. [Token Efficiency Analysis](#9-token-efficiency-analysis)
10. [Implementation Tasks](#10-implementation-tasks)
11. [Dependencies](#11-dependencies)

---

## 1. Overview

Phase 22 makes the Breadbox API surface efficient for AI agent consumption. Today, every `query_transactions` call returns all 18+ fields per transaction, even when an agent only needs `date`, `amount`, and `name` for a spending summary. This wastes tokens and context window space. This phase introduces field selection on transaction endpoints, server-side aggregation via a `transaction_summary` MCP tool, category mapping CRUD through MCP (so agents can manage mappings without the dashboard), and richer overview stats that reduce the need for follow-up queries.

---

## 2. Goals

| Goal | Target |
|------|--------|
| Reduce typical transaction response size | 60-70% smaller with `fields=id,date,amount,name` vs full response |
| Eliminate pagination for spending analysis | `transaction_summary` returns aggregated totals in one call |
| Enable agent-driven category management | Full CRUD on `category_mappings` via MCP tools |
| Richer overview for onboarding | `breadbox://overview` provides enough context that agents rarely need `list_accounts` + `list_users` as warm-up calls |
| Zero breaking changes | Omitting `fields=` returns the full response (backward compatible) |

---

## 3. Field Selection

### 3.1 Supported Endpoints

Field selection applies to:

- `GET /api/v1/transactions` (REST)
- `GET /api/v1/transactions/{id}` (REST)
- `query_transactions` MCP tool

### 3.2 Query Parameter

```
?fields=id,date,amount,name,category
```

- Comma-separated list of field names.
- Case-sensitive, must match JSON field names exactly.
- If `fields` is omitted or empty, all fields are returned (backward compatible).
- Unknown field names return a `400` error with code `INVALID_FIELDS`.

### 3.3 Allowed Fields

Every JSON field on `TransactionResponse` is selectable. The canonical set:

| Field Name | JSON Key | SQL Column(s) | Always Included |
|------------|----------|---------------|-----------------|
| `id` | `id` | `t.id` | Yes (always) |
| `account_id` | `account_id` | `t.account_id` | No |
| `account_name` | `account_name` | `COALESCE(a.display_name, a.name)` | No |
| `user_name` | `user_name` | `u.name` | No |
| `amount` | `amount` | `t.amount` | No |
| `iso_currency_code` | `iso_currency_code` | `t.iso_currency_code` | No |
| `date` | `date` | `t.date` | No |
| `authorized_date` | `authorized_date` | `t.authorized_date` | No |
| `datetime` | `datetime` | `t.datetime` | No |
| `authorized_datetime` | `authorized_datetime` | `t.authorized_datetime` | No |
| `name` | `name` | `t.name` | No |
| `merchant_name` | `merchant_name` | `t.merchant_name` | No |
| `category` | `category` | Structured category object (slug, display_name, icon, color) | No |
| `category_override` | `category_override` | `t.category_override` | No |
| `category_primary_raw` | `category_primary_raw` | `t.category_primary` (raw provider string) | No |
| `category_detailed_raw` | `category_detailed_raw` | `t.category_detailed` (raw provider string) | No |
| `category_confidence` | `category_confidence` | `t.category_confidence` | No |
| `payment_channel` | `payment_channel` | `t.payment_channel` | No |
| `pending` | `pending` | `t.pending` | No |
| `created_at` | `created_at` | `t.created_at` | No |
| `updated_at` | `updated_at` | `t.updated_at` | No |

**`id` is always included** regardless of the `fields` parameter. This ensures every response row is identifiable and cursor pagination continues to work.

### 3.4 Shorthand Aliases

For common agent patterns, provide aliases that expand to multiple fields:

| Alias | Expands To |
|-------|-----------|
| `core` | `id,date,amount,name,iso_currency_code` |
| `category` | `category,category_primary_raw,category_detailed_raw` |
| `timestamps` | `created_at,updated_at,datetime,authorized_datetime` |

Example: `?fields=core,category,account_name` expands to `id,date,amount,name,iso_currency_code,category,category_primary_raw,category_detailed_raw,account_name`.

Aliases and explicit fields can be mixed. Duplicates are deduplicated.

### 3.5 Validation

```go
var validFields = map[string]bool{
    "id": true, "account_id": true, "account_name": true, "user_name": true,
    "amount": true, "iso_currency_code": true, "date": true,
    "authorized_date": true, "datetime": true, "authorized_datetime": true,
    "name": true, "merchant_name": true, "category": true,
    "category_override": true, "category_primary_raw": true,
    "category_detailed_raw": true, "category_confidence": true,
    "payment_channel": true, "pending": true, "created_at": true,
    "updated_at": true,
}

var fieldAliases = map[string][]string{
    "core":       {"id", "date", "amount", "name", "iso_currency_code"},
    "category":   {"category", "category_primary_raw", "category_detailed_raw"},
    "timestamps": {"created_at", "updated_at", "datetime", "authorized_datetime"},
}
```

If any field name is not in `validFields` and not in `fieldAliases`, return:

```json
{
  "error": {
    "code": "INVALID_FIELDS",
    "message": "Unknown field(s): foo, bar. Valid fields: id, account_id, ..."
  }
}
```

### 3.6 SQL Generation Changes

The current `ListTransactions` builds its SELECT clause as a hardcoded string. With field selection, the SELECT clause becomes dynamic.

**Approach: always SELECT all columns from SQL, filter in Go during serialization.** This is simpler and avoids breaking the row scanning logic. The SQL query and `rows.Scan()` call remain unchanged. The filtering happens when building the JSON response.

Rationale: The number of columns (22) is small. The bottleneck for agents is JSON response size and token count, not SQL column count. Keeping SQL generation unchanged avoids scan misalignment bugs and keeps the dynamic WHERE/ORDER/LIMIT builder untouched.

### 3.7 Response Serialization

Introduce a `filterFields` helper that takes a `TransactionResponse` and a field set, and returns a `map[string]any` containing only the requested fields:

```go
// ParseFields parses and validates the fields query parameter.
// Returns nil if no field selection (return all fields).
func ParseFields(raw string) (map[string]bool, error) {
    if raw == "" {
        return nil, nil
    }
    fields := make(map[string]bool)
    for _, f := range strings.Split(raw, ",") {
        f = strings.TrimSpace(f)
        if expanded, ok := fieldAliases[f]; ok {
            for _, ef := range expanded {
                fields[ef] = true
            }
            continue
        }
        if !validFields[f] {
            return nil, fmt.Errorf("unknown field: %s", f)
        }
        fields[f] = true
    }
    fields["id"] = true // always include
    return fields, nil
}

// FilterTransactionFields returns a map with only the requested fields.
// If fields is nil, returns the full struct as-is (no filtering).
func FilterTransactionFields(t TransactionResponse, fields map[string]bool) map[string]any {
    if fields == nil {
        return nil // signal to use full struct
    }
    m := make(map[string]any, len(fields))
    if fields["id"] { m["id"] = t.ID }
    if fields["account_id"] { m["account_id"] = t.AccountID }
    if fields["account_name"] { m["account_name"] = t.AccountName }
    if fields["user_name"] { m["user_name"] = t.UserName }
    if fields["amount"] { m["amount"] = t.Amount }
    if fields["iso_currency_code"] { m["iso_currency_code"] = t.IsoCurrencyCode }
    if fields["date"] { m["date"] = t.Date }
    if fields["authorized_date"] { m["authorized_date"] = t.AuthorizedDate }
    if fields["datetime"] { m["datetime"] = t.Datetime }
    if fields["authorized_datetime"] { m["authorized_datetime"] = t.AuthorizedDatetime }
    if fields["name"] { m["name"] = t.Name }
    if fields["merchant_name"] { m["merchant_name"] = t.MerchantName }
    if fields["category"] { m["category"] = t.Category }
    if fields["category_override"] { m["category_override"] = t.CategoryOverride }
    if fields["category_primary_raw"] { m["category_primary_raw"] = t.CategoryPrimaryRaw }
    if fields["category_detailed_raw"] { m["category_detailed_raw"] = t.CategoryDetailedRaw }
    if fields["category_confidence"] { m["category_confidence"] = t.CategoryConfidence }
    if fields["payment_channel"] { m["payment_channel"] = t.PaymentChannel }
    if fields["pending"] { m["pending"] = t.Pending }
    if fields["created_at"] { m["created_at"] = t.CreatedAt }
    if fields["updated_at"] { m["updated_at"] = t.UpdatedAt }
    return m
}
```

### 3.8 Where Field Filtering Lives

Field filtering is **not** in the service layer. The service layer always returns `TransactionListResult` with full `TransactionResponse` structs. Filtering happens in:

1. **REST API handler** (`internal/api/transactions.go`): parse `fields` from query params, apply `FilterTransactionFields` to each transaction before writing JSON.
2. **MCP tool handler** (`internal/mcp/tools.go`): accept `fields` input param, apply `FilterTransactionFields` to each transaction before marshaling to JSON text content.

This keeps the service layer clean and reusable. The admin dashboard always gets full responses.

### 3.9 MCP Tool Changes for Field Selection

Add `fields` to the `queryTransactionsInput` struct:

```go
type queryTransactionsInput struct {
    // ... existing fields ...
    Fields string `json:"fields,omitempty" jsonschema:"Comma-separated list of fields to include in response. Aliases: core (id,date,amount,name,iso_currency_code), category (category,category_primary_raw,category_detailed_raw), timestamps (created_at,updated_at,datetime,authorized_datetime). Default: all fields. id is always included."`
}
```

Update the `query_transactions` tool description to mention field selection:

```
"... Use the fields parameter to request only the fields you need (e.g., fields=core,category to get id, date, amount, name, currency, and categories). This significantly reduces response size."
```

---

## 4. Category Mapping MCP Tools

### 4.1 Prerequisites

These tools depend on the `categories` and `category_mappings` tables from Phase 20 (category mapping schema). If Phase 20 has not been implemented yet, these tools should be implemented in the same phase or deferred.

### 4.2 Tool: `list_category_mappings`

**Description:** List category mappings that translate provider-specific category strings to user categories. Filter by provider to see mappings for a specific bank data source. Returns the provider, raw provider category string, and the mapped user category slug and display name.

**Input struct:**

```go
type listCategoryMappingsInput struct {
    Provider   string `json:"provider,omitempty" jsonschema:"Filter by provider: plaid, teller, or csv"`
    CategoryID string `json:"category_id,omitempty" jsonschema:"Filter by target category ID"`
}
```

**Output shape:**

```json
{
  "mappings": [
    {
      "id": "uuid",
      "provider": "plaid",
      "provider_category": "FOOD_AND_DRINK_GROCERIES",
      "category_id": "uuid",
      "category_slug": "food_and_drink_groceries",
      "category_display_name": "Groceries",
      "created_at": "2026-03-01T00:00:00Z",
      "updated_at": "2026-03-01T00:00:00Z"
    }
  ],
  "count": 1
}
```

**SQL:**

```sql
SELECT cm.id, cm.provider, cm.provider_category, cm.category_id,
       c.slug AS category_slug, c.display_name AS category_display_name,
       cm.created_at, cm.updated_at
FROM category_mappings cm
JOIN categories c ON cm.category_id = c.id
WHERE ($1::provider_type IS NULL OR cm.provider = $1)
  AND ($2::uuid IS NULL OR cm.category_id = $2)
ORDER BY cm.provider, cm.provider_category;
```

### 4.3 Tool: `create_category_mapping`

**Description:** Create a new category mapping that tells the system how to translate a provider's raw category string to a user category. For example, map Teller's "dining" to your "food_and_drink_restaurant" category. The mapping takes effect on the next sync — existing transactions are not retroactively updated (use re-map for that).

**Input struct:**

```go
type createCategoryMappingInput struct {
    Provider         string `json:"provider" jsonschema:"required,Provider type: plaid, teller, or csv"`
    ProviderCategory string `json:"provider_category" jsonschema:"required,Raw category string from the provider (e.g. FOOD_AND_DRINK_GROCERIES for Plaid or dining for Teller)"`
    CategorySlug     string `json:"category_slug" jsonschema:"required,Slug of the target user category (e.g. food_and_drink_restaurant). Use list_categories to find valid slugs."`
}
```

**Behavior:**

1. Validate `provider` is one of `plaid`, `teller`, `csv`.
2. Look up category by `slug` to get `category_id`. Return error if slug not found.
3. `INSERT INTO category_mappings (provider, provider_category, category_id) VALUES ($1, $2, $3)`.
4. If `UNIQUE(provider, provider_category)` conflict, return error with message: "Mapping already exists for (provider, provider_category). Use update_category_mapping to change it."
5. Return the created mapping (same shape as list item).

**Error cases:**

- Unknown provider: `{"error": "Invalid provider 'foo'. Must be plaid, teller, or csv."}`
- Unknown category slug: `{"error": "Category 'bad_slug' not found. Use list_categories to see valid slugs."}`
- Duplicate mapping: `{"error": "Mapping already exists for (plaid, FOOD_AND_DRINK). Use update_category_mapping to change it."}`

### 4.4 Tool: `update_category_mapping`

**Description:** Update an existing category mapping to point to a different user category. Identified by mapping ID or by the (provider, provider_category) pair. Does not retroactively update transactions — run a re-map or wait for next sync.

**Input struct:**

```go
type updateCategoryMappingInput struct {
    ID               string `json:"id,omitempty" jsonschema:"Mapping ID to update (alternative to provider + provider_category)"`
    Provider         string `json:"provider,omitempty" jsonschema:"Provider type (required if not using id)"`
    ProviderCategory string `json:"provider_category,omitempty" jsonschema:"Raw provider category string (required if not using id)"`
    CategorySlug     string `json:"category_slug" jsonschema:"required,New target category slug"`
}
```

**Behavior:**

1. If `id` is provided, update by ID. Otherwise, require both `provider` and `provider_category` and update by unique constraint.
2. Look up target category by slug to get `category_id`.
3. `UPDATE category_mappings SET category_id = $1, updated_at = NOW() WHERE ...`.
4. If no row matched, return error: "Mapping not found."
5. Return the updated mapping.

### 4.5 Tool: `delete_category_mapping`

**Description:** Delete a category mapping. After deletion, transactions with this provider category string will fall back to 'uncategorized' on next sync. Identified by mapping ID or by (provider, provider_category) pair.

**Input struct:**

```go
type deleteCategoryMappingInput struct {
    ID               string `json:"id,omitempty" jsonschema:"Mapping ID to delete (alternative to provider + provider_category)"`
    Provider         string `json:"provider,omitempty" jsonschema:"Provider type (required if not using id)"`
    ProviderCategory string `json:"provider_category,omitempty" jsonschema:"Raw provider category string (required if not using id)"`
}
```

**Behavior:**

1. Delete by ID or by (provider, provider_category).
2. If no row matched, return error: "Mapping not found."
3. Return `{"deleted": true, "provider": "plaid", "provider_category": "FOOD_AND_DRINK_GROCERIES"}`.

---

## 5. Transaction Summary Tool

### 5.1 Purpose

Today, an agent that wants "spending by category for March" must paginate through all March transactions, accumulate totals client-side, and burn hundreds of thousands of tokens. The `transaction_summary` tool does server-side aggregation and returns compact results.

### 5.2 Tool Definition

**Name:** `transaction_summary`

**Description:** Get aggregated transaction totals grouped by category and/or time period. Replaces the need to paginate through thousands of individual transactions for spending analysis. Amounts follow the convention: positive = money out (debit), negative = money in (credit). Only includes non-deleted, non-pending transactions by default.

**Input struct:**

```go
type transactionSummaryInput struct {
    StartDate     string   `json:"start_date,omitempty" jsonschema:"Start date (YYYY-MM-DD) inclusive. Defaults to 30 days ago."`
    EndDate       string   `json:"end_date,omitempty" jsonschema:"End date (YYYY-MM-DD) exclusive. Defaults to today."`
    GroupBy       string   `json:"group_by" jsonschema:"required,How to group results: category, month, week, day, or category_month"`
    AccountID     string   `json:"account_id,omitempty" jsonschema:"Filter by account ID"`
    UserID        string   `json:"user_id,omitempty" jsonschema:"Filter by user ID (family member)"`
    Category      string   `json:"category,omitempty" jsonschema:"Filter by primary category before aggregating"`
    IncludePending *bool   `json:"include_pending,omitempty" jsonschema:"Include pending transactions (default false)"`
}
```

### 5.3 Group By Options

| `group_by` | GROUP BY clause | Output keys |
|------------|-----------------|-------------|
| `category` | `t.category_primary` | `category` |
| `month` | `date_trunc('month', t.date)` | `period` (YYYY-MM) |
| `week` | `date_trunc('week', t.date)` | `period` (YYYY-MM-DD, Monday of week) |
| `day` | `t.date` | `period` (YYYY-MM-DD) |
| `category_month` | `t.category_primary, date_trunc('month', t.date)` | `category`, `period` |

### 5.4 Output Shape

```json
{
  "summary": [
    {
      "category": "FOOD_AND_DRINK",
      "period": "2026-03",
      "total_amount": 847.32,
      "transaction_count": 45,
      "iso_currency_code": "USD"
    },
    {
      "category": "TRANSPORTATION",
      "period": "2026-03",
      "total_amount": 234.50,
      "transaction_count": 12,
      "iso_currency_code": "USD"
    }
  ],
  "totals": {
    "total_amount": 1081.82,
    "transaction_count": 57
  },
  "filters": {
    "start_date": "2026-03-01",
    "end_date": "2026-04-01",
    "group_by": "category_month"
  }
}
```

**Notes:**

- `category` is omitted from rows when `group_by` is `month`, `week`, or `day`.
- `period` is omitted from rows when `group_by` is `category`.
- Results are grouped per `iso_currency_code`. If transactions span multiple currencies, each currency gets its own rows. The `totals` object only includes totals when all filtered transactions share one currency; otherwise `totals.total_amount` is omitted and `totals.note` says "Multiple currencies — see per-row totals."
- Rows are sorted by `total_amount DESC` (largest spend first) within each period.

### 5.5 SQL Query Approach

The query is built dynamically, reusing the same pattern as `ListTransactions` (positional `$N` params, composable WHERE clauses).

```sql
-- Example: group_by = category_month
SELECT
    t.category_primary AS category,
    to_char(date_trunc('month', t.date), 'YYYY-MM') AS period,
    t.iso_currency_code,
    SUM(t.amount) AS total_amount,
    COUNT(*) AS transaction_count
FROM transactions t
JOIN accounts a ON t.account_id = a.id
LEFT JOIN bank_connections bc ON a.connection_id = bc.id
WHERE t.deleted_at IS NULL
  AND t.pending = false
  AND t.date >= $1
  AND t.date < $2
GROUP BY t.category_primary, date_trunc('month', t.date), t.iso_currency_code
ORDER BY date_trunc('month', t.date) DESC, SUM(t.amount) DESC;
```

For `group_by = category`:

```sql
SELECT
    t.category_primary AS category,
    t.iso_currency_code,
    SUM(t.amount) AS total_amount,
    COUNT(*) AS transaction_count
FROM transactions t
JOIN accounts a ON t.account_id = a.id
LEFT JOIN bank_connections bc ON a.connection_id = bc.id
WHERE t.deleted_at IS NULL
  AND t.pending = false
  AND t.date >= $1
  AND t.date < $2
GROUP BY t.category_primary, t.iso_currency_code
ORDER BY SUM(t.amount) DESC;
```

### 5.6 Default Date Range

If `start_date` is omitted, default to 30 days ago. If `end_date` is omitted, default to today (exclusive, so `today + 1 day`). This ensures agents get useful results without specifying dates.

---

## 6. Overview Resource Enhancement

### 6.1 Current State

The `breadbox://overview` resource currently returns:

```json
{
  "user_count": 3,
  "connection_count": 2,
  "account_count": 5,
  "transaction_count": 1247,
  "earliest_transaction_date": "2024-06-15",
  "latest_transaction_date": "2026-03-07"
}
```

### 6.2 Enhanced Response

Add richer stats so agents can understand the dataset without making multiple follow-up calls:

```json
{
  "user_count": 3,
  "connection_count": 2,
  "account_count": 5,
  "transaction_count": 1247,
  "pending_transaction_count": 8,
  "earliest_transaction_date": "2024-06-15",
  "latest_transaction_date": "2026-03-07",
  "users": [
    {"id": "uuid", "name": "Ricardo"},
    {"id": "uuid", "name": "Maria"}
  ],
  "accounts_by_type": {
    "checking": 2,
    "savings": 1,
    "credit": 2
  },
  "connections": [
    {
      "id": "uuid",
      "provider": "plaid",
      "institution_name": "Chase",
      "status": "active",
      "last_synced_at": "2026-03-07T14:30:00Z",
      "account_count": 3
    }
  ],
  "spending_summary_30d": {
    "total_amount": 4523.47,
    "transaction_count": 187,
    "iso_currency_code": "USD",
    "top_categories": [
      {"category": "FOOD_AND_DRINK", "amount": 1234.56, "count": 67},
      {"category": "TRANSPORTATION", "amount": 543.21, "count": 23},
      {"category": "SHOPPING", "amount": 432.10, "count": 15}
    ]
  }
}
```

### 6.3 New Fields

| Field | SQL | Purpose |
|-------|-----|---------|
| `pending_transaction_count` | `SELECT COUNT(*) FROM transactions WHERE deleted_at IS NULL AND pending = true` | Alerts agents to unposted data |
| `users` | `SELECT id, name FROM users ORDER BY name` | Eliminates need for `list_users` call |
| `accounts_by_type` | `SELECT type, COUNT(*) FROM accounts GROUP BY type` | Quick account profile |
| `connections` | `SELECT id, provider, institution_name, status, last_synced_at, (SELECT COUNT(*) FROM accounts WHERE connection_id = bc.id) FROM bank_connections WHERE status != 'disconnected'` | Connection health at a glance |
| `spending_summary_30d` | Aggregation query over last 30 days | Immediate spending context |
| `spending_summary_30d.top_categories` | `SELECT category_primary, SUM(amount), COUNT(*) ... GROUP BY category_primary ORDER BY SUM(amount) DESC LIMIT 5` | Top spending categories |

### 6.4 Updated `OverviewStats` Struct

```go
type OverviewStats struct {
    UserCount               int                    `json:"user_count"`
    ConnectionCount         int                    `json:"connection_count"`
    AccountCount            int                    `json:"account_count"`
    TransactionCount        int64                  `json:"transaction_count"`
    PendingTransactionCount int64                  `json:"pending_transaction_count"`
    EarliestDate            string                 `json:"earliest_transaction_date,omitempty"`
    LatestDate              string                 `json:"latest_transaction_date,omitempty"`
    Users                   []OverviewUser         `json:"users"`
    AccountsByType          map[string]int         `json:"accounts_by_type"`
    Connections             []OverviewConnection   `json:"connections"`
    SpendingSummary30d      *OverviewSpending      `json:"spending_summary_30d,omitempty"`
}

type OverviewUser struct {
    ID   string `json:"id"`
    Name string `json:"name"`
}

type OverviewConnection struct {
    ID              string  `json:"id"`
    Provider        string  `json:"provider"`
    InstitutionName *string `json:"institution_name"`
    Status          string  `json:"status"`
    LastSyncedAt    *string `json:"last_synced_at"`
    AccountCount    int     `json:"account_count"`
}

type OverviewSpending struct {
    TotalAmount      float64                `json:"total_amount"`
    TransactionCount int64                  `json:"transaction_count"`
    IsoCurrencyCode  string                 `json:"iso_currency_code"`
    TopCategories    []OverviewCategorySpend `json:"top_categories"`
}

type OverviewCategorySpend struct {
    Category string  `json:"category"`
    Amount   float64 `json:"amount"`
    Count    int64   `json:"count"`
}
```

---

## 7. Service Layer Changes

### 7.1 New Types in `internal/service/types.go`

```go
// Field selection
type TransactionListParams struct {
    // ... existing fields unchanged ...
    Fields map[string]bool // nil = all fields (set by caller, not service)
}

// Transaction summary
type TransactionSummaryParams struct {
    StartDate      *time.Time
    EndDate        *time.Time
    GroupBy        string // "category", "month", "week", "day", "category_month"
    AccountID      *string
    UserID         *string
    Category       *string
    IncludePending bool
}

type TransactionSummaryResult struct {
    Summary []TransactionSummaryRow `json:"summary"`
    Totals  TransactionSummaryTotals `json:"totals"`
    Filters TransactionSummaryFilters `json:"filters"`
}

type TransactionSummaryRow struct {
    Category         *string `json:"category,omitempty"`
    Period           *string `json:"period,omitempty"`
    TotalAmount      float64 `json:"total_amount"`
    TransactionCount int64   `json:"transaction_count"`
    IsoCurrencyCode  string  `json:"iso_currency_code"`
}

type TransactionSummaryTotals struct {
    TotalAmount      *float64 `json:"total_amount,omitempty"`
    TransactionCount int64    `json:"transaction_count"`
    Note             string   `json:"note,omitempty"`
}

type TransactionSummaryFilters struct {
    StartDate string `json:"start_date"`
    EndDate   string `json:"end_date"`
    GroupBy   string `json:"group_by"`
}

// Category mapping CRUD
type CategoryMappingResponse struct {
    ID                  string `json:"id"`
    Provider            string `json:"provider"`
    ProviderCategory    string `json:"provider_category"`
    CategoryID          string `json:"category_id"`
    CategorySlug        string `json:"category_slug"`
    CategoryDisplayName string `json:"category_display_name"`
    CreatedAt           string `json:"created_at"`
    UpdatedAt           string `json:"updated_at"`
}

type CreateCategoryMappingParams struct {
    Provider         string
    ProviderCategory string
    CategorySlug     string
}

type UpdateCategoryMappingParams struct {
    ID               *string // update by ID
    Provider         *string // update by (provider, provider_category)
    ProviderCategory *string
    CategorySlug     string
}
```

### 7.2 New Service Methods

All in `internal/service/`:

```go
// internal/service/transactions.go (or new file: transaction_summary.go)
func (s *Service) GetTransactionSummary(ctx context.Context, params TransactionSummaryParams) (*TransactionSummaryResult, error)

// internal/service/category_mappings.go (new file)
func (s *Service) ListCategoryMappings(ctx context.Context, provider *string, categoryID *string) ([]CategoryMappingResponse, error)
func (s *Service) CreateCategoryMapping(ctx context.Context, params CreateCategoryMappingParams) (*CategoryMappingResponse, error)
func (s *Service) UpdateCategoryMapping(ctx context.Context, params UpdateCategoryMappingParams) (*CategoryMappingResponse, error)
func (s *Service) DeleteCategoryMapping(ctx context.Context, id *string, provider *string, providerCategory *string) error

// internal/service/overview.go (modify existing)
func (s *Service) GetOverviewStats(ctx context.Context) (*OverviewStats, error)  // enhanced
```

### 7.3 Field Selection Helpers

New file: `internal/service/fields.go`

```go
package service

// ParseFields, FilterTransactionFields, validFields, fieldAliases
// (See Section 3.7 for full implementation)
```

This file is used by API handlers and MCP tool handlers, not by the service methods themselves.

---

## 8. REST API Changes

### 8.1 Modified: `GET /api/v1/transactions`

**New query parameter:** `fields` (optional, comma-separated)

**Before (no field selection):**

```
GET /api/v1/transactions?start_date=2026-03-01&limit=2
```

```json
{
  "transactions": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "account_id": "660e8400-e29b-41d4-a716-446655440001",
      "account_name": "Chase Checking",
      "user_name": "Ricardo",
      "amount": 45.67,
      "iso_currency_code": "USD",
      "date": "2026-03-07",
      "authorized_date": "2026-03-06",
      "datetime": null,
      "authorized_datetime": null,
      "name": "WHOLE FOODS MARKET",
      "merchant_name": "Whole Foods",
      "category_primary": "FOOD_AND_DRINK",
      "category_detailed": "FOOD_AND_DRINK_GROCERIES",
      "category_confidence": "VERY_HIGH",
      "payment_channel": "in store",
      "pending": false,
      "created_at": "2026-03-07T15:00:00Z",
      "updated_at": "2026-03-07T15:00:00Z"
    }
  ],
  "next_cursor": "...",
  "has_more": true,
  "limit": 2
}
```

**After (with field selection):**

```
GET /api/v1/transactions?start_date=2026-03-01&limit=2&fields=core,category
```

```json
{
  "transactions": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "date": "2026-03-07",
      "amount": 45.67,
      "name": "WHOLE FOODS MARKET",
      "iso_currency_code": "USD",
      "category_primary": "FOOD_AND_DRINK",
      "category_detailed": "FOOD_AND_DRINK_GROCERIES"
    }
  ],
  "next_cursor": "...",
  "has_more": true,
  "limit": 2
}
```

### 8.2 Modified: `GET /api/v1/transactions/{id}`

Same `fields` parameter support. If provided, response body contains only the selected fields.

### 8.3 New: `GET /api/v1/transactions/summary` (optional)

Expose the summary aggregation via REST as well (not just MCP). Same parameters as the MCP tool but as query params.

```
GET /api/v1/transactions/summary?start_date=2026-03-01&end_date=2026-04-01&group_by=category
```

Response shape matches Section 5.4.

---

## 9. Token Efficiency Analysis

### 9.1 Methodology

Measure JSON response sizes for a typical `query_transactions` call returning 50 transactions.

### 9.2 Full Response (baseline)

A typical transaction serialized to JSON is approximately **450 bytes** (including all 19 fields, null values, whitespace from encoding).

```
50 transactions x 450 bytes = ~22,500 bytes per page
+ envelope (cursor, has_more, limit) = ~22,600 bytes
```

Token estimate (GPT-4/Claude tokenizer averages ~4 chars/token): **~5,650 tokens per page**.

### 9.3 With `fields=core` (id, date, amount, name, iso_currency_code)

A core-only transaction is approximately **150 bytes**.

```
50 transactions x 150 bytes = ~7,500 bytes per page
+ envelope = ~7,600 bytes
```

Token estimate: **~1,900 tokens per page**.

**Reduction: ~66%** fewer tokens.

### 9.4 With `fields=core,category`

Approximately **210 bytes** per transaction.

```
50 transactions x 210 bytes = ~10,500 bytes
```

Token estimate: **~2,625 tokens per page**.

**Reduction: ~54%** fewer tokens.

### 9.5 `transaction_summary` vs Paginated Queries

An agent analyzing 3 months of spending by category (e.g., 500 transactions across 12 categories):

**Without summary tool:**
- 10 pages x ~5,650 tokens = **~56,500 tokens** consumed
- Plus agent compute to aggregate

**With summary tool:**
- 1 call returning ~12 rows x ~100 bytes = **~300 tokens**

**Reduction: ~99.5%** fewer tokens for aggregation queries.

### 9.6 Enhanced Overview Resource

**Current overview:** ~200 bytes (~50 tokens)

**Enhanced overview:** ~1,500 bytes (~375 tokens)

This is larger, but eliminates the need for the typical agent warm-up pattern of `list_users` + `list_accounts` + `get_sync_status` (~3,000+ tokens combined). **Net savings: ~2,600 tokens** on agent session startup.

---

## 10. Implementation Tasks

### Task 1: Field Selection Infrastructure

**Files:** `internal/service/fields.go` (new)

1. Implement `validFields` map, `fieldAliases` map
2. Implement `ParseFields(raw string) (map[string]bool, error)`
3. Implement `FilterTransactionFields(t TransactionResponse, fields map[string]bool) map[string]any`
4. Unit tests for parse/filter logic (alias expansion, unknown fields, deduplication, id always included)

### Task 2: REST API Field Selection

**Files:** `internal/api/transactions.go`

1. Parse `fields` query param in `ListTransactionsHandler`
2. If fields is non-nil, map each transaction through `FilterTransactionFields` before writing response
3. Parse `fields` query param in `GetTransactionHandler`
4. Apply same filtering to single-transaction response
5. Integration tests for field selection on both endpoints

### Task 3: MCP Field Selection

**Files:** `internal/mcp/tools.go`

1. Add `Fields` to `queryTransactionsInput` struct with `jsonschema` tag
2. In `handleQueryTransactions`, parse fields and apply filtering before JSON marshaling
3. Update `query_transactions` tool description to mention field selection

### Task 4: Transaction Summary Service

**Files:** `internal/service/transaction_summary.go` (new), `internal/service/types.go`

1. Add `TransactionSummaryParams`, `TransactionSummaryResult`, and related types to `types.go`
2. Implement `GetTransactionSummary` with dynamic SQL builder
3. Handle all five `group_by` options
4. Handle multi-currency edge case
5. Default date range (30 days)
6. Unit tests for query building, edge cases

### Task 5: Transaction Summary MCP Tool

**Files:** `internal/mcp/tools.go`

1. Add `transactionSummaryInput` struct
2. Implement `handleTransactionSummary` handler
3. Register `transaction_summary` tool in `registerTools()`
4. Update server instructions in `server.go` to mention the new tool

### Task 6: Transaction Summary REST Endpoint (optional)

**Files:** `internal/api/transactions.go`, `internal/api/router.go`

1. Add `TransactionSummaryHandler` function
2. Register `GET /api/v1/transactions/summary` route
3. Parse query params matching MCP tool inputs

### Task 7: Category Mapping Service Layer

**Files:** `internal/service/category_mappings.go` (new), `internal/service/types.go`

1. Add `CategoryMappingResponse`, `CreateCategoryMappingParams`, `UpdateCategoryMappingParams` types
2. Implement `ListCategoryMappings`
3. Implement `CreateCategoryMapping` (with slug-to-ID lookup, conflict handling)
4. Implement `UpdateCategoryMapping` (by ID or by provider+provider_category)
5. Implement `DeleteCategoryMapping` (by ID or by provider+provider_category)

**Note:** These depend on the `categories` and `category_mappings` tables existing (Phase 20). If those tables are not yet created, this task is blocked until they are.

### Task 8: Category Mapping MCP Tools

**Files:** `internal/mcp/tools.go`

1. Add input structs for all four category mapping tools
2. Implement handlers: `handleListCategoryMappings`, `handleCreateCategoryMapping`, `handleUpdateCategoryMapping`, `handleDeleteCategoryMapping`
3. Register all four tools in `registerTools()`
4. Update server instructions to document category mapping tools

### Task 9: Enhanced Overview Resource

**Files:** `internal/service/overview.go`

1. Expand `OverviewStats` struct with new fields
2. Add `OverviewUser`, `OverviewConnection`, `OverviewSpending`, `OverviewCategorySpend` types
3. Update `GetOverviewStats` to query additional data:
   - Pending transaction count
   - Users list
   - Accounts by type
   - Connections with account counts
   - 30-day spending summary with top 5 categories
4. Keep queries efficient — each is a simple aggregate, no joins needed for most

### Task 10: Update MCP Server Instructions

**Files:** `internal/mcp/server.go`

1. Update the `Instructions` string to mention:
   - Field selection on `query_transactions`
   - `transaction_summary` for aggregation queries
   - Category mapping tools for managing provider mappings
   - Enhanced overview resource
2. Update recommended query patterns

### Task 11: Response Size Benchmarks

1. Write a benchmark test that measures JSON response sizes for:
   - Full transaction list (50 items)
   - `fields=core` transaction list (50 items)
   - `fields=core,category` transaction list (50 items)
   - Transaction summary (category, month, category_month)
   - Old vs new overview resource
2. Document results in a table (can be in test output or a separate section of this doc)

### Task 12: Update CLAUDE.md

1. Add Phase 22 conventions:
   - Field selection pattern (`fields=` param, aliases, always-include `id`)
   - `transaction_summary` tool and its `group_by` options
   - Category mapping MCP tools (list/create/update/delete)
   - Enhanced overview resource fields

---

## 11. Dependencies

### Depends On

| Phase | Dependency | Notes |
|-------|-----------|-------|
| Phase 20 | `categories` + `category_mappings` tables | Category mapping MCP tools require these tables. If Phase 20 is not complete, Tasks 7-8 are blocked. All other tasks (field selection, summary, overview) can proceed independently. |

### Depended On By

| Phase | Relationship |
|-------|-------------|
| Phase 23 (MCP Permissions) | Phase 23 adds per-tool enable/disable and read-only mode. The new tools from Phase 22 need to be covered by Phase 23's permission model. Category mapping write tools should respect read-only mode. |
| Phase 25 (External Agents) | External agents benefit most from field selection and summary tools. Phase 22 optimizations directly improve the external agent experience. |

### Independent Tasks

Field selection (Tasks 1-3), transaction summary (Tasks 4-6), and overview enhancement (Task 9) have no dependencies on Phase 20 and can be implemented immediately. Category mapping tools (Tasks 7-8) require Phase 20.

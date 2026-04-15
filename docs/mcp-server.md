# Breadbox MCP Server Specification

## Overview

Breadbox exposes financial data to AI agents through the [Model Context Protocol (MCP)](https://modelcontextprotocol.io). The MCP server is implemented using the official Go SDK (`github.com/modelcontextprotocol/go-sdk`, v1.4.0) and runs as an integral part of the main `breadbox serve` process — not as a separate service.

MCP tools are thin wrappers around the same service and query layer that backs the REST API. There is no separate data access path for MCP. This means business logic lives in one place, the REST API can be tested independently of MCP concerns, and any bug fix or behavior change automatically propagates to both consumers.

---

## Table of Contents

1. [MCP Server Configuration](#1-mcp-server-configuration)
2. [Client Configuration](#2-client-configuration)
3. [Tool Definitions](#3-tool-definitions)
4. [MCP Resources](#4-mcp-resources)
5. [Error Handling](#5-error-handling)
6. [Pagination Strategy](#6-pagination-strategy)
7. [Relationship to REST API](#7-relationship-to-rest-api)

---

## 1. MCP Server Configuration

### Server Identity

The MCP server advertises the following implementation metadata to connecting clients:

```go
mcp.NewServer(&mcp.Implementation{
    Name:    "breadbox",
    Version: "0.1.0",
}, nil)
```

The version string should match the application version reported by `breadbox --version`.

### Transports

Breadbox supports two transports. Streamable HTTP is the primary transport intended for all normal use. Stdio is a convenience mode for local development.

#### Streamable HTTP (Primary)

The MCP server shares the chi router used by the REST API and admin dashboard. It is mounted at `/mcp` and handles all MCP protocol requests there.

The Go MCP SDK provides `mcp.NewStreamableHTTPHandler`, which returns an `http.Handler` that can be mounted directly on any standard-library-compatible router:

```go
mcpHandler := mcp.NewStreamableHTTPHandler(
    func(r *http.Request) *mcp.Server { return mcpServer },
    &mcp.StreamableHTTPOptions{
        EventStore: mcp.NewMemoryEventStore(nil),
    },
)

chiRouter.Handle("/mcp", mcpHandler)
chiRouter.Handle("/mcp/*", mcpHandler)
```

The `EventStore` enables session resumption — if a client's HTTP connection drops mid-stream, it can reconnect and resume from where it left off using the `Mcp-Session-Id` header. `NewMemoryEventStore` is sufficient for MVP; a persistent event store is a post-MVP concern.

The handler responds to:
- `POST /mcp` — JSON-RPC messages from the client
- `GET /mcp` — Opens a standalone SSE stream or resumes an interrupted one
- `DELETE /mcp` — Terminates the session

#### Stdio (Secondary — Local Dev)

A separate entry point `breadbox mcp-stdio` runs the same MCP server over stdin/stdout. This is useful for connecting Claude Desktop on the same machine without needing a running HTTP server. It uses the same `mcpServer` instance (wired to the same pgx pool) but with a different transport:

```go
// Entry point: breadbox mcp-stdio
mcpServer.Run(ctx, &mcp.StdioTransport{})
```

This command is a blocking call that reads JSON-RPC from stdin and writes responses to stdout. It does not start an HTTP listener.

### Database Connection

The MCP server does not manage its own database connection. It receives the same `*pgxpool.Pool` that is passed to the REST API handlers during application startup. All tool handlers call into service functions that accept this pool, ensuring MCP and REST share identical query behavior.

### Server Capabilities

The server advertises tools and resources. Prompts are not used. Server instructions are provided via `ServerOptions.Instructions` (customizable from the MCP admin page, with a comprehensive default).

```go
server := mcpsdk.NewServer(
    &mcpsdk.Implementation{Name: "breadbox", Version: s.version},
    &mcpsdk.ServerOptions{Instructions: instructions},
)
```

Three resources are registered on every server instance via `registerResources()` (see [Section 4](#4-mcp-resources)).

---

## 2. Client Configuration

### Authentication

MCP over Streamable HTTP shares the same server as the REST API. **MCP endpoints should be protected by the same API key mechanism as the REST API.** Clients must supply the `X-API-Key` header with every HTTP request.

The Go MCP SDK does not enforce authentication internally — it is applied as chi middleware before the MCP handler receives the request:

```go
chiRouter.Group(func(r chi.Router) {
    r.Use(APIKeyAuthMiddleware)
    r.Handle("/mcp", mcpHandler)
    r.Handle("/mcp/*", mcpHandler)
})
```

For stdio mode (`breadbox mcp-stdio`), authentication is not required because the process runs locally with direct database access, not over the network.

**API key validation behavior for Streamable HTTP:** The API key is validated on each HTTP request by the middleware. For SSE streams, the key is validated when the stream is established. If a key is revoked mid-session, subsequent HTTP requests (including new SSE connections) will fail with an authentication error; the active SSE stream itself will not be forcibly terminated until the next request boundary. The `last_used_at` timestamp on the `api_keys` record is updated on session establishment (the initial authenticated request), not on every MCP message within the session.

### Claude Desktop Configuration

Claude Desktop reads `~/Library/Application Support/Claude/claude_desktop_config.json` on macOS. Add a Breadbox entry under `mcpServers`.

**Option A — Streamable HTTP (recommended when Breadbox is deployed remotely or in Docker):**

```json
{
  "mcpServers": {
    "breadbox": {
      "type": "http",
      "url": "http://localhost:8080/mcp",
      "headers": {
        "X-API-Key": "bb_your_api_key_here"
      }
    }
  }
}
```

Replace `localhost:8080` with the actual host and port where Breadbox is running.

**Option B — Stdio (when Breadbox is installed locally and `breadbox` is on your PATH):**

```json
{
  "mcpServers": {
    "breadbox": {
      "type": "stdio",
      "command": "breadbox",
      "args": ["mcp-stdio"]
    }
  }
}
```

Stdio mode connects directly to the database using the credentials configured in Breadbox's environment. No API key is needed, but the `breadbox` binary must be built and accessible.

### Claude Code Configuration

Claude Code reads `.mcp.json` from the project root (or `~/.config/claude/mcp.json` for user-level config). The format mirrors Claude Desktop.

**Streamable HTTP:**

```json
{
  "mcpServers": {
    "breadbox": {
      "type": "http",
      "url": "http://localhost:8080/mcp",
      "headers": {
        "X-API-Key": "bb_your_api_key_here"
      }
    }
  }
}
```

**Stdio:**

```json
{
  "mcpServers": {
    "breadbox": {
      "type": "stdio",
      "command": "breadbox",
      "args": ["mcp-stdio"]
    }
  }
}
```

### Connection URL Summary

| Deployment | Transport | URL or Command |
|---|---|---|
| Docker Compose (local) | HTTP | `http://localhost:8080/mcp` |
| Remote server | HTTP | `https://your-domain.com/mcp` |
| Local binary | Stdio | `breadbox mcp-stdio` |

---

## 3. Tool Definitions

All tools are registered on the MCP server using the generic `mcp.AddTool` function, which infers the JSON schema from the input struct's type and struct tags.

Tool handlers return text content containing a JSON-encoded payload. The LLM receives this as a string it can parse or reason over directly.

### Common Output Envelope

Every tool returns a JSON object as text content. On success:

```json
{ "data": [ ... ] }
```

or for scalar results:

```json
{ "count": 1523 }
{ "message": "Sync triggered for all connections." }
```

On error, the MCP SDK's `IsError: true` flag is set and the content is:

```json
{ "error": "human-readable error description" }
```

---

### Tool: `list_accounts`

**Description (shown to LLM):** List all bank accounts with their current balances. Optionally filter by family member. Returns one object per account including institution, type, and balance.

#### Input Schema

| Parameter | Type | Required | Description |
|---|---|---|---|
| `user_id` | string | No | Filter accounts owned by this family member ID. Omit to return all accounts. |

#### Example Input

```json
{}
```

```json
{ "user_id": "usr_abc123" }
```

#### Output Format

Returns a JSON object with a `data` array. Each element represents one account.

```json
{
  "data": [
    {
      "id": "acc_xyz789",
      "name": "Chase Checking",
      "type": "depository",
      "subtype": "checking",
      "mask": "4321",
      "balance_current": 2450.18,
      "balance_available": 2380.00,
      "balance_limit": null,
      "iso_currency_code": "USD",
      "institution_name": "Chase",
      "user_id": "usr_abc123",
      "user_name": "Alice"
    },
    {
      "id": "acc_def456",
      "name": "Amex Gold",
      "type": "credit",
      "subtype": "credit card",
      "mask": "1008",
      "balance_current": -541.20,
      "balance_available": null,
      "balance_limit": 10000.00,
      "iso_currency_code": "USD",
      "institution_name": "American Express",
      "user_id": "usr_abc123",
      "user_name": "Alice"
    }
  ]
}
```

**Note on credit card balances:** Plaid returns credit card balances as positive numbers representing the amount owed. Breadbox stores them as-is. The LLM should understand that a positive `balance_current` on a credit account means money owed, not money available.

#### Mapping to REST API

Calls `GET /api/v1/accounts` with the optional `user_id` query parameter. Returns all results without pagination (account counts are expected to remain small — tens, not thousands).

**Note on field names:** The MCP output uses `iso_currency_code` (matching the REST API and database column name) and `institution_name` (matching the REST API account schema). The `user_name` field is derived from the connection's associated user record.

#### Edge Cases

- **No accounts:** Returns `{ "data": [] }`. The LLM should inform the user that no accounts are connected yet.
- **Unknown `user_id`:** Returns `{ "data": [] }`. The tool does not error on unrecognized IDs.
- **Multiple currencies:** Accounts with different `iso_currency_code` values are returned together. The LLM must not sum balances across currencies.

---

### Tool: `query_transactions`

**Description (shown to LLM):** Search and filter transactions. Supports date ranges, account, category, amount bounds, free-text search, and pending status. Results are cursor-paginated — default 100 per page, maximum 500. Use `count_transactions` first to understand result size before querying.

#### Input Schema

| Parameter | Type | Required | Description |
|---|---|---|---|
| `start_date` | string (YYYY-MM-DD) | No | Include transactions on or after this date. |
| `end_date` | string (YYYY-MM-DD) | No | Include transactions on or before this date. |
| `account_id` | string | No | Filter to a specific account. |
| `user_id` | string | No | Filter to accounts owned by this family member. |
| `category` | string | No | Filter by primary category (e.g., `FOOD_AND_DRINK`, `TRANSPORTATION`). Case-insensitive. |
| `category_detailed` | string | No | Filter by detailed subcategory (e.g., `FOOD_AND_DRINK_GROCERIES`). Case-sensitive. |
| `min_amount` | number \| null | No | Minimum transaction amount (absolute value, USD). Uses `*float64` internally — zero is a valid filter value. |
| `max_amount` | number \| null | No | Maximum transaction amount (absolute value, USD). Uses `*float64` internally — zero is a valid filter value. |
| `pending` | boolean | No | `true` returns only pending transactions. `false` returns only posted. Omit to return both. |
| `search` | string | No | Full-text search over merchant name and description. |
| `sort_by` | string | No | Sort field: `date` (default), `amount`, or `name`. Cursor pagination only works with `date` sort. |
| `sort_order` | string | No | Sort direction: `desc` (default) or `asc`. |
| `limit` | integer | No | Number of results per page. Default: 100. Maximum: 500. |
| `cursor` | string | No | Pagination cursor returned by a previous call. Omit for the first page. |

#### Example Input

```json
{
  "start_date": "2025-01-01",
  "end_date": "2025-01-31",
  "category": "FOOD_AND_DRINK",
  "limit": 100
}
```

```json
{
  "start_date": "2025-01-01",
  "end_date": "2025-01-31",
  "category": "FOOD_AND_DRINK",
  "cursor": "dHhuXzk4NzY1NA==",
  "limit": 100
}
```

#### Output Format

```json
{
  "data": [
    {
      "id": "txn_111222333",
      "account_id": "acc_xyz789",
      "account_name": "Chase Checking",
      "user_id": "usr_abc123",
      "user_name": "Alice",
      "amount": 14.50,
      "iso_currency_code": "USD",
      "date": "2025-01-15",
      "authorized_date": "2025-01-14",
      "merchant_name": "Blue Bottle Coffee",
      "name": "BLUE BOTTLE COFFEE #12",
      "category_primary": "FOOD_AND_DRINK",
      "category_detailed": "FOOD_AND_DRINK_COFFEE",
      "payment_channel": "in store",
      "pending": false
    }
  ],
  "next_cursor": "dHhuXzExMTIyMzM0",
  "has_more": true
}
```

When there are no more pages:

```json
{
  "data": [ ... ],
  "next_cursor": null,
  "has_more": false
}
```

**Note on amounts:** Amounts are positive for debits (money leaving the account) and negative for credits (money entering). This follows Plaid's convention. The LLM should apply this interpretation when summarizing spending.

#### Token Budget Concern

Each transaction object is approximately 50 tokens of JSON. At the default page size of 100, a single call returns roughly **5,000 tokens of content** before any surrounding context. At the maximum page size of 500, a single call can return **25,000 tokens**.

The LLM should:
1. Call `count_transactions` first when the result size is unknown.
2. Apply date, category, and other filters to narrow results before paginating.
3. Prefer smaller `limit` values (25–50) when only a sample or aggregate is needed.
4. Only paginate through all results when the task genuinely requires complete data.

#### Mapping to REST API

Calls `GET /api/v1/transactions` with all provided filters as query parameters. The cursor is opaque to the caller — the service layer decodes it to a transaction ID used in keyset pagination.

#### Edge Cases

- **No results:** Returns `{ "data": [], "next_cursor": null, "has_more": false }`.
- **`limit` above 500:** Clamped to 500 by the service layer. The tool does not error.
- **Invalid `cursor`:** Returns an error with `IsError: true`. The LLM should restart pagination from the beginning.
- **Invalid date format:** Returns an error describing the expected format.
- **`start_date` after `end_date`:** Returns an error.

---

### Tool: `count_transactions`

**Description (shown to LLM):** Count matching transactions without returning any transaction data. Accepts the same filters as `query_transactions`. Use this before calling `query_transactions` to understand how many results to expect and decide whether to add more filters or paginate.

#### Input Schema

Same as `query_transactions` **minus** `cursor` and `limit`. All parameters are optional.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `start_date` | string (YYYY-MM-DD) | No | Count transactions on or after this date. |
| `end_date` | string (YYYY-MM-DD) | No | Count transactions on or before this date. |
| `account_id` | string | No | Filter to a specific account. |
| `user_id` | string | No | Filter to accounts owned by this family member. |
| `category` | string | No | Filter by primary category. |
| `category_detailed` | string | No | Filter by detailed subcategory. |
| `min_amount` | number \| null | No | Minimum transaction amount. Zero is a valid filter value. |
| `max_amount` | number \| null | No | Maximum transaction amount. Zero is a valid filter value. |
| `pending` | boolean | No | Filter by pending status. |
| `search` | string | No | Full-text search over merchant name and description. |

#### Example Input

```json
{
  "start_date": "2025-01-01",
  "end_date": "2025-01-31"
}
```

#### Output Format

```json
{ "count": 347 }
```

#### Mapping to REST API

Calls `GET /api/v1/transactions/count` with the same filter query parameters supported by `query_transactions`. The endpoint executes a `COUNT(*)` query rather than selecting rows, which is efficient even for large datasets — it does not load transaction data into memory.

#### Recommended Agent Workflow

```
1. count_transactions({ start_date, end_date }) → { count: 347 }
   - If count < 200: proceed to query_transactions directly
   - If count >= 200: add more filters (category, account_id, etc.) and re-count
   - If still large: query_transactions with pagination, processing page by page

2. query_transactions({ start_date, end_date, category: "FOOD_AND_DRINK" })
   → { data: [...100 txns], next_cursor: "...", has_more: true }

3. query_transactions({ ..., cursor: "..." })
   → { data: [...47 txns], next_cursor: null, has_more: false }
```

#### Edge Cases

- **No matching transactions:** Returns `{ "count": 0 }`.
- **Invalid parameters:** Returns an error in the same format as `query_transactions`.

---

### Tool: `list_users`

**Description (shown to LLM):** List all family members tracked in Breadbox. Users are labels for account ownership — they are not login accounts. Use the returned IDs to filter `list_accounts` or `query_transactions` by family member.

#### Input Schema

No parameters. This tool takes an empty input.

#### Example Input

```json
{}
```

#### Output Format

```json
{
  "data": [
    {
      "id": "usr_abc123",
      "name": "Alice",
      "email": "alice@example.com"
    },
    {
      "id": "usr_def456",
      "name": "Bob",
      "email": "bob@example.com"
    }
  ]
}
```

#### Mapping to REST API

Calls `GET /api/v1/users`. Returns all users without pagination (family sizes are small by design).

#### Edge Cases

- **No users configured:** Returns `{ "data": [] }`. This indicates the setup wizard has not been fully completed or no family members have been added.

---

### Tool: `get_sync_status`

**Description (shown to LLM):** Returns the health status of all bank connections — whether they are syncing successfully, when they last synced, and whether any connections need re-authentication. Use this to diagnose why transactions might be missing or stale.

#### Input Schema

No parameters.

#### Example Input

```json
{}
```

#### Output Format

```json
{
  "data": [
    {
      "id": "conn_aaa111",
      "institution_name": "Chase",
      "provider": "plaid",
      "status": "active",
      "last_synced_at": "2025-01-20T14:32:00Z",
      "error_message": null,
      "accounts_count": 3
    },
    {
      "id": "conn_bbb222",
      "institution_name": "Bank of America",
      "provider": "plaid",
      "status": "error",
      "last_synced_at": "2025-01-18T09:15:00Z",
      "error_message": "ITEM_LOGIN_REQUIRED: User must re-authenticate via Plaid Link.",
      "accounts_count": 2
    }
  ]
}
```

**Status values:**

| Status | Meaning |
|---|---|
| `active` | Connection is healthy and syncing normally. |
| `error` | Last sync failed. Check `error_message` for details. |
| `pending_reauth` | Connection requires user re-authentication (e.g., bank login changed). |
| `disconnected` | Connection has been disconnected or removed by the user. |

#### Mapping to REST API

Calls `GET /api/v1/connections`. Each element in the response corresponds to one bank connection (Plaid Item).

#### Edge Cases

- **No connections:** Returns `{ "data": [] }`. No bank accounts have been linked yet.
- **`error_message` is null:** The connection is healthy or has never errored.
- **Stale `last_synced_at`:** If `last_synced_at` is more than 24 hours ago and status is `active`, the sync schedule may be misconfigured or the process restarted recently.

---

### Tool: `trigger_sync`

**Description (shown to LLM):** Manually trigger a data sync to fetch the latest transactions and balances from the bank. Syncs all connections by default, or a specific connection if `connection_id` is provided. The sync runs asynchronously — this tool returns immediately after enqueuing the sync, not after it completes. Check `get_sync_status` afterward to monitor progress.

#### Input Schema

| Parameter | Type | Required | Description |
|---|---|---|---|
| `connection_id` | string | No | Sync only this connection. Omit to sync all active connections. |

#### Example Input

```json
{}
```

```json
{ "connection_id": "conn_aaa111" }
```

#### Output Format

```json
{ "message": "Sync triggered for all connections." }
```

```json
{ "connection_id": "conn_aaa111", "message": "Sync triggered for connection conn_aaa111 (Chase)." }
```

#### Mapping to REST API

Calls `POST /api/v1/sync` with an optional `connection_id` body parameter. The REST endpoint enqueues a sync job and returns immediately.

#### Edge Cases

- **Unknown `connection_id`:** Returns an error: `{ "error": "Connection conn_xxx not found." }` with `IsError: true`.
- **Sync already in progress:** The service layer handles deduplication. A second trigger for the same connection while one is running is a no-op; the tool returns a success message indicating the sync was already underway.
- **Connection in `error` state:** The sync is attempted. If re-authentication is required, the sync will fail again and `get_sync_status` will reflect the error.

---

### Tool: `list_categories`

**Description (shown to LLM):** List all distinct transaction category pairs (primary + detailed) that exist in the database. Useful for understanding the category taxonomy before filtering transactions.

#### Input Schema

No parameters required.

#### Output Format

```json
{
  "data": [
    { "primary": "FOOD_AND_DRINK", "detailed": "FOOD_AND_DRINK_GROCERIES" },
    { "primary": "FOOD_AND_DRINK", "detailed": "FOOD_AND_DRINK_RESTAURANTS" },
    { "primary": "TRANSPORTATION", "detailed": "TRANSPORTATION_GAS" }
  ]
}
```

#### Mapping to REST API

Calls `GET /api/v1/categories`. Returns the same `[]CategoryPair` response.

---

## 4. MCP Resources

MCP resources are passive context documents the LLM can read, similar to files. They do not execute queries at call time in the same on-demand way tools do.

All three resources are registered via `s.registerResources()` in `internal/mcp/server.go`, with handlers in `internal/mcp/resources.go`.

| Resource URI | MIME Type | Description |
|---|---|---|
| `breadbox://overview` | `application/json` | Live dataset summary (users, connections, accounts, transactions, spending) |
| `breadbox://review-guidelines` | `text/markdown` | Guidelines for reviewing transactions and creating rules |
| `breadbox://report-format` | `text/markdown` | Report structure templates and formatting guidelines |

### Resource: `breadbox://overview`

Returns a lightweight summary of the household's financial data — users, connections with account counts, accounts by type, transaction counts, date range, 30-day spending summary with top categories, and pending transaction count. This gives an LLM ambient context about the financial state without a round-trip tool call.

Backed by `service.GetOverviewStats()`.

```json
{
  "total_accounts": 5,
  "total_transactions": 2847,
  "date_range": {
    "earliest": "2024-06-15",
    "latest": "2026-03-07"
  },
  "providers": ["plaid", "teller"]
}
```

### Resource: `breadbox://review-guidelines`

Returns guidelines for reviewing transactions and creating transaction rules. Agents should read this before processing any reviews or creating rules.

Content is user-editable via the MCP Settings admin page (`mcp_review_guidelines` in `app_config`). Falls back to `DefaultReviewGuidelines` (a comprehensive constant in `server.go`) when no custom guidelines are saved.

### Resource: `breadbox://report-format`

Returns report structure templates and formatting guidelines. Agents should read this before submitting reports via the `submit_report` tool.

Content is user-editable via the MCP Settings admin page (`mcp_report_format` in `app_config`). Falls back to `DefaultReportFormat` (a comprehensive constant in `server.go`) when no custom format is saved.

---

## 5. Error Handling

### MCP Error Format

When a tool encounters an error, the handler returns a `*mcp.CallToolResult` with `IsError: true` and a single `TextContent` item containing a JSON error object:

```go
return &mcp.CallToolResult{
    IsError: true,
    Content: []mcp.Content{
        mcp.NewTextContent(`{"error": "start_date must be in YYYY-MM-DD format"}`),
    },
}, nil, nil
```

The MCP SDK does not propagate Go errors returned from tool handlers to the LLM as tool errors — they become protocol-level errors that the client reports differently. All user-facing error conditions should be encoded in the result content with `IsError: true`, not returned as Go errors from the handler.

Go errors from the handler (second return value) should be reserved for truly unexpected failures (e.g., database connection lost) where the tool cannot construct a meaningful response.

### Common Errors

| Condition | `IsError` | Error Message |
|---|---|---|
| Invalid date format | `true` | `"start_date must be in YYYY-MM-DD format"` |
| `start_date` after `end_date` | `true` | `"start_date must be before end_date"` |
| Unknown `connection_id` | `true` | `"Connection {id} not found"` |
| Invalid `cursor` | `true` | `"Invalid or expired pagination cursor"` |
| Database unavailable | Go error | (Protocol-level error, not tool content) |

### How the LLM Should Interpret Errors

When `IsError` is true, the LLM should:
1. Read the `error` field from the JSON content.
2. Report the issue to the user in plain language.
3. Suggest corrective action if one is obvious (e.g., "try a different date range").
4. **Not retry automatically** unless the error message explicitly indicates a transient condition.

The LLM should not attempt to parse or act on protocol-level errors (connection refused, timeout). These indicate infrastructure problems that require human intervention.

---

## 6. Pagination Strategy

### Why Pagination Is Necessary

A household with one year of transaction history across five accounts may have 2,000–5,000 transactions. Returning all of them in a single tool call would:
- Consume 100,000–250,000 tokens of context window
- Exceed the content limits of most LLM APIs
- Provide far more data than most agent tasks require

Cursor-based pagination solves this by letting the agent retrieve exactly as many transactions as needed for the current task, in sequence.

### Cursor Mechanics

The cursor is an opaque base64-encoded string that encodes the ID of the last transaction returned. The service layer decodes it to perform keyset pagination (`WHERE id > $last_id ORDER BY date DESC, id DESC`). Agents must treat the cursor as an opaque token — do not construct or modify cursor values.

Cursors are not time-limited in the MVP, but they may become invalid if underlying data changes significantly (rows deleted). An invalid cursor returns an error; the agent should restart from page one.

### Page Size Guidance

| Use Case | Recommended `limit` |
|---|---|
| Sampling or spot-checking | 25 |
| Single-month analysis | 100 (default) |
| Full dataset processing | 200–500 |
| Maximum allowed | 500 |

### Recommended Agent Workflow

The standard pattern for any transaction-based task:

```
Step 1: Establish scope
  → count_transactions({ start_date: "2025-01-01", end_date: "2025-01-31" })
  ← { count: 347 }

Step 2: Evaluate
  - count < 200 → query directly, one or two pages
  - count >= 200 → narrow filters, or plan multi-page processing

Step 3a: Narrow filters (if count is large)
  → count_transactions({ start_date: "2025-01-01", end_date: "2025-01-31",
                          category: "FOOD_AND_DRINK" })
  ← { count: 52 }
  → query_transactions({ start_date: "2025-01-01", end_date: "2025-01-31",
                          category: "FOOD_AND_DRINK" })
  ← { data: [...52 txns], has_more: false }

Step 3b: Paginate (if full dataset needed)
  → query_transactions({ start_date: "2025-01-01", end_date: "2025-01-31",
                          limit: 200 })
  ← { data: [...200 txns], next_cursor: "abc...", has_more: true }
  → query_transactions({ start_date: "2025-01-01", end_date: "2025-01-31",
                          limit: 200, cursor: "abc..." })
  ← { data: [...147 txns], next_cursor: null, has_more: false }
```

### Why Not Offset Pagination

Offset pagination (`OFFSET N LIMIT M`) degrades at scale — the database must scan and discard N rows on every request. Keyset (cursor) pagination uses an index seek directly to the continuation point, maintaining consistent performance regardless of how deep into the result set the agent is.

---

## 7. Relationship to REST API

### Single Service Layer

MCP tools are not an independent data access mechanism. They are wrappers around the same service functions and sqlc-generated queries that back the REST API endpoints:

```
REST:   GET /api/v1/transactions?start_date=...
           └─→ service.QueryTransactions(ctx, pool, filters)
                    └─→ db.ListTransactions(ctx, sqlc params)

MCP:    query_transactions tool handler
           └─→ service.QueryTransactions(ctx, pool, filters)
                    └─→ db.ListTransactions(ctx, sqlc params)
```

Both paths call the same `service.QueryTransactions` function. The MCP tool handler is responsible only for:
1. Unmarshaling and validating the tool input struct
2. Calling the service function
3. Marshaling the result to JSON text content

### Benefits

**Single source of truth.** Business logic (filter validation, currency handling, soft-delete exclusion, pending→posted linking) lives in the service layer. It cannot drift between REST and MCP.

**Independent testability.** The REST API can be fully exercised with `curl` or any HTTP client. MCP behavior follows automatically — if the REST API returns correct data, MCP does too.

**Simpler MCP tools.** Tool handlers stay thin. They do not contain SQL, pagination logic, or complex data transformations. This makes the MCP layer easy to audit and maintain.

### Mapping Table

| MCP Tool | REST Endpoint |
|---|---|
| `list_accounts` | `GET /api/v1/accounts` |
| `query_transactions` | `GET /api/v1/transactions` |
| `count_transactions` | `GET /api/v1/transactions/count` |
| `list_users` | `GET /api/v1/users` |
| `get_sync_status` | `GET /api/v1/connections` |
| `trigger_sync` | `POST /api/v1/sync` |
| `list_categories` | `GET /api/v1/categories` |

### No MCP-Specific Data Access

There are no database queries written exclusively for MCP. If a query is needed that does not already exist in the service layer, it must be added there first — making it available to both REST and MCP — rather than written directly in the MCP tool handler.

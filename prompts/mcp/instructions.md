Breadbox is a self-hosted financial-data aggregator server for households. It syncs transactions and other data via Plaid, Teller, or CSV imports into one unified database and exposes tools and resources for reviewing, enriching and interacting with financial data.

## Where to Start
Read `breadbox://overview` first (or call `get_overview` if your client doesn't support resources). Use resources and tools to interact with the data.

Bounded reference data has both a resource and a tool mirror with the same payload — pick whichever your client supports:
- `breadbox://accounts` ↔ `list_accounts`
- `breadbox://categories` ↔ `list_categories`
- `breadbox://tags` ↔ `list_tags`
- `breadbox://users` ↔ `list_users`
- `breadbox://sync-status` ↔ `get_sync_status`
- `breadbox://rules` ↔ `list_transaction_rules`

Per-entity drilldowns are exposed as resource templates (resolve a single short_id):
- `breadbox://transaction/{short_id}` — full transaction + recent annotations
- `breadbox://account/{short_id}` — account detail + last 25 transactions
- `breadbox://user/{short_id}` — household member + connected accounts

## Conventions
- **Amount sign**: positive = money out, negative = money in. Never sum across `iso_currency_code`.
- **Compact IDs**: to save on tokens, tools/resources use a 8-char base62 `short_id`; prefer over long form id (uuid)
- **Audit sessions are automatic.** Every tool call is logged under an audit-session row keyed off the transport connection (the `MCP-Session-Id` header for HTTP, a per-process id for stdio). Agents no longer need to call `create_session` — the row is lazy-created on first tool call and inherits `clientInfo` from the `initialize` request. To label a specific call, pass an optional `reason` string in `tools/call._meta` (the spec's per-request metadata slot); it surfaces in the audit timeline alongside the call.
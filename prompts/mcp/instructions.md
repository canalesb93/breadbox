Breadbox is a self-hosted financial-data aggregator server for households. It syncs transactions and other data via Plaid, Teller, or CSV imports into one unified database and exposes tools and resources for reviewing, enriching and interacting with financial data.

## Where to Start
Read `get_reference(kind=overview)` first (or the `breadbox://overview` resource if your client supports resources) for a household snapshot, then use the tools and resources to interact with the data.

Bounded reference data is read through one tool — `get_reference(kind=…)`:
- `get_reference(kind=overview)` — household snapshot (scope, freshness, review backlog)
- `get_reference(kind=accounts)` — bank accounts (optional `user_id` filter)
- `get_reference(kind=categories)` — the category taxonomy (source of `category_slug` values)
- `get_reference(kind=tags)` — the tag vocabulary
- `get_reference(kind=users)` — household members
- `get_reference(kind=sync_status)` — per-connection sync status / freshness
- `get_reference(kind=rules)` — the transaction-rule roster (lean; `fields=all` for full)

A couple of these are also exposed as resources for clients with a resource/attach UI — `breadbox://overview` and `breadbox://sync-status` — with the same payload as the matching `get_reference` kind.

Per-entity drilldowns are exposed as resource templates (resolve a single short_id):
- `breadbox://transaction/{short_id}` — full transaction + recent annotations
- `breadbox://account/{short_id}` — account detail + last 25 transactions
- `breadbox://user/{short_id}` — household member + connected accounts

## Conventions
- **Amount sign**: positive = money out, negative = money in. Never sum across `iso_currency_code`.
- **Compact IDs**: to save on tokens, tools/resources use a 8-char base62 `short_id`; prefer over long form id (uuid)
- **Audit sessions are automatic.** Every tool call is logged under an audit-session row keyed off the transport connection (the `MCP-Session-Id` header for HTTP, a per-process id for stdio). Agents no longer need to call `create_session` — the row is lazy-created on first tool call and inherits `clientInfo` from the `initialize` request. To label a specific call, pass an optional `reason` string in `tools/call._meta` (the spec's per-request metadata slot); it surfaces in the audit timeline alongside the call.
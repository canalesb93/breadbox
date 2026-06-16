Breadbox is a self-hosted financial-data aggregator server for households. It syncs transactions and other data via Plaid, Teller, or CSV imports into one unified database and exposes tools for reviewing, enriching and interacting with financial data. (Everything is a tool — there are no MCP resources.)

## Where to Start
Call `get_overview` first for a household snapshot (scope, freshness, pending-review backlog), then use the tools below to interact with the data.

Bounded reference data — each its own tool:
- `get_overview` — household snapshot
- `list_accounts` — bank accounts (optional `user_id` filter)
- `list_categories` — the category taxonomy (source of `category_slug` values)
- `list_tags` — the tag vocabulary
- `list_users` — household members
- `get_sync_status` — per-connection sync status / freshness
- `list_transaction_rules` — the transaction-rule roster (lean; `fields=all` for full)

Operating-guidance docs — read via `get_reference(kind=…)` when you need them:
- `get_reference(kind=instructions)` — this document
- `get_reference(kind=rule-dsl)` — the transaction-rule condition grammar (read before authoring rules)
- `get_reference(kind=review-guidelines)` — how to work the needs-review queue
- `get_reference(kind=report-format)` — structure/format for `submit_report`

## Conventions
- **Amount sign**: positive = money out, negative = money in. Never sum across `iso_currency_code`.
- **Compact IDs**: to save on tokens, tools use a 8-char base62 `short_id`; prefer over long form id (uuid)
- **Audit sessions are automatic.** Every tool call is logged under an audit-session row keyed off the transport connection (the `MCP-Session-Id` header for HTTP, a per-process id for stdio). Agents no longer need to call `create_session` — the row is lazy-created on first tool call and inherits `clientInfo` from the `initialize` request. To label a specific call, pass an optional `reason` string in `tools/call._meta` (the spec's per-request metadata slot); it surfaces in the audit timeline alongside the call.
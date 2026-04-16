# MCP Tools Reference

Complete reference for all MCP tools available in Breadbox. Tools are the primary way AI agents interact with your financial data.

## Tool Access

Tools are classified as **Read** or **Write**:

- **Read tools** are always available
- **Write tools** require the MCP mode to be set to `read_write` (configurable in the admin dashboard under MCP Settings)
- Individual tools can be disabled from the admin dashboard

## Sessions

Before using any tools, agents should call `create_session` to establish a session. This provides the agent with dataset context and server instructions.

---

## Query & Analysis Tools

### query_transactions (Read)

Query transactions with composable filters and cursor pagination.

| Parameter | Type | Description |
|-----------|------|-------------|
| `account_id` | string | Filter by account ID |
| `user_id` | string | Filter by user ID (includes attributed transactions) |
| `category_slug` | string | Filter by category (e.g., `food_and_drink_dining`) |
| `start_date` | string | Start date (YYYY-MM-DD) |
| `end_date` | string | End date (YYYY-MM-DD) |
| `min_amount` | float | Minimum amount (positive = debits) |
| `max_amount` | float | Maximum amount |
| `search` | string | Search name/merchant. Comma-separated for OR. |
| `exclude_search` | string | Exclude matching name/merchant |
| `search_mode` | string | `contains` (default), `words`, `fuzzy` |
| `pending` | bool | Filter by pending status |
| `tags` | array | AND filter — must have every slug. Use `["needs-review"]` to fetch the review backlog. |
| `any_tag` | array | OR filter — must have at least one slug. |
| `sort_by` | string | `date` (default), `amount`, `name` |
| `sort_order` | string | `desc` (default), `asc` |
| `fields` | string | Field selection. Aliases: `minimal`, `core`, `category`, `timestamps` |
| `cursor` | string | Pagination cursor |
| `limit` | int | Results per page (default 50, max 500) |

### count_transactions (Read)

Count transactions matching filters. Same filter parameters as `query_transactions`. Returns count only — use before paginating large result sets.

### transaction_summary (Read)

Aggregated spending totals. Default date range: 30 days.

| Parameter | Type | Description |
|-----------|------|-------------|
| `group_by` | string | `category`, `month`, `week`, `day`, `category_month` |
| `start_date` | string | Start date |
| `end_date` | string | End date |
| `account_id` | string | Filter by account |
| `user_id` | string | Filter by user |
| `category_slug` | string | Filter by category |

### merchant_summary (Read)

Merchant-level statistics. Default date range: 90 days.

| Parameter | Type | Description |
|-----------|------|-------------|
| `min_count` | int | Minimum transaction count (use 2 for recurring, 3 for subscriptions) |
| `spending_only` | bool | Exclude credits/refunds |
| `search` | string | Search merchant names |
| `exclude_search` | string | Exclude matching merchants |

Plus same date/account/user filters as `transaction_summary`.

---

## Account & Status Tools

### list_accounts (Read)

List all bank accounts across all connections. Returns account name, type, balances, connection info.

### list_users (Read)

List all family members in the household.

### get_sync_status (Read)

Get sync status for all connections — last sync time, status, errors.

### list_categories (Read)

List all categories in the 2-level hierarchy. Returns slug, display name, parent, icon, color.

### export_categories (Read)

Export the full category tree as TSV. Useful for backup or transfer.

---

## Categorization Tools

### categorize_transaction (Write)

Set a transaction's category. Creates a category override.

| Parameter | Type | Description |
|-----------|------|-------------|
| `transaction_id` | string | Transaction ID (8-char short ID or UUID) |
| `category_slug` | string | Category slug (e.g., `food_and_drink_groceries`) |

### reset_transaction_category (Write)

Remove a transaction's category override, reverting to the provider's original category.

| Parameter | Type | Description |
|-----------|------|-------------|
| `transaction_id` | string | Transaction ID |

### batch_categorize_transactions (Write)

Categorize multiple transactions at once. Max 500 items per call.

| Parameter | Type | Description |
|-----------|------|-------------|
| `items` | array | Array of `{ transaction_id, category_slug }` pairs |

### bulk_recategorize (Write)

Server-side UPDATE of all transactions matching filters. Requires at least one filter for safety. Moves transactions matching `from_category` (and other filters) to `to_category`.

| Parameter | Type | Description |
|-----------|------|-------------|
| `to_category` | string | Destination category slug (required) |
| `from_category` | string | Optional source category slug filter |
| Plus filters | | Same as `query_transactions` |
| `target_category_slug` | string | Deprecated — use `to_category` |
| `category_slug` | string | Deprecated — use `from_category` |

### import_categories (Write)

Import categories from TSV format. Creates or updates categories.

---

## Tag & Annotation Tools

### list_tags (Read)

List all registered tags.

### add_transaction_tag (Write)

Attach a tag to a transaction. Auto-creates a persistent tag if the slug is unknown. Idempotent.

| Parameter | Type | Description |
|-----------|------|-------------|
| `transaction_id` | string | UUID or short ID |
| `tag_slug` | string | Tag slug |
| `note` | string | Optional rationale (recorded on the `tag_added` annotation) |

### remove_transaction_tag (Write)

Remove a tag from a transaction. For ephemeral tags (`needs-review`), the `note` is REQUIRED.

| Parameter | Type | Description |
|-----------|------|-------------|
| `transaction_id` | string | UUID or short ID |
| `tag_slug` | string | Tag slug |
| `note` | string | Required for ephemeral tags. Optional for persistent. |

### update_transactions (Write)

Compound batch write — set category, add tags, remove tags, and attach a comment per transaction in a single atomic call. Max 50 operations per request.

| Parameter | Type | Description |
|-----------|------|-------------|
| `operations` | array | Array of operations (see below). |
| `on_error` | string | `continue` (default — per-op tx) or `abort` (single tx, rolls back on first error). |

Each operation:

| Field | Type | Description |
|-------|------|-------------|
| `transaction_id` | string | UUID or short ID. Required. |
| `category_slug` | string | Optional category to set. Sets `category_override=true`. |
| `tags_to_add` | array | `[{slug, note?}, ...]`. Auto-creates persistent tags. |
| `tags_to_remove` | array | `[{slug, note?}, ...]`. `note` REQUIRED when the tag is ephemeral. |
| `comment` | string | Optional comment annotation. |

Use this to close a review entry: `set category + remove needs-review (with note) + comment` in one call.

### list_annotations (Read)

Return the activity timeline for a transaction. Each row is one of: `comment`, `tag_added`, `tag_removed`, `rule_applied`, `category_set`.

### create_tag / update_tag / delete_tag (Write)

Admin-only tag CRUD. Agents typically don't need these — `add_transaction_tag` auto-creates persistent tags. Use these to set display name, color, lifecycle, or cascade-delete a tag.

---

## Transaction Rules Tools

### create_transaction_rule (Write)

Create a rule that auto-categorizes future transactions during sync.

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Rule name |
| `conditions` | object | Recursive condition tree (see below) |
| `category_slug` | string | Category to assign when matched |
| `priority` | int | Higher priority wins (default 0) |
| `enabled` | bool | Default true |
| `expires_in` | string | Optional duration (e.g., `24h`, `30d`, `1w`) |
| `apply_retroactively` | bool | Apply to existing transactions |

### list_transaction_rules (Read)

List all rules with optional search filter.

| Parameter | Type | Description |
|-----------|------|-------------|
| `search` | string | Search rule names |
| `category_slug` | string | Filter by target category |
| `enabled` | bool | Filter by enabled status |

### update_transaction_rule (Write)

Update an existing rule's conditions, category, priority, or enabled status.

### delete_transaction_rule (Write)

Delete a rule by ID.

### apply_rules (Write)

Apply rules retroactively to existing transactions. Optionally specify a single rule ID, or apply all active rules.

| Parameter | Type | Description |
|-----------|------|-------------|
| `rule_id` | string | Optional — apply a specific rule. Omit for all active rules. |

### preview_rule (Read)

Dry-run a condition against existing transactions. Returns match count and sample matches without making changes.

| Parameter | Type | Description |
|-----------|------|-------------|
| `conditions` | object | Condition tree to test |
| `limit` | int | Max sample matches to return |

### batch_create_rules (Write)

Create multiple rules at once.

### Condition Structure

Rules use a recursive JSON condition tree:

```json
{
  "type": "and",
  "conditions": [
    { "field": "name", "operator": "contains", "value": "AMAZON" },
    { "field": "amount", "operator": "gt", "value": 50 }
  ]
}
```

**Logical operators:** `and`, `or`, `not` (wraps a single condition)

**Available fields:** `name`, `merchant_name`, `amount`, `category_primary`, `category_detailed`, `pending`, `provider`, `account_id`, `user_id`, `user_name`

**String operators:** `eq`, `neq`, `contains`, `not_contains`, `matches` (regex), `in`

**Numeric operators:** `eq`, `neq`, `gt`, `gte`, `lt`, `lte`

**Boolean operators:** `eq`, `neq`

---

## Account Link Tools

Account links connect authorized user cards to primary cardholder accounts for transaction deduplication.

### list_account_links (Read)

List all account links with match statistics.

### create_account_link (Write)

Create a link between a dependent and primary account. Auto-runs initial reconciliation.

| Parameter | Type | Description |
|-----------|------|-------------|
| `dependent_account_id` | string | The authorized user's account |
| `primary_account_id` | string | The cardholder's account |

### delete_account_link (Write)

Delete an account link by ID.

### reconcile_account_link (Write)

Re-run matching for a specific link.

### list_transaction_matches (Read)

List matched transaction pairs for a link.

| Parameter | Type | Description |
|-----------|------|-------------|
| `account_link_id` | string | Account link ID |

### confirm_match (Write)

Confirm a matched transaction pair is correct.

### reject_match (Write)

Reject a matched transaction pair.

---

## Comment & Report Tools

### add_transaction_comment (Write)

Add a comment to a transaction.

| Parameter | Type | Description |
|-----------|------|-------------|
| `transaction_id` | string | Transaction ID |
| `body` | string | Comment text |

### list_transaction_comments (Read)

List comments on a transaction.

### submit_report (Write)

Submit a report for human review. Reports appear on the admin dashboard.

| Parameter | Type | Description |
|-----------|------|-------------|
| `title` | string | Report title |
| `body` | string | Markdown body. Use `[Name](/transactions/ID)` for deep links. |

---

## Sync & Session Tools

### create_session (Write)

Establish a session. Returns dataset context (users, accounts, connection status, pending transaction count) and server instructions. Call this first.

### trigger_sync (Write)

Trigger a manual sync for all active connections.

---

## Resources

In addition to tools, Breadbox exposes three MCP resources that provide passive context:

| URI | Description |
|-----|-------------|
| `breadbox://overview` | Live dataset summary (users, accounts, spending, pending transactions) |
| `breadbox://review-guidelines` | Guidelines for reviewing transactions and creating rules |
| `breadbox://report-format` | Report structure templates and formatting guidelines |

Agents should read `breadbox://overview` at the start of a session for ambient context about the household's financial data.

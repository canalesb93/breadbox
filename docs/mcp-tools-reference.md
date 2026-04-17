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

> **Full DSL specification**: see **[`docs/rule-dsl.md`](rule-dsl.md)** for the complete condition grammar, action semantics, trigger matrix, pipeline-stage (priority) ordering, sync-vs-retroactive differences, and the chaining model that lets later-stage rules observe earlier-stage rules' tag/category mutations.

The tools below are thin API skins over the DSL — the DSL doc is the source of truth for what a rule *means*.

### create_transaction_rule (Write)

Create a rule that fires during sync. Actions compose (`set_category` + `add_tag` + `add_comment` in a single rule are all valid).

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Human-readable rule name |
| `conditions` | object | Condition tree. Omit or `{}` for match-all. Supports `and` / `or` / `not` nesting up to depth 10. |
| `actions` | array | Typed actions: `set_category`, `add_tag`, `remove_tag`, `add_comment`. Either this or `category_slug` is required. |
| `category_slug` | string | Shorthand for `actions=[{type:set_category,category_slug:...}]` |
| `trigger` | string | `on_create` (default) / `on_change` / `always`. `on_update` accepted as legacy alias. |
| `stage` | string | **Preferred.** Semantic pipeline stage: `baseline` / `standard` / `refinement` / `override`. Resolves to priority `0 / 10 / 50 / 100`. |
| `priority` | int | Raw pipeline-stage integer, 0–1000. Use for fine-grained slotting within a stage. If both `stage` and `priority` are supplied, `priority` wins. Defaults to `10` (standard) if neither is provided. |
| `enabled` | bool | Default true |
| `expires_in` | string | Optional duration (e.g., `24h`, `30d`, `1w`) |
| `apply_retroactively` | bool | Also back-fill matching existing transactions (materializes `set_category` / `add_tag` / `remove_tag`; `add_comment` is sync-only) |

### list_transaction_rules (Read)

List rules with optional filters. Returns actions, priority, hit_count, last_hit_at.

| Parameter | Type | Description |
|-----------|------|-------------|
| `search` | string | Substring / words / fuzzy search on rule name |
| `category_slug` | string | Filter by target category |
| `enabled` | bool | Filter by enabled status |

### update_transaction_rule (Write)

Every field is optional; omit to leave unchanged. Pass `conditions={}` to explicitly switch to match-all; pass `actions=[...]` to replace the entire action set; pass `expires_at=""` to clear expiry. Pass `stage` (`baseline` | `standard` | `refinement` | `override`) to re-slot a rule into a canonical pipeline stage without guessing a raw `priority`. If both `stage` and `priority` are supplied, `priority` wins.

### delete_transaction_rule (Write)

Delete a rule by ID. System-seeded rules can't be deleted — disable them via update instead.

### apply_rules (Write)

Retroactive apply. Pass `rule_id` for a single rule (no chaining — that rule evaluates in isolation), or omit to run the full active-rule pipeline in priority-ASC order with the same chaining semantics as sync. Materializes `set_category` / `add_tag` / `remove_tag`; skips `add_comment`. Ignores `rule.trigger` (retroactive is a bulk op).

| Parameter | Type | Description |
|-----------|------|-------------|
| `rule_id` | string | Optional — single-rule apply (no chaining). Omit for full pipeline. |

### preview_rule (Read)

Dry-run a condition tree against existing transactions. **Isolation semantics** — evaluates only the supplied condition against stored data; does NOT simulate the full rule pipeline. Use to answer "what does this condition match today" before creating a rule.

| Parameter | Type | Description |
|-----------|------|-------------|
| `conditions` | object | Condition tree to test (same grammar as `create_transaction_rule`) |
| `sample_size` | int | Max sample matches to return (default 10, max 50). `match_count` always reflects the full match set. |

### batch_create_rules (Write)

Create multiple rules in one call. Ideal for composable pipelines — use `stage` (preferred) or raw `priority` on each item to order rules so earlier-stage rules set up tags/categories that later-stage rules react to. `stage` resolves to priority `0 / 10 / 50 / 100`; if both `stage` and `priority` are supplied on an item, `priority` wins. Returns per-item success + errors.

Example pipeline (3 rules that chain):

```json
{
  "rules": [
    {
      "name": "Tag coffee shops",
      "stage": "baseline",
      "conditions": { "field": "merchant_name", "op": "contains", "value": "starbucks" },
      "actions": [ { "type": "add_tag", "tag_slug": "coffee" } ]
    },
    {
      "name": "Categorize coffee-tagged transactions",
      "stage": "standard",
      "conditions": { "field": "tags", "op": "contains", "value": "coffee" },
      "actions": [ { "type": "set_category", "category_slug": "food_and_drink_coffee" } ]
    },
    {
      "name": "Flag expensive coffee",
      "stage": "refinement",
      "conditions": {
        "and": [
          { "field": "tags", "op": "contains", "value": "coffee" },
          { "field": "amount", "op": "gt", "value": 15 }
        ]
      },
      "actions": [ { "type": "add_tag", "tag_slug": "expensive" } ]
    }
  ]
}
```

Rule 2 sees rule 1's `coffee` tag mid-sync; rule 3 sees both the tag and (if we'd conditioned on it) the category rule 2 set. This is the chaining model — see `docs/rule-dsl.md` for the full story.

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

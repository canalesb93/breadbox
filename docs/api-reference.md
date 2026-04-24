# API Quick Reference

Complete list of all REST API endpoints. All endpoints are prefixed with `/api/v1/` unless noted otherwise.

## Authentication

All API endpoints require an API key via the `X-API-Key` header:

```
X-API-Key: bb_your_api_key_here
```

API keys are created from the admin dashboard under **API Keys**. Keys can be scoped as `full_access` or `read_only`. Write endpoints require `full_access` scope.

## Amount Convention

- **Positive** = money out (debits, purchases, fees)
- **Negative** = money in (credits, deposits, refunds)
- Always check `iso_currency_code` -- never sum across different currencies

---

## Health

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/health` | No | Basic liveness check |
| GET | `/health/live` | No | Basic liveness check |
| GET | `/health/ready` | No | Readiness check (verifies DB + scheduler) |
| GET | `/api/v1/version` | No | Current server version and update availability |

## Accounts

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/accounts` | Read | List all accounts |
| GET | `/accounts/{id}` | Read | Get a single account |

## Transactions

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/transactions` | Read | Query transactions with filters and pagination |
| GET | `/transactions/count` | Read | Count transactions matching filters |
| GET | `/transactions/summary` | Read | Aggregated totals by category, month, week, day |
| GET | `/transactions/merchants` | Read | Merchant-level stats (count, total, avg) |
| GET | `/transactions/{id}` | Read | Get a single transaction |
| PATCH | `/transactions/{id}/category` | Write | Set transaction category (override) |
| DELETE | `/transactions/{id}/category` | Write | Reset transaction category to provider default |
| POST | `/transactions/batch-categorize` | Write | Batch categorize multiple transactions (max 500) |
| POST | `/transactions/bulk-recategorize` | Write | Bulk recategorize by filter (server-side UPDATE) |

### Transaction Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `account_id` | string | Filter by account |
| `user_id` | string | Filter by user (includes attributed transactions) |
| `category_slug` | string | Filter by category slug |
| `start_date` | string | Start date (YYYY-MM-DD) |
| `end_date` | string | End date (YYYY-MM-DD) |
| `min_amount` | float | Minimum amount |
| `max_amount` | float | Maximum amount |
| `search` | string | Search name/merchant (comma-separated for OR) |
| `exclude_search` | string | Exclude matching name/merchant (min 2 chars) |
| `search_mode` | string | `contains` (default), `words`, `fuzzy` |
| `pending` | bool | Filter by pending status |
| `tags` | string | Comma-separated slugs; result transactions must carry **every** slug (AND) |
| `any_tag` | string | Comma-separated slugs; result transactions must carry **at least one** slug (OR) |
| `sort_by` | string | `date` (default), `amount`, `provider_name` |
| `sort_order` | string | `desc` (default), `asc` |
| `fields` | string | Field selection. Aliases: `minimal`, `core`, `category`, `timestamps` |
| `cursor` | string | Pagination cursor (only with date sort) |
| `limit` | int | Results per page (default 50, max 500) |

## Categories

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/categories` | Read | List all categories (2-level hierarchy) |
| GET | `/categories/export` | Read | Export categories as TSV |
| GET | `/categories/{id}` | Read | Get a single category |
| POST | `/categories` | Write | Create a category |
| PUT | `/categories/{id}` | Write | Update a category |
| DELETE | `/categories/{id}` | Write | Delete a category |
| POST | `/categories/import` | Write | Import categories from TSV |
| POST | `/categories/{id}/merge` | Write | Merge a category into another |

## Users

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/users` | Read | List all family members |

## Connections

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/connections` | Read | List all bank connections |
| GET | `/connections/{id}/status` | Read | Get connection status and last sync info |

## Sync

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| POST | `/sync` | Write | Trigger manual sync for all connections |

## Transaction Comments

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/transactions/{id}/comments` | Read | List comments on a transaction |
| POST | `/transactions/{id}/comments` | Write | Add a comment to a transaction |
| PUT | `/transactions/{id}/comments/{comment_id}` | Write | Update a comment |
| DELETE | `/transactions/{id}/comments/{comment_id}` | Write | Delete a comment |

## Tags & Reviews

The review queue is a tag. Transactions carrying the seeded `needs-review` tag (or any operator-defined trigger tag) are the backlog. A seeded `on_create` system rule auto-attaches `needs-review` to every newly-synced transaction; disable that rule to opt out. When removing a tag, passing a rationale `note` is optional — if provided, it's recorded on the `tag_removed` annotation.

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/tags` | Read | List all registered tags |
| POST | `/transactions/{id}/tags` | Write | Attach a tag to a transaction (body: `{"slug":"...","note":"..."}`). Auto-creates the tag if the slug is not yet registered. Idempotent — returns `already_present: true` on repeat calls. |
| DELETE | `/transactions/{id}/tags/{slug}` | Write | Detach a tag from a transaction. Optional `?note=...` or JSON body `{"note":"..."}` recorded on the `tag_removed` annotation. Idempotent — returns `already_absent: true` when the tag isn't attached. |

Tag CRUD (create/update/delete tag records themselves) remains on the admin dashboard and MCP (`create_tag`, `update_tag`, `delete_tag`) for now; REST CRUD is tracked as a follow-up. For filtering, pass `tags=slug1,slug2` (AND) or `any_tag=slug1,slug2` (OR) to `/transactions` and `/transactions/count`.

Additional tag-touching operations exposed via MCP: `list_tags`, `add_transaction_tag`, `remove_transaction_tag`, `create_tag`, `update_tag`, `delete_tag`, `update_transactions`, `list_annotations`. The admin dashboard covers the same ground at `/tags`, `/transactions/:id/edit`, and bulk actions on `/transactions`.

## Transaction Rules

Rules auto-categorize transactions during sync by matching conditions on transaction fields.

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/rules` | Read | List all rules with filters |
| GET | `/rules/{id}` | Read | Get a single rule |
| POST | `/rules` | Write | Create a rule |
| PUT | `/rules/{id}` | Write | Update a rule |
| DELETE | `/rules/{id}` | Write | Delete a rule |
| POST | `/rules/{id}/apply` | Write | Apply a single rule retroactively |
| POST | `/rules/apply-all` | Write | Apply all active rules retroactively |
| POST | `/rules/preview` | Write | Dry-run a condition against existing transactions |

### Rule Condition Structure

Rules use a recursive JSON condition tree supporting AND/OR/NOT logic:

```json
{
  "type": "and",
  "conditions": [
    { "field": "provider_name", "operator": "contains", "value": "AMAZON" },
    { "field": "amount", "operator": "gt", "value": 50 }
  ]
}
```

**Available fields:** `provider_name`, `provider_merchant_name`, `amount`, `provider_category_primary`, `provider_category_detailed`, `pending`, `provider`, `account_id`, `user_id`, `user_name`

**String operators:** `eq`, `neq`, `contains`, `not_contains`, `matches` (regex), `in`

**Numeric operators:** `eq`, `neq`, `gt`, `gte`, `lt`, `lte`

**Boolean operators:** `eq`, `neq`

### Pipeline stage (priority)

`POST /rules` and `PUT /rules/{id}` accept either `stage` (semantic) or `priority` (raw integer) on the request body:

| Field | Type | Description |
|-------|------|-------------|
| `stage` | string | `baseline` / `standard` / `refinement` / `override`. Resolves to priority `0 / 10 / 50 / 100`. |
| `priority` | int | Raw pipeline-stage integer, `0..1000`. Lower runs first. |

If both are supplied, `priority` wins. If neither is supplied on create, the rule defaults to `standard` (priority `10`). Stage names are case-insensitive; unknown stage strings return `400 VALIDATION_ERROR`. See [`docs/rule-dsl.md`](rule-dsl.md) for the full priority-as-pipeline-stage model.

## Account Links

Account links connect dependent (authorized user) accounts to primary (cardholder) accounts for cross-connection transaction deduplication.

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/account-links` | Read | List all account links |
| GET | `/account-links/{id}` | Read | Get a single link with match stats |
| GET | `/account-links/{id}/matches` | Read | List matched transaction pairs |
| POST | `/account-links` | Write | Create a link (auto-runs initial reconciliation) |
| PUT | `/account-links/{id}` | Write | Update a link |
| DELETE | `/account-links/{id}` | Write | Delete a link |
| POST | `/account-links/{id}/reconcile` | Write | Re-run matching for a link |
| POST | `/transaction-matches/{id}/confirm` | Write | Confirm a matched pair |
| POST | `/transaction-matches/{id}/reject` | Write | Reject a matched pair |
| POST | `/transaction-matches/manual` | Write | Manually match two transactions |

## Agent Reports

AI agents can submit summaries and flag transactions for human review.

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/reports` | Read | List all reports (bare array, bounded) |
| GET | `/reports/unread-count` | Read | Count of unread reports |
| POST | `/reports` | Write | Submit a report |
| PATCH | `/reports/{id}/read` | Write | Mark a report as read |

### `POST /reports` body

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `title` | string | Yes | Short headline shown in the reports list. |
| `body` | string | Yes | Markdown body. May include transaction references, tables, etc. |
| `priority` | string | No | `low`, `normal` (default), `high`. Surfaced in the admin list view. |
| `tags` | string[] | No | Free-form labels for filtering (e.g., `["monthly-summary", "groceries"]`). |
| `author` | string | No | Override display name for the report author. Defaults to the authenticated actor (API key name). |

### `POST /rules/apply-all` response

```json
{
  "rules_applied": { "<rule_id>": 17, "<rule_id>": 4 },
  "total_affected": 21
}
```

`rules_applied` is an object keyed by rule ID, mapping to the number of transactions updated by that rule in this retroactive pass. Order is not guaranteed. `total_affected` is the sum across all rules.

## OAuth / MCP Auth

OAuth 2.1 endpoints for MCP client authentication:

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/.well-known/oauth-authorization-server` | No | OAuth server metadata |
| GET | `/.well-known/oauth-protected-resource` | No | Protected resource metadata |
| POST | `/oauth/token` | No | Token exchange |
| POST | `/oauth/register` | No | Dynamic client registration |

## List Response Envelopes

Two shapes, picked per-resource:

- **Bounded resources** (small by design: accounts, users, connections, categories, reports) return a **bare JSON array**:

  ```json
  [ { "id": "...", ... }, { "id": "...", ... } ]
  ```

- **Paginated resources** (transactions, rules) return a **resource-keyed envelope** with cursor pagination:

  ```json
  { "transactions": [...], "next_cursor": "eyJ...", "has_more": true, "limit": 50 }
  ```

  ```json
  { "rules": [...], "next_cursor": "eyJ...", "has_more": true, "total": 248 }
  ```

The list key matches the resource name. Pass the `next_cursor` value as the `cursor` query parameter to fetch the next page. Cursor pagination only works with the default date sort on transactions.

## Error Format

All errors return a JSON envelope:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Human-readable description"
  }
}
```

Error codes use `UPPER_SNAKE_CASE`. Common codes: `VALIDATION_ERROR`, `NOT_FOUND`, `UNAUTHORIZED`, `FORBIDDEN`, `RATE_LIMITED`, `INTERNAL_ERROR`.

# MCP Tools Reference

Complete reference for all MCP tools available in Breadbox. Tools are the primary way AI agents interact with your financial data.

## Tool Access

Tools are classified as **Read** or **Write**:

- **Read tools** are always available
- **Write tools** require the MCP mode to be set to `read_write` (configurable in the admin dashboard under MCP Settings)
- Individual tools can be disabled from the admin dashboard

## Response size

Token-heavy read tools are **lean by default** — they return a compact field projection and let you opt into more via the `fields` parameter (`fields=all` for the full payload). See `query_transactions`, `list_series`, and `list_transaction_rules`.

Every tool response is also subject to a **byte cap** (default ~100 KB ≈ 25K tokens). A response over the cap returns a `RESPONSE_TOO_LARGE` error asking you to narrow the query (add filters, lower `limit`, paginate via `next_cursor`, or use `fields`) rather than emitting an oversized payload. Operators can raise or disable the cap via the `BREADBOX_MCP_MAX_RESPONSE_BYTES` environment variable (`0` disables it).

## Sessions

Before using any tools, agents should call `create_session` to establish a session. This provides the agent with dataset context and server instructions.

---

## Query & Analysis Tools

### query_transactions (Read)

Query transactions with composable filters and cursor pagination.

**Lean by default.** When `fields` is omitted the response is a compact projection (`core,category`) rather than all ~22 fields — pass `fields=all` for the full row, or any explicit alias/field list. When every returned row shares one currency, `iso_currency_code` is emitted once at the top level of the envelope and dropped from each row (per-row fallback when a page mixes currencies). This default is MCP-only; the REST API (`GET /api/v1/transactions`) still returns full objects when `?fields=` is omitted.

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
| `sort_by` | string | `date` (default), `amount`, `provider_name` |
| `sort_order` | string | `desc` (default), `asc` |
| `fields` | string | Field selection. Aliases: `minimal`, `core`, `category`, `timestamps`. Omitted → `core,category` (lean default); `all` → every field. `id` always included. |
| `cursor` | string | Pagination cursor |
| `limit` | int | Results per page (default 50, max 500) |
| `count_only` | bool | When true, return just `{ "count": N }` for the same filters — no rows, no pagination. `cursor`/`limit`/`sort_*`/`fields` ignored. Use before paginating large result sets, or to compare counts across ranges. |

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

---

## Reference Data Tools

> MCP resources were retired — these are plain tools (the only way to read this data).

### get_overview (Read)

Household snapshot: scope (users, accounts, currencies), freshness (latest sync, errored connections, recent transactions), pending-review backlog. Read once at the top of a session to ground later filters.

### list_accounts (Read)

Bank accounts with name, type, balances, currency, and connection. Optional `user_id` filter scopes to one household member.

### list_categories (Read)

The 2-level category taxonomy (slug, display name, parent, icon, color). Use the slugs as the canonical `category_slug` handle on filters and writes.

### list_users (Read)

Household members with role and `short_id` (the `short_id` is the `user_id` on filters).

### list_tags (Read)

The registered tag vocabulary (slugs). New slugs auto-register when `update_transactions` adds them.

### get_sync_status (Read)

Per-connection sync status (provider, `active`|`error`|`pending_reauth`|`disconnected`), last-sync time, last error. Check before trusting freshness.

### list_transaction_rules (Read)

The transaction-rule roster. Filter by `category_slug`, `enabled`, or name `search`. Lean `summary` projection by default (no conditions/actions trees); `fields=all` for full definitions. For trigger/creator/hit-count filters or sorting, use `query_transaction_rules`.

### get_reference (Read)

Read an operating-guidance doc by `kind` — the near-static markdown that explains how to drive the server (formerly `breadbox://` markdown resources). Returns markdown.

| Parameter | Type | Description |
|-----------|------|-------------|
| `kind` | string | **Required.** One of `instructions`, `rule-dsl`, `review-guidelines`, `report-format`. |

- `instructions` — data model + conventions overview.
- `rule-dsl` — the transaction-rule condition grammar, action types, pipeline-stage ordering, sync-vs-retroactive semantics. Read before authoring rules.
- `review-guidelines` — principles for reviewing transactions and creating rules. Read before working the needs-review queue.
- `report-format` — structure + formatting conventions for `submit_report`.

`instructions` / `review-guidelines` / `report-format` reflect operator customization (`app_config`); `rule-dsl` is the fixed grammar.

### list_workflows (Read)

List the household's automation layer. Returns two arrays:

- `workflows` — the enabled, preset-backed Workflows. Each row carries `name`, `slug`, `preset` (the workflow-preset slug it was instantiated from), `enabled` (a workflow can be instantiated but paused), `trigger` (`sync` = after each successful sync \| `schedule` = cron \| `manual`), `schedule_cron` (when `trigger=schedule`), `tool_scope` (`read_only` \| `read_write`), and `last_run_status` + `last_run_at` (omitted when the workflow has never run).
- `presets` — the full catalog of available presets it could enable. Each row carries `slug`, `name`, `category`, `description`, `tool_scope`, `trigger`, default `schedule_cron`, and `enabled` (true when already instantiated as a workflow).

Read this to see what runs automatically before proposing new rules or reports — an existing workflow may already cover the task. Hand-authored agents (no source preset) are excluded; enabling or configuring a workflow is an admin-UI action (the `/workflows` gallery), not an MCP write.

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

The tag vocabulary — see **Reference Data Tools → `list_tags`** above.

### add_transaction_tag (Write)

Attach a tag to a transaction. Auto-creates a persistent tag if the slug is unknown. Idempotent.

| Parameter | Type | Description |
|-----------|------|-------------|
| `transaction_id` | string | UUID or short ID |
| `tag_slug` | string | Tag slug |
| `note` | string | Optional rationale (recorded on the `tag_added` annotation) |

### remove_transaction_tag (Write)

Remove a tag from a transaction. An optional `note` lands on the `tag_removed` annotation. Idempotent.

| Parameter | Type | Description |
|-----------|------|-------------|
| `transaction_id` | string | UUID or short ID |
| `tag_slug` | string | Tag slug |
| `note` | string | Optional rationale for removal — recorded on the audit trail. |

### update_transactions (Write)

Compound batch write — set category, add tags, remove tags, attach a comment, and flag/unflag per transaction in a single atomic call. Max 50 operations per request.

| Parameter | Type | Description |
|-----------|------|-------------|
| `operations` | array | Array of operations (see below). |
| `on_error` | string | `continue` (default — per-op tx) or `abort` (single tx, rolls back on first error). |

Each operation:

| Field | Type | Description |
|-------|------|-------------|
| `transaction_id` | string | UUID or short ID. Required. |
| `category_slug` | string | Optional category to set (writes `category_id`; last-writer-wins, no provenance). |
| `tags_to_add` | array | `[{slug, note?}, ...]`. Auto-creates tags if the slug is new. |
| `tags_to_remove` | array | `[{slug, note?}, ...]`. `note` is optional — if provided, lands on the `tag_removed` annotation. |
| `comment` | string | Optional comment annotation. When flagging, put the flag reason here. |
| `flagged` | bool | Optional. `true` flags the transaction for human attention (sets `flagged_at`); `false` clears the flag. Omit to leave it untouched. Retrieve flagged rows with `query_transactions(flagged=true)`. Folds the former `flag_transaction` / `unflag_transaction` tools. |

Use this to close a review entry: `set category + remove needs-review (with note) + comment` in one call.

### list_annotations (Read)

Return the activity timeline for a transaction. Rows are returned in a `{ "annotations": [...] }` envelope (the MCP `structuredContent` slot requires a JSON record, not a bare array — mirrors the REST `/transactions/{id}/annotations` shape). Each row carries a generic `kind` plus an `action` for the specific event:

| `kind` | `action` values | Notes |
|--------|-----------------|-------|
| `comment` | _(omitted)_ | Free-form comment. `payload.content` carries the body. |
| `rule` | `applied` | A transaction rule fired. `payload.rule_name`, `rule_id` populated. |
| `tag` | `added` / `removed` | Tag attached to or detached from the transaction. `tag_id` and `payload.slug` populated. |
| `category` | `set` | Category was set (manual override or via rule). |

| Parameter | Type | Description |
|-----------|------|-------------|
| `transaction_id` | string | UUID or short ID. Required. |
| `kinds` | array | Optional kind filter. Any of: `comment`, `rule`, `tag`, `category`, `sync`. Empty (default) returns all kinds. |
| `actor_types` | array | Optional actor-type filter. Any of: `user`, `agent`, `system`. Empty (default) returns all actors. Pass `['user']` for the canonical "any human input?" check — drops rule churn + prior agent activity in one filter. |
| `since` | string | Optional RFC3339 timestamp. Returns only annotations whose `created_at` is strictly after this time. Pair with `limit` for cheap delta reads. |
| `limit` | int | Optional cap on returned rows; returns the most recent N (timeline tail) in chronological order. `0` (default) returns the full timeline. Max `200`. Negative is rejected. |

Filters compose. The canonical "what did humans say on this transaction?" call returns ~3 rows even on a transaction churned by 30 rule applications:

```
list_annotations(transaction_id="k7Xm9pQ2", actor_types=["user"])
```

Other useful slices:

- `kinds=['comment']` — comment-only view (replaces the deprecated `list_transaction_comments`).
- `kinds=['comment','tag','category']` — skip rule-application churn while keeping all decision-shaped events.
- `actor_types=['user'], kinds=['comment']` — human-authored notes only.
- `since="2026-04-26T12:00:00Z", limit=50` — delta read after a previous full pull. Pair with `actor_types=['user']` to react only to new human input.

Branch on `action` when the add-vs-remove distinction matters (e.g. building a tag-history view).

**Validation errors** (returned as the standard `{ "error": "..." }` envelope):

- Unknown `kinds` value (e.g. raw DB kind `tag_added`) — must use the generic name (`tag`).
- Unknown `actor_types` value — must be one of `user`, `agent`, `system`.
- Malformed `since` — must be an RFC3339 timestamp.
- Negative `limit`.

### create_tag / update_tag / delete_tag (Write)

Admin-only tag CRUD. Agents typically don't need these — `add_transaction_tag` auto-creates tags on demand. Use these to set display name, color, or cascade-delete a tag.

---

## Subscriptions (Recurring Series) Tools

### list_series (Read)

List recurring series — thin, rule-maintained entities: each is a surrogate identity (`id`/`short_id`), a `name`, and a `type` (`subscription` | `bill` | `loan` | `other`), plus its `tags`. Membership comes from `assign_series` rules, not a shipped detector. **Lean by default** (`fields` omitted → `overview` projection: `name`, `type`, `tags`); pass `fields=all` for timestamps. Get a series' charges via `query_transactions(series_id=...)`.

### get_series (Read)

Get one series by short ID or UUID: its `name`, `type`, and `tags`. A series' linked charges come from `query_transactions(series_id=...)`; its governing rules (the `assign_series` rules that define its membership) are visible on the admin Recurring detail page.

### assign_series (Write)

Link transactions to a recurring series, creating it if needed — the agent's path for a **one-off** assignment (encode a durable pattern as an `assign_series` **rule** instead when you want future charges to resolve automatically). Provide `series_id` to assign to an existing series, **or** `series_name` + `create_if_missing:true` to mint/resolve one by name (surrogate-first: the same name always resolves the same series). Optional `type` (`subscription`|`bill`|`loan`|`other`) for a minted series. Pass `transaction_ids` (≤50) to back-link members (NULL-fill only — never steals a charge already in another series).

### update_series (Write)

Edit a recurring series' `name` and/or `type` (`subscription` | `bill` | `loan` | `other`). Both optional — omit to leave unchanged. Renaming onto an existing live series name is rejected (the name is the series' unique mint key).

### unlink_series_transactions (Write)

Detach `transaction_ids` (≤50, each a current member) from a recurring series — the inverse of `assign_series`' link path. Clears each charge's `series_id` and strips the series' inherited tags from them (a tag the user added directly survives). Errors if any listed transaction isn't a current member, so it can't silently no-op or touch another series.

> Series **type** and **tag** edits fold into `update_series` (`type`, `tags_to_add`, `tags_to_remove`) — there are no standalone `set_series_type` / `add_series_tag` / `remove_series_tag` tools.

---

## Counterparties Tools

A counterparty is the canonical, cross-provider "other side" of a charge — merchants **and** non-merchants (Venmo, people, employers). It is a thin, rule-maintained entity: a surrogate identity (`id`/`short_id`) + `name` + optional enrichment (`website_url`, `logo_url`, `category_id`, `mcc`). Membership comes from `assign_counterparty` rules, not a normalizer.

### list_counterparties (Read)

List every live counterparty with its `name` and enrichment fields. Get a counterparty's charges via `query_transactions`; get one counterparty via `get_counterparty`.

### get_counterparty (Read)

Get one counterparty by short ID or UUID: its `name` and enrichment fields. Its governing rules (the `assign_counterparty` rules that define its membership) are visible on the admin Counterparties detail page; its linked charges come from `query_transactions`. Also exposed as the `breadbox://counterparty/{short_id}` resource (detail + governing rules).

### create_counterparty (Write)

Create a new counterparty with a `name` and optional enrichment (`website_url`, `logo_url`, `category_id`, `mcc`). Creating onto an existing live name is rejected — edit that one instead. To bind charges, use `assign_counterparty` (one-off) or author an `assign_counterparty` **rule** (durable).

### update_counterparty (Write)

Enrich a counterparty: edit its `name`, `website_url`, `logo_url`, `category_id` (slug or short ID), and/or `mcc`. Every field optional — omit to leave unchanged; an empty `name` is rejected. This is the enrichment lane (no auto-fetch).

### assign_counterparty (Write)

Bind transactions to a counterparty, creating it if needed — a **one-off** assignment. For durable patterns, author an `assign_counterparty` **rule** so every future matching charge resolves automatically. Provide `counterparty_id` to bind to an existing counterparty, **or** `name` + `create_if_missing:true` to resolve-or-create one by name (surrogate-first; de-dupes on the live name). Pass `transaction_ids` (≤50) to link members (NULL-fill only — never steals a charge already bound elsewhere).

### unlink_counterparty_transaction (Write)

Detach `transaction_ids` (≤50, each a current member) from a counterparty — the inverse of `assign_counterparty`' link path. Clears each charge's `counterparty_id`. Errors if any listed transaction isn't a current member, so it can't silently no-op or touch another counterparty.

---

## Transaction Rules Tools

> **Full DSL specification**: see **[`docs/rule-dsl.md`](rule-dsl.md)** for the complete condition grammar, action semantics, trigger matrix, pipeline-stage (priority) ordering, sync-vs-retroactive differences, and the chaining model that lets later-stage rules observe earlier-stage rules' tag/category mutations.

The tools below are thin API skins over the DSL — the DSL doc is the source of truth for what a rule *means*.

### create_transaction_rule (Write)

Create one or more rules that fire during sync. Pass `rules`: an array of 1..100 rule specs (a single rule is a one-element array). Authoring a chained pipeline in one call orders rules by stage so earlier-stage tag/category writes feed later-stage conditions. Actions compose within a rule (`set_category` + `add_tag` + `set_metadata` + `add_comment` are all valid). (Folds the former `batch_create_rules`.)

| Parameter | Type | Description |
|-----------|------|-------------|
| `rules` | array | **Required.** 1..100 rule specs (fields below). |

Each rule spec:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Human-readable rule name |
| `conditions` | object | Condition tree. Omit or `{}` for match-all. Supports `and` / `or` / `not` nesting up to depth 10. Numeric fields (incl. `amount`) add `approx` (`value` + sibling `tolerance`) and `between` (sibling `min`/`max`). Date-part fields derived from the tz-naive `date` — `day_of_month`, `month`, `day_of_week` (`0`=Sun), `day_of_year` — are numeric; `day_of_month approx` is cyclic + clamped (31 matches Feb's last day). Encode annual cadence as `month` + `day_of_month` (leap-robust), not `day_of_year`. Leaf fields also include `metadata.<key>` to read a key from the free-form metadata blob (ops: `eq`/`neq`/`contains`/`not_contains`/`matches`/`in`/`gt`/`gte`/`lt`/`lte`/`exists`/`not_exists`; an absent key matches only `not_exists`). Full grammar: `docs/rule-dsl.md`. |
| `actions` | array | Typed actions: `set_category`, `add_tag`, `remove_tag`, `add_comment`, `set_metadata` (`metadata_key` + `metadata_value`, any JSON), `remove_metadata` (`metadata_key`), `assign_series`, `flag` (no params — sets `flagged_at`, surfacing the txn for attention), `unflag` (no params — clears `flagged_at`). Either this or `category_slug` is required. |
| `category_slug` | string | Shorthand for `actions=[{type:set_category,category_slug:...}]` |
| `trigger` | string | `on_create` (default) / `on_change` / `always`. `on_update` accepted as legacy alias. |
| `stage` | string | **Preferred.** Semantic pipeline stage: `baseline` / `standard` / `refinement` / `override`. Resolves to priority `0 / 10 / 50 / 100`. |
| `priority` | int | Raw pipeline-stage integer, 0–1000. If both `stage` and `priority` are supplied, `priority` wins. Defaults to `10` (standard). |
| `expires_in` | string | Optional duration (e.g., `24h`, `30d`, `1w`) |
| `apply_retroactively` | bool | Also back-fill matching existing transactions (materializes `set_category` / `add_tag` / `remove_tag` / `set_metadata` / `remove_metadata` / `assign_series` / `flag` / `unflag`; `add_comment` is sync-only) |

Returns `{ created, failed, rules: [{rule, retroactive_matches?}], errors }` so a partial batch is recoverable.

### list_transaction_rules (Read)

The rule roster — see **Reference Data Tools → `list_transaction_rules`** above. For filtered/sorted analysis use `query_transaction_rules` below.

### query_transaction_rules (Read)

The filterable/sortable analogue of `query_transactions`, for the rule set. Same lean-by-default `summary` projection as `list_transaction_rules`, but adds trigger / creator / hit-count filters and sorting so you can ask targeted questions — "which rules never fire?", "highest-impact rules for groceries", "agent-created rules" — instead of dumping the whole roster. To check whether **one** merchant is already covered before creating a rule, prefer `find_matching_rules`; use this when you want a filtered *slice* of rules or coverage analytics.

| Parameter | Type | Description |
|-----------|------|-------------|
| `category_slug` | string | Filter to rules whose `set_category` action targets this slug |
| `enabled` | bool | Filter by enabled status |
| `trigger` | string | `on_create` \| `on_change` (alias `on_update`) \| `always` |
| `creator_type` | string | `user` \| `agent` \| `system` |
| `search` | string | Substring / words / fuzzy search on rule name |
| `search_mode` | string | `contains` (default) \| `words` \| `fuzzy` |
| `min_hit_count` | int | Only rules with `hit_count >= n` (surfaces high-impact rules) |
| `only_unused` | bool | Only rules that have never fired (`hit_count = 0`) — dead/over-specific rules worth pruning |
| `sort_by` | string | `priority` (default, pipeline order) \| `hit_count` \| `last_hit_at` \| `created_at` \| `name` |
| `sort_order` | string | `asc` \| `desc` (per-column default otherwise) |
| `fields` | string | Field selection. Alias: `summary` (default). `all` → full definition. |
| `cursor` | string | Pagination cursor. Only valid with the default `priority` sort; an explicit `sort_by` returns a single top-N page with no `next_cursor`. |
| `limit` | int | Results per page (default 50, max 500) |

### update_transaction_rule (Write)

Every field is optional; omit to leave unchanged. Pass `conditions={}` to explicitly switch to match-all; pass `actions=[...]` to replace the entire action set; pass `expires_at=""` to clear expiry. Pass `stage` (`baseline` | `standard` | `refinement` | `override`) to re-slot a rule into a canonical pipeline stage without guessing a raw `priority`. If both `stage` and `priority` are supplied, `priority` wins.

### delete_transaction_rule (Write)

Delete a rule by ID. System-seeded rules can't be deleted — disable them via update instead.

### apply_rules (Write)

Retroactive apply. Pass `rule_id` for a single rule (no chaining — that rule evaluates in isolation), or omit to run the full active-rule pipeline in priority-ASC order with the same chaining semantics as sync. Materializes `set_category` / `add_tag` / `remove_tag` / `set_metadata` / `remove_metadata` / `assign_series` / `flag` / `unflag`; skips `add_comment`. Ignores `rule.trigger` (retroactive is a bulk op).

| Parameter | Type | Description |
|-----------|------|-------------|
| `rule_id` | string | Optional — single-rule apply (no chaining). Omit for full pipeline. |

### preview_rule (Read)

Dry-run a condition tree against existing transactions. **Isolation semantics** — evaluates only the supplied condition against stored data; does NOT simulate the full rule pipeline. Use to answer "what does this condition match today" before creating a rule.

| Parameter | Type | Description |
|-----------|------|-------------|
| `conditions` | object | Condition tree to test (same grammar as `create_transaction_rule`) |
| `sample_size` | int | Max sample matches to return (default 10, max 50). `match_count` always reflects the full match set. |

### find_matching_rules (Read)

The **inverse of `preview_rule`**: evaluates the full active rule set against a single transaction (or a synthetic merchant context) and returns only the rules that match. Use it BEFORE creating a rule to answer "is this merchant already covered?" with one call — instead of listing every rule and scanning them by hand, which is slow and misses near-duplicates once you have hundreds of rules. Rules come back in priority-ASC (pipeline) order. Trigger is not filtered — a rule is reported whenever its *condition* matches.

| Parameter | Type | Description |
|-----------|------|-------------|
| `transaction_id` | string | A transaction id/short_id to evaluate every condition field (amount, category, tags, provider…) against the real row. Provide exactly one of `transaction_id` or `merchant`. |
| `merchant` | string | Free-text merchant/name — builds a synthetic context with only the name fields set, so it matches name-based rules (`provider_merchant_name` / `provider_name`) but not amount/category/tag rules. Provide exactly one of `transaction_id` or `merchant`. |

Response: `{ matched_count, rules: [{ short_id, name, sets_category, trigger, priority, hit_count, match_all }] }`. A rule with `sets_category` already handling the merchant means **don't** create a duplicate. `match_all=true` flags conditionless rules (e.g. the seeded `needs-review` tagger) that match everything — not merchant coverage.

### create_transaction_rule — chained pipeline example

`create_transaction_rule` takes a `rules` array, so a composable pipeline lands in one call — use `stage` (preferred) or raw `priority` on each item to order rules so earlier-stage rules set up tags/categories that later-stage rules react to. Returns per-item success + errors.

Example pipeline (3 rules that chain):

```json
{
  "rules": [
    {
      "name": "Tag coffee shops",
      "stage": "baseline",
      "conditions": { "field": "provider_merchant_name", "op": "contains", "value": "starbucks" },
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

### list_transaction_comments (Read) — Deprecated

Deprecated: prefer `list_annotations` with `kinds=['comment']`. Returns the same comment data with renamed fields (`author_*` instead of `actor_*`, `content` lifted out of `payload`). Will be removed in a future release.

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

Breadbox exposes **no MCP resources** — they were retired entirely (invisible on clients that can't `resources/list`). Everything is a tool:

- Ambient context: call `get_overview` at the start of a session.
- Operating-guidance docs (formerly `breadbox://` markdown resources): `get_reference(kind=instructions|rule-dsl|review-guidelines|report-format)`.

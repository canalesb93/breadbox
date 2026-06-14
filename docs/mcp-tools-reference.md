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

List all registered tags.

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

Compound batch write — set category, add tags, remove tags, and attach a comment per transaction in a single atomic call. Max 50 operations per request.

| Parameter | Type | Description |
|-----------|------|-------------|
| `operations` | array | Array of operations (see below). |
| `on_error` | string | `continue` (default — per-op tx) or `abort` (single tx, rolls back on first error). |

Each operation:

| Field | Type | Description |
|-------|------|-------------|
| `transaction_id` | string | UUID or short ID. Required. |
| `category_slug` | string | Optional category to set. Sets `category_override='user'`. |
| `tags_to_add` | array | `[{slug, note?}, ...]`. Auto-creates tags if the slug is new. |
| `tags_to_remove` | array | `[{slug, note?}, ...]`. `note` is optional — if provided, lands on the `tag_removed` annotation. |
| `comment` | string | Optional comment annotation. |

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

List detected recurring series. Optional `status` filter (`active` | `candidate` | `paused` | `cancelled`). **Lean by default** (`fields` omitted → `overview` projection): each row carries `type` (`subscription` | `bill` | `loan` | `other` — inferred from category, set via `set_series_type`), `cadence`, `expected_amount` + `iso_currency_code` (never sum across currencies), `next_expected_date`, `occurrence_count`, and `confidence` (`auto` | `confirmed` | `rejected`). Active series also carry a derived `renewal_health` (`active` | `due_soon` | `overdue` | `stale` | `unknown`) and signed `days_until_renewal` (negative = overdue) so you can answer "what renews soon" and "what looks cancelled" without re-deriving cadence math — `stale` means a full cadence cycle elapsed past the expected charge. The verbose `detection_signals` evidence is **omitted** from the lean list — pass `fields=all`, or use `get_series` for one series' full detail. Read `status=candidate` to find series awaiting a verdict.

### get_series (Read)

Get one series by short ID or UUID, including its full `detection_signals` (`occurrence_count`, `interval_cv`, `cadence_snap_error`, `amount_branch`, `merchant_key_is_fallback`). Inspect before reviewing a candidate.

### explain_series_candidates (Read)

Answer "why isn't *merchant* a subscription?". Returns `near_misses` — every recurring-looking merchant group that is **not** already a series, each annotated with the detector's verdict: `qualifies:true` (passes every gate but isn't tracked yet — confirm it with `assign_series`) or a specific `reason` it fell short (`too_few_occurrences`, `irregular_cadence`, `interval_too_variable`, `amount_unstable`, `same_day_duplicates`). Each row carries a human `explanation` plus the raw numbers (`occurrence_count`, `nearest_cadence`, `median_gap_days`, `interval_cv`, `amount_min`/`amount_max`, `first_seen`/`last_seen`). Read-only analysis over the trailing detection window — the precision-first detector deliberately stays quiet on these, so this surfaces what it skipped (ordered most-charges-first, capped at 50).

### review_series (Write)

Apply a verdict: `confirm` (it is a subscription → `active`), `reject` (NOT a subscription → sticky at that amount band, never re-proposed), `pause`, or `cancel`. A user's prior confirmation outranks a later agent write. This is how an agent adjudicates candidates from `list_series(status=candidate)`.

### assign_series (Write)

Create a recurring series detection missed, or link transactions to an existing one — the agent's path to fix gaps. Provide `series_id` to assign to an existing series, **or** `merchant_key` + `create_if_missing:true` to mint one (funnels through the same dedup + sticky-reject arbitration as the detector, so re-creating a user-rejected series at the same signature is a no-op). Pass `transaction_ids` (≤50) to back-link members (NULL-fill only — never steals a charge already in another series). `confirm:true` flips it straight to `active`; omit to leave a reviewable `candidate`. Use after `list_series(status=candidate)` shows nothing for a subscription the user says exists.

### update_series (Write)

Edit a recurring series' user-owned attributes: `name`, `expected_amount` (+ `currency`, `amount_tolerance`), `cadence`, `expected_day`, `category_id`, `user_id` (owner). Every field is optional — omit to leave unchanged. This is a deliberate override, **not** a detection proposal: it bypasses the source-precedence ladder and protects the edited values from being reverted by the next sync's re-detect. Editing `cadence` re-derives `next_expected_date`; changing `currency` or `user_id` is collision-guarded (they're part of the dedup signature, so an edit can't silently merge two series). Use `review_series` for `confirm`/`pause`/`cancel`, `set_series_type` for the type axis, and `rekey_series` for the `merchant_key` — those have their own semantics and are not editable here.

### set_series_type (Write)

Set a recurring series' `type`: `subscription` (streaming/SaaS/memberships), `bill` (rent/utilities/insurance/telecom), `loan` (mortgage/auto/student/personal), or `other`. The detector infers the type from the linked charges' dominant category at first detection; this is the correction handle. The override is **sticky** — re-detection won't revert it. (`assign_series` also accepts an optional `type` when minting.)

### rekey_series (Write)

Correct a series' `merchant_key` when detection grouped it under a wrong or fallback key (e.g. `payment` → `spotify`). Repoints the series and its linked transactions to `new_merchant_key`. Refuses to silently merge: errors if a live series already exists at the new key, or that key is sticky-rejected. Corrects *historical* grouping — incoming charges still key off the provider name at sync time (a merchant-key alias table is future work).

### split_series (Write)

Break an over-grouped series in two: move `transaction_ids` (≤50, each a current member of the source series) into a brand-new series under `new_merchant_key`. The fix for the detector sweeping a stray charge into a real subscription (e.g. a $4.99 add-on bundled with a $139/yr renewal). The new series inherits the source's currency / user / category; rollups recompute on both sides. Errors if `new_merchant_key` equals the source key, already has a series, or any listed transaction isn't a current member. Returns the new series.

### unlink_series_transactions (Write)

Detach `transaction_ids` (≤50, each a current member) from a recurring series — the inverse of `assign_series`' link path. Clears each charge's `series_id`, strips the series' inherited tags from them (a tag the user added directly survives), and recomputes the series' rollups + `next_expected_date`. Errors if any listed transaction isn't a current member, so it can't silently no-op or touch another series. Use to remove a charge the detector wrongly swept in; use `split_series` instead when the stray charges form their own series.

### add_series_tag (Write)

Attach an existing tag to a recurring series. The tag is materialized onto every linked transaction (they inherit it) and applied to future members as they join — so tagging the Netflix series tags all its charges. The tag must already exist (create it with `create_tag` first). Returns the updated series (including its `tags`).

### remove_series_tag (Write)

Detach a tag from a recurring series and strip the series-inherited copies from its linked transactions. Provenance-scoped: a tag a user added directly to a transaction survives.

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

List rules with optional filters and cursor pagination. **Lean by default** (`fields` omitted → `summary` projection): each row carries `name`, `enabled`, `priority`, `trigger`, `category_slug` / `category_display_name`, `hit_count`, `last_hit_at`, `created_by_type` — the roster view, **without** the `conditions` / `actions` trees. Pass `fields=all` to inspect or audit full rule definitions. Mirror of `breadbox://rules` (which always returns full).

| Parameter | Type | Description |
|-----------|------|-------------|
| `search` | string | Substring / words / fuzzy search on rule name |
| `category_slug` | string | Filter by target category |
| `enabled` | bool | Filter by enabled status |
| `fields` | string | Field selection. Alias: `summary` (default). `all` → full definition incl. `conditions`/`actions`. |
| `cursor` | string | Pagination cursor |
| `limit` | int | Results per page (default 50, max 500) |

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

### find_matching_rules (Read)

The **inverse of `preview_rule`**: evaluates the full active rule set against a single transaction (or a synthetic merchant context) and returns only the rules that match. Use it BEFORE creating a rule to answer "is this merchant already covered?" with one call — instead of listing every rule and scanning them by hand, which is slow and misses near-duplicates once you have hundreds of rules. Rules come back in priority-ASC (pipeline) order. Trigger is not filtered — a rule is reported whenever its *condition* matches.

| Parameter | Type | Description |
|-----------|------|-------------|
| `transaction_id` | string | A transaction id/short_id to evaluate every condition field (amount, category, tags, provider…) against the real row. Provide exactly one of `transaction_id` or `merchant`. |
| `merchant` | string | Free-text merchant/name — builds a synthetic context with only the name fields set, so it matches name-based rules (`provider_merchant_name` / `provider_name`) but not amount/category/tag rules. Provide exactly one of `transaction_id` or `merchant`. |

Response: `{ matched_count, rules: [{ short_id, name, sets_category, trigger, priority, hit_count, match_all }] }`. A rule with `sets_category` already handling the merchant means **don't** create a duplicate. `match_all=true` flags conditionless rules (e.g. the seeded `needs-review` tagger) that match everything — not merchant coverage.

### batch_create_rules (Write)

Create multiple rules in one call. Ideal for composable pipelines — use `stage` (preferred) or raw `priority` on each item to order rules so earlier-stage rules set up tags/categories that later-stage rules react to. `stage` resolves to priority `0 / 10 / 50 / 100`; if both `stage` and `priority` are supplied on an item, `priority` wins. Returns per-item success + errors.

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

In addition to tools, Breadbox exposes three MCP resources that provide passive context:

| URI | Description |
|-----|-------------|
| `breadbox://overview` | Live dataset summary (users, accounts, spending, pending transactions) |
| `breadbox://review-guidelines` | Guidelines for reviewing transactions and creating rules |
| `breadbox://report-format` | Report structure templates and formatting guidelines |

Agents should read `breadbox://overview` at the start of a session for ambient context about the household's financial data.

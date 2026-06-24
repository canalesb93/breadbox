# Transaction rule DSL

Rules pattern-match transactions during sync (or via explicit retroactive runs) and apply typed actions. This is the canonical agent-facing reference for the rule grammar served via MCP. The full engineering spec is in `docs/rule-dsl.md`.

Rules are the universal substrate: prefer authoring a rule over a one-off enrichment, because a rule keeps applying to every future matching charge with zero agent runs.

## Rule shape

```json
{
  "name": "provider_category_primary: dining → food_and_drink_restaurant",
  "trigger": "on_create",
  "stage": "standard",
  "conditions": { ... },
  "actions": [ ... ]
}
```

- `name` — descriptive. Convention: `"<pattern type>: <match> → <action>"`.
- `trigger` — when the rule evaluates: `on_create` (new transactions, default), `on_change` (changed re-synced transactions), or `always` (both). `on_update` is a legacy alias for `on_change`.
- `stage` / `priority` — pipeline position. Prefer the semantic `stage` (`baseline|standard|refinement|override` → `0|10|50|100`); supply a raw `priority` int only for fine ordering inside a stage. Lower runs first. See pipeline below.
- `conditions` — JSON tree (see grammar). An empty `{}` matches every transaction.
- `actions` — array of typed actions; at least one required, applied in order.

## Conditions grammar

A condition is one of:

```jsonc
// Logical (nest up to depth 10)
{ "and": [<condition>, <condition>, ...] }
{ "or":  [<condition>, <condition>, ...] }
{ "not": <condition> }

// Leaf — string
{ "field": "<name>", "op": "eq | neq | contains | not_contains | matches | in", "value": "..." }

// Leaf — numeric
{ "field": "amount", "op": "eq | neq | gt | gte | lt | lte", "value": 12.34 }
{ "field": "amount", "op": "approx",  "value": 15.99, "tolerance": 0.50 }   // value ± tolerance
{ "field": "amount", "op": "between", "min": 10, "max": 20 }                 // min ≤ x ≤ max, inclusive

// Leaf — boolean
{ "field": "pending", "op": "eq | neq", "value": true }

// Leaf — tags
{ "field": "tags", "op": "contains | not_contains", "value": "subscription" }
{ "field": "tags", "op": "in", "value": ["subscription", "recurring"] }
```

- `matches` is RE2 regex (no backreferences/lookaround; `(?i)` for case-insensitive). String comparisons are otherwise case-insensitive.
- `approx` requires `tolerance` (≥ 0); `between` requires `min` and `max`.
- Unknown field or op evaluates to false (the rule just won't match); invalid regex / wrong value type is rejected at write time.

## Available fields

**Raw provider fields (immutable — author against these):**
- `provider_name` — raw transaction name from the bank feed.
- `provider_merchant_name` — structured merchant name (Plaid/enriched); prefer over `provider_name` when present.
- `provider_category_primary` — raw provider category (Teller: `dining`, `groceries`, …; Plaid: `FOOD_AND_DRINK_RESTAURANTS`, …).
- `provider_category_detailed` — raw sub-category (Plaid only).
- `provider` — `plaid` | `teller` | `simplefin` | `csv`.
- `amount` — number; signed (positive = money out, negative = money in).
- `pending` — bool.

**Date-part fields (numeric, derived from the transaction's immutable `date`):**
- `day_of_month` — 1..31.
- `month` — 1..12.
- `day_of_week` — 0=Sun .. 6=Sat.
- `day_of_year` — 1..366.

**Identity / membership fields:**
- `account_id`, `account_name`, `user_id`, `user_name` — string.
- `series` — short_id of the recurring series the transaction belongs to (`""` when unassigned). `in_series` — bool.
- `counterparty` — short_id of the counterparty (the other side of the transaction) it's bound to (`""` when unassigned). `has_counterparty` — bool. Both are **derived/mutable** (set by `assign_counterparty` rules/agents, like `series`/`in_series`) — author membership on the **raw provider fields**, not on these.
- `tags` — current tag slugs (see tags ops above).
- `metadata.<key>` — reads a key from the transaction's free-form metadata blob (e.g. `metadata.tax_deductible`); supports the string/numeric ops plus `exists` / `not_exists`.
- `category` — the **assigned** category slug (changes when a rule/agent/user reassigns; useful for rule chaining).

**Stable-vs-mutable guidance.** Condition on the **raw provider fields, date-parts, and `amount`** — they never change, so a rule keeps matching deterministically. Avoid conditioning on `account_name`, `category`, `series`, or `counterparty` unless you specifically want to react to a *current, mutable* value (e.g. rule chaining off `category`, or retroactively tagging existing series members) — those can shift out from under the rule.

## The recurrence idiom

Rules replace the old recurring-charge detector. Express a subscription/recurring charge as an amount-and-day approximate match and assign a series:

```jsonc
// Monthly: ~$15.99 around the 3rd of the month
{
  "and": [
    { "field": "amount",       "op": "approx", "value": 15.99, "tolerance": 0.50 },
    { "field": "day_of_month", "op": "approx", "value": 3,     "tolerance": 2 }
  ]
}
// → actions: [{ "type": "assign_series", "series_name": "Spotify", "create_if_missing": true }]
```

```jsonc
// Annual: every March, around the 15th
{
  "and": [
    { "field": "month",        "op": "eq",     "value": 3 },
    { "field": "day_of_month", "op": "approx", "value": 15, "tolerance": 3 }
  ]
}
```

`day_of_month approx` is **cyclic and clamped**: the target clamps into the actual month length and the distance wraps month-end, so `approx 30 ±2` still matches Feb 28.

## Actions

```jsonc
{ "type": "set_category",    "category_slug": "food_and_drink_groceries" }
{ "type": "add_tag",         "tag_slug": "needs-review" }
{ "type": "remove_tag",      "tag_slug": "needs-review" }
{ "type": "add_comment",     "content": "Auto-flagged by recurring-charge rule" }
{ "type": "set_metadata",    "metadata_key": "tax_deductible", "metadata_value": true }
{ "type": "remove_metadata", "metadata_key": "tax_deductible" }
{ "type": "flag" }
{ "type": "unflag" }
{ "type": "assign_series",   "series_name": "Spotify", "create_if_missing": true }
{ "type": "assign_series",   "series_short_id": "ab12cd34" }
{ "type": "assign_counterparty", "counterparty_short_id": "cp01ab23" }
{ "type": "assign_counterparty", "counterparty_name": "Jane Doe", "create_if_missing": true }
```

- `set_category` — at most one per rule. Use `category_slug` (never `category_id`).
- `add_tag` / `remove_tag` — tag auto-created on add; slug must match `^[a-z0-9][a-z0-9\-:]*[a-z0-9]$`. Same-slug add+remove in one pass cancels (net-diff).
- `add_comment` — sync-only; retroactive apply does not materialize comments.
- `set_metadata` / `remove_metadata` — write/clear a key on the transaction's metadata blob. `metadata_value` is any JSON value.
- `flag` / `unflag` — set/clear the transaction's flag (mirrors the `flag_transaction` tool, sans reason).
- `assign_series` — **surrogate-first**: provide **exactly one** of `series_short_id` (existing series) OR `series_name` + `create_if_missing: true` (resolve-or-mint a thin series by name). There is **no `merchant_key`** — the legacy key is accepted only as a back-compat alias for `series_name`. Link-and-rollup only; never steals a charge already in another series; honors sticky-reject.
- `assign_counterparty` — bind matching transactions to a **counterparty** (the other side of the transaction — merchants AND non-merchants: Venmo recipients, people, employers, landlords). Provide **exactly one** of `counterparty_short_id` (an existing counterparty) OR `counterparty_name` + `create_if_missing: true`. Default is **assign-to-existing**: a counterparty is **never** auto-created unless `create_if_missing` is set. By-name is a **resolve-or-create** that de-dupes on the live name (unlike series there is no UNIQUE on name) — so **reuse an existing counterparty across providers** (look one up first) rather than minting a duplicate. Link-only (NULL-fill).

A rule can carry multiple actions of different types (e.g. `set_category` + `add_tag`). Only `set_category` is singleton.

## Provenance-free semantics

There is **no `category_override` / lock and no precedence ladder** — that model was removed. Rules are **last-writer-wins**: within one sync pass, the highest-priority-stage `set_category` wins; accumulator actions (`add_tag`, `add_comment`) all contribute.

A re-sync will **not** clobber a user's manual edits, because rules only run on **new or changed** transactions — an unchanged re-synced row runs no rules. There is no provenance check to respect; correctness comes from the trigger model, not from a guard.

## Pipeline stages

Rules evaluate in `priority ASC, created_at ASC` — **lower stage runs first**. Earlier-stage tag/category mutations feed later-stage conditions, so rules compose (rule A tags `coffee`; rule B conditioned on `tags contains coffee` sets the category).

| Stage | Priority | Meaning |
|---|---|---|
| `baseline` | 0 | Broad classifications, system defaults |
| `standard` | 10 | Default rule stage |
| `refinement` | 50 | Reacts to baseline/standard output |
| `override` | 100 | Final say on `set_category` |

## Sync vs retroactive

- `on_create` / `on_change` / `always` rules fire automatically during sync.
- `apply_retroactively=true` on `create_transaction_rule` runs the new rule against existing transactions once. Use only during setup.
- `apply_rules` runs active rules against existing transactions — explicit one-off bulk operations only, never during routine review.

## Provider quirks

- **Teller** — `general` is a 30%+ catch-all; don't write `provider_category_primary=general` rules, use name patterns. Other categories map reliably.
- **Plaid** — categories are hierarchical (`FOOD_AND_DRINK_RESTAURANTS`); use as-is.
- **CSV / SimpleFIN** — may lack a provider category; use name patterns, `amount`, or date-parts.

## Authoring checklist

1. Read `list_transaction_rules` to avoid duplicates.
2. `preview_rule` your conditions to verify match count and review a sample of matched transactions.
3. Condition on immutable fields (raw provider fields, date-parts, `amount`).
4. Pick the right `stage` so rules compose predictably.
5. Use `category_slug` (never `category_id`); prefer `contains` / `approx` over exact match — feeds format names and amounts inconsistently.
6. Use `create_transaction_rule` with a `rules` array (max 100) to land related rules in one call.

# Transaction rule DSL

Rules pattern-match transactions during sync (or via explicit retroactive runs) and apply typed actions. This is the canonical reference for the rule grammar served via MCP. The full engineering spec is in `docs/rule-dsl.md`.

## Rule shape

```json
{
  "name": "provider_category_primary: dining → food_and_drink_restaurant",
  "trigger": "on_create",
  "priority": 5,
  "conditions": { ... },
  "actions": [ ... ]
}
```

- `name` — descriptive. Convention: `"<pattern type>: <match> → <category>"`.
- `trigger` — when the rule evaluates. Currently `on_create` (new transaction, including re-sync updates).
- `priority` — integer; higher wins on conflict. See pipeline below.
- `conditions` — JSON tree (see grammar). NULL conditions match every transaction (used by the seeded `needs-review` rule).
- `actions` — array of typed actions, applied in order.

## Conditions grammar

A condition is one of:

```jsonc
// Logical
{ "and": [<condition>, <condition>, ...] }
{ "or":  [<condition>, <condition>, ...] }
{ "not": <condition> }

// Leaf — string
{ "field": "<name>", "op": "eq | neq | contains | not_contains | matches | in", "value": "..." }

// Leaf — numeric
{ "field": "amount", "op": "eq | neq | gt | gte | lt | lte", "value": 12.34 }

// Leaf — boolean
{ "field": "pending", "op": "eq | neq", "value": true }
```

`matches` is regex (RE2 syntax). `in` takes an array value.

## Available fields

- `provider_name` — raw transaction name from the bank feed.
- `provider_merchant_name` — Plaid-only structured merchant name; prefer over `provider_name` when present.
- `provider_category_primary` — raw category from the provider (Teller: `dining`, `groceries`, …; Plaid: `FOOD_AND_DRINK_RESTAURANTS`, …).
- `provider_category_detailed` — sub-category (Plaid only).
- `provider` — `plaid` | `teller` | `csv`.
- `account_id` — short_id of an account.
- `user_id` — short_id of a user.
- `user_name` — convenience, matches the user's display name.
- `amount` — number; signed (positive = debit).
- `pending` — bool.

## Actions

```jsonc
{ "type": "set_category", "category_slug": "food_and_drink_groceries" }
{ "type": "add_tag",      "tag_slug": "needs-review" }
{ "type": "add_comment",  "body": "Auto-flagged by recurring-charge rule" }
```

`set_category` only fires when `category_override = false` — manually-categorized transactions are sacred.

## Pipeline-stage priority

Recommended priority bands (so specific rules outrank broad ones):

| Pattern | Priority |
|---|---|
| Per-merchant rules (single merchant) | 20–30 |
| Name-pattern rules (`contains` on `provider_name`) | 10–20 |
| `provider_category_primary` rules | 1–10 |

If two rules can match the same transaction, the higher priority wins; ties resolve by rule creation order.

## Sync vs retroactive

- `trigger=on_create` rules fire automatically during sync (and on re-sync updates).
- `apply_retroactively=true` on `create_transaction_rule` runs the new rule against existing transactions once at creation time. Use ONLY during initial setup.
- `apply_rules` tool runs every active rule against existing transactions. Reserved for explicit one-off bulk operations — never during routine review.

## Provider quirks

- **Teller** — `general` is a 30%+ catch-all. Don't write `provider_category_primary=general` rules; use name patterns instead. Other Teller categories (dining, groceries, fuel, …) map reliably.
- **Plaid** — categories are hierarchical (`FOOD_AND_DRINK_RESTAURANTS`); use them as-is. Pending transactions get linked to their posted version via `provider_pending_transaction_id`.
- **CSV** — no provider category; rules must use name patterns or amount thresholds.

## Authoring checklist

1. Check `list_transaction_rules` to avoid duplicates.
2. `preview_rule` your conditions to verify match count and review a sample of matched transactions.
3. Pick the right priority band for the pattern type.
4. Use `category_slug` (never `category_id`).
5. Prefer `contains` over exact match — bank feeds format names inconsistently.
6. Use `batch_create_rules` (max 100) to land related rules in one call.

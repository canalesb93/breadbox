# Transaction Rule DSL

Canonical spec for the transaction rule DSL — the condition tree, action types, and trigger semantics used by both humans (admin UI) and agents (MCP). This is the source of truth; tool descriptions, admin form help, and tests should all agree with it.

## At a glance

A rule is a JSON document with four parts:

```json
{
  "name": "Dining out categorization",
  "conditions": { "field": "merchant_name", "op": "contains", "value": "Starbucks" },
  "actions": [ { "type": "set_category", "category_slug": "food_and_drink_coffee" } ],
  "trigger": "on_create",
  "priority": 10
}
```

At sync time, the rule engine evaluates every active rule against every new or changed transaction in priority order. Matching rules' actions merge and apply in a single DB transaction.

## Conditions

A condition is either a **leaf** (field/op/value) or a **combinator** (and/or/not over sub-conditions). Mixing a leaf and a combinator in the same node is rejected.

### Leaf condition

```json
{ "field": "merchant_name", "op": "contains", "value": "Uber" }
```

### Combinators

```json
{ "and": [ <condition>, <condition>, ... ] }
{ "or":  [ <condition>, <condition>, ... ] }
{ "not":   <condition> }
```

Combinators nest. Max depth: **10**. Empty / zero-value condition (`{}`) means **match every transaction**.

### Nested example

```json
{
  "or": [
    {
      "and": [
        { "field": "merchant_name", "op": "contains", "value": "Starbucks" },
        { "field": "amount", "op": "gte", "value": 5 }
      ]
    },
    { "field": "name", "op": "matches", "value": "^COFFEE.*" }
  ]
}
```

### Fields

| Field               | Type    | Source                                                          |
| ------------------- | ------- | --------------------------------------------------------------- |
| `name`              | string  | Transaction name                                                |
| `merchant_name`     | string  | Merchant display name (may be empty for CSV / no-enrich rows)   |
| `amount`            | numeric | Transaction amount (positive = debit, negative = credit)        |
| `category_primary`  | string  | **Raw provider** primary category (Plaid/Teller classification) |
| `category_detailed` | string  | **Raw provider** detailed category                              |
| `pending`           | bool    | Provider-reported pending flag                                  |
| `provider`          | string  | `plaid`, `teller`, or `csv`                                     |
| `account_id`        | string  | Account UUID                                                    |
| `user_id`           | string  | Family member UUID                                              |
| `user_name`         | string  | Family member display name                                      |
| `tags`              | tags    | List of current transaction tag slugs (special, see below)      |

> **Raw vs assigned category.** `category_primary` / `category_detailed` reflect what the provider *said*; they don't change when Breadbox (or a user) reassigns the category. If you need to filter by the *assigned* category, see the roadmap note at the bottom of this doc.

### Operators per field type

| Type        | Operators                                       | Notes                                                      |
| ----------- | ----------------------------------------------- | ---------------------------------------------------------- |
| **string**  | `eq`, `neq`, `contains`, `not_contains`         | Case-insensitive                                           |
|             | `matches`                                       | RE2 regex; case-sensitive by default (`(?i)` for insensitive) |
|             | `in`                                            | Non-empty array; case-insensitive membership test          |
| **numeric** | `eq`, `neq`, `gt`, `gte`, `lt`, `lte`           | `float64` comparison                                       |
| **bool**    | `eq`, `neq`                                     | `true` / `false`                                           |
| **tags**    | `contains`, `not_contains`                      | `value` is a single tag slug; case-insensitive             |
|             | `in`                                            | `value` is an array of slugs; matches if any slug is present |

Unknown field or unknown op → condition evaluates to false (the rule simply won't match). Invalid regex or wrong value type → **rejected at write time**.

### Regex flavor

`matches` uses Go's [RE2](https://pkg.go.dev/regexp/syntax) — no backreferences, no lookaround, linear-time guaranteed. Patterns are not anchored automatically; use `^` and `$` if you want full-match semantics. Use `(?i)` for case-insensitive matching.

### Tag conditions and timing

The `tags` field reflects the transaction's current tag set. For a brand-new transaction (first sync), a rule's tag condition can only see tags added by **earlier-priority rules in the same sync pass** (see "Rule chaining" below). For a re-synced transaction, it sees the tags already persisted on the row.

## Actions

Actions describe what a matching rule does to the transaction. An action array must have at least one element.

### `set_category`

```json
{ "type": "set_category", "category_slug": "food_and_drink_groceries" }
```

Sets the transaction's assigned category. At most one `set_category` per rule.

- Skipped when `category_override = true` on the transaction (user lock).
- Writes a `category_set` annotation with the rule as actor.

### `add_tag`

```json
{ "type": "add_tag", "tag_slug": "needs-review" }
```

Adds a tag. The tag is auto-created with `lifecycle = persistent` if the slug doesn't exist yet. Idempotent — re-adding an existing tag is a no-op.

- Slug format: `^[a-z0-9][a-z0-9\-:]*[a-z0-9]$` (lowercase, digits, hyphens, colons; no leading/trailing punctuation).
- Writes a `tag_added` annotation; deduped against prior annotations of the same tag on the same transaction.

### `add_comment`

```json
{ "type": "add_comment", "content": "Auto-categorized by rule: Dining" }
```

Appends a comment authored by the rule. Accumulates — multiple rules can each add comments in one pass.

- Sync-only. Retroactive apply does **not** materialize `add_comment` actions (they're meant to narrate a specific sync event, not back-fill chatter).

### Combining actions

A rule can carry multiple actions of different types. Override (`category_override=true`) suppresses only the `set_category` part — `add_tag` and `add_comment` still fire.

```json
{
  "conditions": { "field": "merchant_name", "op": "contains", "value": "Uber" },
  "actions": [
    { "type": "set_category", "category_slug": "transportation_rideshare" },
    { "type": "add_tag", "tag_slug": "recurring" }
  ]
}
```

## Triggers

The `trigger` field controls when the rule runs during sync.

| Trigger     | Fires on new (first-synced) transactions | Fires on changed re-synced transactions |
| ----------- | :--------------------------------------: | :-------------------------------------: |
| `on_create` |                    ✅                    |                   ❌                    |
| `on_update` |                    ❌                    |                   ✅                    |
| `always`    |                    ✅                    |                   ✅                    |

A transaction is "changed" when the provider returned a different version of an existing row; a truly-unchanged re-sync runs no rules. Default trigger when omitted: `on_create`.

Retroactive apply (`apply_rules`) ignores trigger — it's a bulk operation intended to evaluate a rule's condition across the entire history regardless of when the transaction was ingested.

## Priority and rule ordering

`priority` is an integer (default `10`, range `0..1000`). Rules load and evaluate in `priority DESC, created_at DESC` order — higher-priority rules run first. Within the same priority, the most-recently-created rule wins.

For `set_category`, the **first rule to match** wins (higher priority = wins). For accumulator actions (`add_tag`, `add_comment`), every matching rule contributes.

`hit_count` increments on every condition match, regardless of whether the rule's action was suppressed (e.g. second `set_category` in the same pass).

## Expiry and enabled state

- `enabled = false` excludes the rule from both sync and retroactive paths.
- `expires_at` is checked at rule load. A rule that expires mid-sync stays in the in-memory snapshot for that run.

## Sync vs retroactive apply

The rule engine has two entry points. They share condition evaluation and priority ordering, but materialize actions differently:

| Aspect                   | Sync (`on_create`/`on_update`/`always`) | Retroactive (`apply_rules`)           |
| ------------------------ | --------------------------------------- | ------------------------------------- |
| Trigger honored?         | Yes                                     | No — runs regardless of trigger       |
| `set_category`           | Applied (respects override)             | Applied (respects override)           |
| `add_tag`                | Applied                                 | Applied                               |
| `add_comment`            | Applied                                 | **Not applied** (by design)           |
| `hit_count`              | +1 per condition match                  | +1 per condition match                |
| `rule_applied` annotation | Written                                | Written (with `applied_by = "retroactive"`) |

> *Historical note:* earlier versions of the engine skipped `add_tag` in the retroactive path as well. This was unified in Phase 2 of the rules polish project (2026-Q2).

## Preview

`preview_rule` evaluates a *single* rule's condition against stored transactions and returns the match count plus a sample. It does **not** simulate the full rule pipeline — higher-priority-stage rules that would normally fire first are not considered. Preview is for answering "what would this rule match right now?" — not "what would the sync outcome be?".

## Roadmap (not yet shipped)

- **Rule chaining.** Lower-priority-stage rules will see the effects of higher-priority-stage rules within a single sync pass (tags added, categories set). Currently, each rule evaluates against a static snapshot of the transaction.
- **Priority inversion to pipeline-stage semantics.** Ordering will flip to `priority ASC` (lower = earlier), with `set_category` becoming last-writer-wins. Outcome for existing rules is unchanged, but the mental model (and docs) changes to "pipeline stage."
- **`on_change` trigger.** `on_update` will be renamed to `on_change` with a DB-level alias for back-compat. Semantics unchanged.
- **New condition fields.** `category` (assigned category slug, distinct from `category_primary` raw provider field) and `account_name`.
- **New action.** `remove_tag`, symmetric with `add_tag`; useful for clearing transient tags like `needs-review` once a rule's conditions pre-categorize.

Until those ship, rules see the raw, pre-rule transaction state each time they evaluate. Compose via explicit conditions (e.g. `and` with all the required clauses) rather than relying on inter-rule state.

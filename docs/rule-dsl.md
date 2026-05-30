# Transaction Rule DSL

Canonical spec for the transaction rule DSL — the condition tree, action types, and trigger semantics used by both humans (admin UI) and agents (MCP). This is the source of truth; tool descriptions, admin form help, and tests should all agree with it.

## At a glance

A rule is a JSON document with four parts:

```json
{
  "name": "Dining out categorization",
  "conditions": { "field": "provider_merchant_name", "op": "contains", "value": "Starbucks" },
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
        { "field": "provider_merchant_name", "op": "contains", "value": "Starbucks" },
        { "field": "amount", "op": "gte", "value": 5 }
      ]
    },
    { "field": "provider_name", "op": "matches", "value": "^COFFEE.*" }
  ]
}
```

### Fields

| Field               | Type    | Source                                                          |
| ------------------- | ------- | --------------------------------------------------------------- |
| `provider_name`     | string  | Transaction name (raw provider)                                 |
| `provider_merchant_name` | string | Merchant display name (may be empty for CSV / no-enrich rows)   |
| `amount`            | numeric | Transaction amount (positive = debit, negative = credit)        |
| `provider_category_primary` | string | **Raw provider** primary category (Plaid/Teller classification) |
| `provider_category_detailed` | string | **Raw provider** detailed category                              |
| `category`          | string  | **Assigned** category slug (reflects Breadbox/rule/user writes) |
| `pending`           | bool    | Provider-reported pending flag                                  |
| `provider`          | string  | `plaid`, `teller`, or `csv`                                     |
| `account_id`        | string  | Account UUID                                                    |
| `account_name`      | string  | Account display name (or bank-reported name if unset)           |
| `user_id`           | string  | Family member UUID                                              |
| `user_name`         | string  | Family member display name                                      |
| `tags`              | tags    | List of current transaction tag slugs (special, see below)      |
| `series`            | string  | `short_id` of the recurring series the transaction belongs to (empty when unassigned) |
| `in_series`         | bool    | Whether the transaction is linked to any recurring series       |

> **Raw vs assigned category.** `provider_category_primary` / `provider_category_detailed` are the provider's classification — they don't change when Breadbox, a rule, or the user reassigns. Use `category` when you want to react to the *current* category, including mid-pass rule updates (see "Rule chaining" below).

> **Series membership timing.** `series` / `in_series` reflect a transaction's *current* `series_id`. A brand-new transaction has no series until the post-sync detector links it, so these fields are `""` / `false` on the create pass — they're most useful for re-synced/changed rows and for **retroactive apply** over historical data (e.g. tag everything already in a series, or exclude series members from a catch-all rule). They are the read-half companion to the `assign_series` action (the write-half). A rule **matches** on series membership but cannot **discover** a series — detection is an aggregate decision the detector owns; see `docs/data-model.md` and the recurring-series design notes.

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

### Rule chaining (tags + category)

Rules evaluate in pipeline order — lower `priority` runs first. As each matching rule applies its actions, a local mutable copy of the transaction context updates and feeds into subsequent rules' condition evaluation:

- `add_tag` appends to the tag slice that `tags contains / not_contains / in` reads from.
- `set_category` updates the slug that `field="category"` reads from.

This enables composition. For example, a pipeline of three rules:

1. `priority: 0`, `provider_name contains "starbucks"` → `add_tag: coffee`
2. `priority: 10`, `tags contains "coffee"` → `set_category: food_and_drink_coffee`
3. `priority: 50`, `category eq "food_and_drink_coffee"` → `add_tag: dining`

Rule 2 sees rule 1's tag. Rule 3 sees rule 2's category. All three fire in a single sync pass; the engine emits the combined `category_set`, `tag_added` (`coffee`, `dining`) annotations atomically.

The `tags` slice starts from tags already persisted on the row (loaded during sync for re-synced transactions; empty for brand-new ones) and the `category` slug starts from the transaction's currently assigned category (or empty if none yet).

Mutations are scoped to the resolver run — the caller's `TransactionContext` (and the incoming tag slice) are not modified.

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

Adds a tag. The tag is auto-created if the slug doesn't exist yet. Idempotent — re-adding an existing tag is a no-op.

- Slug format: `^[a-z0-9][a-z0-9\-:]*[a-z0-9]$` (lowercase, digits, hyphens, colons; no leading/trailing punctuation).
- Writes a `tag_added` annotation; deduped against prior annotations of the same tag on the same transaction.

### `remove_tag`

```json
{ "type": "remove_tag", "tag_slug": "needs-review" }
```

Removes a tag from the transaction. No-op if the tag isn't attached. Slug validation matches `add_tag`.

- Writes a `tag_removed` annotation. The rule's name is captured in the annotation payload as the removal note so the activity timeline carries a source attribution.
- **Net-diff semantics in a pipeline.** If an earlier-stage rule's `add_tag` and a later-stage rule's `remove_tag` target the same slug in a single sync pass, they cancel — neither the INSERT nor the DELETE hits the DB, and no annotations are emitted. This keeps the timeline clean when rules compose.

### `add_comment`

```json
{ "type": "add_comment", "content": "Auto-categorized by rule: Dining" }
```

Appends a comment authored by the rule. Accumulates — multiple rules can each add comments in one pass.

- Sync-only. Retroactive apply does **not** materialize `add_comment` actions (they're meant to narrate a specific sync event, not back-fill chatter).

### `assign_series`

```json
{ "type": "assign_series", "merchant_key": "spotify", "create_if_missing": true }
```

Links the matching transaction to a recurring series (subscription). Provide **exactly one** of:

- `series_short_id` — assign to an existing series by its short ID. Validated at rule-create time (must resolve).
- `merchant_key` + `create_if_missing: true` — mint a household series keyed on `merchant_key` if one doesn't already exist at that signature, then assign.

Behavior:

- Materializes **inside the sync transaction** (resolve-or-mint → back-link → recompute rollups), so the link commits atomically with the rest of the sync.
- **Link-and-rollup only** — it never overwrites a detector-snapped cadence or `detection_signals`, and it back-links NULL-fill only (never steals a charge already in another series).
- Honors **sticky-reject**: minting at a `rejected` signature is a no-op (a rule can't resurrect a series the user dismissed).
- Last-writer-wins across a pipeline: a higher-priority rule's `assign_series` overrides a lower one (a transaction joins at most one series).
- **Retroactive apply** is supported via single-rule apply (`POST /rules/{id}/apply` / `apply_rules` with a `rule_id`): every matching existing transaction is linked using the same resolve-or-mint materialization as the sync path. (The bulk *apply-all* path does not yet materialize `assign_series` — apply the rule individually.)

This is the declarative counterpart to the `assign_series` MCP tool: author the rule once and every future matching charge auto-joins the series with zero agent runs.

### Combining actions

A rule can carry multiple actions of different types. Override (`category_override=true`) suppresses only the `set_category` part — `add_tag` and `add_comment` still fire.

```json
{
  "conditions": { "field": "provider_merchant_name", "op": "contains", "value": "Uber" },
  "actions": [
    { "type": "set_category", "category_slug": "transportation_rideshare" },
    { "type": "add_tag", "tag_slug": "recurring" }
  ]
}
```

#### Which combinations make sense

Only `set_category` is singleton per rule — repeating it is rejected at write time. `add_tag`, `remove_tag`, and `add_comment` can appear multiple times in one rule (e.g. add two tags at once, or add one and remove another). The admin UI disables a second `set_category` dropdown option once one is picked; tag and comment rows are freely repeatable.

Useful combinations:

| Actions | Use case |
| --- | --- |
| `set_category` alone | Straightforward reclassification (e.g. `Uber` → `Transportation > Rideshare`). |
| `set_category` + `add_tag` | Reclassify and annotate simultaneously (e.g. `Uber` → `Transportation > Rideshare` + `recurring`). |
| `add_tag` alone | Add a tag without touching category. Safe pairing with override-protected transactions. |
| `remove_tag` alone | Clean up a tag a prior rule or agent added (e.g. remove `needs-review` once a condition proves the transaction is benign). |
| `add_tag` + `remove_tag` (different slugs) | Transition a transaction between tags (e.g. add `reviewed`, remove `needs-review`). |
| `set_category` + `add_comment` | Reclassify and explain why — useful for audit trails. |

Combinations to avoid:

- **Same-slug `add_tag` + `remove_tag`** — cancels out under net-diff semantics. The admin UI flags this with an inline warning.
- **`set_category` with no conditions** — match-all + reclassify will stomp every transaction on every sync. The form shows an "All transactions" banner for any match-all rule; double-check before saving.
- **`add_comment` on `always` trigger** — fires on every sync, accumulating duplicate comment annotations. Prefer `on_create` or a narrower condition.

See `docs/data-model.md` §annotations for how each action materializes into the timeline, and `internal/sync/engine.go applyRulesToTransaction` for the sync-side ordering guarantees.

## Triggers

The `trigger` field controls when the rule runs during sync.

| Trigger     | Fires on new (first-synced) transactions | Fires on changed re-synced transactions |
| ----------- | :--------------------------------------: | :-------------------------------------: |
| `on_create` |                    ✅                    |                   ❌                    |
| `on_change` |                    ❌                    |                   ✅                    |
| `always`    |                    ✅                    |                   ✅                    |

A transaction is "changed" when the provider returned a different version of an existing row; a truly-unchanged re-sync runs no rules. Default trigger when omitted: `on_create`.

> **Legacy alias.** `on_update` is accepted as a synonym for `on_change` in all inputs (admin UI, MCP, REST). The service normalizes it to `on_change` on write. Pre-existing rows stored as `on_update` continue to fire — the sync resolver treats both values identically.

Retroactive apply (`apply_rules`) ignores trigger — it's a bulk operation intended to evaluate a rule's condition across the entire history regardless of when the transaction was ingested.

## Priority as pipeline stage

`priority` is an integer pipeline stage (default `10`, range `0..1000`). Rules load and evaluate in `priority ASC, created_at ASC` order — **lower priority runs first**. Think of priority as the stage number in a pipeline:

| Stage name   | Priority | Meaning                                                       |
| ------------ | -------- | ------------------------------------------------------------- |
| `baseline`   | `0`      | Foundation — system defaults, broad classifications           |
| `standard`   | `10`     | Default rule stage                                            |
| `refinement` | `50`     | Reacts to baseline/standard output                            |
| `override`   | `100`    | Has the final say for `set_category`                          |

For `set_category`, the **last rule to match wins** (higher-priority stage has final say). For accumulator actions (`add_tag`, `add_comment`), every matching rule contributes.

`hit_count` increments on every condition match, regardless of whether the rule's action was ultimately superseded by a later stage.

### Stage vs priority in API inputs

`create_transaction_rule`, `update_transaction_rule`, and `batch_create_rules` (both MCP and REST) accept a semantic `stage` string alongside the raw `priority` integer. Agents should prefer `stage` so rules from different sources compose predictably on the same shared values.

- Supply `stage` (`"baseline"` | `"standard"` | `"refinement"` | `"override"`) — resolves to `0 / 10 / 50 / 100`.
- Supply raw `priority` — used as-is. Useful for fine-grained ordering inside a stage.
- Supply both — **`priority` wins**. `stage` is effectively a hint in that case.
- Supply neither — defaults to `standard` (`10`).
- Unknown stage values return a `VALIDATION_ERROR` (`invalid stage "foo" (expected baseline|standard|refinement|override)`).

Stage names are case-insensitive and whitespace-trimmed on input.

> *Historical note:* before April 2026, rules evaluated in `priority DESC` order with first-writer-wins `set_category`. The inversion to pipeline-stage semantics preserves "higher priority wins set_category" in meaning, but the mechanism changes from "speaks first" to "speaks last." Outcomes for pre-flip rules are unchanged (the winner of a conflict is the same rule either way) — only the mental model shifts.

## Expiry and enabled state

- `enabled = false` excludes the rule from both sync and retroactive paths.
- `expires_at` is checked at rule load. A rule that expires mid-sync stays in the in-memory snapshot for that run.

## Sync vs retroactive apply

The rule engine has two entry points. They share condition evaluation and priority ordering, but materialize actions differently:

| Aspect                    | Sync (`on_create`/`on_change`/`always`) | Retroactive (`apply_rules`)                 |
| ------------------------- | --------------------------------------- | ------------------------------------------- |
| Trigger honored?          | Yes                                     | No — runs regardless of trigger             |
| `set_category`            | Applied (respects override)             | Applied (respects override)                 |
| `add_tag`                 | Applied                                 | Applied                                     |
| `remove_tag`              | Applied                                 | Applied                                     |
| `add_comment`             | Applied                                 | **Not applied** (by design)                 |
| `hit_count`               | +1 per condition match                  | +1 per condition match                      |
| `rule_applied` annotation | Written                                 | Written (with `applied_by = "retroactive"`) |

**Why `add_comment` is sync-only.** Comments narrate a specific sync event ("auto-categorized during 2026-04-15 sync"). Materializing them retroactively would either date-warp ("auto-categorized during retroactive back-fill on <today>") or duplicate boilerplate across every matched row. Neither is useful; sync-time remains the only place where a rule adds comments.

**Chaining in retroactive.** `apply_rules` (all-rules bulk path) applies the same pipeline-stage chaining as sync: earlier-stage rules' tags and category assignments feed later-stage rules' conditions for each matched transaction. Single-rule retroactive (`apply_rules` with `rule_id`) evaluates just that one rule in isolation — no other rules contribute.

## Preview

`preview_rule` evaluates a *single* rule's condition against stored transactions and returns the match count plus a sample. It does **not** simulate the full rule pipeline — higher-priority-stage rules that would normally fire first are not considered. Preview is for answering "what would this rule match right now?" — not "what would the sync outcome be?".

## Roadmap

Phases 1 and 2 have shipped. Upcoming work:

- **Admin UI polish.** Live preview in the rule form, priority-stage presets ("Baseline / Standard / Refinement / Override"), retroactive-apply confirmation modal, first-class `remove_tag` UI (currently reuses the add-tag input).
- **Correctness sweep.** `rule_applied` annotation fires only on persistence side-effects; deleted-category warnings; belt-and-suspenders slug validation at sync time.

Tag-based chaining is already live in the resolver. The remaining roadmap items polish the surface.

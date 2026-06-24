# Transaction Rule DSL

Canonical spec for the transaction rule DSL — the condition tree, action types, and trigger semantics used by both humans (admin UI) and agents (MCP). This is the source of truth; tool descriptions, admin form help, and tests should all agree with it.

> **Governing doctrine.** Rules are breadbox's single deterministic enrichment layer. The *why* — provider data is immutable, intelligence accrues as rules, and rules must match raw immutable fields to stay drift-proof — lives in the **"Operating Model — the reconciliation flywheel"** section of the root `CLAUDE.md` (read first), with the full design + rollout in Obsidian `planned-features/rules-as-universal-substrate.md` and `rules-substrate-implementation-roadmap.md`.

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
| `day_of_month`      | numeric | Day component of the posting `date`, `1`–`31`. Supports the cyclic/clamped `approx` (see "Date-part conditions"). |
| `month`             | numeric | Month of the posting `date`, `1`–`12`                           |
| `day_of_week`       | numeric | Weekday of the posting `date`, `0`=Sunday … `6`=Saturday        |
| `day_of_year`       | numeric | Ordinal day of the posting `date`, `1`–`366`                    |
| `metadata.<key>`    | metadata | One key from the transaction's free-form metadata blob — e.g. `metadata.tax_deductible` reads `metadata["tax_deductible"]`. See "Metadata conditions" below. |

> **Raw vs assigned category.** `provider_category_primary` / `provider_category_detailed` are the provider's classification — they don't change when Breadbox, a rule, or the user reassigns. Use `category` when you want to react to the *current* category, including mid-pass rule updates (see "Rule chaining" below).

> **Series membership timing.** `series` / `in_series` reflect a transaction's *current* `series_id`. A brand-new transaction has no series until an `assign_series` rule (or an agent) links it, so these fields are `""` / `false` until a rule fires — they're the read-half companion to the `assign_series` action (the write-half), most useful for re-synced/changed rows and for **retroactive apply** over historical data (e.g. tag everything already in a series, or exclude series members from a catch-all rule). There is no detector: a series' membership is exactly the set of charges its `assign_series` rules match. See `docs/data-model.md` and the `assign_series` action below.

### Match-stability contract — what to condition on

This is the **authoritative** stability classification of every matchable field. It is the doctrinal heart of the rules-as-substrate model (see the **Governing doctrine** note at the top of this doc): a rule is durable only when it matches the parts of a transaction that *don't move*. Every field below is tagged by how stable its value is across renames, re-syncs, and prior-stage rule writes.

The classes:

- **raw-immutable** — verbatim provider data. Breadbox never rewrites it; it changes only if the provider re-reports the transaction. A condition on it means the same thing on the create pass, on every re-sync, and on retroactive apply. **This is the substrate. Author here.**
- **stable-surrogate** — a Breadbox-assigned `id` (UUID) that is stable for the life of the entity. Not provider data, but it doesn't drift when a user renames the entity, so it's a safe primary match. **Also safe.**
- **mutable-display** — a human-facing label or a value a prior rule / the user / an agent can change. It silently *breaks* when someone renames the thing (`account_name`, `user_name`), or its truth *depends on pipeline order* (`category`, `tags`, `series`, `in_series` — set by earlier-stage rules in the same pass). **Avoid as a primary match condition.**
- **stable-derived** — a value *computed* deterministically from a raw-immutable column, carrying no independent state. The date-parts (`day_of_month`, `month`, `day_of_week`, `day_of_year`) are pure functions of the immutable, tz-naive `date`, so a condition on them is as durable as one on `amount`. **Safe to author on** (this is the backbone of recurrence rules).

| Field                          | Type     | Class                | Why                                                                                       |
| ------------------------------ | -------- | -------------------- | ----------------------------------------------------------------------------------------- |
| `provider_name`                | string   | **raw-immutable**    | Provider's raw transaction name (`TransactionContext.Name`)                               |
| `provider_merchant_name`       | string   | **raw-immutable**    | Provider's raw merchant string (`MerchantName`); may be empty                             |
| `amount`                       | numeric  | **raw-immutable**    | Provider-reported amount                                                                   |
| `pending`                      | bool     | **raw-immutable**    | Provider-reported pending flag                                                             |
| `provider`                     | string   | **raw-immutable**    | Source system (`plaid`, `teller`, `simplefin`, `csv`)                                      |
| `provider_category_primary`    | string   | **raw-immutable**    | Provider's primary category classification — never rewritten by Breadbox                   |
| `provider_category_detailed`   | string   | **raw-immutable**    | Provider's detailed category classification — never rewritten by Breadbox                  |
| `account_id`                   | string   | **stable-surrogate** | Account UUID — stable across account renames                                              |
| `user_id`                      | string   | **stable-surrogate** | Family-member UUID — stable across member renames                                         |
| `account_name`                 | string   | **mutable-display**  | Display name; **breaks silently** when the account is renamed                             |
| `user_name`                    | string   | **mutable-display**  | Display name; **breaks silently** when the member is renamed                              |
| `category`                     | string   | **mutable-display**  | *Assigned* slug; mutates mid-pass as earlier `set_category` rules fire (pipeline-ordered)  |
| `tags`                         | tags     | **mutable-display**  | Current tag slugs; mutate mid-pass as earlier `add_tag`/`remove_tag` rules fire            |
| `series`                       | string   | **mutable-display**  | Current series `short_id`; set by `assign_series` (a rule write), empty until then         |
| `in_series`                    | bool     | **mutable-display**  | Whether a series link exists; same pipeline/timing caveat as `series`                      |
| `day_of_month`                 | numeric  | **stable-derived**   | Day-of-month of the immutable tz-naive `date`; pure function, no independent state         |
| `month`                        | numeric  | **stable-derived**   | Month of the immutable `date`                                                              |
| `day_of_week`                  | numeric  | **stable-derived**   | Weekday of the immutable `date` (`0`=Sun)                                                  |
| `day_of_year`                  | numeric  | **stable-derived**   | Ordinal day of the immutable `date`                                                        |
| `metadata.<key>`               | metadata | **mutable-display**  | Free-form blob a user / agent / rule writes; no stability guarantee                        |

> **The contract.** *Author conditions on **raw-immutable** + **stable-surrogate** fields.* These are the immutable provider substrate (and stable surrogates of it) the whole model rests on — a rule built on them resolves identically on the create pass, on every future re-sync, and on retroactive apply over history.
>
> **Mutable-display fields silently break or depend on pipeline order — avoid them as primary match conditions.** A rule keyed on `account_name` stops matching the moment the user renames that account (the surrogate `account_id` would not). A rule keyed on `category`, `tags`, `series`, or `in_series` is reacting to *another rule's output within the same pass*, so whether it matches depends entirely on rule priority ordering — that's **chaining**, a deliberate composition tool (see [Rule chaining](#rule-chaining-tags--category)), not a stable predicate on the transaction itself. Use them when you *intend* to react to a prior stage's result; never as the load-bearing condition that decides whether the rule fires at all.

### Operators per field type

| Type        | Operators                                       | Notes                                                      |
| ----------- | ----------------------------------------------- | ---------------------------------------------------------- |
| **string**  | `eq`, `neq`, `contains`, `not_contains`         | Case-insensitive                                           |
|             | `matches`                                       | RE2 regex; case-sensitive by default (`(?i)` for insensitive) |
|             | `in`                                            | Non-empty array; case-insensitive membership test          |
| **numeric** | `eq`, `neq`, `gt`, `gte`, `lt`, `lte`           | `float64` comparison                                       |
|             | `approx`                                        | `value ± tolerance`: matches when `abs(actual - value) ≤ tolerance`. Requires a sibling `tolerance ≥ 0`. For `day_of_month` the comparison is **cyclic + clamped** (see "Date-part conditions"). |
|             | `between`                                       | `min ≤ actual ≤ max` (inclusive). Requires sibling `min` and `max` (with `min ≤ max`); `value` is ignored. |
| **bool**    | `eq`, `neq`                                     | `true` / `false`                                           |
| **tags**    | `contains`, `not_contains`                      | `value` is a single tag slug; case-insensitive             |
|             | `in`                                            | `value` is an array of slugs; matches if any slug is present |
| **metadata** | `eq`, `neq`, `contains`, `not_contains`, `matches`, `in` | string-style ops on the stringified value (`contains`/`matches` case-insensitive / RE2) |
|             | `gt`, `gte`, `lt`, `lte`                         | numeric comparison; both sides must parse as numbers       |
|             | `exists`, `not_exists`                           | key-presence test; `value` is ignored                      |

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

### Metadata conditions

Transactions carry a free-form JSONB `metadata` blob — arbitrary key/value enrichment your household cares about that isn't a first-class field (`tax_deductible`, `trip`, `reimbursable_by`, …). A rule reads one key per leaf via the dotted field `metadata.<key>`:

```json
{ "field": "metadata.tax_deductible", "op": "eq", "value": true }
{ "field": "metadata.trip", "op": "eq", "value": "japan-2026" }
{ "field": "metadata.reimburse_amount", "op": "gte", "value": 100 }
{ "field": "metadata.notes", "op": "contains", "value": "warranty" }
{ "field": "metadata.project_code", "op": "exists" }
```

The key (everything after `metadata.`) must be a valid metadata key — non-empty, ≤128 chars. Metadata keys are **case-sensitive** (they're JSONB keys), unlike tag slugs.

**Semantics:**

- `exists` / `not_exists` test key *presence* and ignore `value`.
- **Every other operator requires the key to be present.** An absent key matches only `not_exists` — so `metadata.foo neq "x"` does **not** match a transaction that simply has no `foo` key (use `not_exists`, or an `or` of the two, if you want "missing OR different"). This avoids a `neq`/`not_contains` rule silently matching every transaction that lacks the key.
- `eq` / `neq` pick their comparison from the **expected value's type**: a boolean expected compares as bool, a numeric expected compares numerically, otherwise the stored value is stringified and compared case-insensitively. So `value: true` matches stored `true` (and the string `"true"`); `value: 100` matches stored `100` or `"100"`.
- `contains` / `not_contains` / `matches` / `in` always operate on the **stringified** stored value (numbers → their decimal form, booleans → `true`/`false`, objects/arrays → their JSON encoding), so a `matches` can pattern-match inside a structured value.
- `gt` / `gte` / `lt` / `lte` require both the stored value and the expected value to parse as numbers; otherwise the leaf is false.

Metadata conditions evaluate identically at sync time, in `preview_rule`, and during retroactive apply. They also chain: a `set_metadata` action by an earlier-stage rule is visible to a later-stage rule's `metadata.<key>` condition within the same pass (see "Rule chaining" above and the `set_metadata` action below).

### Amount tolerance (`approx` / `between`)

Two numeric operators express "near a value" without a pile of `gte`/`lte` pairs — the building block for recurring-charge rules:

```json
{ "field": "amount", "op": "approx", "value": 15.49, "tolerance": 0.50 }
{ "field": "amount", "op": "between", "min": 9.99, "max": 19.99 }
```

- `approx` matches when `abs(amount - value) ≤ tolerance`. The `tolerance` sibling is **required and must be ≥ 0**.
- `between` matches when `min ≤ amount ≤ max` (both ends inclusive). Both `min` and `max` are **required**, with `min ≤ max`; the `value` field is ignored.
- Both work on every numeric field — `amount` and the date-parts below.

### Date-part conditions

Four **numeric** fields are derived from the transaction's posting `date` — the **tz-naive, provider-localized** date column. There is deliberately **no timezone math**: the date is taken exactly as the provider reported it, so a charge "on the 14th" is the 14th in the provider's locale regardless of where the server runs.

| Field          | Range          | Notes                                  |
| -------------- | -------------- | -------------------------------------- |
| `day_of_month` | `1`–`31`       | Cyclic + clamped under `approx` (below) |
| `month`        | `1`–`12`       | January = 1                            |
| `day_of_week`  | `0`–`6`        | `0` = Sunday … `6` = Saturday          |
| `day_of_year`  | `1`–`366`      | Ordinal day; `366` only on leap years  |

They support the full numeric operator set: `eq` / `neq` / `gt` / `gte` / `lt` / `lte`, plus `between` and `approx`. A transaction with no date never matches a date-part condition.

**`day_of_month` `approx` is cyclic and clamped** — this is what makes "around the 1st" and "the 31st" behave sanely:

- **Cyclic:** the distance wraps around the month, so the 1st and the month's last day are **1 day apart**. `day_of_month approx 1 ± 2` therefore matches the 30th/31st of the previous-style cycle as well as the 1st–3rd.
- **Clamped:** a target past the month's actual length collapses to the **last day** of that month. So `day_of_month approx 31 ± 0` matches **Feb 28** (or **Feb 29** in a leap year), April 30, etc. — "bill me on the 31st" still fires in short months.

Both the clamp boundary and the wrap distance use the transaction's **own** month length (28/29/30/31), so the same rule behaves correctly across every month and across leap years.

```json
{ "field": "day_of_month", "op": "approx", "value": 14, "tolerance": 3 }
```

> **Annual cadence — use `month` + `day_of_month`, not `day_of_year`.** `day_of_year` drifts by one after February in leap years (day 60 is Mar 1 in a common year but Feb 29 in a leap year), so an annual rule keyed on `day_of_year` silently misses every fourth year. Express a yearly charge as a conjunction instead, which is leap-robust:
>
> ```json
> { "and": [
>   { "field": "month", "op": "eq", "value": 4 },
>   { "field": "day_of_month", "op": "approx", "value": 15, "tolerance": 2 }
> ] }
> ```

Date-part conditions evaluate identically at sync time, in `preview_rule`, and during retroactive apply (all three read the same tz-naive `date`).

A full recurring-charge rule combines amount tolerance with a date-part — the detector-free way to capture a subscription:

```json
{ "and": [
  { "field": "amount", "op": "approx", "value": 15.49, "tolerance": 0.50 },
  { "field": "day_of_month", "op": "approx", "value": 14, "tolerance": 3 }
] }
```

## Actions

Actions describe what a matching rule does to the transaction. An action array must have at least one element.

### `set_category`

```json
{ "type": "set_category", "category_slug": "food_and_drink_groceries" }
```

Sets the transaction's assigned category. At most one `set_category` per rule.

- Writes `category_id` directly — last-writer-wins. Provenance/precedence was removed in the rules-substrate sprint (P3); rules, agents, and users all write the same field. Rules only run on new or changed transactions, so a user's manual edit on an unchanged row is not continuously re-clobbered.
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
{ "type": "assign_series", "series_name": "Spotify", "create_if_missing": true }
```

Links the matching transaction to a recurring series. Provide **exactly one** of:

- `series_short_id` — assign to an existing series by its short ID. Validated at rule-create time (must resolve).
- `series_name` + `create_if_missing: true` — mint a series by name (surrogate-first) if one doesn't already exist with that live name, then assign. The same name always resolves the same series.

Behavior:

- Materializes **inside the sync transaction** (resolve-or-mint by name/short_id → back-link), so the link commits atomically with the rest of the sync.
- **Link only** — back-links NULL-fill only (never steals a charge already in another series). A series is a thin entity (name + type); there is no detector, cadence, or `detection_signals` to overwrite.
- Last-writer-wins across a pipeline: a higher-priority rule's `assign_series` overrides a lower one (a transaction joins at most one series).
- **Retroactive apply** is supported via single-rule apply (`POST /rules/{id}/apply` / `apply_rules` with a `rule_id`): every matching existing transaction is linked using the same resolve-or-mint materialization as the sync path. (The bulk *apply-all* path does not yet materialize `assign_series` — apply the rule individually.)

This is the declarative counterpart to the `assign_series` MCP tool: author the rule once and every future matching charge auto-joins the series with zero agent runs.

**A series IS its governing rules.** Because membership comes only from `assign_series` rules (plus first-class agent one-off assigns), the rules that target a series _are_ its durable definition. The admin series detail page (`/recurring/{short_id}`) makes this explicit: it shows the linked charges beside the **governing rules** — every rule whose `assign_series` action points at the series (by `series_short_id` or `series_name`) — each linking to the rule editor. `Service.ListGoverningRules` powers that view.

### `set_metadata`

```json
{ "type": "set_metadata", "metadata_key": "tax_deductible", "metadata_value": true }
```

Upserts **one key** in the transaction's free-form metadata blob, leaving every other key untouched. `metadata_value` may be any JSON value (string, number, boolean, object, array). This is the declarative counterpart to the `set_transaction_metadata` MCP tool.

- `metadata_key` must be non-empty and ≤128 chars; `metadata_value` must serialize to ≤4 KiB (the same per-value cap the scoped metadata ops enforce). Both checked at write time.
- Repeatable — a rule may set several keys at once. **Last-writer-wins per key** across the pipeline (a higher-priority rule's `set_metadata` for the same key overrides a lower one).
- Chains: the write is mirrored into the live context, so a later-stage rule's `metadata.<key>` condition observes it within the same pass.
- Materializes inside the sync transaction (and on retroactive apply) via a single `metadata = (metadata - removed) || set` merge. **No dedicated timeline annotation** is emitted (same as `assign_series`); the rule's `hit_count` records the firing.

### `remove_metadata`

```json
{ "type": "remove_metadata", "metadata_key": "needs_receipt" }
```

Deletes **one key** from the metadata blob. No-op if the key isn't present. Repeatable.

- **Net-diff with `set_metadata`** in a single pass: if an earlier-stage rule sets a key and a later-stage rule removes the same key, they cancel — neither hits the DB. A remove-then-set ends as a set. Mirrors `add_tag` / `remove_tag` net-diff semantics.

### `flag`

```json
{ "type": "flag" }
```

Surfaces a matching transaction for human attention by setting `transactions.flagged_at = NOW()`. Takes no parameters. This is the declarative counterpart to the `flag_transaction` MCP tool (minus the tool's optional comment `reason`, which is a per-call affordance). Retrieve flagged rows with `query_transactions(flagged=true)`.

- **Last-writer-wins** across the pipeline: a higher-priority rule's `flag` / `unflag` overrides a lower one (a transaction is flagged or it isn't). Within one rule a `flag` + `unflag` pair resolves to whichever is last.
- Materializes inside the sync transaction (and on retroactive apply). **No dedicated timeline annotation** is emitted (same as `assign_series` / `set_metadata`); the rule's `hit_count` and the `rule_applied` audit record the firing.
- Re-flagging an already-flagged row refreshes `flagged_at` (matches the `flag_transaction` tool).

### `unflag`

```json
{ "type": "unflag" }
```

Clears the flag (`flagged_at = NULL`) on a matching transaction. Takes no parameters. No-op-safe on an already-unflagged row. Use it to auto-retire flags once a follow-up rule's condition no longer holds, or to clear flags the agent raised after review.

### Combining actions

A rule can carry multiple actions of different types; they all fire together. There is no per-action suppression — `set_category` is last-writer-wins with no provenance guard.

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

Only `set_category` is singleton per rule — repeating it is rejected at write time. `add_tag`, `remove_tag`, `add_comment`, `set_metadata`, and `remove_metadata` can appear multiple times in one rule (e.g. add two tags at once, or write two metadata keys). `flag` / `unflag` carry no value, so a second copy is meaningless — the admin UI treats them as singleton (disables the dropdown option once picked) and warns if you add both. The admin UI also disables a second `set_category` dropdown option once one is picked; tag and comment rows are freely repeatable.

Useful combinations:

| Actions | Use case |
| --- | --- |
| `set_category` alone | Straightforward reclassification (e.g. `Uber` → `Transportation > Rideshare`). |
| `set_category` + `add_tag` | Reclassify and annotate simultaneously (e.g. `Uber` → `Transportation > Rideshare` + `recurring`). |
| `add_tag` alone | Add a tag without touching category. |
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
| `set_category`            | Applied (last-writer-wins)              | Applied (last-writer-wins)                  |
| `add_tag`                 | Applied                                 | Applied                                     |
| `remove_tag`              | Applied                                 | Applied                                     |
| `set_metadata`            | Applied                                 | Applied                                     |
| `remove_metadata`         | Applied                                 | Applied                                     |
| `assign_series`           | Applied                                 | Applied                                     |
| `flag`                    | Applied (sets `flagged_at`)             | Applied (sets `flagged_at`)                 |
| `unflag`                  | Applied (clears `flagged_at`)           | Applied (clears `flagged_at`)               |
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

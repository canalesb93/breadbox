---
title: Rules Curriculum
description: When to codify a pattern as a rule vs make a one-off edit, and how to author durable rules
icon: graduation-cap
---

## Rules are how Breadbox remembers

You are a reviewer, but rules are the one thing you teach the system. Every
decision you make is either a **one-off edit** (you fix this transaction and
move on) or a **rule** (you encode the pattern once, and every future sync
resolves it for you — with zero agent runs). A rule is durable memory: each
run you leave behind should make the next sync a little smarter, so the queue
shrinks over time instead of refilling.

Rules are the **single substrate** you write to. Categorization, recurring
series, tags, flags, and metadata are all just rule actions. Learn this one
mechanism and you can automate the whole review loop.

> The grammar — every field, operator, and action shape — lives in
> `get_reference(kind=rule-dsl)`. Read it whenever you author a rule. This block
> teaches the **decision framework**: *when* to write a rule and *which
> fields* to write it on. It does not repeat the grammar.

## Codify vs one-off — the core heuristic

Ask one question: **is this a recurring pattern, or a true exception?**

- **One-off edit** — the transaction is a genuine exception: a one-time
  refund, a miscoded charge that won't repeat, a split you're correcting by
  hand. Fix it directly (`update_transactions`, `assign_series`, `flag`, …)
  and move on. Don't author a rule for something you'll never see again.
- **Author a rule** — the same pattern has appeared **2+ times and you expect
  it again**: the same merchant, the same recurring amount on the same
  cadence, the same provider category that always maps to one of yours. The
  moment you find yourself making the *same* one-off edit twice, that's the
  signal — promote it to a rule so you never make it a third time.

When in doubt, prefer a one-off. A wrong one-off mis-codes a single row; a
wrong rule silently mis-codes every future match. Precision over coverage.

## The 3-step author workflow

Never create a rule blind. Always:

1. **`find_matching_rules`** — dedup first. Pass `merchant="<name>"` (or
   `transaction_id=...` to evaluate every condition field against a specific
   row). If an existing rule already covers the pattern, you're done — do NOT
   author a near-duplicate. With hundreds of rules, this targeted check beats
   dumping the whole set with `list_transaction_rules` and scanning by hand.
2. **`preview_rule`** — dry-run your conditions. It reports how many
   transactions would match and surfaces a sample. Reject anything that
   over-matches, fights a category a human already set, or matches zero rows
   (a typo in the condition).
3. **`create_transaction_rule`** — pass a `rules` array (one element, or
   several related rules in one call, max 100). Only create what previewed clean.

## Stable-field doctrine

A rule is only as durable as the fields it matches on. Author conditions
**exclusively on raw, immutable facts** — values the bank feed stamped and
nothing in Breadbox rewrites:

- `provider_name` — raw transaction name from the feed.
- `provider_merchant_name` — Plaid's structured merchant (prefer it when present).
- `amount` — signed number; the backbone of recurring-charge rules.
- `provider_category_primary` / `provider_category_detailed` — the provider's
  own category.
- Stable date-parts derived from the immutable posting date:
  `day_of_month`, `month`, `day_of_week`, `day_of_year`.

**Never author a condition on a mutable display field** — a value that
Breadbox (or a human, or another rule) can later change. Matching on these
makes a rule that fights itself or silently stops firing:

- `account_name`, `user_name` — display labels, renameable.
- `category` — *output* of categorization; a rule keyed on it chases its own tail.
- `tags`, `series`, `in_series` — set by rules/agents; not bank facts.
- `counterparty`, `has_counterparty` — set by `assign_counterparty` rules/agents; not bank facts.

The date-parts are the one safe *derived* exception: they're pure functions of
the immutable date, so a condition on them is as durable as one on `amount`.

## The recurrence idiom — replacing the detector

There is no subscription "detector." A recurring series **is exactly the set
of charges its `assign_series` rules match.** When you spot a subscription or
recurring bill, encode it as an amount-near-a-value condition ANDed with a
date-part, targeting `assign_series`:

```jsonc
// "Spotify, ~$15.49 around the 14th → join the Spotify series"
{
  "and": [
    { "field": "amount",       "op": "approx", "value": 15.49, "tolerance": 0.50 },
    { "field": "day_of_month", "op": "approx", "value": 14,    "tolerance": 3 }
  ]
}
// action: { "type": "assign_series", "series_name": "Spotify", "create_if_missing": true }
```

For an **annual** charge, key on `month` + `day_of_month` (never `day_of_year`
— it drifts by a day after February in leap years and silently misses every
fourth year):

```jsonc
{
  "and": [
    { "field": "month",        "op": "eq",     "value": 4 },
    { "field": "day_of_month", "op": "approx", "value": 15, "tolerance": 2 }
  ]
}
```

Anchor on the merchant too (`provider_merchant_name contains "…"`) when the
amount alone is ambiguous. Author the rule once and every future matching
charge auto-joins the series — no detector, no re-run.

## Writes are provenance-free — last-writer-wins

Breadbox does **not** track who set a value, and there are no locks or
override flags. A rule's `set_category` (and every other action) simply writes
the value: last writer wins. You don't need to reason about precedence.

This is safe because **rules only run on new or changed transactions.** A
user's manual edit on an existing, unchanged row is never re-clobbered by a
later sync — the rule never re-fires on it. So you can author freely without
fear of stomping a human's correction on settled history.

## Action catalog (summary)

Every action you might put on a rule — the same effects you can also apply
one-off via `update_transactions` (category, tags, comment, metadata, flag),
`assign_series`, and `assign_counterparty`. See `get_reference(kind=rule-dsl)`
for exact shapes.

| Action | What it does |
|---|---|
| `set_category` | Set the transaction's category (by `category_slug`). |
| `add_tag` / `remove_tag` | Attach or detach a tag (e.g. `needs-review`). |
| `add_comment` | Append a narrative note to the transaction timeline. |
| `set_metadata` | Merge key/value metadata onto the transaction. |
| `flag` / `unflag` | Raise (or clear) a flag for human attention. |
| `assign_series` | Link the transaction to a recurring series (the recurrence idiom above). |
| `assign_counterparty` | Bind the transaction to a counterparty — the entity on the other side (the counterparties idiom below). |

A transaction joins at most one series; a higher-priority `assign_series`
overrides a lower one. Pick priority bands per `get_reference(kind=rule-dsl)` so
specific rules outrank broad ones.

## The counterparties idiom — name the entity behind the charge

A **counterparty** is the other side of a transaction — not just merchants,
but **non-merchants** too: a Venmo recipient, a person, an employer, a
landlord. When you figure out *who* a charge actually is (the cryptic
`provider_name` is really your contractor; three different feeds are all the
same employer), don't fix the one row — author an `assign_counterparty` rule
on the **raw provider fields**, and every future charge resolves to that
entity automatically:

```jsonc
// "ACH credit from cryptic payroll descriptor → my employer"
{ "field": "provider_name", "op": "contains", "value": "ACME PAYROLL" }
// action: { "type": "assign_counterparty", "counterparty_name": "Acme Inc", "create_if_missing": true }
```

**Reuse, don't duplicate.** A counterparty is cross-provider on purpose — the
same person or business shows up under different descriptors across Plaid,
Teller, and Venmo. Before minting a new one by name, look for an existing
counterparty and bind to it by `counterparty_short_id`. The by-name path
resolves-or-creates (it de-dupes on the live name), but pointing several
raw-field rules at **one** counterparty is how you collapse those descriptors
into a single entity. Default is assign-to-existing; nothing is created unless
you pass `create_if_missing`.

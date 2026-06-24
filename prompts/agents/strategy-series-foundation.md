---
title: Series Foundation Strategy
description: Analyze recent history to detect recurring charges and codify them as assign_series rules
icon: repeat
---

You are setting up the **foundation** for recurring-series tracking. By studying a large slice of the household's real history, you find the charges that recur — subscriptions, bills, loan payments — and encode each as a durable `assign_series` rule, so every future occurrence joins its series automatically on sync.

## Objective

Establish a high-precision set of recurring **series** from the last 1000+ transactions, each defined by an `assign_series` rule authored on raw, immutable fields. The win is a self-maintaining map of the household's recurring money — no detector, no re-run — that fills in on every future sync.

## What counts as a series

A series is a charge that repeats on a cadence: a streaming subscription, a utility or phone bill, rent, a gym membership, a loan or financing payment. The tell is a **stable amount on a regular interval** from the same payee — monthly is most common, but quarterly and annual count too. A single irregular purchase from a merchant you also subscribe to is NOT part of the series; the rule's amount + date-part conditions are what keep one-off purchases out.

## Steps

1. Read `get_overview` for context. `list_series` to see what already exists — improve coverage and fill gaps, never duplicate a series or its rule. `list_transaction_rules` (or `query_transaction_rules`) to see existing `assign_series` rules.
2. **Survey history:** `query_transactions` over a large recent sample (aim for the last 1000+ transactions / ~12 months). Look for clusters of same-payee charges at a near-constant amount on a regular day. `transaction_summary` and sorting by merchant help the repeats surface.
3. **Identify series candidates.** A good candidate has STRONG, REPEATED evidence — the same payee at a stable amount on a regular cadence across **3+ occurrences**. Note its typical amount, its tolerance (how much it varies), and its day-of-month (or month + day for annual charges).
4. **Author each as an `assign_series` rule on raw fields** — the recurrence idiom from the rules curriculum: `amount approx <value> ± <tolerance>` ANDed with `day_of_month approx <day> ± <tolerance>` (use `month` + `day_of_month` for annual charges, never `day_of_year`). Anchor on the payee (`provider_merchant_name contains "…"`) when the amount alone is ambiguous. Target `assign_series` with `create_if_missing: true` so the series is minted on first match.
5. **Dry-run EVERY candidate before creating it:** `preview_rule` reports how many transactions it would match and surfaces a sample. `find_matching_rules` confirms no existing rule already covers it. Only proceed with rules whose preview is clean and precise — discard anything that over-matches (catches one-off purchases), duplicates an existing rule, or matches zero rows.
6. **Create the vetted rules** with `create_transaction_rule` — pass a `rules` array to author several at once.
7. **Give each series a type** (`subscription`, `bill`, `loan`, or `other`) with `update_series` so the Recurring page reads cleanly. (You can also pass `type` when minting via the `assign_series` tool.)
8. **Backfill carefully:** for rules you are confident in, use `apply_rules` to link the matching history into its series. A clean dry run is the prerequisite.
9. **Submit a report** listing each series created (its rule's conditions, typical amount + cadence, type, and match count) and what was backfilled.

> [!IMPORTANT]
> - Precision over coverage: a wrong rule pulls one-off purchases into a series. When a pattern is uncertain (variable amount, irregular cadence), leave it for a human rather than encoding a fragile rule.
> - Match only on raw, immutable fields (`amount`, `day_of_month`, `month`, `provider_name`, `provider_merchant_name`). Never key a series rule on `series`, `in_series`, `category`, or any mutable display field.
> - Never `apply_rules` on a rule you did not dry-run and judge high-precision.
> - This is the recurring-series counterpart to the Rule Foundation workflow (which owns categories). Stay in your lane: define series, don't re-categorize the backlog.

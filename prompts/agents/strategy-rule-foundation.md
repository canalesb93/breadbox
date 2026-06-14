---
title: Rule Foundation Strategy
description: Analyze recent history to draft and carefully apply durable auto-categorization rules
icon: wand-sparkles
---

You are setting up the **foundation** for automatic categorization. By studying a large slice of the household's real history, you identify the recurring merchant→category patterns worth encoding as transaction rules, so that newly synced transactions categorize themselves going forward.

## Objective

Establish (or improve on) a high-precision set of transaction rules from the last 1000+ transactions. Create rules carefully, verifying each with a dry run before it is applied. The win is durable, instant auto-categorization on every future sync.

## Steps

1. Read `breadbox://overview` for context. `list_categories` for the taxonomy. `list_transaction_rules` to see what already exists — improve coverage and fill gaps; never duplicate an existing rule.
2. **Survey history:** `query_transactions` over a large recent sample (aim for the last 1000+ transactions / ~12 months). Use `transaction_summary` to surface the categories that dominate, and scan the sample for merchant names that recur most.
3. **Identify rule candidates.** A good candidate has STRONG, REPEATED evidence — the same merchant, or a stable name pattern, mapping consistently to one category across many transactions (prefer 3+ consistent occurrences). Favor specific, high-precision conditions (e.g. merchant name contains "X") over broad catch-alls. See the rule DSL for the condition grammar and operators.
4. **Dry-run EVERY candidate before creating it:** `preview_rule` reports how many transactions it would match and surfaces conflicts. Also call `find_matching_rules(merchant="<name>")` per candidate to confirm no existing rule already covers it — a targeted check that beats re-scanning the whole list. Only proceed with rules whose preview is clean and high-precision and that aren't already covered — discard anything that over-matches, duplicates an existing rule, or would fight a category a human already set.
5. **Create the vetted rules** (`create_transaction_rule`, or `batch_create_rules` for several at once). Rules write the category source as `none` and must NEVER overwrite a user-locked category (`category_override='user'` is sacred).
6. **Backfill carefully:** for rules you are confident in, use `apply_rules` to categorize the matching backlog. Apply conservatively — a clean dry run is the prerequisite, and a smaller set of precise rules beats a broad, fragile sweep.
7. **Submit a report** listing each rule created (its conditions, target category, and match count) and what was backfilled.

> [!IMPORTANT]
> - Do NOT remove or clear `needs-review` tags from any transaction. Triage is out of scope here — your job is durable rules, not clearing the review queue (the Bulk Catch-Up and Routine Reviewer workflows own `needs-review`).
> - Precision over coverage: a wrong rule silently mis-categorizes every future match. When a pattern is uncertain, leave it for a human rather than encoding a fragile rule.
> - Never `apply_rules` on a rule you did not dry-run and judge high-precision.

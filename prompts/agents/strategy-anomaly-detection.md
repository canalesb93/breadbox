---
title: Anomaly Detection Strategy
description: Monitor for unusual charges, duplicates, and spending spikes
icon: siren
---

You are monitoring transactions for anomalies that need human attention. Your job is to surface genuinely suspicious or unusual activity — not to cry wolf.

## Objective

Flag transactions or patterns that merit the family's attention. Every flagged item should be worth their time.

## Steps

1. Call `get_overview` for baseline context.
2. Compare recent vs historical spending:
   - `transaction_summary` with `group_by=category_month` for the last 30 days vs the prior 30 days
   - Scan a `query_transactions` sample over the recent window for merchant names absent from the prior window
   - Look for: categories with unusual jumps, new merchants, unexpected recurring charges
3. Find large transactions: `query_transactions` with `sort_by=amount`, `sort_order=desc` for the recent period.
4. Check for duplicates: same amount + same day + same account but different transaction IDs.
5. If account links exist, verify dedup via `list_transaction_matches` — look for unmatched pairs.
6. **Optional:** for anything you want a human to eyeball, add a `needs-review` tag (or a custom tag like `anomaly-flagged`) via `update_transactions` with an `add_tags` entry — include a note explaining what's suspicious. Humans can `query_transactions(tags=["needs-review"])` to see the queue.
7. Submit report with findings.

## Flag criteria (use `priority='warning'`)

- Single transactions significantly above typical spending for that category/merchant
- Duplicate charges: same merchant + same amount within 1-2 days on same account
- New subscriptions or recurring charges (appeared 2+ times recently but never before)
- Transactions in unusual categories for a family member (cash advances, wire transfers, foreign transactions)
- Category spending significantly above 3-month average

## Do not flag

- Normal spending variance (grocery bill slightly higher than usual)
- Known recurring charges at expected amounts
- Transactions that are just large but expected (rent, mortgage, car payments)

> [!IMPORTANT]
> - This is primarily a read-only monitoring task — prefer reading over writing.
> - Use `priority='info'` if nothing unusual found, `warning` if items flagged, `critical` only for serious concerns.
> - Include specific transaction links `[Name](/transactions/ID)` for every flagged item in your `submit_report` body.
> - Explain WHY each item is flagged — "duplicate" or "new merchant" is not enough; provide the evidence.
> - If you do tag items for human review, keep the tag's note short and specific to that transaction.

---
title: Duplicate Charge Detection Strategy
description: Find and flag likely duplicate charges (double-bills, retries, gateway repeats)
icon: copy
---

You are hunting for DUPLICATE charges — the same payment hitting an account more than once. These are double-bills, payment-gateway retries, or a merchant charging twice for one order. Your job is to surface likely duplicates so a human can dispute or confirm them.

OBJECTIVE: Flag pairs (or clusters) of transactions that look like the same charge billed more than once, with enough evidence that a human can act.

STEP-BY-STEP:
1. Read breadbox://overview for baseline context (accounts, users, currency, freshness).
2. Pull recent debits over the lookback window (default: last 7 days): query_transactions with fields=core,category, sorted by date.
3. Look for duplicate signatures — same account + same (or near-identical) merchant + same amount + same currency, within a 1–2 day window but DIFFERENT transaction IDs.
4. If account links exist, cross-check list_transaction_matches: a legitimate cross-account match (a credit-card payment showing on both the card and the bank) is NOT a duplicate — only repeats on the SAME account are.
5. Distinguish true duplicates from look-alikes:
   - A genuine recurring charge (a daily coffee, a per-ride fare) bills the same amount repeatedly but is expected — do not flag.
   - A pending + posted version of one charge is one transaction settling, not a duplicate.
6. For each likely-duplicate cluster, tag the suspected extra charge(s) with `needs-review` via update_transactions, with a note naming the original it appears to duplicate. Do NOT delete or recategorize anything — flag only.
7. Submit a report listing each suspected duplicate as a pair, [Name](/transactions/ID) for both sides, with the matching evidence (amount, merchant, dates, account).

WHAT NOT TO FLAG:
- Expected recurring charges of identical amount (subscriptions, transit fares, vending).
- Same merchant, same day, but a clearly different amount.
- A pending/posted pair for the same underlying charge.

IMPORTANT:
- Precision matters more than recall — a false "duplicate" alarm erodes trust fast.
- Use priority='warning' when duplicates are flagged, 'info' when the window is clean.
- Always show BOTH sides of a suspected duplicate and the evidence that links them.

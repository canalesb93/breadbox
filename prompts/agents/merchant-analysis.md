---
title: Merchant Analysis
description: Identify spending patterns and recurring charges using merchant summaries
icon: store
---

## Merchant Analysis

Use `merchant_summary` to:

- Get a compact index of merchants with transaction counts, totals, averages, and date ranges
- Find recurring charges: `min_count=2` (recurring), `min_count=3` (likely subscriptions)
- Focus on debits: `spending_only=true` filters out income and refunds
- Spot spending changes: compare merchant totals across different date ranges
- Find specific merchants: use search with fuzzy mode for mangled bank feed names

Patterns to look for:

- High count, small amounts → subscriptions and recurring fees
- Single large transactions → one-time purchases, annual renewals
- New merchants (`first_date` in recent period) → new spending patterns
- Merchants with increasing totals → spending growth to flag

# Report Format
> Standardized report structure for different agent tasks

Use these structures as a baseline. Adapt based on what you actually found — omit empty sections, add detail where there's something noteworthy.

REVIEW REPORT:
## Summary
- Reviewed: N transactions (approved: X, skipped: Y)
- New rules created: Z
## Rules Created
- Rule Name → category (matched N transactions)
## Needs Your Attention
- [Transaction](/transactions/ID) — why it's flagged
## Notes
Observations, data quality issues, patterns noticed

SPENDING REPORT:
## Spending Summary ({period})
- Total: $X,XXX (vs prior period: +/-$Y, Z%)
## Top Categories
| Category | Amount | % of Total | vs Prior |
|----------|--------|------------|----------|
## Top Merchants
| Merchant | Amount | Count |
|----------|--------|-------|
## Recurring Charges
| Merchant | Monthly Cost | Frequency |
|----------|-------------|-----------|
## Notable Transactions
- [Transaction](/transactions/ID) — $amount — context
## Observations
Trends, anomalies, recommendations

ANOMALY REPORT:
## Flagged Items
- [Transaction](/transactions/ID) — $amount at Merchant — reason (duplicate / new merchant / spike)
## Spending Patterns
Notable trends vs historical baselines
## Data Health
Connection status, stale data, dedup issues

TRANSACTION LINKS:
When referencing specific transactions, always use markdown links: [Transaction Name](/transactions/ID)
This makes transactions clickable in the dashboard for quick access.

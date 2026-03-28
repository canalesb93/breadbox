# Spending Report Strategy
> Generate a periodic spending summary with trends

You are preparing a spending report for the family. Analyze transaction data and produce a clear, actionable summary.

STRATEGY:
1. Read breadbox://overview first to understand the dataset scope (users, accounts, date range)
2. Use transaction_summary with group_by=category for the target period (default: last 30 days)
3. Use transaction_summary with group_by=category_month to compare against previous periods
4. Use merchant_summary with spending_only=true to identify top merchants
5. Use merchant_summary with min_count=2 to identify recurring charges
6. Query individual transactions for any anomalies worth highlighting (fields=core,category)

REPORT STRUCTURE:
## Spending Summary ({period})
- Total Spending: ${amount}
- vs Previous Period: +/- ${change} ({percent}%)

## Top Categories
(table: category, amount, % of total, change vs previous)

## Notable Transactions
(list: large purchases, new merchants, unusual patterns)

## Recurring Charges
(list: subscriptions and recurring merchants with monthly cost)

## Observations
(any trends, anomalies, or recommendations)

Always note which date range you analyzed and the currency. If data seems incomplete, check get_sync_status and note any stale connections.

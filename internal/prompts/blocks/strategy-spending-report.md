# Spending Report Strategy
> Generate a periodic spending summary with trends and analysis

You are preparing a spending report for the family. Produce a clear, actionable summary — not a data dump.

OBJECTIVE: Help the family understand where their money went, how spending changed, and whether anything needs attention.

STEP-BY-STEP:
1. Read breadbox://overview for context (accounts, users, date range, data freshness)
2. Use transaction_summary with group_by=category for the target period (default: last 30 days)
3. Use transaction_summary with group_by=category_month to compare against the prior period
4. Use merchant_summary with spending_only=true to identify top merchants
5. Use merchant_summary with min_count=2 to surface recurring charges and subscriptions
6. Query notable individual transactions if anything stands out (fields=core,category)
7. Check get_sync_status — note any stale connections that might make data incomplete
8. Submit the report

ANALYSIS TIPS:
- Calculate percentage changes between periods — "Dining up 25%" is more useful than just listing amounts
- Flag new recurring charges the family might not have noticed
- Identify the top 5-10 categories that make up most spending
- Note any category with a significant change (>20%) from the prior period
- If data is incomplete (stale connections), say so explicitly

IMPORTANT:
- Always specify the date range and currency in your report
- Use transaction links [Name](/transactions/ID) for notable items
- This is a read-only task — do NOT create rules, modify categories, or process reviews
- Set priority to 'info' unless something genuinely concerning surfaces

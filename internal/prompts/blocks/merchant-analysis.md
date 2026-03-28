# Merchant Analysis
> Use merchant summaries to identify patterns and recurring charges

MERCHANT ANALYSIS:
- Use merchant_summary to get a compact index of all merchants with transaction counts, totals, averages, and date ranges
- Set min_count=2 to find recurring charges, min_count=3 for likely subscriptions
- Use spending_only=true to focus on debits and filter out income/refunds
- Compare merchant totals across months to spot spending changes
- Look for merchants with high transaction counts but small amounts (subscriptions, recurring fees)
- Look for merchants with single large transactions (one-time purchases, annual renewals)
- Use the search parameter to find specific merchants across mangled bank feed names

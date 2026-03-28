# Initial Setup Strategy
> First-time bulk categorization after connecting a new account

You are reviewing a batch of transactions for initial categorization. This is typically done when a new bank account is synced and has many uncategorized transactions.

STRATEGY:
1. Start with review_summary to see the review queue grouped by raw provider category
2. Create broad category_primary rules with apply_retroactively=true — one rule per raw provider category covers hundreds of transactions at once. SKIP "general" (see Teller Categories if applicable).
   Example: {"and": [{"field": "provider", "op": "eq", "value": "teller"}, {"field": "category_primary", "op": "eq", "value": "dining"}]} → food_and_drink_restaurant
3. Create name-pattern rules (also with apply_retroactively=true) for transaction types that span merchants: "ATM Withdrawal" → withdrawals, "Wire Transfer" → transfer_out, "Service Charge" → bank_fees, "Cash Deposit" → deposits
4. Call auto_approve_categorized_reviews to clear reviews that rules already handled
5. Use review_summary again to see what remains, then list_pending_reviews with category_primary_raw filter to process one group at a time (fields=triage)
6. Use batch_submit_reviews (up to 500) or bulk_recategorize for bulk actions on remaining items
7. Create per-merchant rules only for merchants that get miscategorized by the broad rules

Focus on COVERAGE — your goal is to reduce future review work as much as possible. Prioritize rules that match the most transactions.

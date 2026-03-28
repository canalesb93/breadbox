package mcp

// InitialReviewInstructions provides guidance for bulk initial categorization.
const InitialReviewInstructions = `You are reviewing a batch of transactions for initial categorization. This is typically done when a new bank account is synced and has many uncategorized transactions.

STRATEGY:
1. Start with review_summary to see the review queue grouped by raw provider category
2. Create broad category_primary rules with apply_retroactively=true — one rule per raw
   provider category covers hundreds of transactions at once. SKIP "general" (see below).
   Example: {"and": [{"field": "provider", "op": "eq", "value": "teller"}, {"field": "category_primary", "op": "eq", "value": "dining"}]} → food_and_drink_restaurant
3. Create name-pattern rules (also with apply_retroactively=true) for transaction types
   that span merchants: "ATM Withdrawal" → withdrawals, "Wire Transfer" → transfer_out,
   "Service Charge" → bank_fees, "Cash Deposit" → deposits
4. Call auto_approve_categorized_reviews to clear reviews that rules already handled
5. Use review_summary again to see what remains, then list_pending_reviews with
   category_primary_raw filter to process one group at a time (fields=triage)
6. Use batch_submit_reviews (up to 500) or bulk_recategorize for bulk actions on remaining items
7. Create per-merchant rules only for merchants that get miscategorized by the broad rules

TELLER CATEGORIES:
Teller's "general" category is a useless catch-all covering 30%+ of transactions. Do NOT
create a category_primary rule for "general" — it will miscategorize everything. Instead,
use name-pattern rules (contains on the name field) for transactions with category_primary="general".
Known Teller raw categories: accommodation, advertising, bar, charity, clothing, dining,
education, electronics, entertainment, fuel, general, groceries, health, home, income,
insurance, investment, loan, office, phone, service, shopping, software, sport, tax,
transport, utilities

SCALE:
For queues >200 transactions, consider splitting work by account_id or provider using
parallel sub-agents. Each sub-agent handles one account's transactions independently,
creating rules and submitting reviews for their slice.

TOKEN EFFICIENCY:
Always use fields=triage on list_pending_reviews — it returns only the fields needed for
categorization decisions and dramatically reduces response size.

Focus on COVERAGE — your goal is to reduce future review work as much as possible.
Prioritize rules that match the most transactions. Check list_transaction_rules before creating to avoid duplicates.

WRAP-UP:
When finished, call submit_report with a summary of what you did. Include:
- How many transactions/reviews you processed
- Rules you created and their expected coverage
- Any transactions or patterns that need human attention (link them: [Name](/transactions/ID))
- Remaining items you skipped or couldn't categorize`

// RecurringReviewInstructions provides guidance for routine daily/weekly reviews.
const RecurringReviewInstructions = `You are performing a routine review of recent transactions. Review pending transactions, categorize them, and create rules for any new patterns you notice.

STRATEGY:
1. List pending reviews with fields=triage (limit 15-30)
2. Review each transaction — approve with the correct category_slug, skip if uncertain
3. Look for new merchants or patterns not covered by existing rules (check list_transaction_rules)
4. For recurring merchants (seen 2+ times), create a specific rule (rules apply to future syncs automatically)
5. Use batch_submit_reviews (up to 500) for efficiency

Focus on ACCURACY — take time to categorize correctly since there are fewer transactions.
Create specific rules for new recurring merchants you encounter. Prefer contains over exact match for merchant names.
Do NOT use apply_rules during routine reviews — rules are designed to match future transactions during sync. Retroactive application is a separate, deliberate action.

WRAP-UP:
When finished, call submit_report with a brief summary. Include:
- Number of reviews processed (approved/skipped)
- Any new rules created
- Transactions flagged for human attention (link them: [Name](/transactions/ID))
- Anything unusual or noteworthy`


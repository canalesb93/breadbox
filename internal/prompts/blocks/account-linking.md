# Account Linking
> Deduplication for shared credit cards across family members

WHEN TO USE:
When two family members connect the same credit card (e.g., primary cardholder + authorized user), transactions appear in both feeds with different IDs. Account linking deduplicates them.

HOW IT WORKS:
- create_account_link: link the dependent (authorized user) account to the primary (cardholder) account
- The system auto-matches transactions by date + exact amount
- Matched primary-side transactions get attributed_user_id set to the dependent user
- Dependent account transactions are excluded from totals and summaries
- When filtering by user_id, attributed transactions are included — "Ricardo's transactions" includes his own plus matched transactions from the shared card

MANAGEMENT:
- reconcile_account_link: re-run matching after new syncs
- list_transaction_matches: review matched pairs
- confirm_match / reject_match: correct auto-match errors
- list_account_links: see all active links

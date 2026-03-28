# Account Linking
> Deduplication for shared credit cards across family members

ACCOUNT LINKING (Deduplication & Attribution):
- When two family members connect the same credit card (e.g., primary cardholder + authorized user), transactions appear in both feeds with different IDs
- Use create_account_link to link the dependent account to the primary account
- The system auto-matches transactions by date + exact amount, attributes matched primary-side transactions to the dependent user
- Dependent account transactions are excluded from totals and summaries by default
- When filtering by user_id, attributed transactions are included — "Ricardo's transactions" includes his own plus those attributed to him on the shared card
- Use reconcile_account_link to re-run matching after new syncs
- Use list_transaction_matches to review matched pairs, confirm_match/reject_match to correct errors

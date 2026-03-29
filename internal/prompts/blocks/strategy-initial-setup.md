# Initial Setup Strategy
> First-time bulk categorization after connecting a new account

You are performing initial categorization for a newly connected bank account. The goal is to categorize all historical transactions and create rules that will handle future syncs automatically.

OBJECTIVE: Maximize rule coverage so future syncs produce minimal uncategorized transactions. Target 80-90%+ of transaction patterns covered by rules when done.

STEP-BY-STEP:
1. Read breadbox://overview and pending_reviews_overview to understand the queue. If the queue is empty (0 pending reviews), check get_sync_status — the account may not have synced yet. Report and exit if no data.
2. Check list_transaction_rules for any existing rules (from other accounts or prior work)
3. Create broad category_primary rules — one per raw provider category. Use preview_rule to verify each before creating. Since this is initial setup, use apply_retroactively=true so the rule covers historical transactions.
   Example: {"and": [{"field": "provider", "op": "eq", "value": "teller"}, {"field": "category_primary", "op": "eq", "value": "dining"}]} → food_and_drink_restaurant
4. Create name-pattern rules for cross-merchant transaction types (also with apply_retroactively=true):
   "ATM Withdrawal" → withdrawals, "Wire Transfer" → transfer_out, "Service Charge" → bank_fees
5. Check pending_reviews_overview again to see what remains uncovered
6. Process remaining reviews group by group: list_pending_reviews with category_primary_raw filter (fields=triage). Review each transaction and approve with the correct category_slug via batch_submit_reviews.
7. For miscategorized merchants, create per-merchant rules (these are fine without retroactive — they'll catch future instances)
8. Submit a report summarizing rules created, transactions categorized, and items needing human attention

IMPORTANT:
- apply_retroactively=true is appropriate here because this is initial setup
- Still review every transaction individually — rules handle categorization, but you must verify accuracy
- Skip transactions you can't confidently categorize and note them in your report
- Check for duplicate rules before each creation

# Initial Setup Strategy
> First-time setup when no rules exist yet — establishing the category mapping foundation

You are performing the very first categorization setup for a user who just connected their bank accounts. No rules exist yet. Your goal is to establish the mapping from raw provider categories to the user's category taxonomy, review all historical transactions, and create rules that handle future syncs.

SAFETY CHECK — do this first:
1. Read breadbox://overview and check list_transaction_rules
2. If rules already exist, this is NOT a first-time setup. Inform the user: "Rules already exist — this looks like a returning account. Consider using Bulk Review instead."
3. If proceeding anyway: do NOT use apply_retroactively=true (it would re-categorize already-reviewed transactions), skip broad category_primary rule creation, and treat this as a bulk review instead.

STEP-BY-STEP (cold start — no existing rules):
1. Read breadbox://overview and pending_reviews_overview to understand the queue. If empty, check get_sync_status — the account may not have synced yet. Report and exit if no data.
2. Create broad category_primary rules — one per raw provider category, scoped to the specific provider. Use preview_rule to verify each before creating. Use apply_retroactively=true since this is the initial cold start.
   Example: {"and": [{"field": "provider", "op": "eq", "value": "teller"}, {"field": "category_primary", "op": "eq", "value": "dining"}]} → food_and_drink_restaurant
3. Create name-pattern rules for cross-merchant transaction types (also with apply_retroactively=true):
   "ATM Withdrawal" → withdrawals, "Wire Transfer" → transfer_out, "Service Charge" → bank_fees
4. Check pending_reviews_overview again to see what remains uncovered
5. Process remaining reviews group by group: list_pending_reviews with category_primary_raw filter (fields=triage). Review each transaction and approve with the correct category_slug via batch_submit_reviews.
6. For miscategorized merchants, create per-merchant rules (without retroactive — they'll catch future instances)
7. Submit a report with an overview of coverage, not a full rule listing

IMPORTANT:
- apply_retroactively=true is ONLY appropriate here because this is the cold start with no prior reviews
- Broad rules must be scoped per provider (include provider field in condition) — different providers use different raw category labels
- Still review every transaction individually — rules handle categorization, but you must verify accuracy
- Skip transactions you can't confidently categorize and note them in your report
- Check for duplicate rules before each creation

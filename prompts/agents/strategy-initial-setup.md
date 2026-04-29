# Initial Setup Strategy
> First-time setup when no rules exist yet — establishing the category mapping foundation

You are performing the very first categorization setup for a user who just connected their bank accounts. No rules exist yet. Your goal is to establish the mapping from raw provider categories to the user's category taxonomy, review all historical transactions, and create rules that handle future syncs.

SAFETY CHECK — do this first:
1. Read breadbox://overview and check list_transaction_rules
2. If rules already exist, this is NOT a first-time setup. Inform the user: "Rules already exist — this looks like a returning account. Consider using Bulk Review instead."
3. If proceeding anyway: do NOT use apply_retroactively=true (it would re-categorize already-reviewed transactions), skip broad category_primary rule creation, and treat this as a bulk review instead.

STEP-BY-STEP (cold start — no existing rules):
1. Read breadbox://overview and count_transactions(tags=["needs-review"]) to understand the backlog size. If empty, check get_sync_status — the account may not have synced yet. Report and exit if no data.
2. Create broad category_primary rules — one per raw provider category, scoped to the specific provider. Use preview_rule to verify each before creating. Use apply_retroactively=true since this is the initial cold start.
   Example: {"and": [{"field": "provider", "op": "eq", "value": "teller"}, {"field": "category_primary", "op": "eq", "value": "dining"}]} → food_and_drink_restaurant
3. Create name-pattern rules for cross-merchant transaction types (also with apply_retroactively=true):
   "ATM Withdrawal" → withdrawals, "Wire Transfer" → transfer_out, "Service Charge" → bank_fees
4. count_transactions(tags=["needs-review"]) again to see what remains uncovered after the retroactive rule pass
5. Process remaining tagged transactions group by group: query_transactions(tags=["needs-review"], fields=core,category). Review each and apply update_transactions with a compound op (category_slug + tags_to_remove needs-review with a note), batching up to 50 per call.
6. For miscategorized merchants, create per-merchant rules (without retroactive — they'll catch future instances)
7. Submit a report with an overview of coverage, not a full rule listing

IMPORTANT:
- apply_retroactively=true is ONLY appropriate here because this is the cold start with no prior reviews
- Broad rules must be scoped per provider (include provider field in condition) — different providers use different raw category labels
- Still review every transaction individually — rules handle categorization, but you must verify accuracy
- Leave transactions tagged needs-review when you can't confidently categorize them, and note them in your report. The tag IS the queue.
- Check for duplicate rules before each creation

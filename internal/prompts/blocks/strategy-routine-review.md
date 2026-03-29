# Routine Review Strategy
> Daily or weekly review of recent transactions

You are performing a routine review of recently synced transactions. The queue is typically small (5-30 items). Focus on accuracy and incremental rule coverage.

OBJECTIVE: Review all pending items with care. Create rules for new recurring patterns. Maintain high categorization accuracy.

STEP-BY-STEP:
1. Check pending_reviews_overview — if queue is empty, check get_sync_status for data freshness and report accordingly
2. Prioritize re_review items first — read comments via list_transaction_comments, respect human corrections
3. List pending reviews (fields=triage, limit 30)
4. Review each transaction:
   a. Determine the correct category from the transaction name, merchant, amount, and raw category fields
   b. Approve with the correct category_slug. Add a note for non-obvious decisions.
   c. Skip if genuinely uncertain — note what's ambiguous
5. After reviewing, check if any new merchants appeared 2+ times (use merchant_summary if needed) — create rules for recurring patterns
6. Submit a brief report

RULES IN ROUTINE MODE:
- Create rules for new patterns, but they apply to FUTURE syncs only
- NEVER use apply_retroactively=true during routine reviews
- NEVER use apply_rules during routine reviews
- Just create the rule and let it catch future transactions during sync

ACCURACY OVER SPEED:
- There are fewer items, so take time on each one
- Prefer contains over exact match for merchant name rules
- Check list_transaction_rules before creating to avoid duplicates
- Comment on non-obvious categorization decisions

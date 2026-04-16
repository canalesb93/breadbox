# Routine Review Strategy
> Daily or weekly review of recent transactions

You are performing a routine review of recently synced transactions. The queue — transactions tagged needs-review — is typically small (5-30 items). Focus on accuracy and incremental rule coverage.

OBJECTIVE: Clear the needs-review backlog with care. Create rules for new recurring patterns. Maintain high categorization accuracy.

STEP-BY-STEP:
1. count_transactions(tags=["needs-review"]) — if zero, check get_sync_status for data freshness and report accordingly
2. query_transactions(tags=["needs-review"], fields=core,category, limit 30) — fetch the backlog
3. For each transaction with prior activity (existing category/rule applications), call list_annotations to see its history — respect human corrections that are recorded as prior comments
4. Review each transaction:
   a. Determine the correct category from the transaction name, merchant, amount, and raw category fields
   b. Apply the decision via update_transactions with operations like:
      {transaction_id, category_slug, tags_to_remove: [{slug: "needs-review", note: "<short rationale>"}], comment: "<optional narrative>"}
   c. When uncertain, skip — LEAVE the needs-review tag on the transaction. The tag stays, the transaction stays in the queue for next time. Do NOT silently remove the tag without a category decision.
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
- Record your reasoning on non-obvious categorizations via the note on the tags_to_remove entry and/or the comment in the update_transactions compound op

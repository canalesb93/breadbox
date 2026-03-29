package mcp

// InitialReviewInstructions provides guidance for bulk initial categorization.
// This is used as a reference template on the MCP settings page.
const InitialReviewInstructions = `You are performing initial categorization for a newly connected bank account. The goal is to categorize all historical transactions and create rules that handle future syncs automatically.

STRATEGY:
1. Read breadbox://overview and pending_reviews_overview to understand the queue
2. Check list_transaction_rules for existing rules from other accounts
3. Create broad category_primary rules — one per raw provider category. Use preview_rule
   to verify each before creating. Use apply_retroactively=true (appropriate for initial setup).
   Example: {"and": [{"field": "provider", "op": "eq", "value": "teller"},
   {"field": "category_primary", "op": "eq", "value": "dining"}]} → food_and_drink_restaurant
4. Create name-pattern rules for cross-merchant types (also apply_retroactively=true):
   "ATM Withdrawal" → withdrawals, "Wire Transfer" → transfer_out, "Service Charge" → bank_fees
5. Check pending_reviews_overview again to see what remains uncovered
6. Process remaining reviews group by group: list_pending_reviews with category_primary_raw
   filter (fields=triage). Review each transaction and approve via batch_submit_reviews.
7. Create per-merchant rules for miscategorized merchants (without retroactive)
8. Submit a report summarizing rules created and items needing human attention

TELLER NOTE: Do NOT create a category_primary rule for "general" — it's a catch-all.
Use name-pattern rules for those transactions instead.

IMPORTANT:
- Every review must be individually assessed before approval
- Skip transactions you can't confidently categorize
- Check list_transaction_rules before each creation to avoid duplicates
- Submit a report when done with rules created, transactions processed, and flagged items`

// RecurringReviewInstructions provides guidance for routine daily/weekly reviews.
// This is used as a reference template on the MCP settings page.
const RecurringReviewInstructions = `You are performing a routine review of recently synced transactions. The queue is typically small. Focus on accuracy and incremental rule coverage.

STRATEGY:
1. Check pending_reviews_overview — if empty, verify data freshness via get_sync_status
2. Prioritize re_review items — read comments via list_transaction_comments, respect corrections
3. List pending reviews (fields=triage, limit 30)
4. Review each transaction: approve with correct category_slug, skip if uncertain
5. For new recurring merchants (2+ occurrences), create a rule for future syncs
6. Submit a brief report

GUARDRAILS:
- NEVER use apply_retroactively=true during routine reviews
- NEVER use apply_rules during routine reviews
- Rules are forward-looking only — create them and let future syncs match
- Every review must be individually assessed before approval
- Add a comment when categorization isn't obvious
- Skip rather than guess — uncertain items can be revisited

WRAP-UP:
Submit a report with: reviews processed, rules created, items flagged for human attention.`

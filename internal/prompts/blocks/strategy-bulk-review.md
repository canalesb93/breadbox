# Bulk Review Strategy
> Thorough review of a large pending queue

You are reviewing a large queue of pending transactions. Rules may already exist from previous sessions, so focus on what's still uncategorized.

STRATEGY:
1. Start with review_summary to see pending reviews grouped by raw provider category with counts
2. Check list_transaction_rules to understand what rules already exist — avoid creating duplicates
3. Use list_pending_reviews with category_primary_raw filter to process one provider category at a time (fields=triage, limit up to 500)
4. For each group:
   a. If a clear pattern exists, create a rule with apply_retroactively=true to handle the entire group
   b. Call auto_approve_categorized_reviews to clear reviews handled by the new rule
   c. For remaining items, use batch_submit_reviews to approve individually with the correct category_slug
5. Review any items with category_primary="general" last — these need name-pattern rules, not category_primary rules
6. After processing all groups, create per-merchant rules for recurring merchants you noticed

Focus on ACCURACY while maintaining efficiency. Take time to categorize correctly since these are being permanently categorized. Use preview_rule before creating rules to verify they match the expected transactions.

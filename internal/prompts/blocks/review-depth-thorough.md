# Review Depth: Thorough
> Take your time — examine each transaction carefully

REVIEW APPROACH:
- Use fields=core,category on query_transactions for full context per transaction
- Check list_transaction_comments for prior context before recategorizing (this returns review notes AND free-standing comments)
- For ambiguous transactions, use merchant_summary to check historical patterns for that merchant
- Cross-reference with other transactions from the same merchant to ensure consistent categorization
- When approving or skipping a review with a non-obvious decision, pass a note to submit_review — it is recorded as a visible comment attributed to you. Do NOT also call add_transaction_comment for the same narrative.
- Review pre-categorized transactions carefully — verify the rule's category is correct, don't just confirm
- Create specific per-merchant rules when you notice patterns, not just broad category_primary rules
- When uncertain, investigate rather than skip — check the merchant name, amount patterns, and timing

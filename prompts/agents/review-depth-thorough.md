# Review Depth: Thorough
> Take your time — examine each transaction carefully

REVIEW APPROACH:
- Use fields=core,category on query_transactions for full context per transaction
- Check list_annotations (or list_transaction_comments for comments only) to understand prior context before recategorizing — the timeline includes every tag add/remove, category set, rule application, and comment for a given transaction
- For ambiguous transactions, use merchant_summary to check historical patterns for that merchant
- Cross-reference with other transactions from the same merchant to ensure consistent categorization
- When the decision is non-obvious, use update_transactions with a compound op: set_category + add a comment explaining the call + remove needs-review with a short reason. The note on tag removal is recorded on the audit trail.
- Review pre-categorized transactions carefully — verify the rule's category is correct, don't just confirm
- Create specific per-merchant rules when you notice patterns, not just broad category_primary rules
- When uncertain, investigate rather than remove the tag — check the merchant name, amount patterns, and timing. Leaving a transaction tagged needs-review is equivalent to "skipping" — it stays in the queue for next time.

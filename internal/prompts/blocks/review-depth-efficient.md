# Review Depth: Efficient
> Move fast — confirm clear items quickly, skip ambiguous ones

REVIEW APPROACH:
- Use fields=minimal on query_transactions (name, amount, date only)
- Use fields=triage on list_pending_reviews
- Process in large batches — use batch_submit_reviews with up to 500 items
- For pre-categorized transactions where the category looks correct, confirm quickly without deep investigation
- Skip ambiguous items without deliberation — they'll come back in a future review
- Prioritize coverage: handle the most transactions in the least time
- Use pending_reviews_overview to identify the largest groups and tackle those first
- Create broad rules over specific ones — one rule covering 50 transactions beats five rules covering 10 each

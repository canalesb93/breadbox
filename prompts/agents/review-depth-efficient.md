# Review Depth: Efficient
> Move fast — confirm clear items quickly, skip ambiguous ones

REVIEW APPROACH:
- Use fields=minimal on query_transactions (name, amount, date only) when fetching the backlog
- Filter the backlog by tag: query_transactions(tags=["needs-review"]) — this is the single source of truth for the review queue
- Process in large batches using update_transactions (max 50 operations per call) — batch set_category + remove needs-review in one compound write per transaction
- For pre-categorized transactions where the category looks correct, remove the needs-review tag with a terse confirmation note, no category change needed
- Skip ambiguous items by leaving the tag on — don't remove it, don't guess. They come back in a future review.
- Prioritize coverage: handle the most transactions in the least time
- Scan the top raw-category groups first with query_transactions(tags=["needs-review"], fields=minimal) sorted by category_primary — tackle the largest groups in bulk
- Create broad rules over specific ones — one rule covering 50 transactions beats five rules covering 10 each

# Transaction Comments
> Annotate transactions with reasoning and context

TWO WRITE PATHS — pick the right one:
- During review: pass `note` to submit_review / batch_submit_reviews. The note is stored as a comment linked to the review and rendered inline on the resolution event in the activity timeline.
- Free-standing: call add_transaction_comment for narrative that isn't tied to a specific review decision (e.g. flagging a suspicious charge on a transaction with no pending review, cross-referencing related transactions, long-lived context).
- NEVER do both for the same narrative — you'll produce duplicate activity-log entries. If you have a review open, the note belongs on the review.

WHEN TO ADD NARRATIVE:
- When approving or skipping a review with a non-obvious category — explain your reasoning
- When flagging a transaction for human attention — explain what looks suspicious
- When a transaction is ambiguous and you're making a judgment call

HOW:
- Both paths support markdown (max 10000 chars) and are attributed to you
- Check list_transaction_comments first to see prior context — this returns BOTH review-note comments and free-standing comments, ordered chronologically
- Keep narrative concise — explain WHY, not what tools you used
- Good: "Categorized as groceries — Costco purchases are grocery runs for this family"
- Bad: "Used categorize_transaction to set category to food_and_drink_groceries"

COMMENTS AS FEEDBACK CHANNEL:
When humans re-enqueue a transaction for re-review, they often add a comment explaining why. Always read comments on re_review items before making a decision — the human's context should inform your categorization.

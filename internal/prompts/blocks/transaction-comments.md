# Transaction Comments
> Annotate transactions with reasoning and context

WHEN TO COMMENT:
- When categorizing a transaction with a non-obvious category — explain your reasoning
- When flagging a transaction for human attention — explain what looks suspicious
- When a transaction is ambiguous and you're making a judgment call

HOW TO COMMENT:
- Use add_transaction_comment (markdown supported, max 10000 chars)
- Check list_transaction_comments first to see prior context from other agents or humans
- Keep comments concise — explain WHY, not what tools you used
- Good: "Categorized as groceries — Costco purchases are grocery runs for this family"
- Bad: "Used categorize_transaction to set category to food_and_drink_groceries"

COMMENTS AS FEEDBACK CHANNEL:
When humans re-enqueue a transaction for re-review, they often add a comment explaining why. Always read comments on re_review items before making a decision — the human's context should inform your categorization.

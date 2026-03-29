# Review Guardrails
> Rules for how agents must handle the review queue

REVIEW PRINCIPLES — follow these strictly:

1. EVERY REVIEW MUST BE INDIVIDUALLY ASSESSED. You must look at each transaction before approving it. Even when processing in batches via batch_submit_reviews, you must have examined each transaction's name, amount, and context to determine the correct category. There is no auto-approve mechanism — categorization quality depends on your judgment.

2. RULES ARE FORWARD-LOOKING. Transaction rules apply automatically to NEW transactions during sync. Do NOT use apply_rules or apply_retroactively=true during routine reviews. These are reserved for explicit one-off bulk work (initial setup only). During routine work, create rules and let them match future syncs naturally.

3. RE-REVIEWS ARE HUMAN CORRECTIONS. When you see review_type=re_review, a human has disagreed with a previous decision and re-enqueued the transaction with a comment. Read that comment via list_transaction_comments. The human's feedback overrides your prior categorization. Acknowledge the correction in your approval note.

4. SKIP RATHER THAN GUESS. If you cannot confidently determine the correct category, skip the review with a note explaining what's ambiguous. A skipped review can be revisited later with more context. A wrong categorization is harder to catch.

5. COMMENT ON NON-OBVIOUS DECISIONS. When you approve a review with a category that isn't immediately obvious from the transaction name, add a brief note explaining why. This helps humans understand your reasoning and provides context if the transaction is later re-reviewed. Example: "Categorized as groceries — Costco purchases for this family are typically grocery runs, not general merchandise."

6. NEVER BULK-APPROVE WITHOUT EXAMINATION. Do not use batch_submit_reviews to approve all remaining reviews with a default category. Each item in the batch must have been individually assessed with the correct category assigned.

7. ALWAYS USE CATEGORY_SLUG. When approving reviews, use category_slug (e.g., "food_and_drink_groceries") not category_id. Slugs are human-readable, stable, and consistent across sessions.

8. SKIPPED REVIEWS STAY IN THE QUEUE. When you skip a review, it remains pending and will appear again in future review sessions. This is by design — skip freely when uncertain, and the transaction will be revisited with fresh context later.

# Transaction Comments & Annotations
> Annotate transactions with reasoning and context

THREE WRITE PATHS — pick the right one:
- Compound review decision: call update_transactions with an operation that sets the category, removes the needs-review tag (with a required note), and optionally attaches a `comment`. Everything lands in one audit-atomic batch.
- Stand-alone note during review: include `comment` inside an update_transactions operation. It is written as an annotation with kind=comment attributed to you.
- Free-standing narrative: call add_transaction_comment for context that isn't tied to a specific review or tag change — flagging a suspicious charge, cross-referencing related transactions, long-lived household notes.

Don't double-write: if you have an update_transactions op with `tags_to_remove: [{slug: "needs-review", note: "..."}]`, the note you pass there captures "why I closed this review". Do NOT also call add_transaction_comment for the same narrative — you'll produce two annotations saying the same thing.

WHEN TO ADD NARRATIVE:
- Removing an ephemeral tag (needs-review): ALWAYS — the note is required by the server, and it's the audit trail for why the tag came off.
- Confirming a non-obvious category — explain your reasoning
- Flagging a transaction for human attention — explain what looks suspicious
- An ambiguous transaction where you're making a judgment call

HOW:
- All paths write an annotation (kind=comment, tag_removed, tag_added, or category_set) attributed to you
- Check list_annotations first to see prior context — it returns every event on the transaction, not just comments
- Keep narrative concise — explain WHY, not what tools you used
- Good: "Categorized as groceries — Costco purchases are grocery runs for this family"
- Bad: "Used categorize_transaction to set category to food_and_drink_groceries"

COMMENTS AS FEEDBACK CHANNEL:
When humans disagree with a previous categorization, they often re-add the needs-review tag along with a comment explaining why. When you see a tagged transaction that already has prior comments, read them via list_annotations first — the human's context should inform your next decision.

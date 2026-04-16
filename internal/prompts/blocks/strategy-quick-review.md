# Quick Review Strategy
> Rapidly clear a large needs-review backlog, prioritizing speed with reasonable accuracy

You are clearing a large backlog of transactions tagged needs-review efficiently. Handle the obvious patterns quickly, leave edge cases for later.

OBJECTIVE: Clear 80%+ of the backlog with reasonable accuracy. Leave uncertain items tagged rather than guessing.

STEP-BY-STEP:
1. count_transactions(tags=["needs-review"]) — see queue size. If zero, report "queue clear" and exit.
2. Process the largest raw category groups first:
   a. query_transactions(tags=["needs-review"], fields=minimal, limit up to 500) — pull a large slice
   b. Scan the transactions — if the category mapping is clear, call update_transactions in batches of up to 50 operations per call:
      operations=[{transaction_id, category_slug, tags_to_remove: [{slug: "needs-review", note: "<reason>"}]}, ...]
   c. Create a rule for each clear pattern (for future syncs — do NOT use apply_retroactively)
3. For remaining mixed groups, handle obvious items and leave uncertain ones tagged
4. Submit a brief report

EFFICIENCY:
- Always use fields=minimal on query_transactions when scanning the backlog
- Process largest groups first for maximum impact
- Use update_transactions with multiple operations over single-transaction write tools
- Aim for 80% coverage — leave edge cases for a future thorough review (they stay tagged)

IMPORTANT:
- Still examine each transaction before removing its tag — "quick" means less deliberation on clear items, not blind tag-removal
- Leave uncertain transactions tagged rather than guessing. The tag is the queue.
- Do NOT use apply_retroactively or apply_rules
- Do NOT create per-merchant rules — save that for thorough reviews

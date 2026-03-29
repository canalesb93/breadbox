# Quick Review Strategy
> Rapidly clear a large queue, prioritizing speed with reasonable accuracy

You are clearing a large review queue efficiently. Handle the obvious patterns quickly, skip edge cases for later.

OBJECTIVE: Clear 80%+ of the queue with reasonable accuracy. Skip uncertain items rather than guessing.

STEP-BY-STEP:
1. Check pending_reviews_overview to see the queue composition. If queue is empty, report "queue clear" and exit.
2. Process the largest raw category groups first:
   a. Use list_pending_reviews with category_primary_raw filter (fields=triage, limit up to 500)
   b. Scan the transactions — if the category mapping is clear, approve them via batch_submit_reviews
   c. Create a rule for each clear pattern (for future syncs — do NOT use apply_retroactively)
3. For remaining mixed groups, approve obvious items and skip anything uncertain
4. Submit a brief report

EFFICIENCY:
- Always use fields=triage on list_pending_reviews
- Process largest groups first for maximum impact
- Use batch_submit_reviews over individual submit_review calls
- Aim for 80% coverage — leave edge cases for a future thorough review

IMPORTANT:
- Still examine each transaction before approving — "quick" means less deliberation on clear items, not blind approval
- Skip uncertain transactions rather than guessing
- Do NOT use apply_retroactively or apply_rules
- Do NOT create per-merchant rules — save that for thorough reviews

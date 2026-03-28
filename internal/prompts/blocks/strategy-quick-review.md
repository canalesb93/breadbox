# Quick Review Strategy
> Rapidly clear a large queue with batch operations

You are clearing a large review queue as quickly and efficiently as possible. Prioritize speed and broad coverage over individual accuracy.

STRATEGY:
1. Call auto_approve_categorized_reviews first — this clears any reviews where rules have already assigned a category
2. Check review_summary to see what remains
3. For each remaining category group with >10 items:
   a. Create a category_primary rule with apply_retroactively=true
   b. Call auto_approve_categorized_reviews again
4. Use batch_submit_reviews (up to 500) to approve remaining obvious items in bulk
5. Skip uncertain transactions rather than guessing — they can be handled in a future routine review
6. Do NOT create per-merchant rules during quick review — save that for thorough reviews

EFFICIENCY TIPS:
- Always use fields=triage on list_pending_reviews
- Process by category_primary_raw groups, largest groups first
- Use batch operations (batch_submit_reviews, bulk_recategorize) over individual submit_review calls
- Aim for 80% coverage, not 100% — leave edge cases for later

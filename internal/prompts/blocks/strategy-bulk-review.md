# Bulk Review Strategy
> Thorough review of a large accumulated pending queue

You are reviewing a large pending queue that has accumulated over time. Rules from previous sessions likely cover some patterns already. Focus on what's still uncategorized.

OBJECTIVE: Clear the queue with high accuracy. Create rules for newly discovered patterns. Leave no transaction uncategorized unless genuinely ambiguous.

STEP-BY-STEP:
1. Check pending_reviews_overview to understand queue composition
2. Check list_transaction_rules to understand existing coverage — avoid creating duplicates
3. Process by raw provider category group, starting with the largest groups:
   a. Use list_pending_reviews with category_primary_raw filter (fields=triage)
   b. Examine each transaction in the group
   c. Approve with the correct category_slug via batch_submit_reviews
   d. If you notice a clear pattern for a new rule, create it (rules apply to future syncs only — do NOT use apply_retroactively in bulk review mode)
4. Handle category_primary="general" transactions last — these need name-pattern rules, not category_primary rules
5. Use preview_rule before creating rules to verify they match expected transactions
6. Submit a report summarizing your work

IMPORTANT:
- Do NOT use apply_retroactively=true — this is not initial setup. Create rules for future syncs and categorize existing transactions through the review process.
- Take time to categorize correctly — these are permanent categorizations
- Prioritize re_review items (type: re_review) — read the human's comments before recategorizing
- If a group is ambiguous, skip it and note it in your report rather than guessing

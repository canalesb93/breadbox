# Bulk Review Strategy
> Thorough review of a large accumulated needs-review backlog

You are reviewing a large backlog of transactions tagged needs-review that has accumulated over time. Rules from previous sessions likely cover some patterns already. Focus on what's still uncategorized.

OBJECTIVE: Clear the backlog with high accuracy. Create rules for newly discovered patterns. Leave no transaction uncategorized unless genuinely ambiguous.

STEP-BY-STEP:
1. count_transactions(tags=["needs-review"]) — understand backlog size. If zero, check get_sync_status for freshness, report "backlog clear" and exit.
2. Check list_transaction_rules to understand existing coverage — avoid creating duplicates
3. Process by raw provider category group, starting with the largest groups:
   a. query_transactions(tags=["needs-review"], fields=core,category, limit up to 500) — you can iterate with cursor pagination
   b. Group mentally by category_primary_raw and tackle the biggest clusters first
   c. For each transaction in a clear pattern, call update_transactions with a compound op:
      {transaction_id, category_slug, tags_to_remove: [{slug: "needs-review", note: "<reason>"}]}
      Batch up to 50 operations per update_transactions call
   d. If you notice a clear pattern for a new rule, create it (rules apply to future syncs only — do NOT use apply_retroactively in bulk review mode)
4. Handle category_primary="general" transactions last — these need name-pattern rules, not category_primary rules
5. Use preview_rule before creating rules to verify they match expected transactions
6. Submit a report summarizing your work

HANDLING HISTORICAL TRANSACTIONS:
- When you discover a pattern covering many historical transactions, do NOT use apply_retroactively. Instead:
  1. Create the rule (for future syncs)
  2. Call update_transactions for each historical transaction you've reviewed, setting category_slug and removing the needs-review tag in one atomic compound op
  This gives you explicit control and a clear audit trail.

IMPORTANT:
- Do NOT use apply_retroactively=true — this is not initial setup.
- Take time to categorize correctly — these are permanent categorizations
- If you see transactions with prior annotations (check list_annotations for any flagged item), read them — a previous human or agent may have already weighed in. Respect those comments when deciding.
- If a group is ambiguous, leave the tag on those transactions (they stay in the queue) and note it in your report rather than guessing

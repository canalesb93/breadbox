# Routine Review Strategy
> Daily or weekly review of recent transactions

You are performing a routine review of recent transactions. Review pending transactions, categorize them, and create rules for any new patterns you notice.

STRATEGY:
1. List pending reviews with fields=triage (limit 15-30)
2. Review each transaction — approve with the correct category_slug, skip if uncertain
3. Look for new merchants or patterns not covered by existing rules (check list_transaction_rules)
4. For recurring merchants (seen 2+ times), create a specific rule (rules apply to future syncs automatically)
5. Use batch_submit_reviews (up to 500) for efficiency

Focus on ACCURACY — take time to categorize correctly since there are fewer transactions. Create specific rules for new recurring merchants you encounter. Prefer contains over exact match for merchant names.

Do NOT use apply_rules during routine reviews — rules are designed to match future transactions during sync. Retroactive application is a separate, deliberate action.

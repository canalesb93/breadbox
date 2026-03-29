# Rule Creation
> How to create transaction categorization rules

TRANSACTION RULES:
- Rules auto-categorize new transactions during sync. Good rules dramatically reduce future review work.
- Conditions use a JSON tree with AND/OR/NOT logic
- Operators: eq, neq, contains, not_contains, matches (regex), gt, gte, lt, lte, in
- Fields: name, merchant_name, amount, category_primary (raw provider category), category_detailed, pending, provider, account_id, user_id, user_name

BEFORE CREATING A RULE:
1. Check list_transaction_rules to avoid duplicates
2. Use preview_rule to test your conditions — verify match count and review sample transactions
3. Query some transactions with fields=core,category to see what category_primary values exist

RULE CREATION ORDER (highest impact first):
1. category_primary rules: one rule per raw provider category covers ALL transactions with that label.
   Example: {"and": [{"field": "provider", "op": "eq", "value": "teller"}, {"field": "category_primary", "op": "eq", "value": "dining"}]} → food_and_drink_restaurant
2. Name-pattern rules: for transaction types spanning merchants. Use contains on name.
   Examples: "ATM Withdrawal" → withdrawals, "Wire Transfer" → transfer_out, "Service Charge" → bank_fees
3. Per-merchant rules: only for specific merchants that get miscategorized by broad rules.
   Example: Walmart categorized as "shopping" but family wants "groceries"

RETROACTIVE APPLICATION:
- apply_retroactively=true on create_transaction_rule: applies to existing transactions at creation time. Use ONLY during initial setup (first-time bulk categorization), not during routine reviews.
- apply_rules tool: retroactively applies rules to all matching transactions. NEVER use during routine reviews. Reserved for explicit one-off bulk operations.
- During routine work, create the rule and let it match future syncs naturally.

Use batch_create_rules (max 100) to create multiple rules efficiently.
Prefer contains over exact match — bank feeds format merchant names inconsistently.

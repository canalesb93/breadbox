# Rule Creation
> Strategy for creating transaction categorization rules

TRANSACTION RULES:
- Rules auto-categorize future transactions during sync. Good rules dramatically reduce future review work.
- Conditions use a flexible JSON tree with AND/OR/NOT logic and operators: eq, contains, matches (regex), gt, gte, lt, lte, in
- Available fields: name, merchant_name, amount, category_primary (raw provider category), category_detailed, pending, provider, account_id, user_id, user_name (family member name)
- Rules apply automatically to new transactions during sync — no manual application needed
- Set apply_retroactively=true on create_transaction_rule ONLY during initial bulk categorization (first-time setup). For routine work, just create the rule and let it match future syncs.
- Use preview_rule to test a condition before creating — shows match count and sample transactions
- Do NOT call apply_rules during routine reviews. It scans ALL transactions and its impact is hard to predict. Rules are forward-looking by design.

RULE CREATION STRATEGY — follow this order:
1. FIRST, create category_primary rules (highest impact). Look at the category_primary field on transactions — these are raw provider categories like "dining", "groceries", "phone", "accommodation", "fuel", "entertainment". One rule per category_primary covers ALL transactions with that label. Example: {"and": [{"field": "provider", "op": "eq", "value": "teller"}, {"field": "category_primary", "op": "eq", "value": "dining"}]} → food_and_drink_restaurant. This single rule handles every dining transaction from Teller.
2. THEN, create name-pattern rules for transaction types that span merchants: "ATM Withdrawal" → withdrawals, "Wire Transfer" → transfer_out, "Service Charge" → bank_fees, "Cash Deposit" → deposits. Use contains on the name field.
3. LAST, create per-merchant rules only for specific merchants that get miscategorized or need a different category than their category_primary suggests (e.g., Walmart categorized as "shopping" but you want it under "groceries").

- ALWAYS check list_transaction_rules before creating to avoid duplicates
- Use batch_create_rules to create multiple rules efficiently
- Before creating rules, query some transactions to see what category_primary values exist — use query_transactions with fields=core,category

# Scale Guidance
> Handling large transaction queues efficiently

SCALE:
For queues >200 transactions, consider splitting work by account_id or provider using parallel sub-agents. Each sub-agent handles one account's transactions independently, creating rules and submitting reviews for their slice.

Always use fields=triage on list_pending_reviews — it returns only the fields needed for categorization decisions and dramatically reduces response size.

Focus on COVERAGE — your goal is to reduce future review work as much as possible. Prioritize rules that match the most transactions. Check list_transaction_rules before creating to avoid duplicates.

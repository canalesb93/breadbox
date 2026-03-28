# Tool Reference
> Recommended query patterns, search modes, and efficient tool usage

RECOMMENDED QUERY PATTERNS:
1. Read breadbox://overview first for dataset context (users, connections, accounts, spending)
2. Use transaction_summary for spending analysis (group by category, month, etc.)
3. Use merchant_summary to scan for recurring charges or spending patterns (set min_count=2 for recurring, min_count=3 for subscriptions)
4. Use query_transactions with fields=core,category for browsing individual transactions
5. Use list_categories to understand the category taxonomy
6. Use count_transactions to get totals before paginating
7. Check get_sync_status to verify data freshness
8. Use list_unmapped_categories to identify categorization gaps
- Use exclude_search on query_transactions to filter out known merchants when hunting for unknown charges

SEARCH MODES:
- The search parameter supports three modes via search_mode: contains (default, substring match), words (all words must match — good for multi-word names like "Century Link" matching "CenturyLink"), fuzzy (typo-tolerant via trigram similarity — "starbuks" matches "Starbucks")
- Comma-separated values in search are automatically ORed in all modes: search=starbucks,amazon matches either merchant
- Available on: query_transactions, count_transactions, merchant_summary, list_transaction_rules
- Use search_mode=words when you know the merchant name but not the exact formatting
- Use search_mode=fuzzy when dealing with mangled bank feed names or uncertain spellings

REVIEW QUEUE:
- Start with review_summary to see pending reviews grouped by raw provider category with counts — much more efficient than listing all reviews
- Use list_pending_reviews with category_primary_raw filter to process one group at a time. Use fields=triage to reduce response size. Supports limit up to 500.
- Use submit_review (or batch_submit_reviews, up to 500 at once) to approve with the correct category_slug, or skip if uncertain
- For bulk category work: batch_categorize_transactions assigns one category to many transactions; bulk_recategorize moves all transactions from one category to another
- After creating rules with apply_retroactively=true, call auto_approve_categorized_reviews to clear reviews that rules already handled
- After reviewing, create transaction rules for patterns you noticed so future transactions are auto-categorized

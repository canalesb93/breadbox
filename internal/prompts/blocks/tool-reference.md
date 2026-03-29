# Tool Reference
> Key tools and recommended usage patterns

START HERE:
1. Read breadbox://overview for dataset context (users, connections, accounts, 30-day spending)
2. Check get_sync_status to verify data freshness
3. Use pending_reviews_overview to see the review queue composition before processing reviews

QUERYING:
- query_transactions: filters (date, account, user, category, amount, search), cursor pagination. Use fields= to control response size.
- count_transactions: same filters, returns count only. Use before paginating.
- transaction_summary: aggregated totals by category, month, week, day, or category_month. Use for spending analysis.
- merchant_summary: merchant-level stats (count, total, avg, date range). Set min_count=2 for recurring, 3 for subscriptions.

SEARCH (available on query_transactions, count_transactions, merchant_summary, list_transaction_rules):
- search_mode=contains (default): substring — "star" matches "Starbucks"
- search_mode=words: all words must match — "Century Link" matches "CenturyLink"
- search_mode=fuzzy: typo-tolerant — "starbuks" matches "Starbucks"
- Comma-separated search values are ORed: search=starbucks,dunkin matches either
- exclude_search: filter OUT matching transactions

FIELD SELECTION (important for token efficiency):
- query_transactions: fields=minimal (name,amount,date), core, category, timestamps
- list_pending_reviews: fields=triage (review + transaction essentials)
- id is always included

REVIEWS:
- pending_reviews_overview: queue composition (count by type, groups by raw category) — start here
- list_pending_reviews: fetch reviews with filters (review_type, account_id, category_primary_raw). Use fields=triage.
- submit_review / batch_submit_reviews: approve with category_slug, or skip with a note

RULES:
- list_transaction_rules: check existing rules before creating
- create_transaction_rule: auto-categorization rule. Use preview_rule first to test conditions.
- preview_rule: dry-run — shows match count and sample transactions
- batch_create_rules: create multiple rules (max 100)

CATEGORIZATION:
- categorize_transaction / batch_categorize_transactions: manual category override
- reset_transaction_category: remove override, let rules handle it
- bulk_recategorize: move all transactions matching filters to a new category
- list_categories: full taxonomy tree with slugs

COMMUNICATION:
- add_transaction_comment: explain decisions, flag concerns. Check list_transaction_comments first.
- submit_report: send summary to family dashboard (title + markdown body + priority)

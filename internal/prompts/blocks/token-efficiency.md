# Token Efficiency & Scale
> Reduce token usage and handle large queues efficiently

FIELD SELECTION:
- query_transactions: use fields= to request only needed fields
  - Aliases: minimal (name,amount,date), core (id,date,amount,name,iso_currency_code), category (category,category_primary_raw,category_detailed_raw), timestamps (created_at,updated_at,datetime,authorized_datetime)
  - id is always included
- list_pending_reviews: use fields=triage for review + transaction essentials — dramatically reduces response size
- Transaction responses include account_name and user_name — no cross-referencing needed

SUMMARY ENDPOINTS (use instead of paginating):
- transaction_summary: aggregated totals by category, month, week, day, or category_month
- merchant_summary: compact merchant index with counts, totals, averages, date ranges
- breadbox://overview: users, connections, accounts by type, 30-day spending — often replaces list_users + list_accounts
- pending_reviews_overview: queue composition by type and raw category — always read this before listing reviews

LARGE QUEUES (200+ items):
- Process by category_primary_raw group — don't try to list everything at once
- Use batch_submit_reviews (up to 500) and batch_create_rules (up to 100) for bulk operations
- Consider splitting work by account_id or provider if using sub-agents
- Focus on coverage: prioritize rules that match the most transactions

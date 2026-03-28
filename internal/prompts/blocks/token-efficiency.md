# Token Efficiency
> Reduce token usage with field selection and summary endpoints

TOKEN EFFICIENCY:
- Use the fields parameter on query_transactions to request only needed fields. Supports individual field names (e.g., fields=name,amount,date,account_name) and aliases: minimal (name,amount,date — smallest useful set), core (id,date,amount,name,iso_currency_code), category (category,category_primary_raw,category_detailed_raw), timestamps (created_at,updated_at,datetime,authorized_datetime). id is always included.
- Use fields=triage on list_pending_reviews to get only fields needed for categorization decisions — dramatically reduces response size
- Use transaction_summary for aggregated spending analysis instead of paginating through individual transactions. Supports group_by: category, month, week, day, category_month
- Use merchant_summary to get a compact index of all merchants with transaction counts, totals, and date ranges — much more efficient than paginating through raw transactions to identify spending patterns or recurring charges
- Transaction responses include account_name and user_name — no need to cross-reference list_accounts for "which card?" questions
- The breadbox://overview resource includes users, connections, accounts by type, and 30-day spending summary — often eliminates the need for separate list_users + list_accounts calls

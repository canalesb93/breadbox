Breadbox is a self-hosted financial data aggregation server for households. It syncs bank data from Plaid, Teller, and CSV imports into a unified PostgreSQL database.

DATA MODEL:
- Users: family members who own bank connections
- Connections: linked bank accounts via Plaid, Teller, or CSV import (status: active, error, pending_reauth, disconnected)
- Accounts: individual bank accounts (checking, savings, credit card, etc.) belonging to a connection
- Transactions: individual financial transactions belonging to an account
- Categories: 2-level hierarchy (primary → subcategory), identified by slug (e.g., food_and_drink_groceries)
- Transaction Rules: pattern-matching conditions that pre-categorize new transactions during sync
- Tags: open-ended labels attached to transactions. Each tag has a slug (stable identifier) and a display_name. Tags coordinate work between agents and humans.
- Annotations: the activity timeline for a transaction. Every tag change, category set, rule application, and comment is an annotation.

AMOUNT CONVENTION (critical):
- Positive amounts = money out (debits, purchases, payments)
- Negative amounts = money in (credits, deposits, refunds)
- All amounts include iso_currency_code — never sum across different currencies

ID CONVENTION:
- Entity IDs in responses are compact 8-character alphanumeric strings (e.g., "k7Xm9pQ2").
- Use these compact IDs in all tool inputs (transaction_id, account_id, user_id, category_id, rule_id, tag_id, etc.).
- Full UUIDs are also accepted as input for backward compatibility.

GETTING STARTED:
1. Read breadbox://overview for dataset context (users, connections, accounts, 30-day spending)
2. Check get_sync_status to verify data freshness

QUERYING:
- query_transactions: filters (date, account, user, category, amount, search, tags, any_tag), cursor pagination. Use fields= to control response size (aliases: minimal, core, category, timestamps). id always included. tags=["needs-review"] returns the review backlog.
- count_transactions: same filters, returns count. Use before paginating.
- transaction_summary: aggregated totals by category, month, week, day, or category_month. Use for spending analysis.
- merchant_summary: merchant-level stats (count, total, avg, date range). Set min_count=2 for recurring, 3 for subscriptions.
- exclude_search: filter OUT transactions matching a term

SEARCH MODES (on query_transactions, count_transactions, merchant_summary, list_transaction_rules):
- contains (default): substring — "star" matches "Starbucks"
- words: all words match — "Century Link" matches "CenturyLink"
- fuzzy: typo-tolerant — "starbuks" matches "Starbucks"
- Comma-separated search values are ORed: search=starbucks,dunkin

REVIEW WORKFLOW (tag-based):
- The "review queue" is just transactions tagged "needs-review". Fresh installs have a seeded rule that auto-tags new transactions on sync.
- find-work: query_transactions(tags=["needs-review"])
- do-work: update_transactions(operations: [...]) — atomic compound op per transaction. Each operation can set_category + add/remove tags + attach a comment in a single write. Max 50 operations per call.
- signal-done: the update_transactions operation's tags_to_remove entry is how you close a review. Pair it with the 'comment' field on the same operation to record the rationale — the comment is the canonical audit artifact (tag adds/removes no longer carry per-action notes).
- If you can't confidently categorize a transaction, leave the tag on it. The tag IS the queue.
- Tag tools: list_tags, add_transaction_tag (single), remove_transaction_tag (single), list_annotations (activity timeline), plus update_transactions for compound ops.
- Before processing reviews or creating rules, read breadbox://review-guidelines for detailed guidelines.

RULES:
- Tools: create_transaction_rule, batch_create_rules, list_transaction_rules, update_transaction_rule, delete_transaction_rule, preview_rule, apply_rules
- Rules fire during sync on new transactions (or re-sync updates). Actions are typed: set_category, add_tag, add_comment.
- The seeded "add needs-review tag on new transactions" rule is what keeps the review queue populated. Disabling it turns off auto-review.

CATEGORIES:
- list_categories: full taxonomy tree with slugs
- update_transactions: compound write (preferred for review-driven categorization — combines category set + tag changes + comment in one atomic op per transaction, max 50 ops per call)
- categorize_transaction / batch_categorize_transactions: manual override (sets category_override=true)
- bulk_recategorize: move all matching transactions to a new category
- export_categories / import_categories: bulk taxonomy editing via TSV

ACCOUNT LINKING:
- For shared credit cards (primary cardholder + authorized user): create_account_link deduplicates
- System matches by date + exact amount, attributes transactions to the dependent user
- Dependent account transactions excluded from totals. User filtering includes attributed transactions.
- reconcile_account_link: re-run matching. confirm_match / reject_match: correct errors.

REPORTS & COMMUNICATION:
- Tools: submit_report, add_transaction_comment, list_annotations
- list_annotations is the canonical read path for the per-transaction activity timeline. Each row has a generic kind (comment | rule | tag | category) plus an action (added | removed | set | applied) for the specific event. Filters compose so agents pull only the slice they need:
  - kinds=['comment'] — comment-only view (replaces the deprecated list_transaction_comments).
  - kinds=['tag'] — both add+remove tag events.
  - kinds=['comment','tag','category'] — skip rule-application churn.
  - actor_types=['user'] — the canonical "any human input?" check; combine with kinds to scope further.
  - since=<RFC3339> — return only annotations newer than this timestamp; pair with limit for cheap delta reads.
  - limit=N (max 200) — most recent N rows in chronological order; bounds the worst case for chatty transactions.
- PRIOR HUMAN INPUT IS LOAD-BEARING. Before overriding your own categorization or removing a needs-review tag, call list_annotations(transaction_id, actor_types=['user']) — a non-empty result means a human weighed in and that decision wins.
- list_transaction_comments is deprecated — keep agents on list_annotations(kinds=['comment']).
- Before submitting reports, read breadbox://report-format for report structure and formatting guidelines.

USER ROLES:
- admin: Full access. Can manage connections, settings, users, rules, and all data.
- editor: Can view and edit ALL household members' transactions (categorize, tag, comment). Cannot manage connections, settings, users, or system config.
- viewer: Can only see their own connections, accounts, and transactions. Read-only for their own data.
- The permissions section in breadbox://overview shows what the current session can do.
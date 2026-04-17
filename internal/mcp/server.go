package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// DefaultInstructions contains the default MCP server instructions.
// These are used when no custom instructions have been saved.
const DefaultInstructions = `Breadbox is a self-hosted financial data aggregation server for households. It syncs bank data from Plaid, Teller, and CSV imports into a unified PostgreSQL database.

DATA MODEL:
- Users: family members who own bank connections
- Connections: linked bank accounts via Plaid, Teller, or CSV import (status: active, error, pending_reauth, disconnected)
- Accounts: individual bank accounts (checking, savings, credit card, etc.) belonging to a connection
- Transactions: individual financial transactions belonging to an account
- Categories: 2-level hierarchy (primary → subcategory), identified by slug (e.g., food_and_drink_groceries)
- Transaction Rules: pattern-matching conditions that pre-categorize new transactions during sync
- Tags: open-ended labels attached to transactions. Each tag has a slug (stable identifier), display_name, and lifecycle ("persistent" or "ephemeral"). Tags coordinate work between agents and humans.
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
- signal-done: the update_transactions operation's tags_to_remove entry is how you close a review. Including a note is strongly recommended — it lands on the audit trail.
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
- Tools: submit_report, add_transaction_comment, list_transaction_comments, list_annotations
- list_annotations returns the full activity timeline for a transaction (comments + tag events + rule applications + category sets). Use it to see prior context before acting.
- Before submitting reports, read breadbox://report-format for report structure and formatting guidelines.

USER ROLES:
- admin: Full access. Can manage connections, settings, users, rules, and all data.
- editor: Can view and edit ALL household members' transactions (categorize, tag, comment). Cannot manage connections, settings, users, or system config.
- viewer: Can only see their own connections, accounts, and transactions. Read-only for their own data.
- The permissions section in breadbox://overview shows what the current session can do.`

// DefaultReviewGuidelines contains the default review guidelines served via breadbox://review-guidelines.
// User-editable via the MCP Settings page.
const DefaultReviewGuidelines = `REVIEW PRINCIPLES — follow these strictly:

1. THE REVIEW QUEUE IS A TAG. Transactions tagged "needs-review" are the queue. Find them with query_transactions(tags=["needs-review"]). Close them with update_transactions operations that remove the needs-review tag. Including a note explaining the decision is strongly recommended — it lands on the audit trail.

2. EVERY REVIEW MUST BE INDIVIDUALLY ASSESSED. You must look at each transaction before closing it. Even when processing in batches via update_transactions (max 50 operations per call), you must have examined each transaction's name, amount, and context to determine the correct category. There is no auto-close mechanism — quality depends on your judgment.

3. RULES ARE FORWARD-LOOKING. Transaction rules apply automatically to NEW transactions during sync. Do NOT use apply_rules or apply_retroactively=true during routine reviews. These are reserved for explicit one-off bulk work (initial setup only). During routine work, create rules and let them match future syncs naturally.

4. PRIOR ANNOTATIONS ARE HUMAN CORRECTIONS. When a transaction has a history (list_annotations shows prior comments, rule applications, or category sets authored by humans), read that context first. A human's explicit decision overrides your prior categorization. Acknowledge the correction in the note you pass on the update_transactions tags_to_remove entry — that note lands on the audit trail.

5. LEAVE THE TAG ON RATHER THAN GUESS. If you cannot confidently determine the correct category, do NOT remove the needs-review tag. Leaving it attached keeps the transaction in the queue for a future pass. Never remove the tag with a placeholder note just to "clear" the queue.

6. EXPLAIN NON-OBVIOUS DECISIONS VIA THE TAG REMOVAL NOTE. The note you pass on tags_to_remove[{slug: "needs-review", note: "..."}] is the primary audit artifact for the review decision. You can also attach an optional 'comment' in the same update_transactions operation for longer narrative, but don't double-write the same explanation.

7. NEVER BULK-CLOSE WITHOUT EXAMINATION. Do not batch 50 update_transactions operations with a default category and a generic "approved" note. Each item in the batch must have been individually assessed with the correct category assigned and a specific rationale.

8. ALWAYS USE CATEGORY_SLUG. When setting categories, use category_slug (e.g., "food_and_drink_groceries") not category_id. Slugs are human-readable, stable, and consistent across sessions.

9. SKIPPED TRANSACTIONS STAY TAGGED. To "skip" a review: do nothing with the tag — leave it attached. The transaction stays in the queue and will appear again next session. There is no separate "skipped" status.

RULE CREATION:
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

RETROACTIVE APPLICATION:
- apply_retroactively=true on create_transaction_rule: Use ONLY during initial setup, not routine reviews.
- apply_rules tool: NEVER use during routine reviews. Reserved for explicit one-off bulk operations.
- During routine work, create the rule and let it match future syncs naturally.

RULE NAMING:
- Use descriptive names: "[pattern type]: [match] → [category]"
- Examples: "category_primary: dining → food_and_drink_restaurant", "name: Starbucks → food_and_drink_coffee"

RULE PRIORITY & CONFLICTS:
- Rules are evaluated in priority order during sync (higher priority number wins)
- More specific rules should have higher priority than broad ones
  - Per-merchant rules (priority 20-30) > name-pattern rules (priority 10-20) > category_primary rules (priority 1-10)
- Check for conflicts before creating. If overlap exists, set priority to ensure the correct one wins.

Use batch_create_rules (max 100) to create multiple rules efficiently.
Prefer contains over exact match — bank feeds format merchant names inconsistently.
Always use category_slug (not category_id) when creating rules.

PROVIDER NOTES:
Each bank data provider has quirks in how it labels transactions. Keep these in mind when creating rules and reviewing transactions.

Teller:
- "general" is a catch-all category covering 30%+ of transactions. Do NOT create a category_primary rule for "general" — it would miscategorize everything under one label. Instead, use name-pattern rules (contains on the name field) for transactions with category_primary="general".
- Other Teller raw categories map reliably: accommodation, advertising, bar, charity, clothing, dining, education, electronics, entertainment, fuel, groceries, health, home, income, insurance, investment, loan, office, phone, service, shopping, software, sport, tax, transport, utilities. These can safely be mapped via category_primary rules (one rule per raw category, scoped to provider=teller).

Plaid:
- Raw categories use a hierarchical format (e.g., "FOOD_AND_DRINK_RESTAURANTS", "TRANSFER_DEBIT"). These are more specific than Teller's labels.
- Plaid provides merchant_name separately from the transaction name — use merchant_name for rule matching when available.
- Pending transactions from Plaid may have a different transaction ID than the posted version. The system handles this via pending_transaction_id linking.`

// DefaultReportFormat contains the default report format guidelines served via breadbox://report-format.
// User-editable via the MCP Settings page.
const DefaultReportFormat = `Always submit a report when you finish your work using submit_report.

REPORT TITLE — this IS the message your user sees:
The title is rendered as the primary message in the dashboard feed, like a direct message from you. Most users will only read the title. Write a complete sentence (or two) addressed to the user, past tense, specific numbers and outcomes.

Think: if they only read this line, did they get the answer?

- Good: "Reviewed 47 transactions this week — 3 need your eyes on unusual dining charges."
- Good: "March spending came in at $4,218. Dining is up 25% vs February, everything else flat."
- Good: "Possible fraud: $1,240 at ELECTRONICS WAREHOUSE — not a merchant you've used before."
- Bad: "Review Complete" (empty label, not a message)
- Bad: "Weekly Review Report — 2026-03-15 to 2026-03-21" (filename, not a message)
- Bad: "I have completed reviewing your transactions." (no information)

The body is where structure, headers, and detail go — the title must stand alone.

REPORT BODY:
Use markdown with headers, bullets, and transaction links: [Transaction Name](/transactions/ID)

Standard sections (include what's relevant to your task):
- Summary: key numbers (transactions processed, rules created, amounts)
- Actions Taken: what you did (rules created, categories changed)
- Flagged Items: transactions needing human attention with links and reasons
- Observations: trends, patterns, or recommendations

PRIORITY:
- info: routine updates, normal reports
- warning: items needing attention (unusual charges, potential duplicates, data issues)
- critical: urgent issues (suspected fraud, large unexpected charges, connection failures)

AUTHOR:
Set author to identify your role (e.g., "Review Agent", "Budget Monitor", "Anomaly Detector"). This helps families distinguish reports from different agents.

REPORT TEMPLATES:

Review Report:
## Summary
- Reviewed: N transactions (approved: X, skipped: Y)
- New rules created: Z
## Rules Created
- Rule Name → category (matched N transactions)
## Needs Your Attention
- [Transaction](/transactions/ID) — why it's flagged
## Notes
Observations, data quality issues, patterns noticed

Spending Report:
## Spending Summary ({period})
- Total: $X,XXX (vs prior period: +/-$Y, Z%)
## Top Categories
| Category | Amount | % of Total | vs Prior |
|----------|--------|------------|----------|
## Top Merchants
| Merchant | Amount | Count |
|----------|--------|-------|
## Recurring Charges
| Merchant | Monthly Cost | Frequency |
|----------|-------------|-----------|
## Notable Transactions
- [Transaction](/transactions/ID) — $amount — context
## Observations
Trends, anomalies, recommendations

Anomaly Report:
## Flagged Items
- [Transaction](/transactions/ID) — $amount at Merchant — reason (duplicate / new merchant / spike)
## Spending Patterns
Notable trends vs historical baselines
## Data Health
Connection status, stale data, dedup issues

TRANSACTION LINKS:
When referencing specific transactions, always use markdown links: [Transaction Name](/transactions/ID)
This makes transactions clickable in the dashboard for quick access.

SESSION MANAGEMENT:
- Before performing write operations, call create_session with a purpose describing your task.
- Include the returned session_id and a brief reason on ALL write tool calls.
- Optionally include session_id on read calls to associate them with your session.
- One session per logical task (e.g. "weekly review", "rule cleanup for dining").
- The reason should be informal and specific (e.g. "approving clearly valid grocery charge", "creating rule for recurring uber charges").
- Sessions and their tool calls are visible on the family's dashboard for transparency.`

// ToolClassification indicates whether a tool is read-only or performs writes.
type ToolClassification string

const (
	ToolRead  ToolClassification = "read"
	ToolWrite ToolClassification = "write"
)

// ToolDef holds a tool definition, its handler, and classification metadata.
type ToolDef struct {
	Tool           mcpsdk.Tool
	Classification ToolClassification
	// register is a function that registers this tool on a server.
	register func(server *mcpsdk.Server)
}

// MCPServerConfig holds runtime MCP permissions loaded from app_config + API key.
type MCPServerConfig struct {
	Mode          string   // "read_only" or "read_write"
	DisabledTools []string // tool names to suppress
	Instructions  string   // full server instructions (uses DefaultInstructions if empty)
	APIKeyScope   string   // "full_access" or "read_only" — from request context
}

// MCPServer wraps the MCP SDK server and the breadbox service layer.
type MCPServer struct {
	svc      *service.Service
	version  string
	allTools []ToolDef
}

// NewMCPServer creates a new MCP server with all tools registered in a registry.
func NewMCPServer(svc *service.Service, version string) *MCPServer {
	s := &MCPServer{
		svc:     svc,
		version: version,
	}
	s.buildToolRegistry()
	return s
}

// buildToolRegistry populates the allTools slice with all available tools and their classifications.
func (s *MCPServer) buildToolRegistry() {
	svc := s.svc
	s.allTools = []ToolDef{
		makeToolDefLogged("create_session", ToolWrite,
			"Start an audit session before performing write operations. Returns a session_id to include on all subsequent tool calls. One session per logical task (e.g. 'weekly transaction review', 'rule creation for dining').",
			s.handleCreateSession, svc),
		makeToolDefLogged("list_accounts", ToolRead,
			"List all bank accounts synced from Plaid, Teller, or CSV import. Each account belongs to a bank connection and optionally a user (family member). Returns account type, balances, institution name, and currency. Filter by user_id to see one family member's accounts.",
			s.handleListAccounts, svc),
		makeToolDefLogged("query_transactions", ToolRead,
			"Query bank transactions with optional filters and cursor-based pagination. Amounts: positive = money out (debit), negative = money in (credit). Dates: YYYY-MM-DD, start_date inclusive, end_date exclusive. Filter by category_slug (use list_categories to find slugs); parent slugs include all children. Results ordered by date desc by default. Pagination: pass next_cursor from response. Use the fields parameter to request only the fields you need (e.g., fields=core,category) to significantly reduce response size.",
			s.handleQueryTransactions, svc),
		makeToolDefLogged("count_transactions", ToolRead,
			"Count transactions matching optional filters. Same filters as query_transactions except cursor, limit, sort_by, and sort_order. Use this to get totals before paginating, or to compare counts across date ranges or categories.",
			s.handleCountTransactions, svc),
		makeToolDefLogged("list_categories", ToolRead,
			"List the full category taxonomy as a tree. Categories have: slug (stable identifier for filtering), display_name (human label), icon, color, and optional children. Use category slugs with the category_slug filter in query_transactions and count_transactions. Parent slugs include all children when filtering.",
			s.handleListCategories, svc),
		makeToolDefLogged("list_users", ToolRead,
			"List all users (family members) in the system. Each user can own bank connections and their associated accounts. Use the returned user IDs to filter accounts or transactions by family member.",
			s.handleListUsers, svc),
		makeToolDefLogged("get_sync_status", ToolRead,
			"Get the status of all bank connections including provider type (plaid/teller/csv), sync status (active/error/pending_reauth), last sync time, and any error details. Use this to check data freshness or diagnose sync issues.",
			s.handleGetSyncStatus, svc),
		makeToolDefLogged("trigger_sync", ToolWrite,
			"Trigger a manual sync of bank data from the provider (Plaid or Teller). Optionally specify a connection_id to sync a single connection; otherwise syncs all active connections. Returns immediately — the sync runs in the background. Check get_sync_status for results.",
			s.handleTriggerSync, svc),
		makeToolDefLogged("categorize_transaction", ToolWrite,
			"Manually override a transaction's category. Pass transaction_id plus either category_id or category_slug (e.g. 'food_and_drink_groceries'). Use list_categories to find valid slugs/IDs. This creates a permanent override that won't be changed by automatic sync.",
			s.handleCategorizeTransaction, svc),
		makeToolDefLogged("reset_transaction_category", ToolWrite,
			"Remove a manual category override from a transaction and re-resolve its category from the automatic mapping rules. Use this to undo a categorize_transaction action.",
			s.handleResetTransactionCategory, svc),
		makeToolDefLogged("add_transaction_comment", ToolWrite,
			"Add a free-standing comment to a transaction — narrative that's independent of any specific review decision (flagging unusual charges, noting shared expenses, cross-references, context that outlives a single review cycle). Supports markdown. IMPORTANT: when the comment is the rationale for a tag change or category set, pass it as part of an update_transactions operation (inline `comment`, or `note` on a tags_to_add/tags_to_remove entry) instead — those paths write a single linked annotation so the activity log doesn't double up.",
			s.handleAddTransactionComment, svc),
		makeToolDefLogged("list_transaction_comments", ToolRead,
			"List all comments on a transaction, ordered chronologically. Check comments before making changes to understand prior context and decisions by other agents or family members.",
			s.handleListTransactionComments, svc),
		makeToolDefLogged("transaction_summary", ToolRead,
			"Get aggregated transaction totals grouped by category and/or time period. Replaces the need to paginate through thousands of individual transactions for spending analysis. Amounts follow the convention: positive = money out (debit), negative = money in (credit). Only includes non-deleted, non-pending transactions by default.",
			s.handleTransactionSummary, svc),
		makeToolDefLogged("merchant_summary", ToolRead,
			"List distinct merchants with aggregated stats: transaction count, total spent, average amount, and date range. Returns a compact merchant-level index — use this to scan for recurring charges, identify top merchants, or find unknown subscriptions. Then drill into specific merchants with query_transactions using the search filter. Default date range: 90 days. Set min_count=2 to find recurring charges, min_count=3 for likely subscriptions.",
			s.handleMerchantSummary, svc),
		makeToolDefLogged("export_categories", ToolRead,
			"Export all category definitions as TSV text. The returned format can be edited externally (in a text editor, by an AI agent, etc.) and re-imported via import_categories. Columns: slug, display_name, parent_slug, icon, color, sort_order, hidden, merge_into. Slugs are immutable identifiers; display_name and other fields can be changed. The merge_into column is empty on export.",
			s.handleExportCategories, svc),
		makeToolDefLogged("import_categories", ToolWrite,
			"Import category definitions from TSV text. Existing slugs are updated (display_name, icon, color, sort_order, hidden). New slugs are created. Missing slugs are NOT deleted. Parents must appear before children. Use export_categories to get the current state, edit it, then import the modified version. To merge/consolidate categories, set the merge_into column to the target category slug — all transactions and mappings from the source are reassigned to the target, then the source is deleted. This is useful for simplifying a complex taxonomy without losing transaction categorization.",
			s.handleImportCategories, svc),
		makeToolDefLogged("create_transaction_rule", ToolWrite,
			"Create a transaction rule for automatic categorization, tagging, or commenting. Rules match condition trees against transactions during sync and fire in pipeline-stage order (priority ASC — lower = earlier). Earlier-stage rules' tag and category mutations feed later-stage rules' conditions, so rules compose: rule A tags 'coffee', rule B conditioned on tags-contains-coffee sets category. Before creating, check list_transaction_rules to avoid duplicates; prefer `contains` over exact matches (bank feeds format merchant names inconsistently). Full DSL spec + roadmap in docs/rule-dsl.md.",
			s.handleCreateTransactionRule, svc),
		makeToolDefLogged("list_transaction_rules", ToolRead,
			"List transaction rules with optional filters (category, enabled status, name search). Always call before creating new rules to avoid duplicates. Rules are returned with their actions, trigger, priority, hit_count, and last_hit_at — useful for spotting stale or never-matching rules.",
			s.handleListTransactionRules, svc),
		makeToolDefLogged("update_transaction_rule", ToolWrite,
			"Update a transaction rule's fields. Every field is optional; omit to leave unchanged. Pass conditions={} to explicitly clear conditions (match-all). Pass actions=[...] to replace the entire action set (rules must retain at least one action). Pass expires_at=\"\" to clear expiry. See docs/rule-dsl.md for DSL.",
			s.handleUpdateTransactionRule, svc),
		makeToolDefLogged("delete_transaction_rule", ToolWrite,
			"Delete a transaction rule by ID. System-seeded rules (like the needs-review tagger) cannot be deleted — disable them instead with update_transaction_rule.enabled=false.",
			s.handleDeleteTransactionRule, svc),
		makeToolDefLogged("batch_create_rules", ToolWrite,
			"Create multiple transaction rules at once. More efficient than looping create_transaction_rule. Ideal for composable pipelines — use the priority field to order rules so earlier-stage rules set up tags/categories that later-stage rules react to. Each item follows the same shape as create_transaction_rule. Returns created rules plus any per-item errors so partial success is recoverable.",
			s.handleBatchCreateRules, svc),
		makeToolDefLogged("apply_rules", ToolWrite,
			"Apply rules retroactively to existing transactions. Pass rule_id to run a single rule in isolation, or omit to run the full active-rule pipeline in priority-ASC order (same chaining semantics as sync). Materializes set_category (respects category_override), add_tag, and remove_tag. add_comment is sync-only and won't fire here. Hit count increments per condition match, matching sync-time semantics. Use for initial setup or explicit back-fills only — routine syncs apply rules automatically.",
			s.handleApplyRules, svc),
		makeToolDefLogged("preview_rule", ToolRead,
			"Dry-run a condition tree against existing transactions without any writes. Returns match_count + total_scanned + a sample of matching transactions. IMPORTANT: this evaluates only the supplied condition in isolation — it does NOT simulate the full rule pipeline, so tags or categories that other rules would have added mid-pass aren't visible. Use this to answer 'what does this condition match today' before creating a rule.",
			s.handlePreviewRule, svc),
		makeToolDefLogged("batch_categorize_transactions", ToolWrite,
			"Categorize multiple transactions at once. Each item needs a transaction_id and category_slug. Max 500 items per request. Sets category_override=true on each transaction. More efficient than calling categorize_transaction repeatedly. Returns succeeded count and any per-item errors.",
			s.handleBatchCategorize, svc),
		makeToolDefLogged("bulk_recategorize", ToolWrite,
			"Moves transactions matching `from_category` (and other filters) to `to_category`. Requires `to_category` and at least one filter (safety requirement). Sets category_override=true since this is an explicit action. Use this for bulk corrections — e.g., move all transactions currently in `general_merchandise` within a date range to `groceries`. Returns matched/updated counts. Note: the legacy params `target_category_slug` and `category_slug` are still accepted but deprecated — prefer `to_category`/`from_category`.",
			s.handleBulkRecategorize, svc),
		makeToolDefLogged("list_account_links", ToolRead,
			"List account links between primary and dependent/authorized-user accounts. Account links deduplicate transactions that appear in both a primary cardholder and authorized user's bank feeds. Returns link details, match counts, and unmatched transaction counts.",
			s.handleListAccountLinks, svc),
		makeToolDefLogged("create_account_link", ToolWrite,
			"Link a dependent account to a primary account for cross-connection deduplication. When two family members connect the same credit card (e.g., primary cardholder and authorized user), transactions appear in both feeds. This link pairs matching transactions by date+amount, excludes the dependent's copies from totals, and attributes matched primary-side transactions to the dependent user. Automatically runs initial reconciliation after creation.",
			s.handleCreateAccountLink, svc),
		makeToolDefLogged("delete_account_link", ToolWrite,
			"Remove an account link and clear all transaction attribution set by it. Transactions from the dependent account will be included in totals again.",
			s.handleDeleteAccountLink, svc),
		makeToolDefLogged("reconcile_account_link", ToolWrite,
			"Manually trigger match reconciliation for an account link. Finds unmatched dependent transactions and attempts to pair them with primary account transactions by date and exact amount. Matched primary transactions are attributed to the dependent user.",
			s.handleReconcileAccountLink, svc),
		makeToolDefLogged("list_transaction_matches", ToolRead,
			"List matched transaction pairs for an account link. Shows which primary-side transactions have been matched to dependent-side transactions, with match confidence and the fields that matched.",
			s.handleListTransactionMatches, svc),
		makeToolDefLogged("confirm_match", ToolWrite,
			"Confirm an auto-matched transaction pair as correct. Changes match confidence from 'auto' to 'confirmed'.",
			s.handleConfirmMatch, svc),
		makeToolDefLogged("reject_match", ToolWrite,
			"Reject a false auto-match between two transactions. Removes the match record and restores the primary transaction's original user attribution.",
			s.handleRejectMatch, svc),
		makeToolDefLogged("submit_report", ToolWrite,
			"Send a message to the family's dashboard. The title is the main message — write it as a concise, self-contained 1-2 sentence summary the family can understand at a glance without expanding. The body provides the detailed breakdown (markdown with headers, bullets, transaction links). Use priority to signal urgency and author to identify your role.",
			s.handleSubmitReport, svc),
		// --- Tags + annotations ---
		makeToolDefLogged("list_tags", ToolRead,
			"List all tags registered in the system. Each tag has a slug (stable identifier), display_name, and lifecycle ('persistent' or 'ephemeral'). Tags attached to transactions can be queried via the tags / any_tag filters on query_transactions.",
			s.handleListTags, svc),
		makeToolDefLogged("list_annotations", ToolRead,
			"List the activity timeline for a transaction. Each annotation is a single event: comment, tag_added, tag_removed, rule_applied, or category_set. Ordered by created_at ASC. Payload carries kind-specific fields (content for comments, slug for tags, rule_name for rule applications).",
			s.handleListAnnotations, svc),
		makeToolDefLogged("add_transaction_tag", ToolWrite,
			"Attach a tag to a transaction. Tags are an open-ended labeling system — auto-creates a persistent tag if the slug doesn't exist yet. Use note to attach a short rationale that is stored on the tag_added annotation. Idempotent: returns already_present=true if the tag was already attached.",
			s.handleAddTransactionTag, svc),
		makeToolDefLogged("remove_transaction_tag", ToolWrite,
			"Remove a tag from a transaction. A note is recommended (it's recorded on the tag_removed annotation for auditability) but not required. Idempotent: returns already_absent=true if the tag wasn't attached.",
			s.handleRemoveTransactionTag, svc),
		makeToolDefLogged("update_transactions", ToolWrite,
			"Compound write for up to 50 transactions at once. Each operation can: set a category (category_slug), add tags (tags_to_add), remove tags (tags_to_remove), and attach a comment — all atomically per transaction, with annotations written for every change. The preferred tool for closing review work (set category + remove needs-review + explain) in one call. Example operation: {\"transaction_id\":\"k7Xm9pQ2\",\"category_slug\":\"food_and_drink_groceries\",\"tags_to_remove\":[{\"slug\":\"needs-review\",\"note\":\"clearly groceries\"}],\"comment\":\"Costco run\"}. on_error: 'continue' (default — each op in its own DB tx, partial failures OK) or 'abort' (one DB tx, rolls back on first error). Ephemeral tags (needs-review) REQUIRE a non-empty note on removal.",
			s.handleUpdateTransactions, svc),
		makeToolDefLogged("create_tag", ToolWrite,
			"Register a new tag in the system. Admin-only write — agents can auto-create persistent tags implicitly via add_transaction_tag (pass a new slug), so use create_tag only when users need to set display_name/color/lifecycle up front. Slug regex: ^[a-z0-9][a-z0-9\\-:]*[a-z0-9]$. Lifecycle defaults to 'persistent' (user-defined, long-lived). Use 'ephemeral' for workflow trigger tags like needs-review.",
			s.handleCreateTag, svc),
		makeToolDefLogged("update_tag", ToolWrite,
			"Update a tag's mutable fields (display_name, description, color, icon, lifecycle). Slug is immutable — to rename, create a new tag + bulk re-tag + delete old. Identify the tag by UUID, short ID, or slug.",
			s.handleUpdateTag, svc),
		makeToolDefLogged("delete_tag", ToolWrite,
			"Delete a tag. Cascades to transaction_tags (removes the tag from every transaction). Annotations that reference the tag keep their rows with tag_id=NULL (preserves audit trail). Identify the tag by UUID, short ID, or slug.",
			s.handleDeleteTag, svc),
	}
}

// makeToolDefLogged creates a ToolDef with logging and session enforcement.
// This is called during buildToolRegistry when s.svc is available.
func makeToolDefLogged[T any](name string, classification ToolClassification, description string, handler func(context.Context, *mcpsdk.CallToolRequest, T) (*mcpsdk.CallToolResult, any, error), svc *service.Service) ToolDef {
	return ToolDef{
		Tool: mcpsdk.Tool{
			Name:        name,
			Description: description,
		},
		Classification: classification,
		register: func(server *mcpsdk.Server) {
			wrappedHandler := func(ctx context.Context, req *mcpsdk.CallToolRequest, input T) (*mcpsdk.CallToolResult, any, error) {
				// Extract session context via interface.
				var sessionID, reason string
				if sc, ok := any(input).(sessionContextProvider); ok {
					sessionID = sc.GetSessionID()
					reason = sc.GetReason()
				}

				// Enforce session_id + reason on write tools (except create_session).
				if classification == ToolWrite && name != "create_session" {
					if sessionID == "" {
						return errorResult(fmt.Errorf("session_id is required for write operations; call create_session first")), nil, nil
					}
					if reason == "" {
						return errorResult(fmt.Errorf("reason is required for write operations")), nil, nil
					}
				}

				start := time.Now()
				result, out, err := handler(ctx, req, input)
				duration := time.Since(start)

				// Log tool call asynchronously.
				go func() {
					logCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()

					var reqJSON []byte
					if req != nil && req.Params.Arguments != nil {
						reqJSON = truncateBytes(req.Params.Arguments, maxLogBytes)
					}
					var respJSON []byte
					if result != nil {
						if b, err := json.Marshal(result); err == nil {
							respJSON = truncateBytes(b, maxLogBytes)
						}
					}

					actor := service.ActorFromContext(ctx)
					isErr := (result != nil && result.IsError) || err != nil
					svc.LogToolCall(logCtx, service.ToolCallLogInput{
						SessionID:      sessionID,
						ToolName:       name,
						Classification: string(classification),
						Reason:         reason,
						RequestJSON:    reqJSON,
						ResponseJSON:   respJSON,
						IsError:        isErr,
						Actor:          actor,
						DurationMs:     int(duration.Milliseconds()),
					})
				}()

				return result, out, err
			}
			mcpsdk.AddTool(server, &mcpsdk.Tool{
				Name:        name,
				Description: description,
			}, wrappedHandler)
		},
	}
}

const maxLogBytes = 32768 // 32KB max for stored request/response JSON

// truncateBytes returns b if len <= max, otherwise truncates and appends a marker.
func truncateBytes(b []byte, max int) []byte {
	if len(b) <= max {
		return b
	}
	return append(b[:max-50], []byte(`... [truncated]"}`)...)
}

// BuildServer creates a filtered *mcpsdk.Server for the given config.
func (s *MCPServer) BuildServer(cfg MCPServerConfig) *mcpsdk.Server {
	instructions := cfg.Instructions
	if instructions == "" {
		instructions = DefaultInstructions
	}

	server := mcpsdk.NewServer(
		&mcpsdk.Implementation{Name: "breadbox", Version: s.version},
		&mcpsdk.ServerOptions{Instructions: instructions},
	)

	disabledSet := make(map[string]bool)
	for _, name := range cfg.DisabledTools {
		disabledSet[name] = true
	}

	for _, td := range s.allTools {
		if disabledSet[td.Tool.Name] {
			continue
		}
		if td.Classification == ToolWrite && cfg.APIKeyScope == "read_only" {
			continue
		}
		td.register(server)
	}

	s.registerResources(server)
	return server
}

// Server returns a default MCP server with all tools registered (for backward compat / stdio).
func (s *MCPServer) Server() *mcpsdk.Server {
	return s.BuildServer(MCPServerConfig{
		Mode:        "read_write",
		APIKeyScope: "full_access",
	})
}

// registerResources adds MCP resources to a server.
func (s *MCPServer) registerResources(server *mcpsdk.Server) {
	server.AddResource(&mcpsdk.Resource{
		Name:        "Overview",
		URI:         "breadbox://overview",
		Description: "Live summary of the Breadbox data model: user, connection, account, and transaction counts plus the transaction date range. Read this first to understand the dataset scope.",
		MIMEType:    "application/json",
	}, s.handleOverviewResource)

	server.AddResource(&mcpsdk.Resource{
		Name:        "Review Guidelines",
		URI:         "breadbox://review-guidelines",
		Description: "Guidelines for reviewing transactions and creating rules. Read this before processing any reviews or creating transaction rules.",
		MIMEType:    "text/markdown",
	}, s.handleReviewGuidelinesResource)

	server.AddResource(&mcpsdk.Resource{
		Name:        "Report Format",
		URI:         "breadbox://report-format",
		Description: "Report structure templates and formatting guidelines. Read this before submitting reports via submit_report.",
		MIMEType:    "text/markdown",
	}, s.handleReportFormatResource)
}

// AllToolDefs returns the full tool registry for admin display.
func (s *MCPServer) AllToolDefs() []ToolDef {
	return s.allTools
}

// NewHTTPHandler wraps the MCP server in a Streamable HTTP handler with per-request filtering.
func NewHTTPHandler(s *MCPServer, svc *service.Service) http.Handler {
	return mcpsdk.NewStreamableHTTPHandler(
		func(r *http.Request) *mcpsdk.Server {
			// Load MCP config from DB.
			mcpCfg, err := svc.GetMCPConfig(r.Context())
			if err != nil {
				// Fall back to defaults on error.
				mcpCfg = &service.MCPConfig{
					Mode:          "read_write",
					DisabledTools: []string{},
				}
			}

			// Get API key scope from context.
			apiKeyScope := "full_access"
			if apiKey := mw.GetAPIKey(r.Context()); apiKey != nil {
				apiKeyScope = apiKey.Scope
			}

			return s.BuildServer(MCPServerConfig{
				Mode:          mcpCfg.Mode,
				DisabledTools: mcpCfg.DisabledTools,
				Instructions:  mcpCfg.Instructions,
				APIKeyScope:   apiKeyScope,
			})
		},
		nil,
	)
}

// checkWritePermission verifies the requesting API key has write access and
// that the global MCP mode allows writes. This is a belt-and-suspenders guard
// since BuildServer already filters out write tools — but protects against
// TOCTOU races between config changes and server construction.
func (s *MCPServer) checkWritePermission(ctx context.Context) error {
	if apiKey := mw.GetAPIKey(ctx); apiKey != nil && apiKey.Scope == "read_only" {
		return fmt.Errorf("this API key has read-only access and cannot perform write operations")
	}
	return nil
}

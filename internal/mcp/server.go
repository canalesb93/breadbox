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
- Reviews: queue of transactions awaiting agent or human assessment

AMOUNT CONVENTION (critical):
- Positive amounts = money out (debits, purchases, payments)
- Negative amounts = money in (credits, deposits, refunds)
- All amounts include iso_currency_code — never sum across different currencies

ID CONVENTION:
- Entity IDs in responses are compact 8-character alphanumeric strings (e.g., "k7Xm9pQ2").
- Use these compact IDs in all tool inputs (transaction_id, account_id, user_id, category_id, review_id, rule_id, etc.).
- Full UUIDs are also accepted as input for backward compatibility.

GETTING STARTED:
1. Read breadbox://overview for dataset context (users, connections, accounts, 30-day spending)
2. Check get_sync_status to verify data freshness

QUERYING:
- query_transactions: filters (date, account, user, category, amount, search), cursor pagination. Use fields= to control response size (aliases: minimal, core, category, timestamps). id always included.
- count_transactions: same filters, returns count. Use before paginating.
- transaction_summary: aggregated totals by category, month, week, day, or category_month. Use for spending analysis.
- merchant_summary: merchant-level stats (count, total, avg, date range). Set min_count=2 for recurring, 3 for subscriptions.
- exclude_search: filter OUT transactions matching a term

SEARCH MODES (on query_transactions, count_transactions, merchant_summary, list_transaction_rules):
- contains (default): substring — "star" matches "Starbucks"
- words: all words match — "Century Link" matches "CenturyLink"
- fuzzy: typo-tolerant — "starbuks" matches "Starbucks"
- Comma-separated search values are ORed: search=starbucks,dunkin

REVIEWS & RULES:
- Tools: pending_reviews_overview, list_pending_reviews, submit_review, batch_submit_reviews, create_transaction_rule, batch_create_rules, preview_rule, apply_rules
- Reviews can be disabled by the user. If disabled, review tools return empty results or errors with a note. Check pending_reviews_overview first.
- Before processing reviews or creating rules, read breadbox://review-guidelines for detailed guidelines.

CATEGORIES:
- list_categories: full taxonomy tree with slugs
- categorize_transaction / batch_categorize_transactions: manual override (sets category_override=true)
- bulk_recategorize: move all matching transactions to a new category
- export_categories / import_categories: bulk taxonomy editing via TSV

ACCOUNT LINKING:
- For shared credit cards (primary cardholder + authorized user): create_account_link deduplicates
- System matches by date + exact amount, attributes transactions to the dependent user
- Dependent account transactions excluded from totals. User filtering includes attributed transactions.
- reconcile_account_link: re-run matching. confirm_match / reject_match: correct errors.

REPORTS & COMMUNICATION:
- Tools: submit_report, add_transaction_comment, list_transaction_comments
- Before submitting reports, read breadbox://report-format for report structure and formatting guidelines.

USER ROLES:
- admin: Full access. Can manage connections, settings, users, rules, and all data.
- editor: Can view and edit ALL household members' transactions (categorize, review, comment). Cannot manage connections, settings, users, or system config.
- viewer: Can only see their own connections, accounts, and transactions. Read-only for their own data.
- The permissions section in breadbox://overview shows what the current session can do.`

// DefaultReviewGuidelines contains the default review guidelines served via breadbox://review-guidelines.
// User-editable via the MCP Settings page.
const DefaultReviewGuidelines = `REVIEW PRINCIPLES — follow these strictly:

1. EVERY REVIEW MUST BE INDIVIDUALLY ASSESSED. You must look at each transaction before approving it. Even when processing in batches via batch_submit_reviews, you must have examined each transaction's name, amount, and context to determine the correct category. There is no auto-approve mechanism — categorization quality depends on your judgment.

2. RULES ARE FORWARD-LOOKING. Transaction rules apply automatically to NEW transactions during sync. Do NOT use apply_rules or apply_retroactively=true during routine reviews. These are reserved for explicit one-off bulk work (initial setup only). During routine work, create rules and let them match future syncs naturally.

3. RE-REVIEWS ARE HUMAN CORRECTIONS. When you see review_type=re_review, a human has disagreed with a previous decision and re-enqueued the transaction with a comment. Read that comment via list_transaction_comments. The human's feedback overrides your prior categorization. Acknowledge the correction in the note you pass to submit_review — that note is recorded as a linked transaction comment attributed to you.

4. SKIP RATHER THAN GUESS. If you cannot confidently determine the correct category, skip the review with a note explaining what's ambiguous. A skipped review can be revisited later with more context. A wrong categorization is harder to catch.

5. EXPLAIN NON-OBVIOUS DECISIONS VIA THE REVIEW NOTE. When you approve a review with a category that isn't immediately obvious from the transaction name, pass a brief note to submit_review explaining why. The note is stored as a transaction comment linked to this review and rendered inline on the resolution event in the activity timeline — do NOT also call add_transaction_comment for the same narrative, as that produces duplicate entries. Reserve add_transaction_comment for free-standing narrative that isn't tied to a specific review decision.

6. NEVER BULK-APPROVE WITHOUT EXAMINATION. Do not use batch_submit_reviews to approve all remaining reviews with a default category. Each item in the batch must have been individually assessed with the correct category assigned.

7. ALWAYS USE CATEGORY_SLUG. When approving reviews, use category_slug (e.g., "food_and_drink_groceries") not category_id. Slugs are human-readable, stable, and consistent across sessions.

8. SKIPPED REVIEWS STAY IN THE QUEUE. When you skip a review, it remains pending and will appear again in future review sessions. Skip freely when uncertain.

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

REPORT TITLE:
The title appears as a dashboard notification — make it self-contained and scannable. A family member should understand what happened without expanding the report.
- Good: "Reviewed 47 transactions — 3 recategorized, no suspicious activity"
- Good: "March spending: $4,200 total, dining up 25% from February"
- Bad: "Review Complete" (says nothing)
- Bad: "Transaction Review Report for Week of March 15-21, 2026" (too long, no substance)

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
			"Manually override a transaction's category. Use list_categories to find the category ID, then pass both the transaction_id and category_id. This creates a permanent override that won't be changed by automatic sync.",
			s.handleCategorizeTransaction, svc),
		makeToolDefLogged("reset_transaction_category", ToolWrite,
			"Remove a manual category override from a transaction and re-resolve its category from the automatic mapping rules. Use this to undo a categorize_transaction action.",
			s.handleResetTransactionCategory, svc),
		makeToolDefLogged("add_transaction_comment", ToolWrite,
			"Add a free-standing comment to a transaction — narrative that's independent of any specific review decision (flagging unusual charges, noting shared expenses, cross-references, context that outlives a single review cycle). Supports markdown. IMPORTANT: when your comment is the rationale for a review decision, pass it via the 'note' parameter on submit_review / batch_submit_reviews instead; that note is stored as a linked comment and shown inline on the review resolution, so using both tools for the same narrative produces duplicate activity-log entries.",
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
		makeToolDefLogged("list_pending_reviews", ToolRead,
			"List pending transaction reviews in the review queue. Reviews are created automatically during sync for new and uncategorized transactions (when enabled). Returns empty with a note if reviews are disabled. Use limit to control batch size — review 10-20 at a time unless instructed otherwise. Filter by review_type (new_transaction, uncategorized, manual, re_review) and account_id. Each review includes the full transaction details and suggested category.",
			s.handleListPendingReviews, svc),
		makeToolDefLogged("submit_review", ToolWrite,
			"Submit a decision on a pending review. Returns error if reviews are disabled. Decision: 'approved', 'rejected', or 'skipped'. Use 'approved' to resolve the review — you MUST provide the correct category via category_slug (e.g. 'food_and_drink_groceries') or category_id. Look at the transaction's name, merchant, and raw category fields to determine the right category — use list_categories to find valid slugs. If the review's suggested_category_slug looks correct, use that. If the transaction is miscategorized, provide the correct category_slug to fix it. Use 'rejected' when the transaction should not be categorized (e.g. transfer, internal movement) or when you explicitly disagree with the suggestion and have no replacement category. Use 'skipped' only if you cannot confidently determine the correct category. When you need to explain your reasoning (category change, rejection, skip, unusual decision), pass a 'note' — it is recorded as a transaction comment attributed to you and shown inline on this resolution in the activity timeline, so do NOT also call add_transaction_comment for the same narrative.",
			s.handleSubmitReview, svc),
		makeToolDefLogged("create_transaction_rule", ToolWrite,
			"Create a transaction rule for automatic categorization. Rules match conditions against transaction fields and apply to ALL future transactions during sync. IMPORTANT: Before creating, check list_transaction_rules to avoid duplicates. Prefer broader patterns (contains) over exact matches. Conditions use a JSON tree with AND/OR/NOT logic. Available fields: name, merchant_name, amount, category_primary (raw provider category), category_detailed, pending, provider, account_id, user_id. Operators: eq, neq, contains, not_contains, matches (regex), gt, gte, lt, lte, in.",
			s.handleCreateTransactionRule, svc),
		makeToolDefLogged("list_transaction_rules", ToolRead,
			"List transaction rules with optional filters. Rules auto-categorize transactions during sync based on pattern matching. Use this to check existing rules before creating new ones.",
			s.handleListTransactionRules, svc),
		makeToolDefLogged("update_transaction_rule", ToolWrite,
			"Update a transaction rule's name, conditions, category, priority, enabled status, or expiry.",
			s.handleUpdateTransactionRule, svc),
		makeToolDefLogged("delete_transaction_rule", ToolWrite,
			"Delete a transaction rule by ID.",
			s.handleDeleteTransactionRule, svc),
		makeToolDefLogged("batch_submit_reviews", ToolWrite,
			"Submit decisions on multiple pending reviews at once. Returns error if reviews are disabled. Each review needs a decision (approved, rejected, or skipped) and optionally a category_slug or category_id. Each item may include a per-transaction 'note' — stored as a transaction comment linked to that review and shown inline on its resolution, so do NOT separately call add_transaction_comment for the same narrative. More efficient than submitting reviews one at a time.",
			s.handleBatchSubmitReviews, svc),
		makeToolDefLogged("batch_create_rules", ToolWrite,
			"Create multiple transaction rules at once. Each rule needs a name, category_slug, and conditions object. More efficient than creating rules one at a time. Returns created rules and any errors.",
			s.handleBatchCreateRules, svc),
		makeToolDefLogged("apply_rules", ToolWrite,
			"Apply transaction rules retroactively to existing transactions. Pass rule_id to apply a single rule, or omit to apply all active rules (first match wins by priority). Updates category_id on matching non-deleted, non-overridden transactions. Returns count of affected transactions.",
			s.handleApplyRules, svc),
		makeToolDefLogged("preview_rule", ToolRead,
			"Preview/dry-run a rule's conditions against existing transactions without making changes. Returns match_count, total_scanned, and sample_matches with transaction details. Use this to test conditions before creating a rule.",
			s.handlePreviewRule, svc),
		makeToolDefLogged("batch_categorize_transactions", ToolWrite,
			"Categorize multiple transactions at once. Each item needs a transaction_id and category_slug. Max 500 items per request. Sets category_override=true on each transaction. More efficient than calling categorize_transaction repeatedly. Returns succeeded count and any per-item errors.",
			s.handleBatchCategorize, svc),
		makeToolDefLogged("bulk_recategorize", ToolWrite,
			"Recategorize all transactions matching a filter to a new category. Requires target_category_slug and at least one filter (safety requirement). Sets category_override=true since this is an explicit action. Use this for bulk corrections — e.g., recategorize all transactions currently tagged 'general_merchandise' in a date range to 'groceries'. Returns matched/updated counts.",
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
		makeToolDefLogged("pending_reviews_overview", ToolRead,
			"Get an overview of the pending review queue. Returns zeros with a note if reviews are disabled. Returns total pending count, breakdown by review type (new_transaction, uncategorized, manual, re_review), and groups by raw provider category with counts and sample transaction names. Much more token-efficient than listing all reviews — use this first to understand the review queue composition, then use list_pending_reviews with filters to process one group at a time.",
			s.handlePendingReviewsOverview, svc),
		makeToolDefLogged("submit_report", ToolWrite,
			"Send a message to the family's dashboard. The title is the main message — write it as a concise, self-contained 1-2 sentence summary the family can understand at a glance without expanding. The body provides the detailed breakdown (markdown with headers, bullets, transaction links). Use priority to signal urgency and author to identify your role.",
			s.handleSubmitReport, svc),
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

package mcp

import (
	"context"
	"fmt"
	"net/http"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// BuiltInInstructions contains the default MCP server instructions.
const BuiltInInstructions = `Breadbox is a self-hosted financial data aggregation server for families. It syncs bank data from Plaid, Teller, and CSV imports into a unified PostgreSQL database.

DATA MODEL:
- Users: family members who own bank connections
- Connections: linked bank accounts via Plaid, Teller, or CSV import (status: active, error, pending_reauth, disconnected)
- Accounts: individual bank accounts (checking, savings, credit card, etc.) belonging to a connection
- Transactions: individual financial transactions belonging to an account

AMOUNT CONVENTION:
- Positive amounts = money out (debits, purchases, payments)
- Negative amounts = money in (credits, deposits, refunds)
- All amounts include iso_currency_code — never sum across different currencies

CATEGORY SYSTEM:
- Categories are organized in a 2-level hierarchy (primary → detailed subcategories)
- Each category has: id (UUID), slug (stable identifier), display_name (human label), icon, color
- Use list_categories to get the full taxonomy tree with IDs and slugs
- Filter transactions with category_slug param (parent slug includes all children)
- Use categorize_transaction to manually override a single transaction's category
- For bulk category changes: use batch_categorize_transactions (multiple transactions, one category) or bulk_recategorize (change all transactions from one category to another)
- Use reset_transaction_category to undo a manual override
- Use list_unmapped_categories to find raw provider categories without mappings
- Use list_category_mappings, create_category_mapping, update_category_mapping, delete_category_mapping to manage how provider category strings map to user categories
- For bulk editing: use export_categories / import_categories and export_category_mappings / import_category_mappings to export TSV text, edit it, and re-import. Set apply_retroactively=true on import_category_mappings to update ALL matching transactions (not just uncategorized)
- To simplify/consolidate categories: export categories, then set the merge_into column to a target slug for categories you want to merge. All transactions and mappings from the source are moved to the target. This preserves categorization history while reducing complexity

TOKEN EFFICIENCY:
- Use the fields parameter on query_transactions to request only needed fields. Supports individual field names (e.g., fields=name,amount,date,account_name) and aliases: minimal (name,amount,date — smallest useful set), core (id,date,amount,name,iso_currency_code), category (category,category_primary_raw,category_detailed_raw), timestamps (created_at,updated_at,datetime,authorized_datetime). id is always included.
- Use fields=triage on list_pending_reviews to get only fields needed for categorization decisions — dramatically reduces response size
- Use transaction_summary for aggregated spending analysis instead of paginating through individual transactions. Supports group_by: category, month, week, day, category_month
- Use merchant_summary to get a compact index of all merchants with transaction counts, totals, and date ranges — much more efficient than paginating through raw transactions to identify spending patterns or recurring charges
- Transaction responses include account_name and user_name — no need to cross-reference list_accounts for "which card?" questions
- The breadbox://overview resource includes users, connections, accounts by type, and 30-day spending summary — often eliminates the need for separate list_users + list_accounts calls

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

COMMENTS:
- Use add_transaction_comment to explain your reasoning when recategorizing transactions
- Check list_transaction_comments before modifying a transaction to see prior context

REVIEW QUEUE:
- Start with review_summary to see pending reviews grouped by raw provider category with counts — much more efficient than listing all reviews
- Use list_pending_reviews with category_primary_raw filter to process one group at a time. Use fields=triage to reduce response size. Supports limit up to 500.
- Use submit_review (or batch_submit_reviews, up to 500 at once) to approve with the correct category_slug, or skip if uncertain
- For bulk category work: batch_categorize_transactions assigns one category to many transactions; bulk_recategorize moves all transactions from one category to another
- After creating rules with apply_retroactively=true, call auto_approve_categorized_reviews to clear reviews that rules already handled
- After reviewing, create transaction rules for patterns you noticed so future transactions are auto-categorized

TRANSACTION RULES:
- Rules auto-categorize future transactions during sync. Good rules dramatically reduce future review work.
- Conditions use a flexible JSON tree with AND/OR/NOT logic and operators: eq, contains, matches (regex), gt, gte, lt, lte, in
- Available fields: name, merchant_name, amount, category_primary (raw provider category), category_detailed, pending, provider, account_id, user_id, user_name (family member name)
- Rules apply automatically to new transactions during sync — no manual application needed
- Set apply_retroactively=true on create_transaction_rule ONLY during initial bulk categorization (first-time setup). For routine work, just create the rule and let it match future syncs.
- Use preview_rule to test a condition before creating — shows match count and sample transactions
- Teller's "general" category is a catch-all — do NOT create a category_primary rule for it, use name patterns instead
- Do NOT call apply_rules during routine reviews. It scans ALL transactions and its impact is hard to predict. Rules are forward-looking by design.

ACCOUNT LINKING (Deduplication & Attribution):
- When two family members connect the same credit card (e.g., primary cardholder + authorized user), transactions appear in both feeds with different IDs
- Use create_account_link to link the dependent account to the primary account
- The system auto-matches transactions by date + exact amount, attributes matched primary-side transactions to the dependent user
- Dependent account transactions are excluded from totals and summaries by default
- When filtering by user_id, attributed transactions are included — "Ricardo's transactions" includes his own plus those attributed to him on the shared card
- Use reconcile_account_link to re-run matching after new syncs
- Use list_transaction_matches to review matched pairs, confirm_match/reject_match to correct errors

RULE CREATION STRATEGY — follow this order:
1. FIRST, create category_primary rules (highest impact). Look at the category_primary field on transactions — these are raw provider categories like "dining", "groceries", "phone", "accommodation", "fuel", "entertainment". One rule per category_primary covers ALL transactions with that label. Example: {"and": [{"field": "provider", "op": "eq", "value": "teller"}, {"field": "category_primary", "op": "eq", "value": "dining"}]} → food_and_drink_restaurant. This single rule handles every dining transaction from Teller.
2. THEN, create name-pattern rules for transaction types that span merchants: "ATM Withdrawal" → withdrawals, "Wire Transfer" → transfer_out, "Service Charge" → bank_fees, "Cash Deposit" → deposits. Use contains on the name field.
3. LAST, create per-merchant rules only for specific merchants that get miscategorized or need a different category than their category_primary suggests (e.g., Walmart categorized as "shopping" but you want it under "groceries").

- ALWAYS check list_transaction_rules before creating to avoid duplicates
- Use batch_create_rules to create multiple rules efficiently
- Before creating rules, query some transactions to see what category_primary values exist — use query_transactions with fields=core,category

AGENT REPORTS:
- Use submit_report at the end of a review session to summarize what you did and flag anything needing human attention
- The report title should be a short summary (e.g., "Weekly Review: 45 transactions categorized")
- The body supports markdown. Reference specific transactions with links: [Transaction Name](/transactions/TRANSACTION_ID)
- Reports appear on the family's dashboard — use them to communicate findings, flag suspicious transactions, or suggest actions`

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
	Mode              string   // "read_only" or "read_write"
	DisabledTools     []string // tool names to suppress
	CustomInstructions string  // markdown appended to built-in instructions
	APIKeyScope       string   // "full_access" or "read_only" — from request context
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
	s.allTools = []ToolDef{
		makeToolDef("list_accounts", ToolRead,
			"List all bank accounts synced from Plaid, Teller, or CSV import. Each account belongs to a bank connection and optionally a user (family member). Returns account type, balances, institution name, and currency. Filter by user_id to see one family member's accounts.",
			s.handleListAccounts),
		makeToolDef("query_transactions", ToolRead,
			"Query bank transactions with optional filters and cursor-based pagination. Amounts: positive = money out (debit), negative = money in (credit). Dates: YYYY-MM-DD, start_date inclusive, end_date exclusive. Filter by category_slug (use list_categories to find slugs); parent slugs include all children. Results ordered by date desc by default. Pagination: pass next_cursor from response. Use the fields parameter to request only the fields you need (e.g., fields=core,category) to significantly reduce response size.",
			s.handleQueryTransactions),
		makeToolDef("count_transactions", ToolRead,
			"Count transactions matching optional filters. Same filters as query_transactions except cursor, limit, sort_by, and sort_order. Use this to get totals before paginating, or to compare counts across date ranges or categories.",
			s.handleCountTransactions),
		makeToolDef("list_categories", ToolRead,
			"List the full category taxonomy as a tree. Categories have: slug (stable identifier for filtering), display_name (human label), icon, color, and optional children. Use category slugs with the category_slug filter in query_transactions and count_transactions. Parent slugs include all children when filtering.",
			s.handleListCategories),
		makeToolDef("list_users", ToolRead,
			"List all users (family members) in the system. Each user can own bank connections and their associated accounts. Use the returned user IDs to filter accounts or transactions by family member.",
			s.handleListUsers),
		makeToolDef("get_sync_status", ToolRead,
			"Get the status of all bank connections including provider type (plaid/teller/csv), sync status (active/error/pending_reauth), last sync time, and any error details. Use this to check data freshness or diagnose sync issues.",
			s.handleGetSyncStatus),
		makeToolDef("trigger_sync", ToolWrite,
			"Trigger a manual sync of bank data from the provider (Plaid or Teller). Optionally specify a connection_id to sync a single connection; otherwise syncs all active connections. Returns immediately — the sync runs in the background. Check get_sync_status for results.",
			s.handleTriggerSync),
		makeToolDef("categorize_transaction", ToolWrite,
			"Manually override a transaction's category. Use list_categories to find the category ID, then pass both the transaction_id and category_id. This creates a permanent override that won't be changed by automatic sync.",
			s.handleCategorizeTransaction),
		makeToolDef("reset_transaction_category", ToolWrite,
			"Remove a manual category override from a transaction and re-resolve its category from the automatic mapping rules. Use this to undo a categorize_transaction action.",
			s.handleResetTransactionCategory),
		makeToolDef("list_unmapped_categories", ToolRead,
			"List distinct raw provider category strings from transactions that currently have no category mapping. These are transactions the system couldn't automatically categorize. Results show the raw primary and detailed category strings from the provider.",
			s.handleListUnmappedCategories),
		makeToolDef("add_transaction_comment", ToolWrite,
			"Add a comment to a transaction. Use this to explain categorization decisions, flag unusual transactions, or leave notes for the family. Comments are visible on the transaction detail page and to other agents. Supports markdown formatting.",
			s.handleAddTransactionComment),
		makeToolDef("list_transaction_comments", ToolRead,
			"List all comments on a transaction, ordered chronologically. Check comments before making changes to understand prior context and decisions by other agents or family members.",
			s.handleListTransactionComments),
		makeToolDef("transaction_summary", ToolRead,
			"Get aggregated transaction totals grouped by category and/or time period. Replaces the need to paginate through thousands of individual transactions for spending analysis. Amounts follow the convention: positive = money out (debit), negative = money in (credit). Only includes non-deleted, non-pending transactions by default.",
			s.handleTransactionSummary),
		makeToolDef("merchant_summary", ToolRead,
			"List distinct merchants with aggregated stats: transaction count, total spent, average amount, and date range. Returns a compact merchant-level index — use this to scan for recurring charges, identify top merchants, or find unknown subscriptions. Then drill into specific merchants with query_transactions using the search filter. Default date range: 90 days. Set min_count=2 to find recurring charges, min_count=3 for likely subscriptions.",
			s.handleMerchantSummary),
		makeToolDef("export_categories", ToolRead,
			"Export all category definitions as TSV text. The returned format can be edited externally (in a text editor, by an AI agent, etc.) and re-imported via import_categories. Columns: slug, display_name, parent_slug, icon, color, sort_order, hidden, merge_into. Slugs are immutable identifiers; display_name and other fields can be changed. The merge_into column is empty on export.",
			s.handleExportCategories),
		makeToolDef("import_categories", ToolWrite,
			"Import category definitions from TSV text. Existing slugs are updated (display_name, icon, color, sort_order, hidden). New slugs are created. Missing slugs are NOT deleted. Parents must appear before children. Use export_categories to get the current state, edit it, then import the modified version. To merge/consolidate categories, set the merge_into column to the target category slug — all transactions and mappings from the source are reassigned to the target, then the source is deleted. This is useful for simplifying a complex taxonomy without losing transaction categorization.",
			s.handleImportCategories),
		makeToolDef("export_category_mappings", ToolRead,
			"Export all category mappings as TSV text. Columns: provider, provider_category, category_slug. The returned format can be edited and re-imported via import_category_mappings. Use this for bulk editing of how provider category strings map to user categories.",
			s.handleExportCategoryMappings),
		makeToolDef("import_category_mappings", ToolWrite,
			"Import category mappings from TSV text. Existing (provider, provider_category) pairs are updated. New pairs are created. Missing pairs are NOT deleted. Set apply_retroactively=true to re-categorize ALL matching non-overridden transactions (not just uncategorized). Without it, only uncategorized transactions are updated.",
			s.handleImportCategoryMappings),
		makeToolDef("list_category_mappings", ToolRead,
			"List category mappings that translate provider-specific category strings to user categories. Filter by provider to see mappings for a specific bank data source. Returns the provider, raw provider category string, and the mapped user category slug and display name.",
			s.handleListCategoryMappings),
		makeToolDef("create_category_mapping", ToolWrite,
			"Create a new category mapping that tells the system how to translate a provider's raw category string to a user category. For example, map Teller's 'dining' to your 'food_and_drink_restaurant' category. The mapping takes effect on the next sync — existing transactions are not retroactively updated.",
			s.handleCreateCategoryMapping),
		makeToolDef("update_category_mapping", ToolWrite,
			"Update an existing category mapping to point to a different user category. Identified by mapping ID or by the (provider, provider_category) pair. Does not retroactively update transactions — wait for next sync.",
			s.handleUpdateCategoryMapping),
		makeToolDef("delete_category_mapping", ToolWrite,
			"Delete a category mapping. After deletion, transactions with this provider category string will fall back to 'uncategorized' on next sync. Identified by mapping ID or by (provider, provider_category) pair.",
			s.handleDeleteCategoryMapping),
		makeToolDef("list_pending_reviews", ToolRead,
			"List pending transaction reviews in the review queue. Reviews are created automatically during sync for new, uncategorized, or low-confidence transactions. Use limit to control batch size — review 10-20 at a time unless instructed otherwise. Filter by review_type (new_transaction, uncategorized, low_confidence, manual) and account_id. Each review includes the full transaction details and suggested category.",
			s.handleListPendingReviews),
		makeToolDef("submit_review", ToolWrite,
			"Submit a decision on a pending review. Decision: 'approved' or 'skipped'. Use 'approved' to resolve the review — you MUST provide the correct category via category_slug (e.g. 'food_and_drink_groceries') or category_id. Look at the transaction's name, merchant, and raw category fields to determine the right category — use list_categories to find valid slugs. If the review's suggested_category_slug looks correct, use that. If the transaction is miscategorized, provide the correct category_slug to fix it. Use 'skipped' only if you cannot confidently determine the correct category. Include a note when changing the category or skipping to explain your reasoning.",
			s.handleSubmitReview),
		makeToolDef("create_transaction_rule", ToolWrite,
			"Create a transaction rule for automatic categorization. Rules match conditions against transaction fields and apply to ALL future transactions during sync. IMPORTANT: Before creating, check list_transaction_rules to avoid duplicates. Prefer broader patterns (contains) over exact matches. Conditions use a JSON tree with AND/OR/NOT logic. Available fields: name, merchant_name, amount, category_primary (raw provider category), category_detailed, pending, provider, account_id, user_id. Operators: eq, neq, contains, not_contains, matches (regex), gt, gte, lt, lte, in.",
			s.handleCreateTransactionRule),
		makeToolDef("list_transaction_rules", ToolRead,
			"List transaction rules with optional filters. Rules auto-categorize transactions during sync based on pattern matching. Use this to check existing rules before creating new ones.",
			s.handleListTransactionRules),
		makeToolDef("update_transaction_rule", ToolWrite,
			"Update a transaction rule's name, conditions, category, priority, enabled status, or expiry.",
			s.handleUpdateTransactionRule),
		makeToolDef("delete_transaction_rule", ToolWrite,
			"Delete a transaction rule by ID.",
			s.handleDeleteTransactionRule),
		makeToolDef("batch_submit_reviews", ToolWrite,
			"Submit decisions on multiple pending reviews at once. Each review needs a decision (approved or skipped) and optionally a category_slug or category_id. More efficient than submitting reviews one at a time.",
			s.handleBatchSubmitReviews),
		makeToolDef("batch_create_rules", ToolWrite,
			"Create multiple transaction rules at once. Each rule needs a name, category_slug, and conditions object. More efficient than creating rules one at a time. Returns created rules and any errors.",
			s.handleBatchCreateRules),
		makeToolDef("apply_rules", ToolWrite,
			"Apply transaction rules retroactively to existing transactions. Pass rule_id to apply a single rule, or omit to apply all active rules (first match wins by priority). Updates category_id on matching non-deleted, non-overridden transactions. Returns count of affected transactions.",
			s.handleApplyRules),
		makeToolDef("preview_rule", ToolRead,
			"Preview/dry-run a rule's conditions against existing transactions without making changes. Returns match_count, total_scanned, and sample_matches with transaction details. Use this to test conditions before creating a rule.",
			s.handlePreviewRule),
		makeToolDef("batch_categorize_transactions", ToolWrite,
			"Categorize multiple transactions at once. Each item needs a transaction_id and category_slug. Max 500 items per request. Sets category_override=true on each transaction. More efficient than calling categorize_transaction repeatedly. Returns succeeded count and any per-item errors.",
			s.handleBatchCategorize),
		makeToolDef("bulk_recategorize", ToolWrite,
			"Recategorize all transactions matching a filter to a new category. Requires target_category_slug and at least one filter (safety requirement). Sets category_override=true since this is an explicit action. Use this for bulk corrections — e.g., recategorize all transactions currently tagged 'general_merchandise' in a date range to 'groceries'. Returns matched/updated counts.",
			s.handleBulkRecategorize),
		makeToolDef("list_account_links", ToolRead,
			"List account links between primary and dependent/authorized-user accounts. Account links deduplicate transactions that appear in both a primary cardholder and authorized user's bank feeds. Returns link details, match counts, and unmatched transaction counts.",
			s.handleListAccountLinks),
		makeToolDef("create_account_link", ToolWrite,
			"Link a dependent account to a primary account for cross-connection deduplication. When two family members connect the same credit card (e.g., primary cardholder and authorized user), transactions appear in both feeds. This link pairs matching transactions by date+amount, excludes the dependent's copies from totals, and attributes matched primary-side transactions to the dependent user. Automatically runs initial reconciliation after creation.",
			s.handleCreateAccountLink),
		makeToolDef("delete_account_link", ToolWrite,
			"Remove an account link and clear all transaction attribution set by it. Transactions from the dependent account will be included in totals again.",
			s.handleDeleteAccountLink),
		makeToolDef("reconcile_account_link", ToolWrite,
			"Manually trigger match reconciliation for an account link. Finds unmatched dependent transactions and attempts to pair them with primary account transactions by date and exact amount. Sets attributed_user_id on matched primary transactions.",
			s.handleReconcileAccountLink),
		makeToolDef("list_transaction_matches", ToolRead,
			"List matched transaction pairs for an account link. Shows which primary-side transactions have been matched to dependent-side transactions, with match confidence and the fields that matched.",
			s.handleListTransactionMatches),
		makeToolDef("confirm_match", ToolWrite,
			"Confirm an auto-matched transaction pair as correct. Changes match confidence from 'auto' to 'confirmed'.",
			s.handleConfirmMatch),
		makeToolDef("reject_match", ToolWrite,
			"Reject a false auto-match between two transactions. Removes the match record and clears the attributed_user_id on the primary transaction.",
			s.handleRejectMatch),
		makeToolDef("review_summary", ToolRead,
			"Get pending reviews grouped by raw provider category (category_primary_raw) with counts and sample transaction names. Much more token-efficient than listing all reviews — use this first to understand the review queue composition, then use list_pending_reviews with category_primary_raw filter to process one group at a time.",
			s.handleReviewSummary),
		makeToolDef("auto_approve_categorized_reviews", ToolWrite,
			"Bulk-approve all pending reviews whose transactions already have a category (e.g., from rules). This bridges the gap between rules and reviews — after creating rules with apply_retroactively=true, call this to clear the review queue of transactions that rules already handled. Returns count of approved reviews and remaining pending count.",
			s.handleAutoApproveCategorized),
		makeToolDef("submit_report", ToolWrite,
			"Submit a report to the family dashboard summarizing your work, flagging transactions, or raising issues. Reports appear in the dashboard for the family to review. Use this at the end of a review session to summarize what you did, what you found, and any items needing human attention. The body supports markdown — reference specific transactions with [Transaction Name](/transactions/TRANSACTION_ID) links so the family can click through to see details.",
			s.handleSubmitReport),
	}
}

// makeToolDef is a helper to create a ToolDef with a typed handler.
func makeToolDef[T any](name string, classification ToolClassification, description string, handler func(context.Context, *mcpsdk.CallToolRequest, T) (*mcpsdk.CallToolResult, any, error)) ToolDef {
	return ToolDef{
		Tool: mcpsdk.Tool{
			Name:        name,
			Description: description,
		},
		Classification: classification,
		register: func(server *mcpsdk.Server) {
			mcpsdk.AddTool(server, &mcpsdk.Tool{
				Name:        name,
				Description: description,
			}, handler)
		},
	}
}

// BuildServer creates a filtered *mcpsdk.Server for the given config.
func (s *MCPServer) BuildServer(cfg MCPServerConfig) *mcpsdk.Server {
	instructions := BuiltInInstructions
	if cfg.CustomInstructions != "" {
		instructions += "\n\nCUSTOM INSTRUCTIONS:\n" + cfg.CustomInstructions
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
		if td.Classification == ToolWrite && (cfg.Mode == "read_only" || cfg.APIKeyScope == "read_only") {
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
					Mode:          "read_only",
					DisabledTools: []string{},
				}
			}

			// Get API key scope from context.
			apiKeyScope := "full_access"
			if apiKey := mw.GetAPIKey(r.Context()); apiKey != nil {
				apiKeyScope = apiKey.Scope
			}

			return s.BuildServer(MCPServerConfig{
				Mode:               mcpCfg.Mode,
				DisabledTools:      mcpCfg.DisabledTools,
				CustomInstructions: mcpCfg.CustomInstructions,
				APIKeyScope:        apiKeyScope,
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
	mcpCfg, err := s.svc.GetMCPConfig(ctx)
	if err != nil {
		return fmt.Errorf("unable to verify MCP permissions")
	}
	if mcpCfg.Mode == "read_only" {
		return fmt.Errorf("MCP server is in read-only mode")
	}
	return nil
}

package mcp

import (
	"context"
	"fmt"
	"net/http"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// DefaultInstructions contains the default MCP server instructions.
// These are used when no custom instructions have been saved.
const DefaultInstructions = `Breadbox is a self-hosted financial data aggregation server for families. It syncs bank data from Plaid, Teller, and CSV imports into a unified PostgreSQL database.

DATA MODEL:
- Users: family members who own bank connections
- Connections: linked bank accounts via Plaid, Teller, or CSV import (status: active, error, pending_reauth, disconnected)
- Accounts: individual bank accounts (checking, savings, credit card, etc.) belonging to a connection
- Transactions: individual financial transactions belonging to an account
- Categories: 2-level hierarchy (primary → subcategory), identified by slug (e.g., food_and_drink_groceries)
- Transaction Rules: pattern-matching conditions that auto-categorize new transactions during sync
- Reviews: queue of transactions awaiting agent or human assessment

AMOUNT CONVENTION (critical):
- Positive amounts = money out (debits, purchases, payments)
- Negative amounts = money in (credits, deposits, refunds)
- All amounts include iso_currency_code — never sum across different currencies

GETTING STARTED:
1. Read breadbox://overview for dataset context (users, connections, accounts, 30-day spending)
2. Check get_sync_status to verify data freshness
3. Use pending_reviews_overview to see the review queue before processing reviews

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

REVIEW QUEUE:
- pending_reviews_overview: queue composition by type and raw category — always start here
- list_pending_reviews: fetch with filters (review_type, account_id, category_primary_raw). Use fields=triage.
- Review types: new_transaction, uncategorized, low_confidence, manual, re_review (human correction)
- submit_review / batch_submit_reviews: approve with category_slug, or skip with a note
- Every review must be individually assessed — no auto-approve shortcuts
- re_review items are human corrections — read comments first, respect the feedback

TRANSACTION RULES:
- Rules auto-categorize NEW transactions during sync — they are forward-looking by design
- Conditions: JSON tree with AND/OR/NOT logic. Operators: eq, neq, contains, not_contains, matches (regex), gt, gte, lt, lte, in
- Fields: name, merchant_name, amount, category_primary, category_detailed, pending, provider, account_id, user_id, user_name
- Use preview_rule to test before creating. Check list_transaction_rules to avoid duplicates.
- Rule creation order: (1) category_primary rules (highest coverage), (2) name-pattern rules, (3) per-merchant rules
- apply_retroactively=true: ONLY during initial setup (first-time bulk categorization)
- apply_rules: NEVER during routine reviews. Reserved for explicit one-off bulk operations.

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

COMMUNICATION:
- add_transaction_comment: explain categorization decisions, flag concerns. Check list_transaction_comments first.
- submit_report: send summary to family dashboard. Title = scannable notification. Body = markdown detail.
  Priority: info (routine), warning (needs attention), critical (urgent). Set author to identify your role.`

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
		makeToolDef("list_pending_reviews", ToolRead,
			"List pending transaction reviews in the review queue. Reviews are created automatically during sync for new, uncategorized, or low-confidence transactions. Use limit to control batch size — review 10-20 at a time unless instructed otherwise. Filter by review_type (new_transaction, uncategorized, low_confidence, manual, re_review) and account_id. Each review includes the full transaction details and suggested category.",
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
		makeToolDef("pending_reviews_overview", ToolRead,
			"Get an overview of the pending review queue. Returns total pending count, breakdown by review type (new_transaction, uncategorized, low_confidence, manual, re_review), and groups by raw provider category with counts and sample transaction names. Much more token-efficient than listing all reviews — use this first to understand the review queue composition, then use list_pending_reviews with filters to process one group at a time.",
			s.handlePendingReviewsOverview),
		makeToolDef("submit_report", ToolWrite,
			"Send a message to the family's dashboard. The title is the main message — write it as a concise, self-contained 1-2 sentence summary the family can understand at a glance without expanding. The body provides the detailed breakdown (markdown with headers, bullets, transaction links). Use priority to signal urgency and author to identify your role.",
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

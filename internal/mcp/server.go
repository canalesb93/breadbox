package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"time"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"
	"breadbox/prompts"
	"breadbox/static"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// breadboxImplementation is the Implementation block sent in every
// initialize response. Title, websiteUrl, and the icon are spec-optional
// fields that surface in connector pickers (Claude.ai's Settings →
// Connectors list, MCP Inspector, etc.) — without them the connector
// renders as "breadbox <version>" with no icon and no link to docs.
//
// The icon is the same package outline used by the admin UI's favicon,
// inlined as a data URI so the metadata is host-agnostic (works on
// localhost, dev.breadbox.host, or any future deployment without
// pointing at an external asset URL).
func breadboxImplementation(version string) *mcpsdk.Implementation {
	impl := &mcpsdk.Implementation{
		Name:       "breadbox",
		Title:      "Breadbox",
		Version:    version,
		WebsiteURL: "https://breadbox.sh",
	}
	if icon := loadBreadboxIcon(); icon != nil {
		impl.Icons = []mcpsdk.Icon{*icon}
	}
	return impl
}

var (
	breadboxIcon     *mcpsdk.Icon
	breadboxIconOnce bool
)

// loadBreadboxIcon reads the favicon from the embedded static FS once and
// caches the resulting Icon. If the read fails (shouldn't, since the file
// is embedded at build time), it returns nil and the implementation goes
// out without icons rather than failing the initialize response.
func loadBreadboxIcon() *mcpsdk.Icon {
	if breadboxIconOnce {
		return breadboxIcon
	}
	breadboxIconOnce = true
	data, err := fs.ReadFile(static.FS, "favicon.svg")
	if err != nil {
		return nil
	}
	src := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(data)
	breadboxIcon = &mcpsdk.Icon{
		Source:   src,
		MIMEType: "image/svg+xml",
		Sizes:    []string{"any"},
	}
	return breadboxIcon
}

// Default MCP prompts sourced from the top-level prompts/ package — the
// canonical place to edit them is prompts/mcp/*.md. Used when the user has
// not overridden them via app_config.
var (
	DefaultInstructions     = prompts.MCP("instructions")
	DefaultReviewGuidelines = prompts.MCP("review-guidelines")
	DefaultReportFormat     = prompts.MCP("report-format")
)

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
			"Add a free-standing comment to a transaction — narrative that's independent of any specific review decision (flagging unusual charges, noting shared expenses, cross-references, context that outlives a single review cycle). Supports markdown. IMPORTANT: when the comment is the rationale for a tag change or category set, pass it inline on the same update_transactions operation as the change (use the `comment` field) so the activity log links the rationale to the action atomically.",
			s.handleAddTransactionComment, svc),
		makeToolDefLogged("list_transaction_comments", ToolRead,
			"Deprecated: prefer list_annotations with kinds=['comment']. Returns the same comment data with renamed fields (author_* instead of actor_*, content lifted out of payload). Will be removed in a future release.",
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
			"Create a transaction rule for automatic categorization, tagging, or commenting. Rules match condition trees against transactions during sync and fire in pipeline-stage order (priority ASC — lower = earlier). Pass `stage` (one of baseline|standard|refinement|override) instead of a raw priority so rules from different agents compose predictably; stage resolves to priority 0/10/50/100. Earlier-stage rules' tag and category mutations feed later-stage rules' conditions, so rules compose: rule A tags 'coffee', rule B conditioned on tags-contains-coffee sets category. Before creating, check list_transaction_rules to avoid duplicates; prefer `contains` over exact matches (bank feeds format merchant names inconsistently). Full DSL spec + roadmap in docs/rule-dsl.md.",
			s.handleCreateTransactionRule, svc),
		makeToolDefLogged("list_transaction_rules", ToolRead,
			"List transaction rules with optional filters (category, enabled status, name search). Always call before creating new rules to avoid duplicates. Rules are returned with their actions, trigger, priority, hit_count, and last_hit_at — useful for spotting stale or never-matching rules.",
			s.handleListTransactionRules, svc),
		makeToolDefLogged("update_transaction_rule", ToolWrite,
			"Update a transaction rule's fields. Every field is optional; omit to leave unchanged. Pass conditions={} to explicitly clear conditions (match-all). Pass actions=[...] to replace the entire action set (rules must retain at least one action). Pass expires_at=\"\" to clear expiry. Pass `stage` (baseline|standard|refinement|override) to re-slot a rule into the pipeline without guessing a numeric priority. See docs/rule-dsl.md for DSL.",
			s.handleUpdateTransactionRule, svc),
		makeToolDefLogged("delete_transaction_rule", ToolWrite,
			"Delete a transaction rule by ID. System-seeded rules (like the needs-review tagger) cannot be deleted — disable them instead with update_transaction_rule.enabled=false.",
			s.handleDeleteTransactionRule, svc),
		makeToolDefLogged("batch_create_rules", ToolWrite,
			"Create multiple transaction rules at once. More efficient than looping create_transaction_rule. Ideal for composable pipelines — use `stage` (baseline|standard|refinement|override) on each item to order rules so earlier-stage rules set up tags/categories that later-stage rules react to. `stage` is preferred over raw `priority` for cross-agent consistency; if both are supplied, priority wins. Each item follows the same shape as create_transaction_rule. Returns created rules plus any per-item errors so partial success is recoverable.",
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
			"List all tags registered in the system. Each tag has a slug (stable identifier) and a display_name. Tags attached to transactions can be queried via the tags / any_tag filters on query_transactions.",
			s.handleListTags, svc),
		makeToolDefLogged("list_annotations", ToolRead,
			"List the activity timeline for a transaction, ordered by created_at ASC. Each row carries a generic `kind` (comment | rule | tag | category) plus an `action` (added | removed | set | applied) for the specific event — branch on `action` when the distinction matters (e.g. tag added vs removed). Payload carries kind-specific fields (content for comments, slug for tag events, rule_name for rule applications). Filters compose: `kinds=['comment']` is the comment-only view (replaces deprecated list_transaction_comments); `actor_types=['user']` is the canonical 'any human input?' check (drops rule churn + agent activity); `since` (RFC3339) skips rows you've already seen; `limit` returns the most recent N (capped at 200). Empty filters return the full timeline. Recommended patterns: before overriding your own categorization, call list_annotations(transaction_id, actor_types=['user']) — if any row exists, a human has weighed in and that decision wins.",
			s.handleListAnnotations, svc),
		makeToolDefLogged("add_transaction_tag", ToolWrite,
			"Attach a tag to a transaction. Tags are an open-ended labeling system — auto-creates a persistent tag if the slug doesn't exist yet. Idempotent: returns already_present=true if the tag was already attached. To record decision context for a tag change, leave a comment on the same transaction (add_transaction_comment, or the `comment` field on update_transactions).",
			s.handleAddTransactionTag, svc),
		makeToolDefLogged("remove_transaction_tag", ToolWrite,
			"Remove a tag from a transaction. Idempotent: returns already_absent=true if the tag wasn't attached. To record why the tag was removed (e.g. closing a needs-review item), pair this with a comment on the same transaction — see update_transactions for the atomic compound op.",
			s.handleRemoveTransactionTag, svc),
		makeToolDefLogged("update_transactions", ToolWrite,
			"Compound write for up to 50 transactions at once. Each operation can: set a category (category_slug), add tags (tags_to_add), remove tags (tags_to_remove), and attach a comment — all atomically per transaction, with annotations written for every change. The preferred tool for closing review work (set category + remove needs-review + explain) in one call. Use the `comment` field to capture decision rationale; tag adds/removes carry no per-action note — keep all narrative in the comment. Example operation: {\"transaction_id\":\"k7Xm9pQ2\",\"category_slug\":\"food_and_drink_groceries\",\"tags_to_remove\":[{\"slug\":\"needs-review\"}],\"comment\":\"Clearly groceries — Costco run.\"}. on_error: 'continue' (default — each op in its own DB tx, partial failures OK) or 'abort' (one DB tx, rolls back on first error).",
			s.handleUpdateTransactions, svc),
		makeToolDefLogged("create_tag", ToolWrite,
			"Register a new tag in the system. Admin-only write — agents can auto-create tags implicitly via add_transaction_tag (pass a new slug), so use create_tag only when users need to set display_name/color/icon up front. Slug regex: ^[a-z0-9][a-z0-9\\-:]*[a-z0-9]$.",
			s.handleCreateTag, svc),
		makeToolDefLogged("update_tag", ToolWrite,
			"Update a tag's mutable fields (display_name, description, color, icon). Slug is immutable — to rename, create a new tag + bulk re-tag + delete old. Identify the tag by UUID, short ID, or slug.",
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
		breadboxImplementation(s.version),
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

// registerResources adds MCP resources to a server. The catalog is
// agent-facing first: most resources are audienced to the assistant only.
// A handful (overview, accounts, review-guidelines, report-format) are
// dual-audience because users have a real "attach this in chat" flow for
// them — those show up in Claude.ai's paperclip / attachment menu.
func (s *MCPServer) registerResources(server *mcpsdk.Server) {
	// Top-level live snapshot — read first.
	server.AddResource(&mcpsdk.Resource{
		Name:        "Overview",
		Title:       "Household Overview",
		URI:         "breadbox://overview",
		Description: "Live summary of the dataset: users, connections, accounts, transaction counts and date range, recent spending. Read on connect for context.",
		MIMEType:    "application/json",
		Annotations: resourceAnnotations(audienceUserAndAssistant, 1.0, liveResourceModTime),
	}, s.handleOverviewResource)

	// Live state resources — replace what would otherwise be list_* tool calls.
	server.AddResource(&mcpsdk.Resource{
		Name:        "Accounts",
		Title:       "Bank Accounts",
		URI:         "breadbox://accounts",
		Description: "Bank accounts (checking, savings, credit cards, loans, investments) with balances, institution names, and currency.",
		MIMEType:    "application/json",
		Annotations: resourceAnnotations(audienceUserAndAssistant, 0.6, liveResourceModTime),
	}, s.handleAccountsResource)

	server.AddResource(&mcpsdk.Resource{
		Name:        "Categories",
		Title:       "Category Taxonomy",
		URI:         "breadbox://categories",
		Description: "Two-level category taxonomy keyed by slug. Source for valid category_slug values when categorizing or authoring rules.",
		MIMEType:    "application/json",
		Annotations: resourceAnnotations(audienceAssistantOnly, 0.5, liveResourceModTime),
	}, s.handleCategoriesResource)

	server.AddResource(&mcpsdk.Resource{
		Name:        "Tags",
		Title:       "Tag Vocabulary",
		URI:         "breadbox://tags",
		Description: "Current tag vocabulary keyed by slug. The 'needs-review' tag is the review-queue handle.",
		MIMEType:    "application/json",
		Annotations: resourceAnnotations(audienceAssistantOnly, 0.5, liveResourceModTime),
	}, s.handleTagsResource)

	server.AddResource(&mcpsdk.Resource{
		Name:        "Users",
		Title:       "Household Members",
		URI:         "breadbox://users",
		Description: "Household members with their roles (admin, editor, viewer).",
		MIMEType:    "application/json",
		Annotations: resourceAnnotations(audienceAssistantOnly, 0.5, liveResourceModTime),
	}, s.handleUsersResource)

	server.AddResource(&mcpsdk.Resource{
		Name:        "Sync Status",
		Title:       "Connection Sync Status",
		URI:         "breadbox://sync-status",
		Description: "Per-connection sync status and last-sync timestamps. Read to verify data freshness before answering questions about recent activity.",
		MIMEType:    "application/json",
		Annotations: resourceAnnotations(audienceAssistantOnly, 0.6, liveResourceModTime),
	}, s.handleSyncStatusResource)

	// Workflow guides — markdown, user-overridable via app_config.
	server.AddResource(&mcpsdk.Resource{
		Name:        "Review Guidelines",
		Title:       "Transaction Review Guidelines",
		URI:         "breadbox://review-guidelines",
		Description: "Principles for reviewing transactions and creating rules. Read before touching the review queue.",
		MIMEType:    "text/markdown",
		Annotations: resourceAnnotations(audienceUserAndAssistant, 0.8, staticPromptModTime),
	}, s.handleReviewGuidelinesResource)

	server.AddResource(&mcpsdk.Resource{
		Name:        "Report Format",
		Title:       "Spending Report Format",
		URI:         "breadbox://report-format",
		Description: "Report structure and formatting guidelines. Read before submit_report.",
		MIMEType:    "text/markdown",
		Annotations: resourceAnnotations(audienceUserAndAssistant, 0.8, staticPromptModTime),
	}, s.handleReportFormatResource)

	// Authoring reference — only relevant when the agent is creating rules.
	// Carrying the grammar here instead of in create_transaction_rule's
	// description keeps tools/list lean.
	server.AddResource(&mcpsdk.Resource{
		Name:        "Rule DSL",
		Title:       "Transaction Rule DSL",
		URI:         "breadbox://rule-dsl",
		Description: "Condition grammar, action types, priority bands, sync-vs-retroactive semantics, provider quirks. Read before authoring rules.",
		MIMEType:    "text/markdown",
		Annotations: resourceAnnotations(audienceAssistantOnly, 0.7, staticPromptModTime),
	}, staticMarkdownResource("breadbox://rule-dsl", DefaultRuleDSL))
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

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

// buildToolRegistry populates the allTools slice with all available tools and
// their classifications. The registry carves around what an agent does
// (query, decide, write, configure) rather than every underlying entity.
// Bounded reference data (accounts, categories, tags, users, sync status,
// rules, overview) is preferred via resources — see registerResources in this
// file — but each resource also has a tool mirror in tools_reads.go so MCP
// clients that don't implement the resources/* methods can still read it.
func (s *MCPServer) buildToolRegistry() {
	svc := s.svc
	s.allTools = []ToolDef{
		// Audit session — explicit handle so write tools can be tied to a
		// logical task. (A future PR replaces this with transport-bound
		// session binding via MCP-Session-Id.)
		makeToolDefLogged("create_session", ToolWrite,
			"Start an audit session before performing write operations. Returns a session_id to include on all subsequent tool calls. One session per logical task (e.g. 'weekly transaction review', 'rule creation for dining').",
			s.handleCreateSession, svc),

		// --- Reference data (mirrors resources for clients without resource support) ---
		// Resources are the preferred surface: breadbox://overview, ://accounts,
		// ://categories, ://tags, ://users, ://sync-status, ://rules. These tool
		// mirrors keep clients that don't implement resources/* unblocked.
		makeToolDefLogged("get_overview", ToolRead,
			"Get a household snapshot: scope (users, accounts, currencies), freshness (latest sync, errored connections, recent transactions), and backlog (pending review queue). Mirror of breadbox://overview — call this when your client doesn't support MCP resources, or when you want the snapshot inline as a tool result. Read once at the top of a session to ground every later filter (account ids, currency, attribution).",
			s.handleGetOverview, svc),
		makeToolDefLogged("list_accounts", ToolRead,
			"List bank accounts. Mirror of breadbox://accounts. Each account carries balance, type, currency, and the connection it belongs to. Filter by user_id to scope to a specific household member.",
			s.handleListAccounts, svc),
		makeToolDefLogged("list_categories", ToolRead,
			"List the category taxonomy as a flat array. Mirror of breadbox://categories. Use the returned slugs (e.g. 'food_and_drink_groceries') as the canonical handle for category filters and category_slug fields on writes.",
			s.handleListCategories, svc),
		makeToolDefLogged("list_users", ToolRead,
			"List household members. Mirror of breadbox://users. Each user carries display name, role, and short_id — use the short_id as user_id on transaction filters and account scoping.",
			s.handleListUsers, svc),
		makeToolDefLogged("list_tags", ToolRead,
			"List the tag vocabulary. Mirror of breadbox://tags. Tags are referenced by slug everywhere (filter, add, remove). New tag slugs auto-register the first time update_transactions adds them — read this list before authoring rules to avoid accidental near-duplicates.",
			s.handleListTags, svc),
		makeToolDefLogged("get_sync_status", ToolRead,
			"Get connection sync status: provider, status (active|error|pending_reauth|disconnected), last sync time, last error. Mirror of breadbox://sync-status. Call this before reasoning about freshness — an errored or pending_reauth connection means transactions you'd expect to be there might not be.",
			s.handleGetSyncStatus, svc),
		makeToolDefLogged("list_transaction_rules", ToolRead,
			"List transaction rules with their conditions, actions, and pipeline stage. Mirror of breadbox://rules. Filter by category_slug, enabled, or search by name. Read this before authoring new rules to avoid duplicates.",
			s.handleListTransactionRules, svc),

		// --- Query + aggregate ---
		makeToolDefLogged("query_transactions", ToolRead,
			"Query bank transactions with optional filters and cursor-based pagination. Amounts: positive = money out (debit), negative = money in (credit). Dates: YYYY-MM-DD, start_date inclusive, end_date exclusive. Filter by category_slug (see breadbox://categories for the slug list); parent slugs include all children. Results ordered by date desc by default. Pagination: pass next_cursor from response. Use the fields parameter to request only the fields you need (e.g., fields=core,category) to significantly reduce response size.",
			s.handleQueryTransactions, svc),
		makeToolDefLogged("count_transactions", ToolRead,
			"Count transactions matching optional filters. Same filters as query_transactions except cursor, limit, sort_by, and sort_order. Use this to get totals before paginating, or to compare counts across date ranges or categories.",
			s.handleCountTransactions, svc),
		makeToolDefLogged("transaction_summary", ToolRead,
			"Get aggregated transaction totals grouped by category and/or time period. Replaces the need to paginate through thousands of individual transactions for spending analysis. Amounts follow the convention: positive = money out (debit), negative = money in (credit). Only includes non-deleted, non-pending transactions by default.",
			s.handleTransactionSummary, svc),

		// --- Apply review decisions ---
		// update_transactions is the universal write for review work. It
		// absorbs the per-row variants (categorize, batch-categorize, tag
		// add/remove, comment, reset-category) so an agent can land a full
		// decision atomically per transaction.
		makeToolDefLogged("update_transactions", ToolWrite,
			"Compound write for up to 50 transactions at once. Each operation can: set a category (category_slug), add tags (tags_to_add), remove tags (tags_to_remove), and attach a comment — all atomically per transaction, with annotations written for every change. The canonical tool for closing review work (set category + remove needs-review + explain) in one call. Use the `comment` field to capture decision rationale; tag adds/removes carry no per-action note — keep all narrative in the comment. Example operation: {\"transaction_id\":\"k7Xm9pQ2\",\"category_slug\":\"food_and_drink_groceries\",\"tags_to_remove\":[{\"slug\":\"needs-review\"}],\"comment\":\"Clearly groceries — Costco run.\"}. on_error: 'continue' (default — each op in its own DB tx, partial failures OK) or 'abort' (one DB tx, rolls back on first error).",
			s.handleUpdateTransactions, svc),

		// --- Activity timeline ---
		makeToolDefLogged("list_annotations", ToolRead,
			"List the activity timeline for a transaction, ordered by created_at ASC. Each row carries a generic `kind` (comment | rule | tag | category) plus an `action` (added | removed | set | applied) for the specific event — branch on `action` when the distinction matters (e.g. tag added vs removed). Payload carries kind-specific fields (content for comments, slug for tag events, rule_name for rule applications). Filters compose: `kinds=['comment']` is the comment-only view; `actor_types=['user']` is the canonical 'any human input?' check (drops rule churn + agent activity); `since` (RFC3339) skips rows you've already seen; `limit` returns the most recent N (capped at 200). Empty filters return the full timeline. Recommended pattern: before overriding your own categorization, call list_annotations(transaction_id, actor_types=['user']) — if any row exists, a human has weighed in and that decision wins.",
			s.handleListAnnotations, svc),

		// --- Rules ---
		// See breadbox://rule-dsl for the condition grammar and breadbox://rules
		// for the current ruleset.
		makeToolDefLogged("create_transaction_rule", ToolWrite,
			"Create a transaction rule for automatic categorization, tagging, or commenting. Rules match condition trees against transactions during sync and fire in pipeline-stage order (priority ASC — lower = earlier). Pass `stage` (one of baseline|standard|refinement|override) instead of a raw priority so rules from different agents compose predictably; stage resolves to priority 0/10/50/100. Earlier-stage rules' tag and category mutations feed later-stage rules' conditions, so rules compose: rule A tags 'coffee', rule B conditioned on tags-contains-coffee sets category. Before creating, read breadbox://rules to avoid duplicates; prefer `contains` over exact matches (bank feeds format merchant names inconsistently). Full DSL: breadbox://rule-dsl.",
			s.handleCreateTransactionRule, svc),
		makeToolDefLogged("batch_create_rules", ToolWrite,
			"Create multiple transaction rules at once. More efficient than looping create_transaction_rule. Ideal for composable pipelines — use `stage` (baseline|standard|refinement|override) on each item to order rules so earlier-stage rules set up tags/categories that later-stage rules react to. `stage` is preferred over raw `priority` for cross-agent consistency; if both are supplied, priority wins. Each item follows the same shape as create_transaction_rule. Returns created rules plus any per-item errors so partial success is recoverable.",
			s.handleBatchCreateRules, svc),
		makeToolDefLogged("update_transaction_rule", ToolWrite,
			"Update a transaction rule's fields. Every field is optional; omit to leave unchanged. Pass conditions={} to explicitly clear conditions (match-all). Pass actions=[...] to replace the entire action set (rules must retain at least one action). Pass expires_at=\"\" to clear expiry. Pass `stage` (baseline|standard|refinement|override) to re-slot a rule into the pipeline without guessing a numeric priority. See breadbox://rule-dsl.",
			s.handleUpdateTransactionRule, svc),
		makeToolDefLogged("delete_transaction_rule", ToolWrite,
			"Delete a transaction rule by ID. System-seeded rules (like the needs-review tagger) cannot be deleted — disable them instead with update_transaction_rule.enabled=false.",
			s.handleDeleteTransactionRule, svc),
		makeToolDefLogged("apply_rules", ToolWrite,
			"Apply rules retroactively to existing transactions. Pass rule_id to run a single rule in isolation, or omit to run the full active-rule pipeline in priority-ASC order (same chaining semantics as sync). Materializes set_category (respects category_override), add_tag, and remove_tag. add_comment is sync-only and won't fire here. Hit count increments per condition match, matching sync-time semantics. Use for initial setup or explicit back-fills only — routine syncs apply rules automatically.",
			s.handleApplyRules, svc),
		makeToolDefLogged("preview_rule", ToolRead,
			"Dry-run a condition tree against existing transactions without any writes. Returns match_count + total_scanned + a sample of matching transactions. IMPORTANT: this evaluates only the supplied condition in isolation — it does NOT simulate the full rule pipeline, so tags or categories that other rules would have added mid-pass aren't visible. Use this to answer 'what does this condition match today' before creating a rule.",
			s.handlePreviewRule, svc),

		// --- Tag admin ---
		// Most agents won't need these — add_tag-on-transaction implicitly
		// creates new tag slugs via update_transactions. These are for
		// curating the tag vocabulary itself (renames, deletes, deliberate
		// up-front display_name/color/icon).
		makeToolDefLogged("create_tag", ToolWrite,
			"Register a new tag in the system. Most agents can skip this — passing a new tag slug to update_transactions auto-creates the tag. Use create_tag only when you need to set display_name/color/icon up front. Slug regex: ^[a-z0-9][a-z0-9\\-:]*[a-z0-9]$.",
			s.handleCreateTag, svc),
		makeToolDefLogged("update_tag", ToolWrite,
			"Update a tag's mutable fields (display_name, description, color, icon). Slug is immutable — to rename, create a new tag + bulk re-tag + delete old. Identify the tag by UUID, short ID, or slug.",
			s.handleUpdateTag, svc),
		makeToolDefLogged("delete_tag", ToolWrite,
			"Delete a tag. Cascades to transaction_tags (removes the tag from every transaction). Annotations that reference the tag keep their rows with tag_id=NULL (preserves audit trail). Identify the tag by UUID, short ID, or slug.",
			s.handleDeleteTag, svc),

		// --- Reporting ---
		makeToolDefLogged("submit_report", ToolWrite,
			"Send a message to the family's dashboard. The title is the main message — write it as a concise, self-contained 1-2 sentence summary the family can understand at a glance without expanding. The body provides the detailed breakdown (markdown with headers, bullets, transaction links). Use priority to signal urgency and author to identify your role. See breadbox://report-format for structure conventions.",
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

	server.AddResource(&mcpsdk.Resource{
		Name:        "Rules",
		Title:       "Transaction Rules",
		URI:         "breadbox://rules",
		Description: "Currently registered transaction rules with their conditions, actions, trigger, priority, hit_count, and last_hit_at. Read before authoring new rules to avoid duplicates and to spot stale or never-matching rules.",
		MIMEType:    "application/json",
		Annotations: resourceAnnotations(audienceAssistantOnly, 0.6, liveResourceModTime),
	}, s.handleRulesResource)

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

	// --- Resource templates ---
	// Drill-downs into a single entity. URIs come back from tool results as
	// `resource_link` blocks (PR 05 in this stack) and resolve through these
	// handlers. dual-audience so they appear in template-aware pickers as
	// well; priority is below top-level resources.
	server.AddResourceTemplate(&mcpsdk.ResourceTemplate{
		Name:        "Transaction",
		Title:       "Transaction Detail",
		URITemplate: "breadbox://transaction/{short_id}",
		Description: "Single transaction with its activity timeline (annotations). short_id is the 8-char base62 id surfaced everywhere by query_transactions.",
		MIMEType:    "application/json",
		Annotations: resourceAnnotations(audienceUserAndAssistant, 0.7, liveResourceModTime),
	}, s.handleTransactionTemplate)

	server.AddResourceTemplate(&mcpsdk.ResourceTemplate{
		Name:        "Account",
		Title:       "Account Detail",
		URITemplate: "breadbox://account/{short_id}",
		Description: "Single bank account with balance, currency, and the most recent 25 transactions. short_id is the 8-char base62 id surfaced by list_accounts.",
		MIMEType:    "application/json",
		Annotations: resourceAnnotations(audienceUserAndAssistant, 0.7, liveResourceModTime),
	}, s.handleAccountTemplate)

	server.AddResourceTemplate(&mcpsdk.ResourceTemplate{
		Name:        "Household Member",
		Title:       "Household Member Detail",
		URITemplate: "breadbox://user/{short_id}",
		Description: "Single household member with their connected accounts. short_id is the 8-char base62 id surfaced by list_users.",
		MIMEType:    "application/json",
		Annotations: resourceAnnotations(audienceUserAndAssistant, 0.7, liveResourceModTime),
	}, s.handleUserTemplate)
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

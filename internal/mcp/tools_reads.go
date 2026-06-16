//go:build !lite

package mcp

import (
	"context"
	"fmt"

	"breadbox/internal/service"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tools in this file back the bounded reference reads, surfaced to agents
// through get_reference(kind=…). They're the canonical (and for most kinds,
// only) way to read this data: accounts, categories, tags, users, sync status,
// and the rule roster. A subset (overview, sync-status) is ALSO exposed as a
// breadbox:// resource for hosts with an attach UI (Claude.ai's paperclip menu,
// Inspector); those resource handlers in resources.go call the same service
// path, so the two surfaces never drift. Each handler here returns a
// tool-shaped envelope.

// --- Input types ---

type listAccountsInput struct {
	UserID string `json:"user_id,omitempty" jsonschema:"Filter accounts by user ID"`
}

type listCategoriesInput struct {
}

type listUsersInput struct {
}

type listTagsInput struct {
}

type getSyncStatusInput struct {
}

type getOverviewInput struct {
}

type listTransactionRulesInput struct {
	CategorySlug string `json:"category_slug,omitempty" jsonschema:"Filter by category slug"`
	Enabled      *bool  `json:"enabled,omitempty" jsonschema:"Filter by enabled status"`
	Search       string `json:"search,omitempty" jsonschema:"Search by rule name. Comma-separated values are ORed."`
	SearchMode   string `json:"search_mode,omitempty" jsonschema:"Search mode: contains (default), words, fuzzy"`
	Limit        int    `json:"limit,omitempty" jsonschema:"Max results (default 50, max 500)"`
	Cursor       string `json:"cursor,omitempty" jsonschema:"Pagination cursor from previous result"`
	Fields       string `json:"fields,omitempty" jsonschema:"Comma-separated fields to include, to cut response size. Alias: summary (name,enabled,priority,trigger,category,hit_count,last_hit_at; the default — omits the conditions and actions trees). Default when omitted: summary. Pass fields=all for every field including the full conditions/actions. id is always included."`
}

// queryTransactionRulesInput is the rich, filterable/sortable query over the
// rule set — the rules analogue of query_transactions. It is lean-by-default
// (summary projection) like list_transaction_rules, but adds trigger / creator
// / hit-count filters and sorting so an agent can ask targeted questions ("which
// rules never fire?", "highest-impact rules for groceries", "agent-created
// rules") instead of dumping the whole roster and scanning it.
type queryTransactionRulesInput struct {
	CategorySlug string `json:"category_slug,omitempty" jsonschema:"Filter to rules whose set_category action targets this category slug."`
	Enabled      *bool  `json:"enabled,omitempty" jsonschema:"Filter by enabled status (true=enabled only, false=disabled only)."`
	Trigger      string `json:"trigger,omitempty" jsonschema:"Filter by firing trigger: on_create, on_change (alias on_update), always."`
	CreatorType  string `json:"creator_type,omitempty" jsonschema:"Filter by who created the rule: user, agent, system."`
	Search       string `json:"search,omitempty" jsonschema:"Search by rule name. Comma-separated values are ORed."`
	SearchMode   string `json:"search_mode,omitempty" jsonschema:"Search mode: contains (default), words, fuzzy"`
	MinHitCount  *int   `json:"min_hit_count,omitempty" jsonschema:"Filter to rules whose hit_count is >= this value — surfaces high-impact rules. Ignored when only_unused=true."`
	OnlyUnused   *bool  `json:"only_unused,omitempty" jsonschema:"When true, return only rules that have never fired (hit_count=0) — dead or over-specific rules worth pruning."`
	SortBy       string `json:"sort_by,omitempty" jsonschema:"Sort: priority (default, pipeline execution order), hit_count, last_hit_at, created_at, name."`
	SortOrder    string `json:"sort_order,omitempty" jsonschema:"Sort direction: asc or desc. Default per column (desc for hit_count/last_hit_at/created_at, asc for priority/name)."`
	Limit        int    `json:"limit,omitempty" jsonschema:"Max results (default 50, max 500)"`
	Cursor       string `json:"cursor,omitempty" jsonschema:"Pagination cursor from previous result. Only valid with the default priority sort; an explicit sort_by returns a single top-N page with no next_cursor."`
	Fields       string `json:"fields,omitempty" jsonschema:"Comma-separated fields to include, to cut response size. Alias: summary (name,enabled,priority,trigger,category,hit_count,last_hit_at; the default — omits the conditions and actions trees). Default when omitted: summary. Pass fields=all for every field including the full conditions/actions. id is always included."`
}

// getReferenceInput selects which operating-guidance doc to read. These are the
// near-static markdown docs that teach an agent how to drive the server — the
// content that used to live in `breadbox://` markdown resources before resources
// were retired. Serving them as a tool means clients that can't read MCP
// resources (e.g. Claude.ai) can still pull the guidance on demand.
type getReferenceInput struct {
	Kind string `json:"kind" jsonschema:"required,Which guidance doc to read: 'instructions' (how the server is organized + conventions) | 'rule-dsl' (the full transaction-rule condition grammar, action types, and pipeline-stage semantics — read before authoring rules) | 'review-guidelines' (principles for reviewing transactions and creating rules — read before working the review queue) | 'report-format' (structure + formatting for submit_report)."`
}

// handleGetReference returns the requested guidance doc as markdown. The
// instructions / review-guidelines / report-format docs honor the operator's
// app_config overrides (falling back to the embedded defaults); rule-dsl is the
// fixed embedded grammar.
func (s *MCPServer) handleGetReference(_ context.Context, _ *mcpsdk.CallToolRequest, input getReferenceInput) (*mcpsdk.CallToolResult, any, error) {
	cfg, err := s.svc.GetMCPConfig(context.Background())
	if err != nil {
		return errorResult(err), nil, nil
	}
	var content string
	switch input.Kind {
	case "instructions":
		content = orDefault(cfg.Instructions, DefaultInstructions)
	case "review-guidelines":
		content = orDefault(cfg.ReviewGuidelines, DefaultReviewGuidelines)
	case "report-format":
		content = orDefault(cfg.ReportFormat, DefaultReportFormat)
	case "rule-dsl":
		content = DefaultRuleDSL
	default:
		return errorResult(fmt.Errorf("unknown kind %q: must be one of instructions, rule-dsl, review-guidelines, report-format", input.Kind)), nil, nil
	}
	return markdownResult(content), nil, nil
}

// orDefault returns v, or def when v is empty.
func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// markdownResult wraps a guidance doc as a single markdown text content block.
// Unlike jsonResult it does not JSON-encode or ID-compact — the payload is
// human-readable markdown, not a data record.
func markdownResult(md string) *mcpsdk.CallToolResult {
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: md}},
	}
}

// --- Handlers ---

func (s *MCPServer) handleListAccounts(_ context.Context, _ *mcpsdk.CallToolRequest, input listAccountsInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	accounts, err := s.svc.ListAccounts(ctx, optStr(input.UserID))
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(map[string]any{"accounts": accounts})
}

func (s *MCPServer) handleListCategories(_ context.Context, _ *mcpsdk.CallToolRequest, _ listCategoriesInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	categories, err := s.svc.ListCategories(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(map[string]any{"categories": categories})
}

func (s *MCPServer) handleListUsers(_ context.Context, _ *mcpsdk.CallToolRequest, _ listUsersInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	users, err := s.svc.ListUsers(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(map[string]any{"users": users})
}

func (s *MCPServer) handleListTags(_ context.Context, _ *mcpsdk.CallToolRequest, _ listTagsInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	tags, err := s.svc.ListTags(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(map[string]any{"tags": tags})
}

func (s *MCPServer) handleGetSyncStatus(_ context.Context, _ *mcpsdk.CallToolRequest, _ getSyncStatusInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	connections, err := s.svc.ListConnections(ctx, nil)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(map[string]any{"connections": connections})
}

func (s *MCPServer) handleGetOverview(_ context.Context, _ *mcpsdk.CallToolRequest, _ getOverviewInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	stats, err := s.svc.GetOverviewStats(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(stats)
}

func (s *MCPServer) handleListTransactionRules(_ context.Context, _ *mcpsdk.CallToolRequest, input listTransactionRulesInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()

	params := service.TransactionRuleListParams{
		Limit:        input.Limit,
		Cursor:       input.Cursor,
		CategorySlug: optStr(input.CategorySlug),
		Enabled:      input.Enabled,
		Search:       optStr(input.Search),
	}
	var err error
	if params.SearchMode, err = parseSearchMode(input.SearchMode); err != nil {
		return errorResult(err), nil, nil
	}

	// Lean-by-default: the rule roster omits the conditions/actions trees (the
	// heavy, deeply-nested part) unless the caller asks. Pass fields=all to
	// inspect or audit full rule definitions.
	fieldSet, err := parseRuleFieldsDefault(input.Fields)
	if err != nil {
		return errorResult(err), nil, nil
	}

	result, err := s.svc.ListTransactionRules(ctx, params)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return ruleListResult(result, fieldSet)
}

func (s *MCPServer) handleQueryTransactionRules(_ context.Context, _ *mcpsdk.CallToolRequest, input queryTransactionRulesInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()

	params := service.TransactionRuleListParams{
		Limit:        input.Limit,
		Cursor:       input.Cursor,
		CategorySlug: optStr(input.CategorySlug),
		Enabled:      input.Enabled,
		Trigger:      optStr(input.Trigger),
		CreatorType:  optStr(input.CreatorType),
		Search:       optStr(input.Search),
		MinHitCount:  input.MinHitCount,
		SortBy:       input.SortBy,
		SortDir:      input.SortOrder,
	}
	if input.OnlyUnused != nil {
		params.OnlyUnused = *input.OnlyUnused
	}
	var err error
	if params.SearchMode, err = parseSearchMode(input.SearchMode); err != nil {
		return errorResult(err), nil, nil
	}

	fieldSet, err := parseRuleFieldsDefault(input.Fields)
	if err != nil {
		return errorResult(err), nil, nil
	}

	result, err := s.svc.ListTransactionRules(ctx, params)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return ruleListResult(result, fieldSet)
}

// parseRuleFieldsDefault resolves the shared lean-by-default fields contract for
// the rule list/query tools: "" → summary projection, "all" → full struct.
func parseRuleFieldsDefault(raw string) (map[string]bool, error) {
	switch raw {
	case "":
		raw = service.DefaultRuleFields
	case "all":
		raw = "" // ParseRuleFields("") → nil → full struct
	}
	return service.ParseRuleFields(raw)
}

// ruleListResult shapes a rule list result, applying the field projection when
// one is requested. Shared by list_transaction_rules and query_transaction_rules.
func ruleListResult(result *service.TransactionRuleListResult, fieldSet map[string]bool) (*mcpsdk.CallToolResult, any, error) {
	if fieldSet != nil {
		projected := make([]map[string]any, len(result.Rules))
		for i, r := range result.Rules {
			projected[i] = service.FilterRuleFields(r, fieldSet)
		}
		return jsonResult(map[string]any{
			"rules":       projected,
			"next_cursor": result.NextCursor,
			"has_more":    result.HasMore,
			"total":       result.Total,
		})
	}
	return jsonResult(result)
}

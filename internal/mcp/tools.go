package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"breadbox/internal/service"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- Session context types ---

// sessionContextProvider extracts session tracking fields from tool inputs.
type sessionContextProvider interface {
	GetSessionID() string
	GetReason() string
}

// WriteSessionContext is embedded in write tool inputs. session_id and reason are required.
type WriteSessionContext struct {
	SessionID string `json:"session_id" jsonschema:"required,Session ID from create_session tool. Call create_session first."`
	Reason    string `json:"reason" jsonschema:"required,Brief reason for this action (e.g. 'categorizing grocery transactions')."`
}

func (w WriteSessionContext) GetSessionID() string { return w.SessionID }
func (w WriteSessionContext) GetReason() string    { return w.Reason }

// ReadSessionContext is embedded in read tool inputs. Both fields are optional.
type ReadSessionContext struct {
	SessionID string `json:"session_id,omitempty" jsonschema:"Optional session ID to associate this read with a session."`
	Reason    string `json:"reason,omitempty" jsonschema:"Optional brief reason for this query."`
}

func (r ReadSessionContext) GetSessionID() string { return r.SessionID }
func (r ReadSessionContext) GetReason() string    { return r.Reason }

// --- Session tool input ---

type createSessionInput struct {
	Purpose string `json:"purpose" jsonschema:"required,Brief label for this session (e.g. 'weekly transaction review', 'rule creation for dining')."`
}

// --- Input types ---

type queryTransactionsInput struct {
	ReadSessionContext
	StartDate     string   `json:"start_date,omitempty" jsonschema:"Start date (YYYY-MM-DD) inclusive"`
	EndDate       string   `json:"end_date,omitempty" jsonschema:"End date (YYYY-MM-DD) exclusive"`
	AccountID     string   `json:"account_id,omitempty" jsonschema:"Filter by account ID"`
	UserID        string   `json:"user_id,omitempty" jsonschema:"Filter by user ID"`
	CategorySlug  string   `json:"category_slug,omitempty" jsonschema:"Filter by category slug (parent slug includes all children). See breadbox://categories for the slug list."`
	MinAmount     *float64 `json:"min_amount,omitempty" jsonschema:"Minimum amount (positive=debit, negative=credit)"`
	MaxAmount     *float64 `json:"max_amount,omitempty" jsonschema:"Maximum amount (positive=debit, negative=credit)"`
	Pending       *bool    `json:"pending,omitempty" jsonschema:"Filter by pending status"`
	Search        string   `json:"search,omitempty" jsonschema:"Search transaction name or merchant. Comma-separated values are ORed (e.g. starbucks,amazon matches either)."`
	SearchMode    string   `json:"search_mode,omitempty" jsonschema:"How to match the search term: contains (default, substring match), words (all words must match, good for multi-word queries), fuzzy (typo-tolerant via trigram similarity)"`
	ExcludeSearch string   `json:"exclude_search,omitempty" jsonschema:"Exclude transactions whose name or merchant matches this text. Comma-separated values are ORed. Use to filter out known merchants."`
	Tags          []string `json:"tags,omitempty" jsonschema:"Filter to transactions that have EVERY tag slug in this list (AND semantics). See breadbox://tags for the available vocabulary."`
	AnyTag        []string `json:"any_tag,omitempty" jsonschema:"Filter to transactions that have AT LEAST ONE tag slug in this list (OR semantics)."`
	Limit         int      `json:"limit,omitempty" jsonschema:"Max results (default 50, max 500)"`
	Cursor        string   `json:"cursor,omitempty" jsonschema:"Pagination cursor from previous result"`
	SortBy        string   `json:"sort_by,omitempty" jsonschema:"Sort: date (default), amount, provider_name"`
	SortOrder     string   `json:"sort_order,omitempty" jsonschema:"Sort direction: desc (default) or asc"`
	Fields        string   `json:"fields,omitempty" jsonschema:"Comma-separated list of fields to include in response. Aliases: minimal (provider_name,amount,date), core (id,date,amount,provider_name,iso_currency_code), category (category,provider_category_primary,provider_category_detailed), timestamps (created_at,updated_at,datetime,authorized_datetime). Default: all fields. id is always included."`
}

type countTransactionsInput struct {
	ReadSessionContext
	StartDate     string   `json:"start_date,omitempty" jsonschema:"Start date (YYYY-MM-DD) inclusive"`
	EndDate       string   `json:"end_date,omitempty" jsonschema:"End date (YYYY-MM-DD) exclusive"`
	AccountID     string   `json:"account_id,omitempty" jsonschema:"Filter by account ID"`
	UserID        string   `json:"user_id,omitempty" jsonschema:"Filter by user ID"`
	CategorySlug  string   `json:"category_slug,omitempty" jsonschema:"Filter by category slug"`
	MinAmount     *float64 `json:"min_amount,omitempty" jsonschema:"Minimum amount"`
	MaxAmount     *float64 `json:"max_amount,omitempty" jsonschema:"Maximum amount"`
	Pending       *bool    `json:"pending,omitempty" jsonschema:"Filter by pending status"`
	Search        string   `json:"search,omitempty" jsonschema:"Search name or merchant. Comma-separated values are ORed."`
	SearchMode    string   `json:"search_mode,omitempty" jsonschema:"Search mode: contains (default), words, fuzzy"`
	ExcludeSearch string   `json:"exclude_search,omitempty" jsonschema:"Exclude transactions matching this text"`
	Tags          []string `json:"tags,omitempty" jsonschema:"Filter to transactions that have EVERY tag slug in this list (AND semantics)."`
	AnyTag        []string `json:"any_tag,omitempty" jsonschema:"Filter to transactions that have AT LEAST ONE tag slug in this list (OR semantics)."`
}

type transactionSummaryInput struct {
	ReadSessionContext
	StartDate      string `json:"start_date,omitempty" jsonschema:"Start date (YYYY-MM-DD) inclusive. Defaults to 30 days ago."`
	EndDate        string `json:"end_date,omitempty" jsonschema:"End date (YYYY-MM-DD) exclusive. Defaults to today."`
	GroupBy        string `json:"group_by" jsonschema:"required,How to group results: category, month, week, day, or category_month"`
	AccountID      string `json:"account_id,omitempty" jsonschema:"Filter by account ID"`
	UserID         string `json:"user_id,omitempty" jsonschema:"Filter by user ID (family member)"`
	Category       string `json:"category,omitempty" jsonschema:"Filter by primary category before aggregating"`
	IncludePending *bool  `json:"include_pending,omitempty" jsonschema:"Include pending transactions (default false)"`
}

// --- Handlers ---

func (s *MCPServer) handleCreateSession(reqCtx context.Context, _ *mcpsdk.CallToolRequest, input createSessionInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(reqCtx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.Purpose == "" {
		return errorResult(fmt.Errorf("purpose is required")), nil, nil
	}
	actor := service.ActorFromContext(reqCtx)
	session, err := s.svc.CreateMCPSession(context.Background(), actor, input.Purpose)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(session)
}

// handleQueryTransactions runs the canonical transaction read.
//
// Example call (find every needs-review transaction in March, lean fields):
//
//	{
//	  "tags": ["needs-review"],
//	  "start_date": "2026-03-01",
//	  "end_date": "2026-04-01",
//	  "fields": "core,category",
//	  "limit": 100
//	}
//
// Example response:
//
//	{
//	  "transactions": [
//	    {"id":"k7Xm9pQ2","date":"2026-03-15","amount":4.5,
//	     "provider_name":"Starbucks","iso_currency_code":"USD",
//	     "category":{"slug":"food_and_drink_coffee","display_name":"Coffee"}},
//	    ...
//	  ],
//	  "next_cursor": "eyJkYXRlIjoi...",
//	  "has_more": true,
//	  "limit": 100
//	}
func (s *MCPServer) handleQueryTransactions(_ context.Context, _ *mcpsdk.CallToolRequest, input queryTransactionsInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	params := service.TransactionListParams{
		Cursor:        input.Cursor,
		Limit:         input.Limit,
		AccountID:     optStr(input.AccountID),
		UserID:        optStr(input.UserID),
		CategorySlug:  optStr(input.CategorySlug),
		MinAmount:     input.MinAmount,
		MaxAmount:     input.MaxAmount,
		Pending:       input.Pending,
		Search:        optStr(input.Search),
		ExcludeSearch: optStr(input.ExcludeSearch),
		Tags:          input.Tags,
		AnyTag:        input.AnyTag,
		SortBy:        optStr(input.SortBy),
		SortOrder:     optStr(input.SortOrder),
	}

	var err error
	if params.StartDate, params.EndDate, err = parseDateRange(input.StartDate, input.EndDate); err != nil {
		return errorResult(err), nil, nil
	}
	if params.SearchMode, err = parseSearchMode(input.SearchMode); err != nil {
		return errorResult(err), nil, nil
	}

	fieldSet, err := service.ParseFields(input.Fields)
	if err != nil {
		return errorResult(err), nil, nil
	}

	result, err := s.svc.ListTransactions(ctx, params)
	if err != nil {
		return errorResult(err), nil, nil
	}

	// Normalize attribution: merge attributed_user into user_name so MCP
	// consumers see a single effective user without needing to understand
	// the account-linking internals.
	for i := range result.Transactions {
		service.NormalizeTransactionAttribution(&result.Transactions[i])
	}

	if fieldSet != nil {
		filtered := make([]map[string]any, len(result.Transactions))
		for i, t := range result.Transactions {
			filtered[i] = service.FilterTransactionFields(t, fieldSet)
		}
		return jsonResult(map[string]any{
			"transactions": filtered,
			"next_cursor":  result.NextCursor,
			"has_more":     result.HasMore,
			"limit":        result.Limit,
		})
	}

	return jsonResult(result)
}

func (s *MCPServer) handleCountTransactions(_ context.Context, _ *mcpsdk.CallToolRequest, input countTransactionsInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	params := service.TransactionCountParams{
		AccountID:     optStr(input.AccountID),
		UserID:        optStr(input.UserID),
		CategorySlug:  optStr(input.CategorySlug),
		MinAmount:     input.MinAmount,
		MaxAmount:     input.MaxAmount,
		Pending:       input.Pending,
		Search:        optStr(input.Search),
		ExcludeSearch: optStr(input.ExcludeSearch),
		Tags:          input.Tags,
		AnyTag:        input.AnyTag,
	}

	var err error
	if params.StartDate, params.EndDate, err = parseDateRange(input.StartDate, input.EndDate); err != nil {
		return errorResult(err), nil, nil
	}
	if params.SearchMode, err = parseSearchMode(input.SearchMode); err != nil {
		return errorResult(err), nil, nil
	}

	count, err := s.svc.CountTransactionsFiltered(ctx, params)
	if err != nil {
		return errorResult(err), nil, nil
	}

	return jsonResult(map[string]int64{"count": count})
}

// handleTransactionSummary aggregates totals over a window.
//
// Example call (last month grouped by category):
//
//	{
//	  "group_by": "category",
//	  "start_date": "2026-03-01",
//	  "end_date": "2026-04-01"
//	}
//
// Example response:
//
//	{
//	  "summary": [
//	    {"category":"food_and_drink_groceries","total_amount":612.40,"transaction_count":18},
//	    {"category":"food_and_drink_restaurant","total_amount":238.15,"transaction_count":11},
//	    ...
//	  ],
//	  "totals": {"total_amount": 4521.30, "transaction_count": 142, "iso_currency_code": "USD"},
//	  "filters": {"start_date":"2026-03-01","end_date":"2026-04-01","group_by":"category"}
//	}
func (s *MCPServer) handleTransactionSummary(_ context.Context, _ *mcpsdk.CallToolRequest, input transactionSummaryInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()

	params := service.TransactionSummaryParams{
		GroupBy:   input.GroupBy,
		AccountID: optStr(input.AccountID),
		UserID:    optStr(input.UserID),
		Category:  optStr(input.Category),
	}

	var err error
	if params.StartDate, params.EndDate, err = parseDateRange(input.StartDate, input.EndDate); err != nil {
		return errorResult(err), nil, nil
	}
	if input.IncludePending != nil && *input.IncludePending {
		params.IncludePending = true
	}

	result, err := s.svc.GetTransactionSummary(ctx, params)
	if err != nil {
		return errorResult(err), nil, nil
	}

	return jsonResult(result)
}

// --- Transaction Rules ---

type createTransactionRuleInput struct {
	WriteSessionContext
	Name               string              `json:"name" jsonschema:"required,Name for this rule (human-readable description)"`
	Conditions         map[string]any      `json:"conditions,omitempty" jsonschema:"JSON condition tree. Omit or pass {} to match every transaction. Leaf: {\"field\":\"...\",\"op\":\"...\",\"value\":...}. Combinators: {\"and\":[...]}, {\"or\":[...]}, {\"not\":{...}} (nest freely, max depth 10). Fields: provider_name provider_merchant_name amount provider_category_primary provider_category_detailed category(assigned slug, live-updated by earlier-stage rules) pending provider account_id account_name user_id user_name tags. Ops: string/category=eq|neq|contains|not_contains|matches(RE2)|in; numeric=eq|neq|gt|gte|lt|lte; bool=eq|neq; tags=contains|not_contains|in. Nested example: {\"or\":[{\"and\":[{\"field\":\"provider_merchant_name\",\"op\":\"contains\",\"value\":\"starbucks\"},{\"field\":\"amount\",\"op\":\"gte\",\"value\":5}]},{\"field\":\"tags\",\"op\":\"contains\",\"value\":\"coffee\"}]}. Full spec: docs/rule-dsl.md."`
	Actions            []map[string]string `json:"actions,omitempty" jsonschema:"Array of typed actions. {\"type\":\"set_category\",\"category_slug\":\"...\"} | {\"type\":\"add_tag\",\"tag_slug\":\"...\"} | {\"type\":\"remove_tag\",\"tag_slug\":\"...\"} | {\"type\":\"add_comment\",\"content\":\"...\"}. Actions compose: a rule can set a category AND add a tag AND add a comment in the same match. add_comment fires only at sync time (not on retroactive apply). remove_tag net-diffs against add_tag within the same sync pass. If omitted, use category_slug instead."`
	CategorySlug       string              `json:"category_slug,omitempty" jsonschema:"Shorthand for actions: [{\"type\":\"set_category\",\"category_slug\":\"<slug>\"}]. Either actions or category_slug is required."`
	Trigger            string              `json:"trigger,omitempty" jsonschema:"When the rule fires during sync: 'on_create' (default — first-synced transactions) | 'on_change' (existing transactions that changed on re-sync) | 'always' (both). 'on_update' is accepted as a legacy alias for 'on_change'. Retroactive apply ignores trigger."`
	Stage              string              `json:"stage,omitempty" jsonschema:"Semantic pipeline stage — preferred over raw priority for agent-authored rules. One of: 'baseline' (runs first, broad defaults), 'standard' (default), 'refinement' (reacts to earlier stages), 'override' (runs last, wins set_category). Resolves to priority 0/10/50/100. If both stage and priority are supplied, priority wins. Leave both unset for 'standard'."`
	Priority           int                 `json:"priority,omitempty" jsonschema:"Raw pipeline-stage integer, 0..1000. Lower runs first. Prefer 'stage' for shared vocabulary. Canonical values: 0=baseline, 10=standard (default), 50=refinement, 100=override. Higher-priority rules observe earlier-stage rules' tag/category mutations via conditions, and win set_category under last-writer semantics."`
	ExpiresIn          string              `json:"expires_in,omitempty" jsonschema:"Optional expiry duration: 24h, 30d, 1w. Rule auto-disables after this period."`
	ApplyRetroactively bool                `json:"apply_retroactively,omitempty" jsonschema:"If true, immediately apply this rule to existing transactions after creation. Materializes set_category / add_tag / remove_tag; skips add_comment (sync-only). Hit count reflects every condition match, matching sync-time semantics."`
}

type updateTransactionRuleInput struct {
	WriteSessionContext
	ID           string               `json:"id" jsonschema:"required,UUID of the rule to update"`
	Name         *string              `json:"name,omitempty" jsonschema:"New name for the rule. Omit to leave unchanged."`
	Conditions   map[string]any       `json:"conditions,omitempty" jsonschema:"New condition tree (same format as create). Pass {} to explicitly change to match-all. Omit entirely to leave conditions unchanged."`
	Actions      *[]map[string]string `json:"actions,omitempty" jsonschema:"Replace the entire actions array with typed actions: {\"type\":\"set_category|add_tag|remove_tag|add_comment\", ...}. Pass an empty array to reject (rules must have at least one action). Omit to leave actions unchanged."`
	CategorySlug *string              `json:"category_slug,omitempty" jsonschema:"Shorthand: replace only the set_category action. Other action types on the rule are preserved. Omit to leave unchanged."`
	Trigger      *string              `json:"trigger,omitempty" jsonschema:"New trigger: on_create, on_change, or always. 'on_update' accepted as alias for on_change. Omit to leave unchanged."`
	Stage        *string              `json:"stage,omitempty" jsonschema:"New semantic pipeline stage: baseline | standard | refinement | override (resolves to priority 0/10/50/100). Preferred alias for agent-authored updates. If both stage and priority are supplied, priority wins. Omit to leave unchanged."`
	Priority     *int                 `json:"priority,omitempty" jsonschema:"New raw priority (pipeline stage). Prefer 'stage' for shared vocabulary. Omit to leave unchanged."`
	Enabled      *bool                `json:"enabled,omitempty" jsonschema:"Enable or disable the rule. Disabled rules are excluded from sync + retroactive apply."`
	ExpiresAt    *string              `json:"expires_at,omitempty" jsonschema:"New expiry timestamp (RFC3339) or empty string to clear expiry entirely. Omit to leave unchanged."`
}

type deleteTransactionRuleInput struct {
	WriteSessionContext
	ID string `json:"id" jsonschema:"required,UUID of the rule to delete"`
}

type batchCreateRulesInput struct {
	WriteSessionContext
	Rules []batchRuleItem `json:"rules" jsonschema:"required,Array of rules to create. Ideal for composable pipelines. Example — tagging then categorizing then flagging: [{\"name\":\"Tag coffee shops\",\"priority\":0,\"conditions\":{\"field\":\"provider_merchant_name\",\"op\":\"contains\",\"value\":\"starbucks\"},\"actions\":[{\"type\":\"add_tag\",\"tag_slug\":\"coffee\"}]},{\"name\":\"Categorize coffee-tagged\",\"priority\":10,\"conditions\":{\"field\":\"tags\",\"op\":\"contains\",\"value\":\"coffee\"},\"actions\":[{\"type\":\"set_category\",\"category_slug\":\"food_and_drink_coffee\"}]},{\"name\":\"Flag expensive coffee\",\"priority\":50,\"conditions\":{\"and\":[{\"field\":\"tags\",\"op\":\"contains\",\"value\":\"coffee\"},{\"field\":\"amount\",\"op\":\"gt\",\"value\":15}]},\"actions\":[{\"type\":\"add_tag\",\"tag_slug\":\"expensive\"}]}]"`
}

type batchRuleItem struct {
	Name         string              `json:"name" jsonschema:"required,Human-readable rule name"`
	Actions      []map[string]string `json:"actions,omitempty" jsonschema:"Actions array (typed — same format as create_transaction_rule)"`
	CategorySlug string              `json:"category_slug,omitempty" jsonschema:"Shorthand for set_category action. Either actions or category_slug required."`
	Conditions   map[string]any      `json:"conditions,omitempty" jsonschema:"Condition tree as JSON object. Omit or {} for match-all."`
	Trigger      string              `json:"trigger,omitempty" jsonschema:"on_create (default), on_change, or always. 'on_update' accepted as alias for on_change."`
	Stage        string              `json:"stage,omitempty" jsonschema:"Semantic pipeline stage: baseline | standard (default) | refinement | override — resolves to priority 0/10/50/100. Prefer over raw priority for cross-agent consistency. If both are supplied, priority wins."`
	Priority     int                 `json:"priority,omitempty" jsonschema:"Raw priority integer. Prefer 'stage' for shared vocabulary. Defaults to 10 (standard)."`
	ExpiresIn    string              `json:"expires_in,omitempty" jsonschema:"Optional expiry duration"`
}

type applyRulesInput struct {
	WriteSessionContext
	RuleID string `json:"rule_id,omitempty" jsonschema:"Optional ID (UUID or short_id) of a specific rule to apply. When supplied, only that rule runs — no chaining. Omit to apply all active rules in pipeline-stage order (priority ASC); earlier rules' tag/category mutations feed later rules' conditions, exactly like sync-time. Materializes set_category / add_tag / remove_tag; add_comment stays sync-only. Ignores rule.trigger (retroactive is a bulk op)."`
}

type previewRuleInput struct {
	ReadSessionContext
	Conditions map[string]any `json:"conditions" jsonschema:"required,Condition tree to evaluate against existing transactions. Same grammar as create_transaction_rule.conditions. Preview evaluates this single condition in isolation against stored data — it does NOT simulate the full rule pipeline, so tags or categories that other rules would have added don't influence the result. Use this to answer 'what does this condition match today' before creating the rule."`
	SampleSize int            `json:"sample_size,omitempty" jsonschema:"Number of sample matching transactions to return (default 10, max 50). The match_count in the response reflects the full match set, not just the sample."`
}

// handleCreateTransactionRule wires a single rule into the auto-categorization
// pipeline. See breadbox://rule-dsl for the condition grammar.
//
// Example call (per-merchant override at the refinement stage):
//
//	{
//	  "name": "provider_merchant_name: Costco → groceries",
//	  "stage": "refinement",
//	  "trigger": "on_create",
//	  "conditions": {
//	    "field": "provider_merchant_name",
//	    "op": "contains",
//	    "value": "costco"
//	  },
//	  "category_slug": "food_and_drink_groceries"
//	}
//
// Example response:
//
//	{
//	  "id": "p9Q4nT2x",
//	  "name": "provider_merchant_name: Costco → groceries",
//	  "trigger": "on_create",
//	  "priority": 50,
//	  "actions": [{"type":"set_category","category_slug":"food_and_drink_groceries"}],
//	  "enabled": true,
//	  "hit_count": 0
//	}
func (s *MCPServer) handleCreateTransactionRule(ctx context.Context, _ *mcpsdk.CallToolRequest, input createTransactionRuleInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.Name == "" {
		return errorResult(fmt.Errorf("name is required")), nil, nil
	}
	if len(input.Actions) == 0 && input.CategorySlug == "" {
		return errorResult(fmt.Errorf("either actions or category_slug is required")), nil, nil
	}

	conditions, err := parseConditions(input.Conditions)
	if err != nil {
		return errorResult(err), nil, nil
	}

	actor := service.ActorFromContext(ctx)

	rule, err := s.svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:         input.Name,
		Conditions:   conditions,
		Actions:      convertMCPActions(input.Actions),
		CategorySlug: input.CategorySlug,
		Trigger:      input.Trigger,
		Priority:     input.Priority,
		Stage:        input.Stage,
		ExpiresIn:    input.ExpiresIn,
		Actor:        actor,
	})
	if err != nil {
		return errorResult(err), nil, nil
	}

	resp := map[string]any{"rule": rule}
	if input.ApplyRetroactively {
		count, err := s.svc.ApplyRuleRetroactively(ctx, rule.ID)
		if err != nil {
			resp["retroactive_error"] = err.Error()
		} else {
			resp["retroactive_matches"] = count
		}
	}
	return jsonResult(resp)
}

func (s *MCPServer) handleUpdateTransactionRule(ctx context.Context, _ *mcpsdk.CallToolRequest, input updateTransactionRuleInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.ID == "" {
		return errorResult(fmt.Errorf("id is required")), nil, nil
	}

	var conditionsPtr *service.Condition
	if len(input.Conditions) > 0 {
		c, err := parseConditions(input.Conditions)
		if err != nil {
			return errorResult(err), nil, nil
		}
		conditionsPtr = &c
	}

	var actionsPtr *[]service.RuleAction
	if input.Actions != nil {
		converted := convertMCPActions(*input.Actions)
		actionsPtr = &converted
	}

	rule, err := s.svc.UpdateTransactionRule(ctx, input.ID, service.UpdateTransactionRuleParams{
		Name:         input.Name,
		Conditions:   conditionsPtr,
		Actions:      actionsPtr,
		CategorySlug: input.CategorySlug,
		Trigger:      input.Trigger,
		Priority:     input.Priority,
		Stage:        input.Stage,
		Enabled:      input.Enabled,
		ExpiresAt:    input.ExpiresAt,
	})
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(rule)
}

func (s *MCPServer) handleDeleteTransactionRule(ctx context.Context, _ *mcpsdk.CallToolRequest, input deleteTransactionRuleInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.ID == "" {
		return errorResult(fmt.Errorf("id is required")), nil, nil
	}

	if err := s.svc.DeleteTransactionRule(ctx, input.ID); err != nil {
		return errorResult(err), nil, nil
	}

	return jsonResult(map[string]any{
		"deleted": true,
		"id":      input.ID,
	})
}

func (s *MCPServer) handleApplyRules(ctx context.Context, _ *mcpsdk.CallToolRequest, input applyRulesInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}

	if input.RuleID != "" {
		count, err := s.svc.ApplyRuleRetroactively(ctx, input.RuleID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(map[string]any{
			"rule_id":        input.RuleID,
			"affected_count": count,
		})
	}

	results, err := s.svc.ApplyAllRulesRetroactively(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	var totalAffected int64
	for _, count := range results {
		totalAffected += count
	}
	return jsonResult(map[string]any{
		"rules_applied":  results,
		"total_affected": totalAffected,
	})
}

func (s *MCPServer) handlePreviewRule(_ context.Context, _ *mcpsdk.CallToolRequest, input previewRuleInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()

	// An empty conditions object is allowed (match-all) — service layer accepts
	// a zero-value Condition as "match every transaction".
	conditions, err := parseConditions(input.Conditions)
	if err != nil {
		return errorResult(err), nil, nil
	}

	result, err := s.svc.PreviewRule(ctx, conditions, input.SampleSize)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result)
}

func (s *MCPServer) handleBatchCreateRules(ctx context.Context, _ *mcpsdk.CallToolRequest, input batchCreateRulesInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if len(input.Rules) == 0 {
		return errorResult(fmt.Errorf("rules array is required and must not be empty")), nil, nil
	}
	if len(input.Rules) > 100 {
		return errorResult(fmt.Errorf("maximum 100 rules per batch")), nil, nil
	}

	actor := service.ActorFromContext(ctx)
	var created []service.TransactionRuleResponse
	var errors []map[string]string

	for i, r := range input.Rules {
		if r.Name == "" || (len(r.Actions) == 0 && r.CategorySlug == "") {
			errors = append(errors, map[string]string{
				"index": strconv.Itoa(i),
				"error": "name and either actions or category_slug are required",
			})
			continue
		}

		// Empty conditions == match-all.
		conditions, err := parseConditions(r.Conditions)
		if err != nil {
			errors = append(errors, map[string]string{
				"index": strconv.Itoa(i),
				"name":  r.Name,
				"error": err.Error(),
			})
			continue
		}

		rule, err := s.svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
			Name:         r.Name,
			Conditions:   conditions,
			Actions:      convertMCPActions(r.Actions),
			CategorySlug: r.CategorySlug,
			Trigger:      r.Trigger,
			Priority:     r.Priority,
			Stage:        r.Stage,
			ExpiresIn:    r.ExpiresIn,
			Actor:        actor,
		})
		if err != nil {
			errors = append(errors, map[string]string{
				"index": strconv.Itoa(i),
				"name":  r.Name,
				"error": err.Error(),
			})
			continue
		}
		created = append(created, *rule)
	}

	return jsonResult(map[string]any{
		"created": len(created),
		"failed":  len(errors),
		"rules":   created,
		"errors":  errors,
	})
}

// --- Agent Reports ---

type submitReportInput struct {
	WriteSessionContext
	Title    string   `json:"title" jsonschema:"required,A concise 1-2 sentence summary that reads like a notification or message. This is the primary thing the family sees on their dashboard — make it informative and self-contained. Good: 'Reviewed 47 transactions this week — 3 recategorized and no suspicious activity found.' Bad: 'Weekly Review Complete' (too vague to be useful without opening the full report)."`
	Body     string   `json:"body" jsonschema:"required,Detailed breakdown in markdown format with supporting data. This is shown when the user expands the report for more detail. Use headers and bullet points and transaction links: [Transaction Name](/transactions/TRANSACTION_ID)."`
	Priority string   `json:"priority" jsonschema:"Severity level. Valid values: info (default — routine updates and summaries), warning (needs attention soon), critical (urgent action required)"`
	Tags     []string `json:"tags" jsonschema:"Short labels for categorizing reports (e.g. 'weekly-review' or 'anomaly'). Max 10 tags."`
	Author   string   `json:"author" jsonschema:"Custom author name to sign this report with. Overrides the API key name for display (e.g. 'Review Agent' or 'Budget Monitor')."`
}

func (s *MCPServer) handleSubmitReport(reqCtx context.Context, _ *mcpsdk.CallToolRequest, input submitReportInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()

	if err := s.checkWritePermission(reqCtx); err != nil {
		return errorResult(err), nil, nil
	}

	actor := service.ActorFromContext(reqCtx)
	report, err := s.svc.CreateAgentReport(ctx, input.Title, input.Body, actor, input.Priority, input.Tags, input.Author, input.SessionID)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(report)
}

// --- Helpers ---

// parseConditions converts a map[string]any (from MCP input) to a service.Condition.
// convertMCPActions converts MCP action maps to service RuleAction slice.
//
// Accepts the typed shape {type, category_slug|tag_slug|content}. Legacy
// shape {field:"category", value:"<slug>"} is auto-translated to
// {type:"set_category", category_slug:"<slug>"} for back-compat.
func convertMCPActions(actions []map[string]string) []service.RuleAction {
	if len(actions) == 0 {
		return nil
	}
	result := make([]service.RuleAction, len(actions))
	for i, a := range actions {
		act := service.RuleAction{
			Type:         a["type"],
			CategorySlug: a["category_slug"],
			TagSlug:      a["tag_slug"],
			Content:      a["content"],
		}
		// Legacy shape: {"field":"category","value":"<slug>"}.
		if act.Type == "" {
			if field, ok := a["field"]; ok && field == "category" {
				act.Type = "set_category"
				act.CategorySlug = a["value"]
			}
		}
		result[i] = act
	}
	return result
}

func parseConditions(m map[string]any) (service.Condition, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return service.Condition{}, fmt.Errorf("invalid conditions: %w", err)
	}
	var c service.Condition
	if err := json.Unmarshal(data, &c); err != nil {
		return service.Condition{}, fmt.Errorf("invalid conditions: %w", err)
	}
	return c, nil
}

func jsonResult(v any) (*mcpsdk.CallToolResult, any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return errorResult(fmt.Errorf("marshal result: %w", err)), nil, nil
	}

	// Compact IDs for MCP responses: replace "id" with short_id value, drop "short_id" field.
	// This reduces token usage by ~75% per ID without changing the schema agents see.
	// Operates directly on JSON bytes to avoid unmarshal→remarshal overhead.
	data = compactIDsBytes(data)

	// StructuredContent is the post-2025-06-18 spec slot for typed tool
	// output. We populate it alongside the TextContent block so:
	//   - clients on the new spec get the structured payload they can
	//     validate against an OutputSchema (added incrementally on per-tool
	//     defs).
	//   - clients still on the older spec keep getting the same TextContent
	//     bytes they always have — backwards-compatible.
	// Stash the *compacted* bytes as a json.RawMessage rather than
	// unmarshalling into any. Round-tripping through map[string]any sorts
	// keys alphabetically on the remarshal, which would silently drift the
	// structured view from the text view; RawMessage is encoding/json's
	// designated escape hatch — Marshal emits the bytes verbatim.

	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: string(data)},
		},
		StructuredContent: json.RawMessage(data),
	}, nil, nil
}

// compactIDs recursively walks a JSON structure and compacts each object's own
// id/short_id pair in place. When a "short_id" key with a non-null string
// value sits next to a sibling "id" field, the "id" value is replaced with
// the short_id value and the "short_id" key is removed.
//
// FK fields (e.g. account_id, transaction_id, rule_id) carry their referenced
// row's short_id directly — that resolution happens at the SQL layer in the
// service queries (JOINs that select the FK target's short_id). No
// "*_short_id" sibling appears in production responses, and FK keys are
// emitted as-is here.
func compactIDs(v any) {
	switch val := v.(type) {
	case map[string]any:
		compactIDPairs(val)
		for _, child := range val {
			compactIDs(child)
		}
	case []any:
		for _, item := range val {
			compactIDs(item)
		}
	}
}

// compactIDPairs applies id/short_id compaction to a single object map.
func compactIDPairs(m map[string]any) {
	for key, val := range m {
		prefix, ok := shortIDPrefix(key)
		if !ok {
			continue
		}
		shortVal, isString := val.(string)
		if !isString || shortVal == "" {
			// Non-string or null short_id: drop the sibling short_id key but
			// leave the id value intact (compaction is ambiguous without a
			// concrete short value).
			delete(m, key)
			continue
		}
		idKey := "id"
		if prefix != "" {
			idKey = prefix + "_id"
		}
		if _, hasID := m[idKey]; hasID {
			m[idKey] = shortVal
		}
		delete(m, key)
	}
}

// shortIDPrefix reports whether key names the row's own short-id sibling.
// Only the bare "short_id" key is recognized — FK fields are pre-resolved to
// short_ids at the SQL layer and are emitted as-is by compaction.
func shortIDPrefix(key string) (string, bool) {
	if key == "short_id" {
		return "", true
	}
	return "", false
}

// compactIDsBytes performs id/short_id compaction directly on JSON bytes,
// avoiding the unmarshal→walk→remarshal cycle. It scans the byte stream and
// collapses each object's own id/short_id pair:
//
//   {"id":"<uuid>","short_id":"<short>"} → {"id":"<short>"}
//
// Only the bare "short_id" key triggers the rewrite; the "short_id" key is
// then dropped. If the sibling "id" field is missing or "short_id" is null,
// the id value is left untouched.
//
// FK fields (account_id, transaction_id, rule_id, …) already carry their
// referenced row's short_id — resolved at the SQL layer in the service
// queries — and are emitted unchanged.
func compactIDsBytes(data []byte) []byte {
	// Quick check: if no short_id key exists anywhere, return as-is. Matches
	// both "short_id" and any "*_short_id" suffix.
	if !bytes.Contains(data, []byte(`short_id"`)) {
		return data
	}

	out := make([]byte, 0, len(data))
	out = compactIDsScan(data, out)
	return out
}

// compactIDsScan scans JSON bytes for objects with "id"+"short_id" pairs and
// performs the compaction. The input must be valid JSON (from json.Marshal).
// Returns the appended output.
func compactIDsScan(data []byte, out []byte) []byte {
	i := 0
	n := len(data)

	for i < n {
		b := data[i]
		if b == '{' {
			i, out = compactIDsScanObject(data, i, out)
		} else if b == '[' {
			i, out = compactIDsScanArray(data, i, out)
		} else {
			// Copy byte as-is (outside objects/arrays — top-level scalars, whitespace).
			out = append(out, b)
			i++
		}
	}
	return out
}

// compactIDsScanObject processes a JSON object starting at data[pos] (the '{')
// and compacts any id/short_id pairs within it (including FK pairs like
// account_id/account_short_id). Operates in two phases: collect each entry's
// key and value range, then emit — replacing id values with their sibling
// short-id value when present, and dropping *_short_id keys.
func compactIDsScanObject(data []byte, pos int, out []byte) (int, []byte) {
	pos++ // skip '{'

	type objEntry struct {
		keyStart, keyEnd int // raw quoted key bytes in data
		key              string
		valStart, valEnd int
	}

	var entries []objEntry
	// Small stack allocation for common case (≤8 fields typical; most responses fit within 32).
	var entriesBuf [32]objEntry
	entries = entriesBuf[:0]

	for pos < len(data) && data[pos] != '}' {
		if data[pos] == ',' {
			pos++
		}
		keyStart := pos
		key, keyEnd := scanJSONString(data, pos)
		pos = keyEnd
		if pos < len(data) && data[pos] == ':' {
			pos++
		}
		valStart := pos
		valEnd := skipJSONValue(data, pos)
		pos = valEnd
		entries = append(entries, objEntry{keyStart, keyEnd, key, valStart, valEnd})
	}
	if pos < len(data) {
		pos++ // skip '}'
	}

	// Build prefix → entry index for *_short_id keys (only non-null string values
	// are eligible; null/non-string short_ids can't replace an id value).
	var shortByPrefix map[string]int
	for i := range entries {
		e := &entries[i]
		prefix, ok := shortIDPrefix(e.key)
		if !ok {
			continue
		}
		// Skip if value is null — we still drop the short_id key below, but
		// don't compact the id.
		val := data[e.valStart:e.valEnd]
		if len(val) == 0 || val[0] != '"' {
			continue
		}
		if shortByPrefix == nil {
			shortByPrefix = make(map[string]int, 4)
		}
		shortByPrefix[prefix] = i
	}

	// Emit entries, skipping short_id keys and swapping id values when paired.
	out = append(out, '{')
	first := true
	for i := range entries {
		e := &entries[i]
		if _, isShort := shortIDPrefix(e.key); isShort {
			continue // drop short_id/*_short_id keys
		}

		vs, ve := e.valStart, e.valEnd
		if shortByPrefix != nil {
			if prefix, isIDKey := idKeyPrefix(e.key); isIDKey {
				if idx, ok := shortByPrefix[prefix]; ok {
					vs, ve = entries[idx].valStart, entries[idx].valEnd
				}
			}
		}

		if !first {
			out = append(out, ',')
		}
		first = false
		out = append(out, data[e.keyStart:e.keyEnd]...)
		out = append(out, ':')

		// If the value source is the original id position (not swapped), scan
		// for nested compaction. When swapped, the short-id value is a plain
		// string so we can copy verbatim.
		if vs == e.valStart && ve == e.valEnd && ve > vs {
			switch data[vs] {
			case '{':
				_, out = compactIDsScanObject(data, vs, out)
				continue
			case '[':
				_, out = compactIDsScanArray(data, vs, out)
				continue
			}
		}
		out = append(out, data[vs:ve]...)
	}
	out = append(out, '}')
	return pos, out
}

// idKeyPrefix reports whether key names the row's own id sibling. Only the
// bare "id" key is recognized — FK *_id fields carry their referenced row's
// short_id directly (resolved at the SQL layer) and are emitted unchanged.
func idKeyPrefix(key string) (string, bool) {
	if key == "id" {
		return "", true
	}
	return "", false
}

// compactIDsScanArray processes a JSON array starting at data[pos] (the '[').
func compactIDsScanArray(data []byte, pos int, out []byte) (int, []byte) {
	out = append(out, '[')
	pos++ // skip '['
	first := true

	for pos < len(data) && data[pos] != ']' {
		if data[pos] == ',' {
			pos++
		}
		if !first {
			out = append(out, ',')
		}
		first = false
		pos, out = copyJSONValue(data, pos, out)
	}

	if pos < len(data) {
		pos++ // skip ']'
	}
	out = append(out, ']')
	return pos, out
}

// copyJSONValue copies a single JSON value from data[pos] to out, recursively
// processing nested objects for compaction. Returns the new position and output.
func copyJSONValue(data []byte, pos int, out []byte) (int, []byte) {
	if pos >= len(data) {
		return pos, out
	}
	switch data[pos] {
	case '{':
		return compactIDsScanObject(data, pos, out)
	case '[':
		return compactIDsScanArray(data, pos, out)
	case '"':
		end := skipJSONString(data, pos)
		out = append(out, data[pos:end]...)
		return end, out
	default:
		// Number, bool, null — scan to next delimiter.
		end := pos
		for end < len(data) && data[end] != ',' && data[end] != '}' && data[end] != ']' {
			end++
		}
		out = append(out, data[pos:end]...)
		return end, out
	}
}

// skipJSONValue skips over a complete JSON value at data[pos] and returns
// the position after the value. Does not produce output.
func skipJSONValue(data []byte, pos int) int {
	if pos >= len(data) {
		return pos
	}
	switch data[pos] {
	case '{':
		return skipJSONObject(data, pos)
	case '[':
		return skipJSONArray(data, pos)
	case '"':
		return skipJSONString(data, pos)
	default:
		end := pos
		for end < len(data) && data[end] != ',' && data[end] != '}' && data[end] != ']' {
			end++
		}
		return end
	}
}

// skipJSONObject skips a JSON object at data[pos].
func skipJSONObject(data []byte, pos int) int {
	pos++ // skip '{'
	for pos < len(data) && data[pos] != '}' {
		if data[pos] == ',' {
			pos++
		}
		pos = skipJSONString(data, pos) // key
		if pos < len(data) && data[pos] == ':' {
			pos++
		}
		pos = skipJSONValue(data, pos) // value
	}
	if pos < len(data) {
		pos++ // skip '}'
	}
	return pos
}

// skipJSONArray skips a JSON array at data[pos].
func skipJSONArray(data []byte, pos int) int {
	pos++ // skip '['
	for pos < len(data) && data[pos] != ']' {
		if data[pos] == ',' {
			pos++
		}
		pos = skipJSONValue(data, pos)
	}
	if pos < len(data) {
		pos++ // skip ']'
	}
	return pos
}

// skipJSONString skips a JSON string at data[pos] (including quotes).
func skipJSONString(data []byte, pos int) int {
	pos++ // skip opening '"'
	for pos < len(data) {
		if data[pos] == '\\' {
			pos += 2 // skip escape sequence
			continue
		}
		if data[pos] == '"' {
			return pos + 1
		}
		pos++
	}
	return pos
}

// scanJSONString reads a JSON string at data[pos], returning the unquoted value
// and the position after the closing quote. Uses direct byte comparison for
// the common case (no escape sequences) to avoid json.Unmarshal allocation.
func scanJSONString(data []byte, pos int) (string, int) {
	end := skipJSONString(data, pos)
	raw := data[pos+1 : end-1] // strip quotes
	// Fast path: no backslash means no escape sequences — direct conversion.
	if bytes.IndexByte(raw, '\\') < 0 {
		return string(raw), end
	}
	// Slow path: contains escape sequences, use json.Unmarshal.
	var s string
	if err := json.Unmarshal(data[pos:end], &s); err != nil {
		return string(raw), end
	}
	return s, end
}


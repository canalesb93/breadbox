package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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

type listAccountsInput struct {
	ReadSessionContext
	UserID string `json:"user_id,omitempty" jsonschema:"Filter accounts by user ID"`
}

type queryTransactionsInput struct {
	ReadSessionContext
	StartDate     string   `json:"start_date,omitempty" jsonschema:"Start date (YYYY-MM-DD) inclusive"`
	EndDate       string   `json:"end_date,omitempty" jsonschema:"End date (YYYY-MM-DD) exclusive"`
	AccountID     string   `json:"account_id,omitempty" jsonschema:"Filter by account ID"`
	UserID        string   `json:"user_id,omitempty" jsonschema:"Filter by user ID"`
	CategorySlug  string   `json:"category_slug,omitempty" jsonschema:"Filter by category slug (parent slug includes all children). Use list_categories to find slugs."`
	MinAmount     *float64 `json:"min_amount,omitempty" jsonschema:"Minimum amount (positive=debit, negative=credit)"`
	MaxAmount     *float64 `json:"max_amount,omitempty" jsonschema:"Maximum amount (positive=debit, negative=credit)"`
	Pending       *bool    `json:"pending,omitempty" jsonschema:"Filter by pending status"`
	Search        string   `json:"search,omitempty" jsonschema:"Search transaction name or merchant. Comma-separated values are ORed (e.g. starbucks,amazon matches either)."`
	SearchMode    string   `json:"search_mode,omitempty" jsonschema:"How to match the search term: contains (default, substring match), words (all words must match, good for multi-word queries), fuzzy (typo-tolerant via trigram similarity)"`
	ExcludeSearch string   `json:"exclude_search,omitempty" jsonschema:"Exclude transactions whose name or merchant matches this text. Comma-separated values are ORed. Use to filter out known merchants."`
	Tags          []string `json:"tags,omitempty" jsonschema:"Filter to transactions that have EVERY tag slug in this list (AND semantics). Use list_tags to see available tags."`
	AnyTag        []string `json:"any_tag,omitempty" jsonschema:"Filter to transactions that have AT LEAST ONE tag slug in this list (OR semantics)."`
	Limit         int      `json:"limit,omitempty" jsonschema:"Max results (default 50, max 500)"`
	Cursor        string   `json:"cursor,omitempty" jsonschema:"Pagination cursor from previous result"`
	SortBy        string   `json:"sort_by,omitempty" jsonschema:"Sort: date (default), amount, name"`
	SortOrder     string   `json:"sort_order,omitempty" jsonschema:"Sort direction: desc (default) or asc"`
	Fields        string   `json:"fields,omitempty" jsonschema:"Comma-separated list of fields to include in response. Aliases: minimal (name,amount,date), core (id,date,amount,name,iso_currency_code), category (category,category_primary_raw,category_detailed_raw), timestamps (created_at,updated_at,datetime,authorized_datetime). Default: all fields. id is always included."`
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

type triggerSyncInput struct {
	WriteSessionContext
	ConnectionID string `json:"connection_id,omitempty" jsonschema:"Sync a specific connection by ID. If omitted syncs all connections."`
}

type categorizeTransactionInput struct {
	WriteSessionContext
	TransactionID string `json:"transaction_id" jsonschema:"The transaction ID to categorize"`
	CategoryID    string `json:"category_id,omitempty" jsonschema:"Category ID to assign. Provide either category_id or category_slug (not both)."`
	CategorySlug  string `json:"category_slug,omitempty" jsonschema:"Category slug to assign (e.g. food_and_drink_groceries). Alternative to category_id — the slug is resolved to an ID automatically."`
}

type resetTransactionCategoryInput struct {
	WriteSessionContext
	TransactionID string `json:"transaction_id" jsonschema:"The transaction ID to reset"`
}

type addTransactionCommentInput struct {
	WriteSessionContext
	TransactionID string `json:"transaction_id" jsonschema:"required,UUID of the transaction to comment on"`
	Content       string `json:"content" jsonschema:"required,Free-standing comment about the transaction (markdown supported, max 10000 chars). For rationale tied to a tag change or category set, pass it via update_transactions — either as the inline 'comment' field or the 'note' on a tags_to_add/tags_to_remove entry — so the audit trail stays as a single linked annotation."`
}

type listTransactionCommentsInput struct {
	ReadSessionContext
	TransactionID string `json:"transaction_id" jsonschema:"required,UUID of the transaction"`
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

type merchantSummaryInput struct {
	ReadSessionContext
	StartDate     string   `json:"start_date,omitempty" jsonschema:"Start date (YYYY-MM-DD) inclusive. Defaults to 90 days ago."`
	EndDate       string   `json:"end_date,omitempty" jsonschema:"End date (YYYY-MM-DD) exclusive. Defaults to today."`
	AccountID     string   `json:"account_id,omitempty" jsonschema:"Filter by account ID"`
	UserID        string   `json:"user_id,omitempty" jsonschema:"Filter by user ID (family member)"`
	CategorySlug  string   `json:"category_slug,omitempty" jsonschema:"Filter by category slug"`
	MinAmount     *float64 `json:"min_amount,omitempty" jsonschema:"Minimum transaction amount"`
	MaxAmount     *float64 `json:"max_amount,omitempty" jsonschema:"Maximum transaction amount"`
	Search        string   `json:"search,omitempty" jsonschema:"Search merchant/transaction names. Comma-separated values are ORed."`
	SearchMode    string   `json:"search_mode,omitempty" jsonschema:"Search mode: contains (default), words, fuzzy"`
	ExcludeSearch string   `json:"exclude_search,omitempty" jsonschema:"Exclude merchants matching this text. Comma-separated values are ORed."`
	MinCount      int      `json:"min_count,omitempty" jsonschema:"Minimum transaction count to include a merchant (default 1). Set to 2+ to find recurring charges."`
	SpendingOnly  *bool    `json:"spending_only,omitempty" jsonschema:"Only include spending (positive amounts). Default false."`
}

type listCategoriesInput struct {
	ReadSessionContext
}

type listUsersInput struct {
	ReadSessionContext
}

type getSyncStatusInput struct {
	ReadSessionContext
}

type exportCategoriesInput struct {
	ReadSessionContext
}

type importCategoriesInput struct {
	WriteSessionContext
	Content string `json:"content" jsonschema:"required,TSV content with category definitions. Columns: slug, display_name, parent_slug, icon, color, sort_order, hidden, merge_into. The merge_into column is optional — set to a target slug to merge the source category into the target (transactions reassigned then source deleted)."`
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

func (s *MCPServer) handleListAccounts(_ context.Context, _ *mcpsdk.CallToolRequest, input listAccountsInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	var userID *string
	if input.UserID != "" {
		userID = &input.UserID
	}

	accounts, err := s.svc.ListAccounts(ctx, userID)
	if err != nil {
		return errorResult(err), nil, nil
	}

	return jsonResult(accounts)
}

func (s *MCPServer) handleQueryTransactions(_ context.Context, _ *mcpsdk.CallToolRequest, input queryTransactionsInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	params := service.TransactionListParams{
		Cursor: input.Cursor,
		Limit:  input.Limit,
	}

	if input.StartDate != "" {
		t, err := time.Parse("2006-01-02", input.StartDate)
		if err != nil {
			return errorResult(fmt.Errorf("invalid start_date: %w", err)), nil, nil
		}
		params.StartDate = &t
	}
	if input.EndDate != "" {
		t, err := time.Parse("2006-01-02", input.EndDate)
		if err != nil {
			return errorResult(fmt.Errorf("invalid end_date: %w", err)), nil, nil
		}
		params.EndDate = &t
	}
	if input.AccountID != "" {
		params.AccountID = &input.AccountID
	}
	if input.UserID != "" {
		params.UserID = &input.UserID
	}
	if input.CategorySlug != "" {
		params.CategorySlug = &input.CategorySlug
	}
	if input.MinAmount != nil {
		params.MinAmount = input.MinAmount
	}
	if input.MaxAmount != nil {
		params.MaxAmount = input.MaxAmount
	}
	if input.Pending != nil {
		params.Pending = input.Pending
	}
	if input.Search != "" {
		params.Search = &input.Search
	}
	if input.SearchMode != "" {
		if !service.ValidateSearchMode(input.SearchMode) {
			return errorResult(fmt.Errorf("invalid search_mode: %s. Must be one of: contains, words, fuzzy", input.SearchMode)), nil, nil
		}
		params.SearchMode = &input.SearchMode
	}
	if input.ExcludeSearch != "" {
		params.ExcludeSearch = &input.ExcludeSearch
	}
	if len(input.Tags) > 0 {
		params.Tags = input.Tags
	}
	if len(input.AnyTag) > 0 {
		params.AnyTag = input.AnyTag
	}
	if input.SortBy != "" {
		params.SortBy = &input.SortBy
	}
	if input.SortOrder != "" {
		params.SortOrder = &input.SortOrder
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
	params := service.TransactionCountParams{}

	if input.StartDate != "" {
		t, err := time.Parse("2006-01-02", input.StartDate)
		if err != nil {
			return errorResult(fmt.Errorf("invalid start_date: %w", err)), nil, nil
		}
		params.StartDate = &t
	}
	if input.EndDate != "" {
		t, err := time.Parse("2006-01-02", input.EndDate)
		if err != nil {
			return errorResult(fmt.Errorf("invalid end_date: %w", err)), nil, nil
		}
		params.EndDate = &t
	}
	if input.AccountID != "" {
		params.AccountID = &input.AccountID
	}
	if input.UserID != "" {
		params.UserID = &input.UserID
	}
	if input.CategorySlug != "" {
		params.CategorySlug = &input.CategorySlug
	}
	if input.MinAmount != nil {
		params.MinAmount = input.MinAmount
	}
	if input.MaxAmount != nil {
		params.MaxAmount = input.MaxAmount
	}
	if input.Pending != nil {
		params.Pending = input.Pending
	}
	if input.Search != "" {
		params.Search = &input.Search
	}
	if input.SearchMode != "" {
		if !service.ValidateSearchMode(input.SearchMode) {
			return errorResult(fmt.Errorf("invalid search_mode: %s. Must be one of: contains, words, fuzzy", input.SearchMode)), nil, nil
		}
		params.SearchMode = &input.SearchMode
	}
	if input.ExcludeSearch != "" {
		params.ExcludeSearch = &input.ExcludeSearch
	}
	if len(input.Tags) > 0 {
		params.Tags = input.Tags
	}
	if len(input.AnyTag) > 0 {
		params.AnyTag = input.AnyTag
	}

	count, err := s.svc.CountTransactionsFiltered(ctx, params)
	if err != nil {
		return errorResult(err), nil, nil
	}

	return jsonResult(map[string]int64{"count": count})
}

func (s *MCPServer) handleListCategories(_ context.Context, _ *mcpsdk.CallToolRequest, _ listCategoriesInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	categories, err := s.svc.ListCategoryTree(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(categories)
}

func (s *MCPServer) handleListUsers(_ context.Context, _ *mcpsdk.CallToolRequest, _ listUsersInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	users, err := s.svc.ListUsers(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	return jsonResult(users)
}

func (s *MCPServer) handleGetSyncStatus(_ context.Context, _ *mcpsdk.CallToolRequest, _ getSyncStatusInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	connections, err := s.svc.ListConnections(ctx, nil)
	if err != nil {
		return errorResult(err), nil, nil
	}

	return jsonResult(connections)
}

func (s *MCPServer) handleTriggerSync(ctx context.Context, _ *mcpsdk.CallToolRequest, input triggerSyncInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	ctx = context.Background()
	var connectionID *string
	if input.ConnectionID != "" {
		connectionID = &input.ConnectionID
	}

	if err := s.svc.TriggerSync(ctx, connectionID); err != nil {
		return errorResult(err), nil, nil
	}

	msg := "Sync triggered for all connections."
	if connectionID != nil {
		msg = fmt.Sprintf("Sync triggered for connection %s.", *connectionID)
	}

	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: msg},
		},
	}, nil, nil
}

func (s *MCPServer) handleCategorizeTransaction(ctx context.Context, _ *mcpsdk.CallToolRequest, input categorizeTransactionInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	ctx = context.Background()
	if input.TransactionID == "" {
		return errorResult(fmt.Errorf("transaction_id is required")), nil, nil
	}
	if input.CategoryID == "" && input.CategorySlug == "" {
		return errorResult(fmt.Errorf("either category_id or category_slug is required")), nil, nil
	}

	// Resolve category_slug to category_id if provided. category_id takes precedence.
	categoryID := input.CategoryID
	if categoryID == "" && input.CategorySlug != "" {
		cat, err := s.svc.GetCategoryBySlug(ctx, input.CategorySlug)
		if err != nil {
			return errorResult(fmt.Errorf("invalid category_slug %q: %w", input.CategorySlug, err)), nil, nil
		}
		categoryID = cat.ID
	}

	if err := s.svc.SetTransactionCategory(ctx, input.TransactionID, categoryID); err != nil {
		return errorResult(err), nil, nil
	}
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: fmt.Sprintf("Transaction %s categorized successfully.", input.TransactionID)},
		},
	}, nil, nil
}

func (s *MCPServer) handleResetTransactionCategory(ctx context.Context, _ *mcpsdk.CallToolRequest, input resetTransactionCategoryInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	ctx = context.Background()
	if input.TransactionID == "" {
		return errorResult(fmt.Errorf("transaction_id is required")), nil, nil
	}
	if err := s.svc.ResetTransactionCategory(ctx, input.TransactionID); err != nil {
		return errorResult(err), nil, nil
	}
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: fmt.Sprintf("Transaction %s category reset to automatic.", input.TransactionID)},
		},
	}, nil, nil
}

func (s *MCPServer) handleAddTransactionComment(ctx context.Context, _ *mcpsdk.CallToolRequest, input addTransactionCommentInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.TransactionID == "" || input.Content == "" {
		return errorResult(fmt.Errorf("transaction_id and content are required")), nil, nil
	}
	actor := service.ActorFromContext(ctx)
	comment, err := s.svc.CreateComment(ctx, service.CreateCommentParams{
		TransactionID: input.TransactionID,
		Content:       input.Content,
		Actor:         actor,
	})
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(comment)
}

func (s *MCPServer) handleListTransactionComments(ctx context.Context, _ *mcpsdk.CallToolRequest, input listTransactionCommentsInput) (*mcpsdk.CallToolResult, any, error) {
	if input.TransactionID == "" {
		return errorResult(fmt.Errorf("transaction_id is required")), nil, nil
	}
	comments, err := s.svc.ListComments(ctx, input.TransactionID)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(comments)
}

func (s *MCPServer) handleTransactionSummary(_ context.Context, _ *mcpsdk.CallToolRequest, input transactionSummaryInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()

	params := service.TransactionSummaryParams{
		GroupBy: input.GroupBy,
	}

	if input.StartDate != "" {
		t, err := time.Parse("2006-01-02", input.StartDate)
		if err != nil {
			return errorResult(fmt.Errorf("invalid start_date: %w", err)), nil, nil
		}
		params.StartDate = &t
	}
	if input.EndDate != "" {
		t, err := time.Parse("2006-01-02", input.EndDate)
		if err != nil {
			return errorResult(fmt.Errorf("invalid end_date: %w", err)), nil, nil
		}
		params.EndDate = &t
	}
	if input.AccountID != "" {
		params.AccountID = &input.AccountID
	}
	if input.UserID != "" {
		params.UserID = &input.UserID
	}
	if input.Category != "" {
		params.Category = &input.Category
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

func (s *MCPServer) handleMerchantSummary(_ context.Context, _ *mcpsdk.CallToolRequest, input merchantSummaryInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()

	params := service.MerchantSummaryParams{
		MinCount: input.MinCount,
	}

	if input.StartDate != "" {
		t, err := time.Parse("2006-01-02", input.StartDate)
		if err != nil {
			return errorResult(fmt.Errorf("invalid start_date: %w", err)), nil, nil
		}
		params.StartDate = &t
	}
	if input.EndDate != "" {
		t, err := time.Parse("2006-01-02", input.EndDate)
		if err != nil {
			return errorResult(fmt.Errorf("invalid end_date: %w", err)), nil, nil
		}
		params.EndDate = &t
	}
	if input.AccountID != "" {
		params.AccountID = &input.AccountID
	}
	if input.UserID != "" {
		params.UserID = &input.UserID
	}
	if input.CategorySlug != "" {
		params.CategorySlug = &input.CategorySlug
	}
	if input.MinAmount != nil {
		params.MinAmount = input.MinAmount
	}
	if input.MaxAmount != nil {
		params.MaxAmount = input.MaxAmount
	}
	if input.Search != "" {
		params.Search = &input.Search
	}
	if input.SearchMode != "" {
		if !service.ValidateSearchMode(input.SearchMode) {
			return errorResult(fmt.Errorf("invalid search_mode: %s. Must be one of: contains, words, fuzzy", input.SearchMode)), nil, nil
		}
		params.SearchMode = &input.SearchMode
	}
	if input.ExcludeSearch != "" {
		params.ExcludeSearch = &input.ExcludeSearch
	}
	if input.SpendingOnly != nil && *input.SpendingOnly {
		params.SpendingOnly = true
	}

	result, err := s.svc.GetMerchantSummary(ctx, params)
	if err != nil {
		return errorResult(err), nil, nil
	}

	return jsonResult(result)
}

func (s *MCPServer) handleExportCategories(_ context.Context, _ *mcpsdk.CallToolRequest, _ exportCategoriesInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	tsv, err := s.svc.ExportCategoriesTSV(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: tsv},
		},
	}, nil, nil
}

func (s *MCPServer) handleImportCategories(ctx context.Context, _ *mcpsdk.CallToolRequest, input importCategoriesInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.Content == "" {
		return errorResult(fmt.Errorf("content is required")), nil, nil
	}
	result, err := s.svc.ImportCategoriesTSV(context.Background(), input.Content, false)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result)
}

// --- Transaction Rules ---

type createTransactionRuleInput struct {
	WriteSessionContext
	Name               string              `json:"name" jsonschema:"required,Name for this rule (human-readable description)"`
	Conditions         map[string]any      `json:"conditions,omitempty" jsonschema:"JSON condition object. Omit or pass {} to match every transaction. Simple: {\"field\":\"name\",\"op\":\"contains\",\"value\":\"uber\"}. AND/OR/NOT. Fields: name merchant_name amount category_primary category_detailed category(assigned slug) pending provider account_id account_name user_id user_name tags. Ops: eq neq contains not_contains matches(regex) gt gte lt lte in. See docs/rule-dsl.md for the complete DSL."`
	Actions            []map[string]string `json:"actions,omitempty" jsonschema:"Array of typed actions. {\"type\":\"set_category\",\"category_slug\":\"...\"}, {\"type\":\"add_tag\",\"tag_slug\":\"...\"}, {\"type\":\"remove_tag\",\"tag_slug\":\"...\"}, or {\"type\":\"add_comment\",\"content\":\"...\"}. If omitted, use category_slug instead."`
	CategorySlug       string              `json:"category_slug,omitempty" jsonschema:"Shorthand for actions: [{\"type\":\"set_category\",\"category_slug\":\"<slug>\"}]. Either actions or category_slug is required."`
	Trigger            string              `json:"trigger,omitempty" jsonschema:"When the rule fires: 'on_create' (default — new transactions), 'on_change' (only on re-sync changes), 'always' (both). 'on_update' is accepted as a legacy alias for 'on_change'."`
	Priority           int                 `json:"priority,omitempty" jsonschema:"Priority (higher wins when multiple rules set the same field). Default 10."`
	ExpiresIn          string              `json:"expires_in,omitempty" jsonschema:"Optional expiry duration: 24h, 30d, 1w. Rule auto-disables after this period."`
	ApplyRetroactively bool                `json:"apply_retroactively,omitempty" jsonschema:"If true, immediately apply this rule to all existing non-overridden transactions after creation."`
}

type listTransactionRulesInput struct {
	ReadSessionContext
	CategorySlug string `json:"category_slug,omitempty" jsonschema:"Filter by category slug"`
	Enabled      *bool  `json:"enabled,omitempty" jsonschema:"Filter by enabled status"`
	Search       string `json:"search,omitempty" jsonschema:"Search by rule name. Comma-separated values are ORed."`
	SearchMode   string `json:"search_mode,omitempty" jsonschema:"Search mode: contains (default), words, fuzzy"`
	Limit        int    `json:"limit,omitempty" jsonschema:"Max results (default 50, max 500)"`
	Cursor       string `json:"cursor,omitempty" jsonschema:"Pagination cursor from previous result"`
}

type updateTransactionRuleInput struct {
	WriteSessionContext
	ID           string               `json:"id" jsonschema:"required,UUID of the rule to update"`
	Name         *string              `json:"name,omitempty" jsonschema:"New name for the rule"`
	Conditions   map[string]any       `json:"conditions,omitempty" jsonschema:"New condition tree (same format as create). Pass {} to change to match-all."`
	Actions      *[]map[string]string `json:"actions,omitempty" jsonschema:"Replace actions array with typed actions. Each: {\"type\":\"set_category\",\"category_slug\":\"...\"}, {\"type\":\"add_tag\",\"tag_slug\":\"...\"}, {\"type\":\"remove_tag\",\"tag_slug\":\"...\"}, or {\"type\":\"add_comment\",\"content\":\"...\"}."`
	CategorySlug *string              `json:"category_slug,omitempty" jsonschema:"Shorthand: replace the set_category action. Other actions are kept."`
	Trigger      *string              `json:"trigger,omitempty" jsonschema:"New trigger: on_create, on_change, or always. 'on_update' accepted as alias for on_change."`
	Priority     *int                 `json:"priority,omitempty" jsonschema:"New priority"`
	Enabled      *bool                `json:"enabled,omitempty" jsonschema:"Enable or disable the rule"`
	ExpiresAt    *string              `json:"expires_at,omitempty" jsonschema:"New expiry timestamp (RFC3339) or empty string to clear"`
}

type deleteTransactionRuleInput struct {
	WriteSessionContext
	ID string `json:"id" jsonschema:"required,UUID of the rule to delete"`
}

type batchCreateRulesInput struct {
	WriteSessionContext
	Rules []batchRuleItem `json:"rules" jsonschema:"required,Array of rules to create"`
}

type batchRuleItem struct {
	Name         string              `json:"name" jsonschema:"required,Human-readable rule name"`
	Actions      []map[string]string `json:"actions,omitempty" jsonschema:"Actions array (typed — same format as create_transaction_rule)"`
	CategorySlug string              `json:"category_slug,omitempty" jsonschema:"Shorthand for set_category action. Either actions or category_slug required."`
	Conditions   map[string]any      `json:"conditions,omitempty" jsonschema:"Condition tree as JSON object. Omit or {} for match-all."`
	Trigger      string              `json:"trigger,omitempty" jsonschema:"on_create (default), on_change, or always. 'on_update' accepted as alias for on_change."`
	Priority     int                 `json:"priority,omitempty" jsonschema:"Priority (default 10)"`
	ExpiresIn    string              `json:"expires_in,omitempty" jsonschema:"Optional expiry duration"`
}

type applyRulesInput struct {
	WriteSessionContext
	RuleID string `json:"rule_id,omitempty" jsonschema:"Optional UUID of a specific rule to apply. If omitted, applies all active rules (first match wins by priority)."`
}

type previewRuleInput struct {
	ReadSessionContext
	Conditions map[string]any `json:"conditions" jsonschema:"required,Condition tree to evaluate against existing transactions (same format as create_transaction_rule conditions)."`
	SampleSize int            `json:"sample_size,omitempty" jsonschema:"Number of sample matching transactions to return (default 10, max 50)."`
}

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
	priority := input.Priority
	if priority == 0 {
		priority = 10
	}

	rule, err := s.svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
		Name:         input.Name,
		Conditions:   conditions,
		Actions:      convertMCPActions(input.Actions),
		CategorySlug: input.CategorySlug,
		Trigger:      input.Trigger,
		Priority:     priority,
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

func (s *MCPServer) handleListTransactionRules(_ context.Context, _ *mcpsdk.CallToolRequest, input listTransactionRulesInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()

	params := service.TransactionRuleListParams{
		Limit:  input.Limit,
		Cursor: input.Cursor,
	}
	if input.CategorySlug != "" {
		params.CategorySlug = &input.CategorySlug
	}
	if input.Enabled != nil {
		params.Enabled = input.Enabled
	}
	if input.Search != "" {
		params.Search = &input.Search
	}
	if input.SearchMode != "" {
		if !service.ValidateSearchMode(input.SearchMode) {
			return errorResult(fmt.Errorf("invalid search_mode: %s. Must be one of: contains, words, fuzzy", input.SearchMode)), nil, nil
		}
		params.SearchMode = &input.SearchMode
	}

	result, err := s.svc.ListTransactionRules(ctx, params)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result)
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
				"index": fmt.Sprintf("%d", i),
				"error": "name and either actions or category_slug are required",
			})
			continue
		}

		// Empty conditions == match-all.
		conditions, err := parseConditions(r.Conditions)
		if err != nil {
			errors = append(errors, map[string]string{
				"index": fmt.Sprintf("%d", i),
				"name":  r.Name,
				"error": err.Error(),
			})
			continue
		}

		priority := r.Priority
		if priority == 0 {
			priority = 10
		}

		rule, err := s.svc.CreateTransactionRule(ctx, service.CreateTransactionRuleParams{
			Name:         r.Name,
			Conditions:   conditions,
			Actions:      convertMCPActions(r.Actions),
			CategorySlug: r.CategorySlug,
			Trigger:      r.Trigger,
			Priority:     priority,
			ExpiresIn:    r.ExpiresIn,
			Actor:        actor,
		})
		if err != nil {
			errors = append(errors, map[string]string{
				"index": fmt.Sprintf("%d", i),
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

// --- Batch categorize / Bulk recategorize ---

type batchCategorizeInput struct {
	WriteSessionContext
	Items []batchCategorizeItemInput `json:"items" jsonschema:"required,Array of transaction/category pairs (max 500)"`
}

type batchCategorizeItemInput struct {
	TransactionID string `json:"transaction_id" jsonschema:"required,UUID of the transaction"`
	CategorySlug  string `json:"category_slug" jsonschema:"required,Category slug to assign (e.g. food_and_drink_restaurant). Use list_categories to find slugs."`
}

type bulkRecategorizeInput struct {
	WriteSessionContext
	FromCategory       string   `json:"from_category,omitempty" jsonschema:"Source category slug — only transactions currently in this category are matched. Optional if other filters are provided."`
	ToCategory         string   `json:"to_category,omitempty" jsonschema:"Destination category slug — matching transactions are moved here. Required (or provide target_category_slug)."`
	StartDate          string   `json:"start_date,omitempty" jsonschema:"Start date (YYYY-MM-DD) inclusive"`
	EndDate            string   `json:"end_date,omitempty" jsonschema:"End date (YYYY-MM-DD) exclusive"`
	AccountID          string   `json:"account_id,omitempty" jsonschema:"Filter by account ID"`
	UserID             string   `json:"user_id,omitempty" jsonschema:"Filter by user ID (family member)"`
	MinAmount          *float64 `json:"min_amount,omitempty" jsonschema:"Minimum amount (positive=debit, negative=credit)"`
	MaxAmount          *float64 `json:"max_amount,omitempty" jsonschema:"Maximum amount (positive=debit, negative=credit)"`
	Pending            *bool    `json:"pending,omitempty" jsonschema:"Filter by pending status"`
	Search             string   `json:"search,omitempty" jsonschema:"Search transaction name or merchant"`
	NameContains       string   `json:"name_contains,omitempty" jsonschema:"Filter transactions whose name contains this string"`
	// Deprecated fields — retained for backward compatibility with older agent sessions.
	TargetCategorySlug string `json:"target_category_slug,omitempty" jsonschema:"Deprecated: use to_category instead. Destination category slug."`
	CategorySlug       string `json:"category_slug,omitempty" jsonschema:"Deprecated: use from_category instead. Source category slug filter."`
}

func (s *MCPServer) handleBatchCategorize(ctx context.Context, _ *mcpsdk.CallToolRequest, input batchCategorizeInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if len(input.Items) == 0 {
		return errorResult(fmt.Errorf("items array is required and must not be empty")), nil, nil
	}

	items := make([]service.BatchCategorizeItem, len(input.Items))
	for i, item := range input.Items {
		items[i] = service.BatchCategorizeItem{
			TransactionID: item.TransactionID,
			CategorySlug:  item.CategorySlug,
		}
	}

	result, err := s.svc.BatchSetTransactionCategory(ctx, items)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result)
}

func (s *MCPServer) handleBulkRecategorize(ctx context.Context, _ *mcpsdk.CallToolRequest, input bulkRecategorizeInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}

	// Prefer new param names; fall back to deprecated aliases for backward compatibility.
	toCategory := input.ToCategory
	if toCategory == "" {
		toCategory = input.TargetCategorySlug
	}
	fromCategory := input.FromCategory
	if fromCategory == "" {
		fromCategory = input.CategorySlug
	}

	if toCategory == "" {
		return errorResult(fmt.Errorf("to_category is required")), nil, nil
	}

	params := service.BulkRecategorizeParams{
		TargetCategorySlug: toCategory,
	}

	if input.StartDate != "" {
		t, err := time.Parse("2006-01-02", input.StartDate)
		if err != nil {
			return errorResult(fmt.Errorf("invalid start_date: %w", err)), nil, nil
		}
		params.StartDate = &t
	}
	if input.EndDate != "" {
		t, err := time.Parse("2006-01-02", input.EndDate)
		if err != nil {
			return errorResult(fmt.Errorf("invalid end_date: %w", err)), nil, nil
		}
		params.EndDate = &t
	}
	if input.AccountID != "" {
		params.AccountID = &input.AccountID
	}
	if input.UserID != "" {
		params.UserID = &input.UserID
	}
	if fromCategory != "" {
		params.CategorySlug = &fromCategory
	}
	if input.MinAmount != nil {
		params.MinAmount = input.MinAmount
	}
	if input.MaxAmount != nil {
		params.MaxAmount = input.MaxAmount
	}
	if input.Pending != nil {
		params.Pending = input.Pending
	}
	if input.Search != "" {
		params.Search = &input.Search
	}
	if input.NameContains != "" {
		params.NameContains = &input.NameContains
	}

	result, err := s.svc.BulkRecategorizeByFilter(ctx, params)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result)
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

	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: string(data)},
		},
	}, nil, nil
}

// compactIDs recursively walks a JSON structure and compacts ID pairs in place.
// For every key ending in "_short_id" (or the bare "short_id") with a non-null
// string value, the sibling "{prefix}_id" (or bare "id") field is replaced with
// the short_id value and the "_short_id" key is removed. This covers both an
// object's own id/short_id pair and its FK references (e.g. account_id +
// account_short_id → account_id=<short>).
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

// shortIDPrefix reports whether key names a short-id sibling and returns its
// prefix. "short_id" → ("", true); "account_short_id" → ("account", true);
// anything else → ("", false).
func shortIDPrefix(key string) (string, bool) {
	if key == "short_id" {
		return "", true
	}
	if strings.HasSuffix(key, "_short_id") && len(key) > len("_short_id") {
		return key[:len(key)-len("_short_id")], true
	}
	return "", false
}

// compactIDsBytes performs id/short_id compaction directly on JSON bytes,
// avoiding the unmarshal→walk→remarshal cycle. It scans the byte stream and
// collapses short-id sibling pairs:
//
//   - own id: {"id":"<uuid>","short_id":"<short>"} → {"id":"<short>"}
//   - FK id:  {"account_id":"<uuid>","account_short_id":"<short>"} →
//             {"account_id":"<short>"}
//
// Any key ending in "_short_id" (or the bare "short_id") triggers the rewrite;
// the "_short_id" key is always dropped. If the sibling id field is missing or
// the short-id value is null, the id value is left untouched.
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

// idKeyPrefix reports whether key names an id sibling and returns its prefix.
// "id" → ("", true); "account_id" → ("account", true); else ("", false).
func idKeyPrefix(key string) (string, bool) {
	if key == "id" {
		return "", true
	}
	if strings.HasSuffix(key, "_id") && len(key) > len("_id") {
		return key[:len(key)-len("_id")], true
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


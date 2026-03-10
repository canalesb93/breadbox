package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"breadbox/internal/service"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- Input types ---

type listAccountsInput struct {
	UserID string `json:"user_id,omitempty" jsonschema:"Filter accounts by user ID"`
}

type queryTransactionsInput struct {
	StartDate    string   `json:"start_date,omitempty" jsonschema:"Start date (YYYY-MM-DD) inclusive"`
	EndDate      string   `json:"end_date,omitempty" jsonschema:"End date (YYYY-MM-DD) exclusive"`
	AccountID    string   `json:"account_id,omitempty" jsonschema:"Filter by account ID"`
	UserID       string   `json:"user_id,omitempty" jsonschema:"Filter by user ID"`
	CategorySlug string   `json:"category_slug,omitempty" jsonschema:"Filter by category slug (parent slug includes all children). Use list_categories to find slugs."`
	MinAmount    *float64 `json:"min_amount,omitempty" jsonschema:"Minimum amount (positive=debit, negative=credit)"`
	MaxAmount    *float64 `json:"max_amount,omitempty" jsonschema:"Maximum amount (positive=debit, negative=credit)"`
	Pending      *bool    `json:"pending,omitempty" jsonschema:"Filter by pending status"`
	Search       string   `json:"search,omitempty" jsonschema:"Search transaction name or merchant"`
	Limit        int      `json:"limit,omitempty" jsonschema:"Max results (default 50, max 500)"`
	Cursor       string   `json:"cursor,omitempty" jsonschema:"Pagination cursor from previous result"`
	SortBy       string   `json:"sort_by,omitempty" jsonschema:"Sort: date (default), amount, name"`
	SortOrder    string   `json:"sort_order,omitempty" jsonschema:"Sort direction: desc (default) or asc"`
	Fields       string   `json:"fields,omitempty" jsonschema:"Comma-separated list of fields to include in response. Aliases: core (id,date,amount,name,iso_currency_code), category (category,category_primary_raw,category_detailed_raw), timestamps (created_at,updated_at,datetime,authorized_datetime). Default: all fields. id is always included."`
}

type countTransactionsInput struct {
	StartDate    string   `json:"start_date,omitempty" jsonschema:"Start date (YYYY-MM-DD) inclusive"`
	EndDate      string   `json:"end_date,omitempty" jsonschema:"End date (YYYY-MM-DD) exclusive"`
	AccountID    string   `json:"account_id,omitempty" jsonschema:"Filter by account ID"`
	UserID       string   `json:"user_id,omitempty" jsonschema:"Filter by user ID"`
	CategorySlug string   `json:"category_slug,omitempty" jsonschema:"Filter by category slug"`
	MinAmount    *float64 `json:"min_amount,omitempty" jsonschema:"Minimum amount"`
	MaxAmount    *float64 `json:"max_amount,omitempty" jsonschema:"Maximum amount"`
	Pending      *bool    `json:"pending,omitempty" jsonschema:"Filter by pending status"`
	Search       string   `json:"search,omitempty" jsonschema:"Search name or merchant"`
}

type triggerSyncInput struct {
	ConnectionID string `json:"connection_id,omitempty" jsonschema:"Sync a specific connection by ID. If omitted syncs all connections."`
}

type categorizeTransactionInput struct {
	TransactionID string `json:"transaction_id" jsonschema:"The transaction ID to categorize"`
	CategoryID    string `json:"category_id" jsonschema:"The category ID to assign (use list_categories to find IDs)"`
}

type resetTransactionCategoryInput struct {
	TransactionID string `json:"transaction_id" jsonschema:"The transaction ID to reset"`
}

type addTransactionCommentInput struct {
	TransactionID string `json:"transaction_id" jsonschema:"required,UUID of the transaction to comment on"`
	Content       string `json:"content" jsonschema:"required,Comment text (markdown supported, max 10000 chars)"`
}

type listTransactionCommentsInput struct {
	TransactionID string `json:"transaction_id" jsonschema:"required,UUID of the transaction"`
}

type getTransactionHistoryInput struct {
	TransactionID string `json:"transaction_id" jsonschema:"required,UUID of the transaction"`
	Limit         int    `json:"limit,omitempty" jsonschema:"Max entries to return (default 50, max 200)"`
}

type queryAuditLogInput struct {
	EntityType string `json:"entity_type,omitempty" jsonschema:"Filter by entity type: transaction, account, connection, user"`
	ActorType  string `json:"actor_type,omitempty" jsonschema:"Filter by who made the change: user, agent, system"`
	Limit      int    `json:"limit,omitempty" jsonschema:"Max entries to return (default 50, max 200)"`
	Cursor     string `json:"cursor,omitempty" jsonschema:"Pagination cursor from previous result"`
}

type transactionSummaryInput struct {
	StartDate      string `json:"start_date,omitempty" jsonschema:"Start date (YYYY-MM-DD) inclusive. Defaults to 30 days ago."`
	EndDate        string `json:"end_date,omitempty" jsonschema:"End date (YYYY-MM-DD) exclusive. Defaults to today."`
	GroupBy        string `json:"group_by" jsonschema:"required,How to group results: category, month, week, day, or category_month"`
	AccountID      string `json:"account_id,omitempty" jsonschema:"Filter by account ID"`
	UserID         string `json:"user_id,omitempty" jsonschema:"Filter by user ID (family member)"`
	Category       string `json:"category,omitempty" jsonschema:"Filter by primary category before aggregating"`
	IncludePending *bool  `json:"include_pending,omitempty" jsonschema:"Include pending transactions (default false)"`
}

type reviewTransactionsInput struct {
	Limit        int    `json:"limit,omitempty" jsonschema:"Max items to return (default 20, max 100). Start small to avoid overwhelming context."`
	AccountID    string `json:"account_id,omitempty" jsonschema:"Filter by account UUID."`
	UserID       string `json:"user_id,omitempty" jsonschema:"Filter by user (family member) UUID."`
	CategorySlug string `json:"category_slug,omitempty" jsonschema:"Filter by suggested category slug."`
}

type submitReviewInput struct {
	Reviews []reviewDecisionInput `json:"reviews" jsonschema:"required,Array of review decisions. Submit one or more at a time."`
}

type reviewDecisionInput struct {
	ReviewID             string  `json:"review_id" jsonschema:"required,UUID of the review queue item."`
	Decision             string  `json:"decision" jsonschema:"required,One of: approve, reject, skip."`
	OverrideCategorySlug *string `json:"override_category_slug,omitempty" jsonschema:"Category slug to apply when rejecting the suggested category. Required when decision is reject."`
	Comment              *string `json:"comment,omitempty" jsonschema:"Explanation of the decision. Highly recommended for audit trail. Supports markdown."`
}

type listCategoryMappingsInput struct {
	Provider     string `json:"provider,omitempty" jsonschema:"Filter by provider: plaid, teller, or csv"`
	CategorySlug string `json:"category_slug,omitempty" jsonschema:"Filter by target category slug (e.g. food_and_drink_restaurant). Use list_categories to find valid slugs."`
}

type createCategoryMappingInput struct {
	Provider         string `json:"provider" jsonschema:"required,Provider type: plaid, teller, or csv"`
	ProviderCategory string `json:"provider_category" jsonschema:"required,Raw category string from the provider (e.g. FOOD_AND_DRINK_GROCERIES for Plaid or dining for Teller)"`
	CategorySlug     string `json:"category_slug" jsonschema:"required,Slug of the target user category (e.g. food_and_drink_restaurant). Use list_categories to find valid slugs."`
}

type updateCategoryMappingInput struct {
	ID               string `json:"id,omitempty" jsonschema:"Mapping ID to update (alternative to provider + provider_category)"`
	Provider         string `json:"provider,omitempty" jsonschema:"Provider type (required if not using id)"`
	ProviderCategory string `json:"provider_category,omitempty" jsonschema:"Raw provider category string (required if not using id)"`
	CategorySlug     string `json:"category_slug" jsonschema:"required,New target category slug"`
}

type deleteCategoryMappingInput struct {
	ID               string `json:"id,omitempty" jsonschema:"Mapping ID to delete (alternative to provider + provider_category)"`
	Provider         string `json:"provider,omitempty" jsonschema:"Provider type (required if not using id)"`
	ProviderCategory string `json:"provider_category,omitempty" jsonschema:"Raw provider category string (required if not using id)"`
}

// --- Handlers ---

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

	count, err := s.svc.CountTransactionsFiltered(ctx, params)
	if err != nil {
		return errorResult(err), nil, nil
	}

	return jsonResult(map[string]int64{"count": count})
}

func (s *MCPServer) handleListCategories(_ context.Context, _ *mcpsdk.CallToolRequest, _ any) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	categories, err := s.svc.ListCategoryTree(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(categories)
}

func (s *MCPServer) handleListUsers(_ context.Context, _ *mcpsdk.CallToolRequest, _ any) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	users, err := s.svc.ListUsers(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}

	return jsonResult(users)
}

func (s *MCPServer) handleGetSyncStatus(_ context.Context, _ *mcpsdk.CallToolRequest, _ any) (*mcpsdk.CallToolResult, any, error) {
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
	if input.TransactionID == "" || input.CategoryID == "" {
		return errorResult(fmt.Errorf("transaction_id and category_id are required")), nil, nil
	}
	if err := s.svc.SetTransactionCategory(ctx, input.TransactionID, input.CategoryID); err != nil {
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

func (s *MCPServer) handleListUnmappedCategories(_ context.Context, _ *mcpsdk.CallToolRequest, _ any) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	unmapped, err := s.svc.ListUnmappedCategories(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(unmapped)
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

func (s *MCPServer) handleGetTransactionHistory(ctx context.Context, _ *mcpsdk.CallToolRequest, input getTransactionHistoryInput) (*mcpsdk.CallToolResult, any, error) {
	if input.TransactionID == "" {
		return errorResult(fmt.Errorf("transaction_id is required")), nil, nil
	}
	result, err := s.svc.ListAuditLog(ctx, service.AuditLogListParams{
		EntityType: "transaction",
		EntityID:   input.TransactionID,
		Limit:      input.Limit,
	})
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result)
}

func (s *MCPServer) handleQueryAuditLog(ctx context.Context, _ *mcpsdk.CallToolRequest, input queryAuditLogInput) (*mcpsdk.CallToolResult, any, error) {
	params := service.AuditLogGlobalParams{
		Limit:  input.Limit,
		Cursor: input.Cursor,
	}
	if input.EntityType != "" {
		params.EntityType = &input.EntityType
	}
	if input.ActorType != "" {
		params.ActorType = &input.ActorType
	}
	result, err := s.svc.ListAuditLogGlobal(ctx, params)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result)
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

func (s *MCPServer) handleListCategoryMappings(_ context.Context, _ *mcpsdk.CallToolRequest, input listCategoryMappingsInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()

	var provider *string
	if input.Provider != "" {
		provider = &input.Provider
	}

	mappings, err := s.svc.ListMappings(ctx, provider, input.CategorySlug)
	if err != nil {
		return errorResult(err), nil, nil
	}

	return jsonResult(map[string]any{
		"mappings": mappings,
		"count":    len(mappings),
	})
}

func (s *MCPServer) handleCreateCategoryMapping(ctx context.Context, _ *mcpsdk.CallToolRequest, input createCategoryMappingInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	ctx = context.Background()

	if input.Provider == "" || input.ProviderCategory == "" || input.CategorySlug == "" {
		return errorResult(fmt.Errorf("provider, provider_category, and category_slug are required")), nil, nil
	}

	switch input.Provider {
	case "plaid", "teller", "csv":
	default:
		return errorResult(fmt.Errorf("invalid provider '%s'. Must be plaid, teller, or csv", input.Provider)), nil, nil
	}

	mapping, err := s.svc.CreateMappingBySlug(ctx, input.Provider, input.ProviderCategory, input.CategorySlug)
	if err != nil {
		return errorResult(err), nil, nil
	}

	return jsonResult(mapping)
}

func (s *MCPServer) handleUpdateCategoryMapping(ctx context.Context, _ *mcpsdk.CallToolRequest, input updateCategoryMappingInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	ctx = context.Background()

	if input.CategorySlug == "" {
		return errorResult(fmt.Errorf("category_slug is required")), nil, nil
	}
	if input.ID == "" && (input.Provider == "" || input.ProviderCategory == "") {
		return errorResult(fmt.Errorf("either id or (provider, provider_category) is required")), nil, nil
	}

	var id, provider, providerCategory *string
	if input.ID != "" {
		id = &input.ID
	}
	if input.Provider != "" {
		provider = &input.Provider
	}
	if input.ProviderCategory != "" {
		providerCategory = &input.ProviderCategory
	}

	mapping, err := s.svc.UpdateMappingBySlug(ctx, id, provider, providerCategory, input.CategorySlug)
	if err != nil {
		return errorResult(err), nil, nil
	}

	return jsonResult(mapping)
}

func (s *MCPServer) handleDeleteCategoryMapping(ctx context.Context, _ *mcpsdk.CallToolRequest, input deleteCategoryMappingInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	ctx = context.Background()

	if input.ID == "" && (input.Provider == "" || input.ProviderCategory == "") {
		return errorResult(fmt.Errorf("either id or (provider, provider_category) is required")), nil, nil
	}

	var id, provider, providerCategory *string
	if input.ID != "" {
		id = &input.ID
	}
	if input.Provider != "" {
		provider = &input.Provider
	}
	if input.ProviderCategory != "" {
		providerCategory = &input.ProviderCategory
	}

	prov, provCat, err := s.svc.DeleteMappingByLookup(ctx, id, provider, providerCategory)
	if err != nil {
		return errorResult(err), nil, nil
	}

	return jsonResult(map[string]any{
		"deleted":           true,
		"provider":          prov,
		"provider_category": provCat,
	})
}

// --- Review Queue ---

func (s *MCPServer) handleReviewTransactions(_ context.Context, _ *mcpsdk.CallToolRequest, input reviewTransactionsInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	params := service.PendingReviewParams{
		Limit:               20,
		IncludeInstructions: true,
	}
	if input.Limit > 0 {
		if input.Limit > 100 {
			input.Limit = 100
		}
		params.Limit = input.Limit
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

	result, err := s.svc.ListPendingReviews(ctx, params)
	if err != nil {
		return errorResult(err), nil, nil
	}

	// Build response with instructions at top level
	response := map[string]any{
		"items":         result.Items,
		"total_pending": result.TotalPending,
		"has_more":      result.HasMore,
	}
	if result.NextCursor != "" {
		response["next_cursor"] = result.NextCursor
	}
	if result.ReviewInstructions != nil {
		response["instructions"] = *result.ReviewInstructions
	}

	return jsonResult(response)
}

func (s *MCPServer) handleSubmitReview(ctx context.Context, _ *mcpsdk.CallToolRequest, input submitReviewInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if len(input.Reviews) == 0 {
		return errorResult(fmt.Errorf("reviews array is required and must not be empty")), nil, nil
	}

	actor := service.ActorFromContext(ctx)

	// Convert input to service types
	decisions := make([]service.ReviewDecision, len(input.Reviews))
	for i, r := range input.Reviews {
		decisions[i] = service.ReviewDecision{
			ReviewID:             r.ReviewID,
			Decision:             r.Decision,
			OverrideCategorySlug: r.OverrideCategorySlug,
			Comment:              r.Comment,
		}
	}

	result, err := s.svc.SubmitReviews(ctx, decisions, actor)
	if err != nil {
		return errorResult(err), nil, nil
	}

	return jsonResult(result)
}

// --- Helpers ---

func jsonResult(v any) (*mcpsdk.CallToolResult, any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return errorResult(fmt.Errorf("marshal result: %w", err)), nil, nil
	}
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: string(data)},
		},
	}, nil, nil
}

func errorResult(err error) *mcpsdk.CallToolResult {
	errJSON, _ := json.Marshal(map[string]string{"error": err.Error()})
	return &mcpsdk.CallToolResult{
		IsError: true,
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: string(errJSON)},
		},
	}
}

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

type transactionSummaryInput struct {
	StartDate      string `json:"start_date,omitempty" jsonschema:"Start date (YYYY-MM-DD) inclusive. Defaults to 30 days ago."`
	EndDate        string `json:"end_date,omitempty" jsonschema:"End date (YYYY-MM-DD) exclusive. Defaults to today."`
	GroupBy        string `json:"group_by" jsonschema:"required,How to group results: category, month, week, day, or category_month"`
	AccountID      string `json:"account_id,omitempty" jsonschema:"Filter by account ID"`
	UserID         string `json:"user_id,omitempty" jsonschema:"Filter by user ID (family member)"`
	Category       string `json:"category,omitempty" jsonschema:"Filter by primary category before aggregating"`
	IncludePending *bool  `json:"include_pending,omitempty" jsonschema:"Include pending transactions (default false)"`
}

type listPendingReviewsInput struct {
	ReviewType string `json:"review_type,omitempty" jsonschema:"Filter by review type: new_transaction, uncategorized, low_confidence, manual"`
	AccountID  string `json:"account_id,omitempty" jsonschema:"Filter by account ID"`
	UserID     string `json:"user_id,omitempty" jsonschema:"Filter by user ID (family member)"`
	Limit      int    `json:"limit,omitempty" jsonschema:"Max results (default 20, max 100)"`
	Cursor     string `json:"cursor,omitempty" jsonschema:"Pagination cursor from previous result"`
}

type submitReviewInput struct {
	ReviewID     string `json:"review_id" jsonschema:"required,UUID of the review to submit"`
	Decision     string `json:"decision" jsonschema:"required,Decision: approved or skipped"`
	CategoryID   string `json:"category_id,omitempty" jsonschema:"Category ID to assign. Provide either category_id or category_slug (not both)."`
	CategorySlug string `json:"category_slug,omitempty" jsonschema:"Category slug to assign (e.g. food_and_drink_groceries). Alternative to category_id — the slug is resolved to an ID automatically."`
	Note         string `json:"note,omitempty" jsonschema:"Optional note explaining the decision"`
}

type exportCategoriesInput struct{}

type importCategoriesInput struct {
	Content string `json:"content" jsonschema:"required,TSV content with category definitions. Columns: slug, display_name, parent_slug, icon, color, sort_order, hidden, merge_into. The merge_into column is optional — set to a target slug to merge the source category into the target (transactions and mappings reassigned then source deleted)."`
}

type exportCategoryMappingsInput struct{}

type importCategoryMappingsInput struct {
	Content            string `json:"content" jsonschema:"required,TSV content with category mappings. Columns: provider, provider_category, category_slug"`
	ApplyRetroactively bool   `json:"apply_retroactively,omitempty" jsonschema:"Apply mapping changes to ALL matching non-overridden transactions not just uncategorized. Default false."`
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

// --- Bulk export/import ---

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

func (s *MCPServer) handleExportCategoryMappings(_ context.Context, _ *mcpsdk.CallToolRequest, _ exportCategoryMappingsInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	tsv, err := s.svc.ExportMappingsTSV(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: tsv},
		},
	}, nil, nil
}

func (s *MCPServer) handleImportCategoryMappings(ctx context.Context, _ *mcpsdk.CallToolRequest, input importCategoryMappingsInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.Content == "" {
		return errorResult(fmt.Errorf("content is required")), nil, nil
	}
	result, err := s.svc.ImportMappingsTSV(context.Background(), input.Content, input.ApplyRetroactively, false)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result)
}

// --- Review Queue ---

func (s *MCPServer) handleListPendingReviews(_ context.Context, _ *mcpsdk.CallToolRequest, input listPendingReviewsInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	status := "pending"
	params := service.ReviewListParams{
		Status: &status,
		Limit:  20,
		Cursor: input.Cursor,
	}
	if input.ReviewType != "" {
		params.ReviewType = &input.ReviewType
	}
	if input.AccountID != "" {
		params.AccountID = &input.AccountID
	}
	if input.UserID != "" {
		params.UserID = &input.UserID
	}
	if input.Limit > 0 {
		if input.Limit > 100 {
			input.Limit = 100
		}
		params.Limit = input.Limit
	}
	result, err := s.svc.ListReviews(ctx, params)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result)
}

func (s *MCPServer) handleSubmitReview(ctx context.Context, _ *mcpsdk.CallToolRequest, input submitReviewInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.ReviewID == "" || input.Decision == "" {
		return errorResult(fmt.Errorf("review_id and decision are required")), nil, nil
	}
	actor := service.ActorFromContext(ctx)

	// Resolve category_slug to category_id if provided.
	categoryID := input.CategoryID
	if categoryID == "" && input.CategorySlug != "" {
		cat, err := s.svc.GetCategoryBySlug(ctx, input.CategorySlug)
		if err != nil {
			return errorResult(fmt.Errorf("invalid category_slug %q: %w", input.CategorySlug, err)), nil, nil
		}
		categoryID = cat.ID
	}

	params := service.SubmitReviewParams{
		ReviewID: input.ReviewID,
		Decision: input.Decision,
		Actor:    actor,
	}
	if categoryID != "" {
		params.CategoryID = &categoryID
	}
	if input.Note != "" {
		params.Note = &input.Note
	}
	result, err := s.svc.SubmitReview(ctx, params)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result)
}

// --- Transaction Rules ---

type createTransactionRuleInput struct {
	Name         string          `json:"name" jsonschema:"required,Name for this rule (human-readable description)"`
	CategorySlug string          `json:"category_slug" jsonschema:"required,Category slug to assign when this rule matches (e.g. food_and_drink_restaurant). Use list_categories to find slugs."`
	Conditions   map[string]any `json:"conditions" jsonschema:"required,JSON condition object. Simple: {\"field\": \"name\", \"op\": \"contains\", \"value\": \"uber\"}. AND: {\"and\": [{...}, {...}]}. OR: {\"or\": [{...}, {...}]}. NOT: {\"not\": {...}}. Fields: name merchant_name amount category_primary category_detailed pending provider account_id user_id user_name. Ops: eq neq contains not_contains matches(regex) gt gte lt lte in."`
	Priority     int            `json:"priority,omitempty" jsonschema:"Priority (higher wins when multiple rules match). Default 10."`
	ExpiresIn    string          `json:"expires_in,omitempty" jsonschema:"Optional expiry duration: 24h, 30d, 1w. Rule auto-disables after this period."`
}

type listTransactionRulesInput struct {
	CategorySlug string `json:"category_slug,omitempty" jsonschema:"Filter by category slug"`
	Enabled      *bool  `json:"enabled,omitempty" jsonschema:"Filter by enabled status"`
	Search       string `json:"search,omitempty" jsonschema:"Search by rule name"`
	Limit        int    `json:"limit,omitempty" jsonschema:"Max results (default 50, max 200)"`
	Cursor       string `json:"cursor,omitempty" jsonschema:"Pagination cursor from previous result"`
}

type updateTransactionRuleInput struct {
	ID           string           `json:"id" jsonschema:"required,UUID of the rule to update"`
	Name         *string          `json:"name,omitempty" jsonschema:"New name for the rule"`
	Conditions   map[string]any `json:"conditions,omitempty" jsonschema:"New condition tree (same format as create)"`
	CategorySlug *string          `json:"category_slug,omitempty" jsonschema:"New category slug"`
	Priority     *int             `json:"priority,omitempty" jsonschema:"New priority"`
	Enabled      *bool            `json:"enabled,omitempty" jsonschema:"Enable or disable the rule"`
	ExpiresAt    *string          `json:"expires_at,omitempty" jsonschema:"New expiry timestamp (RFC3339) or empty string to clear"`
}

type deleteTransactionRuleInput struct {
	ID string `json:"id" jsonschema:"required,UUID of the rule to delete"`
}

type batchCreateRulesInput struct {
	Rules []batchRuleItem `json:"rules" jsonschema:"required,Array of rules to create"`
}

type batchRuleItem struct {
	Name         string          `json:"name" jsonschema:"required,Human-readable rule name"`
	CategorySlug string          `json:"category_slug" jsonschema:"required,Category slug to assign"`
	Conditions   map[string]any `json:"conditions" jsonschema:"required,Condition tree as JSON object"`
	Priority     int             `json:"priority,omitempty" jsonschema:"Priority (default 10)"`
	ExpiresIn    string          `json:"expires_in,omitempty" jsonschema:"Optional expiry duration"`
}

type batchSubmitReviewsInput struct {
	Reviews []batchReviewItem `json:"reviews" jsonschema:"required,Array of review decisions to submit"`
}

type batchReviewItem struct {
	ReviewID     string  `json:"review_id" jsonschema:"required,UUID of the review"`
	Decision     string  `json:"decision" jsonschema:"required,Decision: approved or skipped"`
	CategorySlug *string `json:"category_slug,omitempty" jsonschema:"Category slug to assign (alternative to category_id). Use list_categories to find slugs."`
	CategoryID   *string `json:"category_id,omitempty" jsonschema:"Category ID to assign (alternative to category_slug)"`
	Note         *string `json:"note,omitempty" jsonschema:"Optional note explaining the decision"`
}

func (s *MCPServer) handleCreateTransactionRule(ctx context.Context, _ *mcpsdk.CallToolRequest, input createTransactionRuleInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.Name == "" || input.CategorySlug == "" {
		return errorResult(fmt.Errorf("name and category_slug are required")), nil, nil
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
		CategorySlug: input.CategorySlug,
		Priority:     priority,
		ExpiresIn:    input.ExpiresIn,
		Actor:        actor,
	})
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(rule)
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

	rule, err := s.svc.UpdateTransactionRule(ctx, input.ID, service.UpdateTransactionRuleParams{
		Name:         input.Name,
		Conditions:   conditionsPtr,
		CategorySlug: input.CategorySlug,
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

func (s *MCPServer) handleBatchSubmitReviews(ctx context.Context, _ *mcpsdk.CallToolRequest, input batchSubmitReviewsInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if len(input.Reviews) == 0 {
		return errorResult(fmt.Errorf("reviews array is required and must not be empty")), nil, nil
	}

	actor := service.ActorFromContext(ctx)

	// Resolve category slugs to IDs
	items := make([]service.BulkReviewItem, len(input.Reviews))
	for i, r := range input.Reviews {
		items[i] = service.BulkReviewItem{
			ReviewID: r.ReviewID,
			Decision: r.Decision,
			Note:     r.Note,
		}

		// CategoryID takes precedence over CategorySlug
		if r.CategoryID != nil && *r.CategoryID != "" {
			items[i].CategoryID = r.CategoryID
		} else if r.CategorySlug != nil && *r.CategorySlug != "" {
			cat, err := s.svc.GetCategoryBySlug(ctx, *r.CategorySlug)
			if err != nil {
				return errorResult(fmt.Errorf("review %s: category slug %q not found", r.ReviewID, *r.CategorySlug)), nil, nil
			}
			items[i].CategoryID = &cat.ID
		}
	}

	result, err := s.svc.BulkSubmitReviews(ctx, service.BulkSubmitReviewParams{
		Reviews: items,
		Actor:   actor,
	})
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
	if len(input.Rules) > 50 {
		return errorResult(fmt.Errorf("maximum 50 rules per batch")), nil, nil
	}

	actor := service.ActorFromContext(ctx)
	var created []service.TransactionRuleResponse
	var errors []map[string]string

	for i, r := range input.Rules {
		if r.Name == "" || r.CategorySlug == "" || len(r.Conditions) == 0 {
			errors = append(errors, map[string]string{
				"index": fmt.Sprintf("%d", i),
				"error": "name, category_slug, and conditions are required",
			})
			continue
		}

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
			CategorySlug: r.CategorySlug,
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

// --- Helpers ---

// parseConditions converts a map[string]any (from MCP input) to a service.Condition.
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

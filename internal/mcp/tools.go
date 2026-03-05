package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"breadbox/internal/service"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *MCPServer) registerTools() {
	mcpsdk.AddTool(s.server, &mcpsdk.Tool{
		Name:        "list_accounts",
		Description: "List all bank accounts synced from Plaid, Teller, or CSV import. Each account belongs to a bank connection and optionally a user (family member). Returns account type, balances, institution name, and currency. Filter by user_id to see one family member's accounts.",
	}, s.handleListAccounts)

	mcpsdk.AddTool(s.server, &mcpsdk.Tool{
		Name:        "query_transactions",
		Description: "Query bank transactions with optional filters and cursor-based pagination. Amounts follow the convention: positive = money out (debit/purchase), negative = money in (credit/deposit). Dates are YYYY-MM-DD; start_date is inclusive, end_date is exclusive. Categories use Plaid's taxonomy (e.g. FOOD_AND_DRINK, TRANSPORTATION). Use category_detailed for subcategories (e.g. FOOD_AND_DRINK_GROCERIES). Results are ordered by date descending by default. Use sort_by and sort_order to change ordering. Pagination: pass the next_cursor from the response as cursor in the next request.",
	}, s.handleQueryTransactions)

	mcpsdk.AddTool(s.server, &mcpsdk.Tool{
		Name:        "count_transactions",
		Description: "Count transactions matching optional filters. Same filters as query_transactions except cursor, limit, sort_by, and sort_order. Use this to get totals before paginating, or to compare counts across date ranges or categories.",
	}, s.handleCountTransactions)

	mcpsdk.AddTool(s.server, &mcpsdk.Tool{
		Name:        "list_categories",
		Description: "List all distinct transaction categories. Returns (primary, detailed) pairs representing the category hierarchy. Primary categories are broad (e.g. FOOD_AND_DRINK, TRANSPORTATION). Detailed categories are specific (e.g. FOOD_AND_DRINK_GROCERIES, TRANSPORTATION_GAS). Use these values with the category and category_detailed filters in query_transactions and count_transactions.",
	}, s.handleListCategories)

	mcpsdk.AddTool(s.server, &mcpsdk.Tool{
		Name:        "list_users",
		Description: "List all users (family members) in the system. Each user can own bank connections and their associated accounts. Use the returned user IDs to filter accounts or transactions by family member.",
	}, s.handleListUsers)

	mcpsdk.AddTool(s.server, &mcpsdk.Tool{
		Name:        "get_sync_status",
		Description: "Get the status of all bank connections including provider type (plaid/teller/csv), sync status (active/error/pending_reauth), last sync time, and any error details. Use this to check data freshness or diagnose sync issues.",
	}, s.handleGetSyncStatus)

	mcpsdk.AddTool(s.server, &mcpsdk.Tool{
		Name:        "trigger_sync",
		Description: "Trigger a manual sync of bank data from the provider (Plaid or Teller). Optionally specify a connection_id to sync a single connection; otherwise syncs all active connections. Returns immediately — the sync runs in the background. Check get_sync_status for results.",
	}, s.handleTriggerSync)
}

// --- Input types ---

type listAccountsInput struct {
	UserID string `json:"user_id,omitempty" jsonschema:"Filter accounts by user ID"`
}

type queryTransactionsInput struct {
	StartDate        string   `json:"start_date,omitempty" jsonschema:"Start date (YYYY-MM-DD) inclusive"`
	EndDate          string   `json:"end_date,omitempty" jsonschema:"End date (YYYY-MM-DD) exclusive"`
	AccountID        string   `json:"account_id,omitempty" jsonschema:"Filter by account ID"`
	UserID           string   `json:"user_id,omitempty" jsonschema:"Filter by user ID"`
	Category         string   `json:"category,omitempty" jsonschema:"Filter by primary category"`
	CategoryDetailed string   `json:"category_detailed,omitempty" jsonschema:"Filter by detailed category (e.g. FOOD_AND_DRINK_GROCERIES)"`
	MinAmount        *float64 `json:"min_amount,omitempty" jsonschema:"Minimum transaction amount (positive=debit, negative=credit). Amounts follow convention: positive = money out, negative = money in."`
	MaxAmount        *float64 `json:"max_amount,omitempty" jsonschema:"Maximum transaction amount (positive=debit, negative=credit). Amounts follow convention: positive = money out, negative = money in."`
	Pending          *bool    `json:"pending,omitempty" jsonschema:"Filter by pending status"`
	Search           string   `json:"search,omitempty" jsonschema:"Search transaction name or merchant name"`
	Limit            int      `json:"limit,omitempty" jsonschema:"Maximum number of results (default 50, max 500)"`
	Cursor           string   `json:"cursor,omitempty" jsonschema:"Pagination cursor from previous result"`
	SortBy           string   `json:"sort_by,omitempty" jsonschema:"Sort field: date (default), amount, or name. Cursor pagination only works with date sort."`
	SortOrder        string   `json:"sort_order,omitempty" jsonschema:"Sort direction: desc (default) or asc"`
}

type countTransactionsInput struct {
	StartDate        string   `json:"start_date,omitempty" jsonschema:"Start date (YYYY-MM-DD) inclusive"`
	EndDate          string   `json:"end_date,omitempty" jsonschema:"End date (YYYY-MM-DD) exclusive"`
	AccountID        string   `json:"account_id,omitempty" jsonschema:"Filter by account ID"`
	UserID           string   `json:"user_id,omitempty" jsonschema:"Filter by user ID"`
	Category         string   `json:"category,omitempty" jsonschema:"Filter by primary category"`
	CategoryDetailed string   `json:"category_detailed,omitempty" jsonschema:"Filter by detailed category (e.g. FOOD_AND_DRINK_GROCERIES)"`
	MinAmount        *float64 `json:"min_amount,omitempty" jsonschema:"Minimum transaction amount (positive=debit, negative=credit). Amounts follow convention: positive = money out, negative = money in."`
	MaxAmount        *float64 `json:"max_amount,omitempty" jsonschema:"Maximum transaction amount (positive=debit, negative=credit). Amounts follow convention: positive = money out, negative = money in."`
	Pending          *bool    `json:"pending,omitempty" jsonschema:"Filter by pending status"`
	Search           string   `json:"search,omitempty" jsonschema:"Search transaction name or merchant name"`
}

type triggerSyncInput struct {
	ConnectionID string `json:"connection_id,omitempty" jsonschema:"Sync a specific connection by ID. If omitted syncs all connections."`
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
	if input.Category != "" {
		params.Category = &input.Category
	}
	if input.CategoryDetailed != "" {
		params.CategoryDetailed = &input.CategoryDetailed
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

	result, err := s.svc.ListTransactions(ctx, params)
	if err != nil {
		return errorResult(err), nil, nil
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
	if input.Category != "" {
		params.Category = &input.Category
	}
	if input.CategoryDetailed != "" {
		params.CategoryDetailed = &input.CategoryDetailed
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
	categories, err := s.svc.ListDistinctCategories(ctx)
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

func (s *MCPServer) handleTriggerSync(_ context.Context, _ *mcpsdk.CallToolRequest, input triggerSyncInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
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

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
		Description: "Query bank transactions with optional filters and cursor-based pagination. Amounts: positive = money out (debit), negative = money in (credit). Dates: YYYY-MM-DD, start_date inclusive, end_date exclusive. Filter by category_slug (use list_categories to find slugs); parent slugs include all children. Results ordered by date desc by default. Pagination: pass next_cursor from response.",
	}, s.handleQueryTransactions)

	mcpsdk.AddTool(s.server, &mcpsdk.Tool{
		Name:        "count_transactions",
		Description: "Count transactions matching optional filters. Same filters as query_transactions except cursor, limit, sort_by, and sort_order. Use this to get totals before paginating, or to compare counts across date ranges or categories.",
	}, s.handleCountTransactions)

	mcpsdk.AddTool(s.server, &mcpsdk.Tool{
		Name:        "list_categories",
		Description: "List the full category taxonomy as a tree. Categories have: slug (stable identifier for filtering), display_name (human label), icon, color, and optional children. Use category slugs with the category_slug filter in query_transactions and count_transactions. Parent slugs include all children when filtering.",
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

	mcpsdk.AddTool(s.server, &mcpsdk.Tool{
		Name:        "categorize_transaction",
		Description: "Manually override a transaction's category. Use list_categories to find the category ID, then pass both the transaction_id and category_id. This creates a permanent override that won't be changed by automatic sync.",
	}, s.handleCategorizeTransaction)

	mcpsdk.AddTool(s.server, &mcpsdk.Tool{
		Name:        "reset_transaction_category",
		Description: "Remove a manual category override from a transaction and re-resolve its category from the automatic mapping rules. Use this to undo a categorize_transaction action.",
	}, s.handleResetTransactionCategory)

	mcpsdk.AddTool(s.server, &mcpsdk.Tool{
		Name:        "list_unmapped_categories",
		Description: "List distinct raw provider category strings from transactions that currently have no category mapping. These are transactions the system couldn't automatically categorize. Results show the raw primary and detailed category strings from the provider.",
	}, s.handleListUnmappedCategories)
}

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

func (s *MCPServer) handleCategorizeTransaction(_ context.Context, _ *mcpsdk.CallToolRequest, input categorizeTransactionInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
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

func (s *MCPServer) handleResetTransactionCategory(_ context.Context, _ *mcpsdk.CallToolRequest, input resetTransactionCategoryInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
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

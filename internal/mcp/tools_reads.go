package mcp

import (
	"context"

	"breadbox/internal/service"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tools in this file mirror the bounded reference resources registered in
// resources.go (breadbox://accounts, ://categories, ://tags, ://users,
// ://sync-status, ://rules, ://overview). Resources are the preferred surface
// for application-driven hosts (Claude.ai's paperclip menu, Inspector), but
// not every MCP client implements the resources/* methods — clients without
// resource support need tool fallbacks for the same reads. Each handler here
// calls into the same service path the matching resource handler uses, with
// a tool-shaped envelope.

// --- Input types ---

type listAccountsInput struct {
	ReadSessionContext
	UserID string `json:"user_id,omitempty" jsonschema:"Filter accounts by user ID"`
}

type listCategoriesInput struct {
	ReadSessionContext
}

type listUsersInput struct {
	ReadSessionContext
}

type listTagsInput struct {
	ReadSessionContext
}

type getSyncStatusInput struct {
	ReadSessionContext
}

type getOverviewInput struct {
	ReadSessionContext
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

	result, err := s.svc.ListTransactionRules(ctx, params)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result)
}

package mcp

import (
	"net/http"

	"breadbox/internal/service"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPServer wraps the MCP SDK server and the breadbox service layer.
type MCPServer struct {
	server *mcpsdk.Server
	svc    *service.Service
}

// NewMCPServer creates a new MCP server, registers all tools, and returns it.
func NewMCPServer(svc *service.Service, version string) *MCPServer {
	server := mcpsdk.NewServer(
		&mcpsdk.Implementation{
			Name:    "breadbox",
			Version: version,
		},
		&mcpsdk.ServerOptions{
			Instructions: `Breadbox is a self-hosted financial data aggregation server for families. It syncs bank data from Plaid, Teller, and CSV imports into a unified PostgreSQL database.

DATA MODEL:
- Users: family members who own bank connections
- Connections: linked bank accounts via Plaid, Teller, or CSV import (status: active, error, pending_reauth, disconnected)
- Accounts: individual bank accounts (checking, savings, credit card, etc.) belonging to a connection
- Transactions: individual financial transactions belonging to an account

AMOUNT CONVENTION:
- Positive amounts = money out (debits, purchases, payments)
- Negative amounts = money in (credits, deposits, refunds)
- All amounts include iso_currency_code — never sum across different currencies

CATEGORY SYSTEM:
- Categories are organized in a 2-level hierarchy (primary → detailed subcategories)
- Each category has: id (UUID), slug (stable identifier), display_name (human label), icon, color
- Use list_categories to get the full taxonomy tree with IDs and slugs
- Filter transactions with category_slug param (parent slug includes all children)
- Use categorize_transaction to manually override a transaction's category
- Use reset_transaction_category to undo a manual override
- Use list_unmapped_categories to find raw provider categories without mappings
- Use list_category_mappings, create_category_mapping, update_category_mapping, delete_category_mapping to manage how provider category strings map to user categories

TOKEN EFFICIENCY:
- Use the fields parameter on query_transactions to request only needed fields (e.g., fields=core,category). Aliases: core (id,date,amount,name,iso_currency_code), category (category,category_primary_raw,category_detailed_raw), timestamps (created_at,updated_at,datetime,authorized_datetime)
- Use transaction_summary for aggregated spending analysis instead of paginating through individual transactions. Supports group_by: category, month, week, day, category_month
- The breadbox://overview resource includes users, connections, accounts by type, and 30-day spending summary — often eliminates the need for separate list_users + list_accounts calls

RECOMMENDED QUERY PATTERNS:
1. Read breadbox://overview first for dataset context (users, connections, accounts, spending)
2. Use transaction_summary for spending analysis (group by category, month, etc.)
3. Use query_transactions with fields=core,category for browsing individual transactions
4. Use list_categories to understand the category taxonomy
5. Use count_transactions to get totals before paginating
6. Check get_sync_status to verify data freshness
7. Use list_unmapped_categories to identify categorization gaps

COMMENTS & AUDIT LOG:
- Use add_transaction_comment to explain your reasoning when recategorizing transactions
- Check list_transaction_comments before modifying a transaction to see prior context
- Use get_transaction_history to understand how a transaction has been modified over time
- Use query_audit_log with actor_type='user' to learn the family's categorization preferences`,
		},
	)

	s := &MCPServer{
		server: server,
		svc:    svc,
	}

	s.registerTools()
	s.registerResources()

	return s
}

// Server returns the underlying MCP SDK server.
func (s *MCPServer) Server() *mcpsdk.Server {
	return s.server
}

// NewHTTPHandler wraps the MCP server in a Streamable HTTP handler.
func NewHTTPHandler(s *MCPServer) http.Handler {
	return mcpsdk.NewStreamableHTTPHandler(
		func(r *http.Request) *mcpsdk.Server {
			return s.server
		},
		nil,
	)
}

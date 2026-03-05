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
- category_primary: broad category (e.g. FOOD_AND_DRINK, TRANSPORTATION, SHOPPING)
- category_detailed: specific subcategory (e.g. FOOD_AND_DRINK_GROCERIES, TRANSPORTATION_GAS)
- Use list_categories to discover all available category values

RECOMMENDED QUERY PATTERNS:
1. Start with list_users to identify family members
2. Use list_accounts to see available bank accounts (filter by user_id for one member)
3. Query transactions with date ranges and category filters for analysis
4. Use count_transactions to get totals before paginating large result sets
5. Check get_sync_status to verify data freshness before analysis`,
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

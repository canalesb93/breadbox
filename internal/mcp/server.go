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
			Instructions: "Breadbox is a financial data aggregation server. Use the available tools to query accounts, transactions, users, and sync status.",
		},
	)

	s := &MCPServer{
		server: server,
		svc:    svc,
	}

	s.registerTools()

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

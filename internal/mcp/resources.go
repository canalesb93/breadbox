package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *MCPServer) registerResources() {
	s.server.AddResource(&mcpsdk.Resource{
		Name:        "Overview",
		URI:         "breadbox://overview",
		Description: "Live summary of the Breadbox data model: user, connection, account, and transaction counts plus the transaction date range. Read this first to understand the dataset scope.",
		MIMEType:    "application/json",
	}, s.handleOverviewResource)
}

func (s *MCPServer) handleOverviewResource(_ context.Context, _ *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
	ctx := context.Background()
	stats, err := s.svc.GetOverviewStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("get overview stats: %w", err)
	}

	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal overview: %w", err)
	}

	return &mcpsdk.ReadResourceResult{
		Contents: []*mcpsdk.ResourceContents{
			{
				URI:      "breadbox://overview",
				MIMEType: "application/json",
				Text:     string(data),
			},
		},
	}, nil
}

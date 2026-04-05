package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"breadbox/internal/service"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *MCPServer) handleOverviewResource(_ context.Context, _ *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
	ctx := context.Background()
	stats, err := s.svc.GetOverviewStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("get overview stats: %w", err)
	}

	// Always include full household context.
	// Add default permissions (full access) -- MCP agents always see the full picture.
	// Per-request API key scope is handled at the tool level, not the resource level.
	stats.Permissions = &service.OverviewPermissions{
		Role:                   "full_access",
		CanReadAllTransactions: true,
		CanEditTransactions:    true,
		CanManageConnections:   true,
		CanManageSettings:      true,
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

func (s *MCPServer) handleReviewGuidelinesResource(_ context.Context, _ *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
	ctx := context.Background()
	cfg, err := s.svc.GetMCPConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("get mcp config: %w", err)
	}

	content := cfg.ReviewGuidelines
	if content == "" {
		content = DefaultReviewGuidelines
	}

	return &mcpsdk.ReadResourceResult{
		Contents: []*mcpsdk.ResourceContents{
			{
				URI:      "breadbox://review-guidelines",
				MIMEType: "text/markdown",
				Text:     content,
			},
		},
	}, nil
}

func (s *MCPServer) handleReportFormatResource(_ context.Context, _ *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
	ctx := context.Background()
	cfg, err := s.svc.GetMCPConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("get mcp config: %w", err)
	}

	content := cfg.ReportFormat
	if content == "" {
		content = DefaultReportFormat
	}

	return &mcpsdk.ReadResourceResult{
		Contents: []*mcpsdk.ResourceContents{
			{
				URI:      "breadbox://report-format",
				MIMEType: "text/markdown",
				Text:     content,
			},
		},
	}, nil
}

package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *MCPServer) handleOverviewResource(_ context.Context, _ *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
	ctx := context.Background()
	stats, err := s.svc.GetOverviewStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("get overview stats: %w", err)
	}

	// Permissions are intentionally omitted from the resource-level response.
	// The resource handler has no access to the per-request MCPServerConfig
	// (API key scope, MCP mode), so we cannot populate permissions accurately here.
	// Per-request scoping is enforced at the tool level via BuildServer filtering.
	stats.Permissions = nil

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

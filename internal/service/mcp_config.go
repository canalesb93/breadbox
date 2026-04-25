package service

import (
	"context"
	"encoding/json"
	"fmt"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
)

// MCPConfig represents the MCP permission and instruction settings.
type MCPConfig struct {
	Mode             string   `json:"mode"`              // "read_only" or "read_write"
	DisabledTools    []string `json:"disabled_tools"`     // tool names
	Instructions     string   `json:"instructions"`       // full server instructions
	ReviewGuidelines string   `json:"review_guidelines"`  // breadbox://review-guidelines content
	ReportFormat     string   `json:"report_format"`      // breadbox://report-format content
}

// GetMCPConfig loads MCP configuration from app_config.
func (s *Service) GetMCPConfig(ctx context.Context) (*MCPConfig, error) {
	cfg := &MCPConfig{
		Mode:          "read_write",
		DisabledTools: []string{},
	}

	rows, err := s.Queries.ListAppConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("list app config: %w", err)
	}

	for _, row := range rows {
		switch row.Key {
		case "mcp_mode":
			cfg.Mode = pgconv.TextOr(row.Value, cfg.Mode)
		case "mcp_disabled_tools":
			if raw := pgconv.TextOr(row.Value, ""); raw != "" {
				if err := json.Unmarshal([]byte(raw), &cfg.DisabledTools); err != nil {
					cfg.DisabledTools = []string{}
				}
			}
		case "mcp_instructions":
			cfg.Instructions = pgconv.TextOr(row.Value, "")
		case "mcp_review_guidelines":
			cfg.ReviewGuidelines = pgconv.TextOr(row.Value, "")
		case "mcp_report_format":
			cfg.ReportFormat = pgconv.TextOr(row.Value, "")
		}
	}

	return cfg, nil
}

// SaveMCPMode saves the global MCP mode.
func (s *Service) SaveMCPMode(ctx context.Context, mode string) error {
	if mode != "read_only" && mode != "read_write" {
		return fmt.Errorf("invalid MCP mode: %s", mode)
	}
	return s.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
		Key:   "mcp_mode",
		Value: pgconv.Text(mode),
	})
}

// SaveMCPDisabledTools saves the list of disabled tool names.
func (s *Service) SaveMCPDisabledTools(ctx context.Context, tools []string) error {
	if tools == nil {
		tools = []string{}
	}
	data, err := json.Marshal(tools)
	if err != nil {
		return fmt.Errorf("marshal disabled tools: %w", err)
	}
	return s.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
		Key:   "mcp_disabled_tools",
		Value: pgconv.Text(string(data)),
	})
}

// SaveMCPInstructions saves server instructions.
func (s *Service) SaveMCPInstructions(ctx context.Context, instructions string) error {
	if len(instructions) > 20000 {
		return fmt.Errorf("instructions exceed maximum length of 20,000 characters")
	}
	return s.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
		Key:   "mcp_instructions",
		Value: pgconv.Text(instructions),
	})
}

// SaveMCPReviewGuidelines saves review guidelines (served via breadbox://review-guidelines).
func (s *Service) SaveMCPReviewGuidelines(ctx context.Context, guidelines string) error {
	if len(guidelines) > 30000 {
		return fmt.Errorf("review guidelines exceed maximum length of 30,000 characters")
	}
	return s.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
		Key:   "mcp_review_guidelines",
		Value: pgconv.Text(guidelines),
	})
}

// SaveMCPReportFormat saves report format guidelines (served via breadbox://report-format).
func (s *Service) SaveMCPReportFormat(ctx context.Context, format string) error {
	if len(format) > 20000 {
		return fmt.Errorf("report format exceeds maximum length of 20,000 characters")
	}
	return s.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
		Key:   "mcp_report_format",
		Value: pgconv.Text(format),
	})
}

package service

import (
	"context"
	"encoding/json"
	"fmt"

	"breadbox/internal/db"

	"github.com/jackc/pgx/v5/pgtype"
)

// MCPConfig represents the MCP permission and instruction settings.
type MCPConfig struct {
	Mode          string   `json:"mode"`          // "read_only" or "read_write"
	DisabledTools []string `json:"disabled_tools"` // tool names
	Instructions  string   `json:"instructions"`   // full server instructions
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
			if row.Value.Valid {
				cfg.Mode = row.Value.String
			}
		case "mcp_disabled_tools":
			if row.Value.Valid && row.Value.String != "" {
				if err := json.Unmarshal([]byte(row.Value.String), &cfg.DisabledTools); err != nil {
					cfg.DisabledTools = []string{}
				}
			}
		case "mcp_instructions":
			if row.Value.Valid {
				cfg.Instructions = row.Value.String
			}
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
		Value: pgtype.Text{String: mode, Valid: true},
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
		Value: pgtype.Text{String: string(data), Valid: true},
	})
}

// SaveMCPInstructions saves server instructions.
func (s *Service) SaveMCPInstructions(ctx context.Context, instructions string) error {
	if len(instructions) > 20000 {
		return fmt.Errorf("instructions exceed maximum length of 20,000 characters")
	}
	return s.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
		Key:   "mcp_instructions",
		Value: pgtype.Text{String: instructions, Valid: true},
	})
}

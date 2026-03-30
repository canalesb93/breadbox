package prompts

import (
	"fmt"
	"strings"
)

// BlockRole determines how a block appears in the editor.
type BlockRole string

const (
	BlockCore     BlockRole = "core"     // always included, editable, not removable
	BlockDefault  BlockRole = "default"  // optional, enabled by default
	BlockOptional BlockRole = "optional" // optional, disabled by default (shown as pill to add)
)

// Block is a prompt section loaded from a .md file.
type Block struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	Content         string `json:"content"`
	OriginalContent string `json:"originalContent"`
	Role            string `json:"role"`    // "core", "default", "optional"
	Enabled         bool   `json:"enabled"` // true for core and default, false for optional
}

// AgentTypeConfig defines the block composition for one agent type.
type AgentTypeConfig struct {
	Type        string
	Label       string
	Description string
	Icon        string
	Color       string
	Core        []string // block IDs always included
	Default     []string // block IDs enabled by default
	Optional    []string // block IDs available but off by default
}

// agentConfigs maps URL slug to agent config.
var agentConfigs = map[string]AgentTypeConfig{
	"initial-setup": {
		Type:        "initial-setup",
		Label:       "Initial Setup",
		Description: "First-time bulk categorization after connecting a new account",
		Icon:        "sparkles",
		Color:       "primary",
		Core:        []string{"strategy-initial-setup"},
		Default:     []string{"review-depth-efficient"},
		Optional:    []string{"review-depth-thorough", "gmail-integration", "account-linking", "category-system", "sync-management", "transaction-comments", "merchant-analysis"},
	},
	"bulk-review": {
		Type:        "bulk-review",
		Label:       "Bulk Review",
		Description: "Thorough review of a large pending queue",
		Icon:        "layers",
		Color:       "primary",
		Core:        []string{"strategy-bulk-review"},
		Default:     []string{"review-depth-thorough"},
		Optional:    []string{"review-depth-efficient", "gmail-integration", "account-linking", "category-system", "sync-management", "transaction-comments", "merchant-analysis"},
	},
	"quick-review": {
		Type:        "quick-review",
		Label:       "Quick Review",
		Description: "Rapidly clear a large queue with batch operations",
		Icon:        "zap",
		Color:       "primary",
		Core:        []string{"strategy-quick-review"},
		Default:     []string{"review-depth-efficient"},
		Optional:    []string{"review-depth-thorough", "gmail-integration", "account-linking", "category-system", "sync-management", "transaction-comments", "merchant-analysis"},
	},
	"routine-review": {
		Type:        "routine-review",
		Label:       "Routine Review",
		Description: "Daily or weekly review of recent transactions",
		Icon:        "repeat",
		Color:       "success",
		Core:        []string{"strategy-routine-review"},
		Default:     []string{"review-depth-thorough", "transaction-comments"},
		Optional:    []string{"review-depth-efficient", "gmail-integration", "account-linking", "category-system", "sync-management", "merchant-analysis"},
	},
	"spending-report": {
		Type:        "spending-report",
		Label:       "Spending Report",
		Description: "Weekly or monthly spending summary with trends",
		Icon:        "bar-chart-3",
		Color:       "violet",
		Core:        []string{"strategy-spending-report"},
		Default:     []string{"category-system", "merchant-analysis"},
		Optional:    []string{"gmail-integration", "account-linking", "sync-management", "transaction-comments"},
	},
	"anomaly-detection": {
		Type:        "anomaly-detection",
		Label:       "Anomaly Detection",
		Description: "Monitor for unusual charges, duplicates, and spending spikes",
		Icon:        "shield-alert",
		Color:       "warning",
		Core:        []string{"strategy-anomaly-detection"},
		Default:     []string{"merchant-analysis", "account-linking"},
		Optional:    []string{"gmail-integration", "category-system", "sync-management", "transaction-comments"},
	},
	"custom": {
		Type:        "custom",
		Label:       "Custom Agent",
		Description: "Start from scratch with your own goals and instructions",
		Icon:        "plus",
		Color:       "base",
		Core:        []string{},
		Default:     []string{},
		Optional: []string{
			"review-depth-efficient", "review-depth-thorough",
			"category-system", "account-linking",
			"gmail-integration", "sync-management", "transaction-comments", "merchant-analysis",
		},
	},
}

// GetAgentConfig returns the config for an agent type.
func GetAgentConfig(agentType string) (AgentTypeConfig, bool) {
	cfg, ok := agentConfigs[agentType]
	return cfg, ok
}

// AllAgentTypes returns all registered agent type slugs.
func AllAgentTypes() []string {
	types := make([]string, 0, len(agentConfigs))
	for k := range agentConfigs {
		types = append(types, k)
	}
	return types
}

// LoadBlock reads a block .md file, parsing title and description from the first lines.
func LoadBlock(id string) (Block, error) {
	data, err := blocksFS.ReadFile("blocks/" + id + ".md")
	if err != nil {
		return Block{}, fmt.Errorf("read block %q: %w", id, err)
	}

	content := string(data)
	lines := strings.SplitN(content, "\n", 4)

	var title, description, body string

	if len(lines) >= 1 {
		title = strings.TrimPrefix(strings.TrimSpace(lines[0]), "# ")
	}
	if len(lines) >= 2 {
		description = strings.TrimPrefix(strings.TrimSpace(lines[1]), "> ")
	}
	// Skip blank line between description and body.
	if len(lines) >= 4 {
		body = strings.TrimSpace(lines[3])
	}

	return Block{
		ID:              id,
		Title:           title,
		Description:     description,
		Content:         body,
		OriginalContent: body,
	}, nil
}

// LoadAgentBlocks loads all blocks for an agent type with roles and enabled state set.
func LoadAgentBlocks(agentType string) ([]Block, error) {
	cfg, ok := agentConfigs[agentType]
	if !ok {
		return nil, fmt.Errorf("unknown agent type: %s", agentType)
	}

	// Build role map.
	roles := make(map[string]BlockRole)
	for _, id := range cfg.Core {
		roles[id] = BlockCore
	}
	for _, id := range cfg.Default {
		roles[id] = BlockDefault
	}
	for _, id := range cfg.Optional {
		roles[id] = BlockOptional
	}

	// Load blocks in order: core, default, optional.
	var blocks []Block
	allIDs := make([]string, 0, len(cfg.Core)+len(cfg.Default)+len(cfg.Optional))
	allIDs = append(allIDs, cfg.Core...)
	allIDs = append(allIDs, cfg.Default...)
	allIDs = append(allIDs, cfg.Optional...)

	for _, id := range allIDs {
		block, err := LoadBlock(id)
		if err != nil {
			return nil, err
		}
		role := roles[id]
		block.Role = string(role)
		block.Enabled = role == BlockCore || role == BlockDefault
		blocks = append(blocks, block)
	}

	return blocks, nil
}

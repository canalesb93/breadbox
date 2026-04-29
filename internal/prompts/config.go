package prompts

import (
	"fmt"
	"strings"
	"sync"

	rootprompts "breadbox/prompts"

	"gopkg.in/yaml.v3"
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

// AgentTypeConfig is the legacy view of one agent's block composition.
// Kept for backward compatibility with the prompt-builder handler.
type AgentTypeConfig struct {
	Type        string
	Label       string
	Description string
	Icon        string
	Color       string
	Core        []string
	Default     []string
	Optional    []string
}

// Section groups agents on the landing page.
type Section struct {
	ID               string `yaml:"id"`
	Title            string `yaml:"title"`
	ShowPendingCount bool   `yaml:"show_pending_count"`
}

// AgentBlocks is the per-agent block composition declared in agents.yaml.
type AgentBlocks struct {
	Core     []string `yaml:"core"`
	Default  []string `yaml:"default"`
	Optional []string `yaml:"optional"`
}

// Agent is one entry in agents.yaml — covers both the prompt-builder header
// and the landing-page card.
type Agent struct {
	Slug        string      `yaml:"slug"`
	Label       string      `yaml:"label"`
	Description string      `yaml:"description"`
	Body        string      `yaml:"body"`
	Icon        string      `yaml:"icon"`
	Color       string      `yaml:"color"`
	Section     string      `yaml:"section"`
	Badge       string      `yaml:"badge"`
	BadgeStyle  string      `yaml:"badge_style"`
	Counter     string      `yaml:"counter"`
	WarnNoRules bool        `yaml:"warn_no_rules"`
	Layout      string      `yaml:"layout"`
	Blocks      AgentBlocks `yaml:"blocks"`
}

// Catalog is the parsed agents.yaml.
type Catalog struct {
	Sections []Section `yaml:"sections"`
	Agents   []Agent   `yaml:"agents"`
}

var (
	catalogOnce sync.Once
	catalog     *Catalog
	catalogErr  error
)

// loadCatalog parses prompts/agents.yaml and validates that every referenced
// block exists. Cached after first call.
func loadCatalog() (*Catalog, error) {
	catalogOnce.Do(func() {
		data, err := rootprompts.AgentsConfig()
		if err != nil {
			catalogErr = err
			return
		}
		var c Catalog
		if err := yaml.Unmarshal(data, &c); err != nil {
			catalogErr = fmt.Errorf("parse agents.yaml: %w", err)
			return
		}
		// Build a lookup of valid section IDs.
		validSection := make(map[string]bool, len(c.Sections))
		for _, s := range c.Sections {
			validSection[s.ID] = true
		}
		// Validate agents: unique slug, known section, every block resolves.
		seen := make(map[string]bool, len(c.Agents))
		for _, a := range c.Agents {
			if a.Slug == "" {
				catalogErr = fmt.Errorf("agents.yaml: agent missing slug")
				return
			}
			if seen[a.Slug] {
				catalogErr = fmt.Errorf("agents.yaml: duplicate agent slug %q", a.Slug)
				return
			}
			seen[a.Slug] = true
			if a.Section != "" && !validSection[a.Section] {
				catalogErr = fmt.Errorf("agents.yaml: agent %q references unknown section %q", a.Slug, a.Section)
				return
			}
			ids := make([]string, 0, len(a.Blocks.Core)+len(a.Blocks.Default)+len(a.Blocks.Optional))
			ids = append(ids, a.Blocks.Core...)
			ids = append(ids, a.Blocks.Default...)
			ids = append(ids, a.Blocks.Optional...)
			for _, id := range ids {
				if _, err := LoadBlock(id); err != nil {
					catalogErr = fmt.Errorf("agents.yaml: agent %q references unknown block %q: %w", a.Slug, id, err)
					return
				}
			}
		}
		catalog = &c
	})
	return catalog, catalogErr
}

// findAgent returns the catalog entry matching slug.
func findAgent(slug string) (Agent, bool) {
	c, err := loadCatalog()
	if err != nil || c == nil {
		return Agent{}, false
	}
	for _, a := range c.Agents {
		if a.Slug == slug {
			return a, true
		}
	}
	return Agent{}, false
}

// GetAgentConfig returns the legacy view of an agent's config.
func GetAgentConfig(agentType string) (AgentTypeConfig, bool) {
	a, ok := findAgent(agentType)
	if !ok {
		return AgentTypeConfig{}, false
	}
	return AgentTypeConfig{
		Type:        a.Slug,
		Label:       a.Label,
		Description: a.Description,
		Icon:        a.Icon,
		Color:       a.Color,
		Core:        a.Blocks.Core,
		Default:     a.Blocks.Default,
		Optional:    a.Blocks.Optional,
	}, true
}

// ListAgents returns every agent in declaration order.
func ListAgents() ([]Agent, error) {
	c, err := loadCatalog()
	if err != nil {
		return nil, err
	}
	return c.Agents, nil
}

// ListSections returns every section in declaration order.
func ListSections() ([]Section, error) {
	c, err := loadCatalog()
	if err != nil {
		return nil, err
	}
	return c.Sections, nil
}

// LoadBlock reads a block .md file, parsing title and description from the
// first lines. The title is the first H1, the description is the first
// blockquote (`> ...`), and the rest is the body.
func LoadBlock(id string) (Block, error) {
	data, err := rootprompts.Agent(id)
	if err != nil {
		return Block{}, err
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

	// Prepend the title as an H1 so it appears in the composed prompt, not just the UI.
	content = body
	if title != "" {
		content = "# " + title + "\n\n" + body
	}

	return Block{
		ID:              id,
		Title:           title,
		Description:     description,
		Content:         content,
		OriginalContent: content,
	}, nil
}

// LoadAgentBlocks loads all blocks for an agent type with roles and enabled state set.
func LoadAgentBlocks(agentType string) ([]Block, error) {
	a, ok := findAgent(agentType)
	if !ok {
		return nil, fmt.Errorf("unknown agent type: %s", agentType)
	}

	roles := make(map[string]BlockRole, len(a.Blocks.Core)+len(a.Blocks.Default)+len(a.Blocks.Optional))
	for _, id := range a.Blocks.Core {
		roles[id] = BlockCore
	}
	for _, id := range a.Blocks.Default {
		roles[id] = BlockDefault
	}
	for _, id := range a.Blocks.Optional {
		roles[id] = BlockOptional
	}

	allIDs := make([]string, 0, len(roles))
	allIDs = append(allIDs, a.Blocks.Core...)
	allIDs = append(allIDs, a.Blocks.Default...)
	allIDs = append(allIDs, a.Blocks.Optional...)

	blocks := make([]Block, 0, len(allIDs))
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

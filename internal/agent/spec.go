//go:build !lite

// Package agent provides the Go-side runner and types for the Claude Agent SDK
// integration. The breadbox-agent sidecar binary (built from agent/sidecar/)
// is exec'd per run; this package assembles the JobSpec, streams NDJSON events
// from stdout, and persists the agent_runs row.
package agent

// MCPServerConfig describes one MCP server the sidecar should connect to.
// breadbox itself is always present, pointing at the local `breadbox mcp` stdio.
type MCPServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// AuthConfig carries credentials for one run. Mode picks which env var the
// sidecar sets; the other is unset before invoking the SDK to avoid the
// precedence trap where ANTHROPIC_API_KEY silently wins over CLAUDE_CODE_OAUTH_TOKEN.
type AuthConfig struct {
	Mode  string `json:"mode"`  // "subscription" | "api_key"
	Token string `json:"token"` // sk-ant-oat01-… (subscription) or sk-ant-… (api_key)
}

// JobSpec is the JSON document written to the sidecar's stdin.
// camelCase JSON tags match the TypeScript sidecar's zod schema.
type JobSpec struct {
	// Identity (passed through for log correlation)
	RunID             string `json:"runId"`
	AgentDefinitionID string `json:"agentDefinitionId"`

	// Prompt
	Prompt       string `json:"prompt"`
	SystemPrompt string `json:"systemPrompt,omitempty"`

	// Model parameters
	Model        string  `json:"model"`
	MaxTurns     int     `json:"maxTurns"`
	MaxBudgetUsd float64 `json:"maxBudgetUsd"`

	// Tool config
	ToolScope    string   `json:"toolScope"`              // "read_only" | "read_write"
	AllowedTools []string `json:"allowedTools,omitempty"` // MCP tool names; empty = SDK default for scope

	// MCP servers the sidecar should mount
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`

	// Auth
	Auth AuthConfig `json:"auth"`

	// Transcript: absolute path; the sidecar mirrors NDJSON events here.
	TranscriptPath string `json:"transcriptPath,omitempty"`

	// SessionID: when non-empty, the SDK resumes the prior session.
	SessionID string `json:"sessionId,omitempty"`
}

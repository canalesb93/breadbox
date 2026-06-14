//go:build !lite

// Package agent provides the Go-side runner and types for the Claude Agent SDK
// integration. The breadbox-agent sidecar binary (built from agent/sidecar/)
// is exec'd per run; this package assembles the JobSpec, streams NDJSON events
// from stdout, and persists the agent_runs row.
package agent

// MCPServerConfig describes one MCP server the sidecar should connect to.
// It models BOTH transports the TS sidecar's zod union accepts (see
// agent/sidecar/spec.ts::McpServerConfigSchema):
//
//   - stdio: Command (+ Args/Env). breadbox itself is always present this way,
//     pointing at the local `breadbox mcp` stdio.
//   - http:  Type="http" + URL (+ Headers). Used by custom connectors, e.g. a
//     remote Gmail MCP reached with an Authorization bearer header.
//
// Command is `omitempty` deliberately: an HTTP entry must NOT serialize an
// empty "command", or the TS union would match it against the stdio variant
// (whose command is just z.string(), which "" satisfies) instead of falling
// through to the HTTP variant.
type MCPServerConfig struct {
	// stdio transport
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`

	// http transport
	Type    string            `json:"type,omitempty"` // "http" when set
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// AuthConfig carries credentials for one run. Mode picks which env var the
// runner sets on the sidecar process; the other is unset before exec to
// avoid the precedence trap where ANTHROPIC_API_KEY silently wins over
// CLAUDE_CODE_OAUTH_TOKEN.
//
// Token is `json:"-"` deliberately: secrets never travel via the sidecar's
// stdin JSON, only as scoped env vars that Sidecar.Run sets on cmd.Env.
// This closes the leak class where any future `slog.Warn("spec", spec)`
// or similar would otherwise put the plaintext token in logs / OTel.
type AuthConfig struct {
	Mode  string `json:"mode"`  // "subscription" | "api_key" — metadata, safe on the wire
	Token string `json:"-"`     // sk-ant-oat01-… or sk-ant-…; flows to sidecar via env, NEVER JSON
}

// String returns a redacted representation safe for logs / %v formatting.
// Without this an inadvertent `fmt.Sprintf("%+v", spec)` in any handler
// would print the plaintext token. Belt-and-suspenders alongside `json:"-"`.
func (a AuthConfig) String() string {
	suffix := ""
	if n := len(a.Token); n >= 4 {
		suffix = a.Token[n-4:]
	}
	return "AuthConfig{Mode=" + a.Mode + ", Token=…" + suffix + "}"
}

// GoString mirrors String for `%#v` formatting (verbose / debug log style).
func (a AuthConfig) GoString() string { return a.String() }

// JobSpec is the JSON document written to the sidecar's stdin.
// camelCase JSON tags match the TypeScript sidecar's zod schema.
type JobSpec struct {
	// Identity (passed through for log correlation)
	RunID             string `json:"runId"`
	WorkflowID string `json:"workflowId"`

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

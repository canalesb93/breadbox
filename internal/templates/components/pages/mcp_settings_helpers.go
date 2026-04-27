package pages

// stdioConfigJSON is the static stdio MCP-server snippet shown in the
// Connection card. Kept in Go so the templ file can interpolate it as a
// plain string instead of a backtick raw literal (templ rejects those
// inside element bodies).
func stdioConfigJSON() string {
	return `{
  "mcpServers": {
    "breadbox": {
      "command": "docker",
      "args": ["exec", "-i", "breadbox-app-1", "/app/breadbox", "mcp-stdio"]
    }
  }
}`
}

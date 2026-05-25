//go:build !headless && !lite

package pages

import "strings"

// AgentRunFriendlyError maps known raw error_message strings on an
// agent_run row to operator-friendly text. Returns "" when no mapping
// applies — callers should fall back to the raw message.
//
// Mirrors the shape of internal/sync/errors.go (sync logs) but is a
// separate list because the failure modes differ: the only overlap
// today is the startup orphan-cleanup message, which both subsystems
// write verbatim and both want to surface as "safe to retry".
func AgentRunFriendlyError(raw string) string {
	lower := strings.ToLower(raw)
	for _, m := range AgentRunFriendlyErrorMappings {
		if strings.Contains(lower, m.pattern) {
			return m.message
		}
	}
	return ""
}

type agentRunErrorMapping struct {
	pattern string
	message string
}

var AgentRunFriendlyErrorMappings = []agentRunErrorMapping{
	{
		pattern: "interrupted by server restart",
		message: "Interrupted by a server restart — safe to re-run.",
	},
}

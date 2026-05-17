//go:build !lite

package cli

import "breadbox/internal/agent"

// These aliases let the tag-free errors.go reference agent sentinels
// without importing internal/agent directly (which would break the lite
// build). The lite sibling defines unreachable sentinels so the
// MapExitCode match cases compile but never fire.
var (
	agentErrAuthNotConfigured = agent.ErrAuthNotConfigured
	agentErrBinaryNotFound    = agent.ErrBinaryNotFound
)

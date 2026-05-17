//go:build lite

package cli

import "errors"

// Lite builds don't include internal/agent — the CLI smoke-test command
// isn't reachable here. These unreachable sentinels exist only so the
// tag-free errors.go::MapExitCode references compile cleanly.
var (
	agentErrAuthNotConfigured = errors.New("agent not configured (unreachable under -tags=lite)")
	agentErrBinaryNotFound    = errors.New("agent binary not found (unreachable under -tags=lite)")
)

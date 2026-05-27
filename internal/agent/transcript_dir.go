//go:build !lite

package agent

import (
	"os"
	"path/filepath"
)

// TranscriptDirEnvVar is the env-var name that supplies the fallback
// transcript_dir when app_config has no explicit value.
const TranscriptDirEnvVar = "BREADBOX_AGENT_TRANSCRIPT_DIR"

// DefaultTranscriptDir resolves the fallback transcript directory used
// when `app_config.agent.transcript_dir` is empty.
//
// Precedence (highest first):
//
//  1. operator config in app_config — handled by the caller via
//     appconfig.String(..., KeyAgentTranscriptDir, DefaultTranscriptDir(dataDir))
//  2. BREADBOX_AGENT_TRANSCRIPT_DIR env var — local-dev hook for sharing
//     one transcript dir across multiple worktree servers
//  3. <dataDir>/transcripts/agents when dataDir is non-empty — derived
//     from BB_DATA_DIR (defaults to /var/lib/breadbox in ENVIRONMENT=docker)
//  4. "transcripts/agents" — cwd-relative, used when no data root is
//     configured (local dev outside Docker)
//
// In Docker images dataDir resolves to /var/lib/breadbox, so transcripts
// land at /var/lib/breadbox/transcripts/agents alongside the backups
// dir — one volume covers both. Local worktree sessions get a stable
// home-shared dir via the session-start hook (env var path 2).
func DefaultTranscriptDir(dataDir string) string {
	if v := os.Getenv(TranscriptDirEnvVar); v != "" {
		return v
	}
	if dataDir != "" {
		return filepath.Join(dataDir, "transcripts", "agents")
	}
	return "transcripts/agents"
}

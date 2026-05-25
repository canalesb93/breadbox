//go:build !lite

package agent

import "os"

// TranscriptDirEnvVar is the env-var name that supplies the fallback
// transcript_dir when app_config has no explicit value.
const TranscriptDirEnvVar = "BREADBOX_AGENT_TRANSCRIPT_DIR"

// DefaultTranscriptDir resolves the fallback transcript directory used
// when `app_config.agent.transcript_dir` is empty.
//
// Precedence (highest first):
//
//  1. operator config in app_config — handled by the caller via
//     appconfig.String(..., KeyAgentTranscriptDir, DefaultTranscriptDir())
//  2. BREADBOX_AGENT_TRANSCRIPT_DIR env var — local-dev hook for sharing
//     one transcript dir across multiple worktree servers
//  3. "transcripts/agents" — cwd-relative, matches the Docker image's
//     /app/transcripts/agents working directory
//
// The Docker image still resolves to /app/transcripts/agents because the
// env var is unset and the working directory is /app. Local worktree
// sessions get a stable home-shared dir via the session-start hook.
func DefaultTranscriptDir() string {
	if v := os.Getenv(TranscriptDirEnvVar); v != "" {
		return v
	}
	return "transcripts/agents"
}

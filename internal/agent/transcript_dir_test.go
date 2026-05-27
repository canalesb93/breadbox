//go:build !lite

package agent_test

import (
	"testing"

	"breadbox/internal/agent"
)

func TestDefaultTranscriptDir(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		dataDir string
		want    string
	}{
		{
			name: "no env, no dataDir → cwd-relative default",
			want: "transcripts/agents",
		},
		{
			name:    "no env, dataDir set → joined under dataDir",
			dataDir: "/var/lib/breadbox",
			want:    "/var/lib/breadbox/transcripts/agents",
		},
		{
			name:    "env set wins over dataDir",
			env:     "/tmp/shared-transcripts",
			dataDir: "/var/lib/breadbox",
			want:    "/tmp/shared-transcripts",
		},
		{
			name: "env set, no dataDir → env honored",
			env:  "/tmp/shared-transcripts",
			want: "/tmp/shared-transcripts",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(agent.TranscriptDirEnvVar, tc.env)
			if got := agent.DefaultTranscriptDir(tc.dataDir); got != tc.want {
				t.Fatalf("DefaultTranscriptDir(%q) = %q, want %q", tc.dataDir, got, tc.want)
			}
		})
	}
}

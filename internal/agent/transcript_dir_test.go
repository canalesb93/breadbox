//go:build !lite

package agent_test

import (
	"testing"

	"breadbox/internal/agent"
)

func TestDefaultTranscriptDir(t *testing.T) {
	cases := []struct {
		name string
		env  string
		want string
	}{
		{name: "unset → cwd-relative default", env: "", want: "transcripts/agents"},
		{name: "set → honored", env: "/var/lib/breadbox/transcripts/agents", want: "/var/lib/breadbox/transcripts/agents"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(agent.TranscriptDirEnvVar, tc.env)
			if got := agent.DefaultTranscriptDir(); got != tc.want {
				t.Fatalf("DefaultTranscriptDir() = %q, want %q", got, tc.want)
			}
		})
	}
}

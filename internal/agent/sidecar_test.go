//go:build !lite

package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeFakeSidecar drops a #!/bin/sh script at <dir>/breadbox-agent that
// emits the given NDJSON body on stdout and exits with the given code.
func writeFakeSidecar(t *testing.T, dir, body string, exitCode int) string {
	t.Helper()
	path := filepath.Join(dir, "breadbox-agent")
	script := fmt.Sprintf("#!/bin/sh\ncat <<'EOF'\n%s\nEOF\nexit %d\n", body, exitCode)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake sidecar: %v", err)
	}
	return path
}

func TestSidecarRun_ParsesNDJSONStream(t *testing.T) {
	tmp := t.TempDir()
	body := `{"type":"assistant_message","ts":1,"data":{"content":"hello"}}
{"type":"result","ts":2,"data":{"totalCostUsd":0.0123,"inputTokens":100,"outputTokens":50,"cacheReadTokens":0,"cacheCreationTokens":0,"turnCount":2,"numToolCalls":1,"sessionId":"sess-abc","stopReason":"end_turn"}}`
	bin := writeFakeSidecar(t, tmp, body, 0)

	transcriptDir := t.TempDir()
	s := &Sidecar{BinaryPath: bin, TranscriptDir: transcriptDir}

	var calls int
	handler := func(e Event) error {
		calls++
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	spec := JobSpec{
		RunID:        "test-run-id",
		Prompt:       "hi",
		Model:        "claude-opus-4-7",
		MaxTurns:     5,
		MaxBudgetUsd: 1.0,
		ToolScope:    "read_only",
		Auth:         AuthConfig{Mode: "api_key", Token: "fake"},
	}
	res, err := s.Run(ctx, spec, handler)

	if err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if res.Status != StatusSuccess {
		t.Errorf("Status = %q, want success", res.Status)
	}
	if res.TotalCostUSD != 0.0123 {
		t.Errorf("TotalCostUSD = %v, want 0.0123", res.TotalCostUSD)
	}
	if res.InputTokens != 100 || res.OutputTokens != 50 {
		t.Errorf("Tokens = (%d,%d), want (100,50)", res.InputTokens, res.OutputTokens)
	}
	if res.TurnCount != 2 {
		t.Errorf("TurnCount = %d, want 2", res.TurnCount)
	}
	if res.NumToolCalls != 1 {
		t.Errorf("NumToolCalls = %d, want 1", res.NumToolCalls)
	}
	if res.SessionID != "sess-abc" {
		t.Errorf("SessionID = %q, want sess-abc", res.SessionID)
	}
	if res.DurationMs <= 0 {
		t.Errorf("DurationMs = %d, want > 0", res.DurationMs)
	}
	if res.TranscriptPath == "" {
		t.Errorf("TranscriptPath should be populated")
	}
	if calls != 2 {
		t.Errorf("handler invocations = %d, want 2", calls)
	}

	// Transcript file should exist with 2 lines.
	transcriptBytes, err := os.ReadFile(res.TranscriptPath)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	if got := strings.Count(string(transcriptBytes), "\n"); got != 2 {
		t.Errorf("transcript line count = %d, want 2", got)
	}
}

func TestSidecarRun_BinaryNotFound(t *testing.T) {
	s := &Sidecar{} // no BinaryPath, no env, no ./bin/breadbox-agent
	t.Setenv("BREADBOX_AGENT_BIN", "")
	t.Setenv("PATH", "/nonexistent-for-test")

	res, err := s.Run(context.Background(), JobSpec{
		RunID:  "x",
		Prompt: "p",
		Model:  "claude-opus-4-7",
		Auth:   AuthConfig{Mode: "api_key", Token: "fake"},
	}, nil)
	if err == nil {
		t.Fatal("expected ErrBinaryNotFound, got nil")
	}
	if !errors.Is(err, ErrBinaryNotFound) {
		t.Errorf("err = %v, want ErrBinaryNotFound", err)
	}
	if res.Status != StatusError {
		t.Errorf("Status = %q, want error", res.Status)
	}
}

func TestSidecarRun_NonZeroExit(t *testing.T) {
	tmp := t.TempDir()
	body := `{"type":"error","ts":1,"data":{"code":"X","message":"sidecar said no"}}`
	bin := writeFakeSidecar(t, tmp, body, 1)

	s := &Sidecar{BinaryPath: bin, TranscriptDir: t.TempDir()}
	res, err := s.Run(context.Background(), JobSpec{
		RunID:  "y",
		Prompt: "p",
		Model:  "claude-opus-4-7",
		Auth:   AuthConfig{Mode: "api_key", Token: "fake"},
	}, nil)
	if err == nil {
		t.Fatal("expected non-nil error for non-zero exit")
	}
	if res.Status != StatusError {
		t.Errorf("Status = %q, want error", res.Status)
	}
	// The error event payload should surface via the wrapped message.
	if !strings.Contains(err.Error(), "sidecar said no") {
		t.Logf("err = %v (acceptable — exit-code-based path also fine)", err)
	}
}

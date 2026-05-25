//go:build !lite

package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestSidecarRun_ProcessGroupReapsGrandchild verifies the actual root-cause
// fix for the RK7U4E06 incident: when the parent ctx is cancelled, both the
// sidecar parent process AND the long-running child it spawned must be
// dead by the time Run returns.
//
// Before this PR, exec.CommandContext only sent SIGKILL to the parent;
// the child (here a `sleep`, in production the SDK's Node cli.js) survived
// orphaned to init. We now put the spawned cmd in its own process group
// (Setpgid:true) and kill the negative pgid on cancel.
func TestSidecarRun_ProcessGroupReapsGrandchild(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "breadbox-agent")
	// Fake sidecar: prints "child=<pid>" then keeps the parent alive too.
	// Critically the child is spawned in the same process group as the
	// parent (the shell's default behavior under our Setpgid:true on the
	// sidecar). If our group-kill works, both die when ctx cancels.
	//
	// We use `sleep 30` (no `exec`) so the shell is its own process and
	// the child is a distinct PID we can probe.
	script := `#!/bin/sh
sleep 30 &
echo "child=$!"
sleep 30
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake sidecar: %v", err)
	}

	s := &Sidecar{BinaryPath: bin, TranscriptDir: t.TempDir()}

	// We need the child pid out-of-band. Hijack the event handler — the
	// sidecar writes "child=<pid>" as a stdout line, which doesn't parse
	// as NDJSON so it never reaches the handler; we instead read the
	// transcript file after.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run in a goroutine so we can cancel from this test goroutine.
	type runResult struct {
		res RunResult
		err error
	}
	done := make(chan runResult, 1)
	go func() {
		res, err := s.Run(ctx, JobSpec{
			RunID:  "pgtest",
			Prompt: "x",
			Model:  "claude-haiku-4-5",
			Auth:   AuthConfig{Mode: "api_key", Token: "fake"},
		}, nil)
		done <- runResult{res, err}
	}()

	// Give the fake sidecar 500ms to print its child=<pid> line.
	transcriptPath := filepath.Join(s.TranscriptDir, "pgtest.ndjson")
	childPid := waitForChildPid(t, transcriptPath, 2*time.Second)
	t.Logf("fake sidecar's child pid = %d", childPid)

	// Sanity check: the child should be alive before we cancel.
	if err := syscall.Kill(childPid, 0); err != nil {
		t.Fatalf("child %d unexpectedly not alive pre-cancel: %v", childPid, err)
	}

	// Pull the rug.
	cancel()

	// Wait for Run() to return — it should within a couple of seconds
	// thanks to cmd.WaitDelay and the group-kill.
	select {
	case r := <-done:
		if !errors.Is(r.err, context.Canceled) && !errors.Is(r.err, context.DeadlineExceeded) {
			// Non-fatal — the exact error class depends on whether the
			// sidecar exited on its own first; what matters is that we
			// returned promptly.
			t.Logf("Run returned err=%v (status=%q)", r.err, r.res.Status)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Sidecar.Run did not return within 10s of ctx cancel — orphan-grandchild fix broken or WaitDelay not honored")
	}

	// Now the critical assertion: the child must be dead. Give the kernel
	// up to 500ms to reap the SIGKILL'd child.
	if !waitProcessGone(childPid, 500*time.Millisecond) {
		// Try once more with SIGTERM to confirm it's reachable (and clean up).
		// If kill -0 succeeds the child is alive, which is the bug.
		if err := syscall.Kill(childPid, 0); err == nil {
			_ = syscall.Kill(childPid, syscall.SIGKILL)
			t.Fatalf("child pid %d survived ctx cancel — orphan-grandchild fix not working", childPid)
		}
	}
}

func waitForChildPid(t *testing.T, transcriptPath string, deadline time.Duration) int {
	t.Helper()
	stop := time.Now().Add(deadline)
	for time.Now().Before(stop) {
		b, err := os.ReadFile(transcriptPath)
		if err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				if !strings.HasPrefix(line, "child=") {
					continue
				}
				pid, perr := strconv.Atoi(strings.TrimPrefix(line, "child="))
				if perr == nil && pid > 0 {
					return pid
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("did not see child=<pid> line in %s within %v", transcriptPath, deadline)
	return 0
}

func waitProcessGone(pid int, deadline time.Duration) bool {
	stop := time.Now().Add(deadline)
	for time.Now().Before(stop) {
		if err := syscall.Kill(pid, 0); err != nil {
			return true // ESRCH or similar: process is gone
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// TestSidecarRun_ConcurrentRunsNoLongerSerialize verifies that dropping
// Sidecar.mu actually lets two concurrent runs proceed in parallel — the
// previous mutex serialized everything regardless of the orchestrator's
// semaphore setting.
func TestSidecarRun_ConcurrentRunsNoLongerSerialize(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "breadbox-agent")
	// Fake sidecar that sleeps 300ms and emits a minimal result event.
	script := `#!/bin/sh
sleep 0.3
printf '{"type":"result","ts":1,"data":{"totalCostUsd":0,"inputTokens":0,"outputTokens":0,"cacheReadTokens":0,"cacheCreationTokens":0,"turnCount":1,"numToolCalls":0,"sessionId":"s","stopReason":"end_turn"}}\n'
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake sidecar: %v", err)
	}

	s := &Sidecar{BinaryPath: bin, TranscriptDir: t.TempDir()}

	// Run 3 in parallel. If the mutex were still there, total time ≥ 900ms.
	// With it gone, total ≈ 300ms (plus startup overhead).
	start := time.Now()
	errs := make(chan error, 3)
	for i := 0; i < 3; i++ {
		go func(i int) {
			_, err := s.Run(context.Background(), JobSpec{
				RunID:  fmt.Sprintf("conc-%d", i),
				Prompt: "x",
				Model:  "claude-haiku-4-5",
				Auth:   AuthConfig{Mode: "api_key", Token: "fake"},
			}, nil)
			errs <- err
		}(i)
	}
	for i := 0; i < 3; i++ {
		if err := <-errs; err != nil {
			t.Errorf("Run %d: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	// Allow generous headroom: even with parallelism, fork+exec on macOS
	// can take ~100ms each. The previous serialized worst-case would be ~900ms+;
	// 700ms is comfortably below that and well above the parallel ideal.
	if elapsed > 700*time.Millisecond {
		t.Errorf("3 concurrent runs took %v — expected well under 700ms (parallelism lost?)", elapsed)
	}
}

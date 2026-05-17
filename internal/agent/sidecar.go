//go:build !lite

package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Sidecar locates and exec's the breadbox-agent binary. Implements Runner.
// Concurrency: holds an internal mutex as a safety net; the orchestrator
// (added in Iteration 3) is responsible for the server-wide concurrency cap.
type Sidecar struct {
	// BinaryPath, when set, overrides binary discovery. Wire this from
	// app_config.agent.runtime_path at startup.
	BinaryPath string

	// TranscriptDir is the root directory for NDJSON transcripts.
	// One file per run: <TranscriptDir>/<runID>.ndjson. Created if missing.
	TranscriptDir string

	mu sync.Mutex
}

// resolveBinary finds the sidecar binary in priority order:
//  1. s.BinaryPath
//  2. $BREADBOX_AGENT_BIN
//  3. ./bin/breadbox-agent (process cwd)
//  4. PATH lookup (`breadbox-agent`)
func (s *Sidecar) resolveBinary() (string, error) {
	if s.BinaryPath != "" {
		return s.BinaryPath, nil
	}
	if v := os.Getenv("BREADBOX_AGENT_BIN"); v != "" {
		return v, nil
	}
	local := filepath.Join("bin", "breadbox-agent")
	if _, err := os.Stat(local); err == nil {
		abs, err := filepath.Abs(local)
		if err == nil {
			return abs, nil
		}
		return local, nil
	}
	p, err := exec.LookPath("breadbox-agent")
	if err != nil {
		return "", ErrBinaryNotFound
	}
	return p, nil
}

// Run implements Runner. Spawns the sidecar, pipes spec to stdin, parses
// NDJSON events from stdout (forwarding each to handler and to the
// transcript file on disk), and returns a populated RunResult on exit.
//
// Always returns a non-nil RunResult. The returned error mirrors result.Err
// when the run failed; callers can prefer one or the other.
func (s *Sidecar) Run(ctx context.Context, spec JobSpec, handler EventHandler) (RunResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	start := time.Now()
	result := RunResult{Status: StatusError}

	bin, err := s.resolveBinary()
	if err != nil {
		result.Err = err
		result.DurationMs = time.Since(start).Milliseconds()
		return result, err
	}

	// Open the transcript file. If TranscriptDir is empty, skip the file
	// and let the sidecar stream-only.
	var transcript *os.File
	if s.TranscriptDir != "" && spec.RunID != "" {
		if err := os.MkdirAll(s.TranscriptDir, 0o755); err != nil {
			result.Err = fmt.Errorf("agent: mkdir transcript dir: %w", err)
			result.DurationMs = time.Since(start).Milliseconds()
			return result, result.Err
		}
		p := filepath.Join(s.TranscriptDir, spec.RunID+".ndjson")
		f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			result.Err = fmt.Errorf("agent: open transcript file: %w", err)
			result.DurationMs = time.Since(start).Milliseconds()
			return result, result.Err
		}
		transcript = f
		result.TranscriptPath = p
		// Also tell the sidecar so it can flush on crash even if our parser dies.
		spec.TranscriptPath = p
		defer transcript.Close()
	}

	specJSON, err := json.Marshal(spec)
	if err != nil {
		result.Err = fmt.Errorf("agent: marshal spec: %w", err)
		result.DurationMs = time.Since(start).Milliseconds()
		return result, result.Err
	}

	cmd := exec.CommandContext(ctx, bin)
	cmd.Stdin = bytes.NewReader(specJSON)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		result.Err = fmt.Errorf("agent: stdout pipe: %w", err)
		result.DurationMs = time.Since(start).Milliseconds()
		return result, result.Err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		result.Err = fmt.Errorf("agent: start sidecar: %w", err)
		result.DurationMs = time.Since(start).Milliseconds()
		return result, result.Err
	}

	var (
		lastResult ResultPayload
		gotResult  bool
		lastError  ErrorPayload
		gotError   bool
		handlerErr error
	)

	scanner := bufio.NewScanner(stdout)
	// SDK assistant messages can be large; bump the scanner buffer to 1 MiB.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		if transcript != nil {
			_, _ = transcript.Write(line)
			_, _ = transcript.Write([]byte{'\n'})
		}
		evt, err := ParseEvent(line)
		if err != nil {
			// Malformed line: log via stderr buffer concept but don't crash.
			// (Truly unrecoverable parse failures are surfaced via the final RunResult.)
			continue
		}
		switch evt.Type {
		case EventTypeResult:
			if p, perr := evt.ParseResult(); perr == nil {
				lastResult = p
				gotResult = true
			}
		case EventTypeError, EventTypeCostCapHit:
			if p, perr := evt.ParseError(); perr == nil {
				lastError = p
				gotError = true
			}
		}
		if handler != nil {
			if herr := handler(evt); herr != nil {
				handlerErr = herr
				_ = cmd.Process.Kill()
				// Drain remaining output silently.
				_, _ = io.Copy(io.Discard, stdout)
				break
			}
		}
	}

	// Scanner-level error (e.g. token-too-long); recorded but processed-after-wait.
	scanErr := scanner.Err()

	waitErr := cmd.Wait()

	result.DurationMs = time.Since(start).Milliseconds()

	// Populate cost/token fields from the final result event if we saw one.
	if gotResult {
		result.TotalCostUSD = lastResult.TotalCostUSD
		result.InputTokens = lastResult.InputTokens
		result.OutputTokens = lastResult.OutputTokens
		result.CacheReadTokens = lastResult.CacheReadTokens
		result.CacheCreationTokens = lastResult.CacheCreationTokens
		result.TurnCount = lastResult.TurnCount
		result.NumToolCalls = lastResult.NumToolCalls
		result.SessionID = lastResult.SessionID
	}

	switch {
	case handlerErr != nil:
		result.Status = StatusError
		result.Err = handlerErr
	case ctx.Err() == context.DeadlineExceeded:
		result.Status = StatusTimeout
		result.Err = ctx.Err()
	case ctx.Err() != nil:
		result.Status = StatusError
		result.Err = ctx.Err()
	case gotError:
		result.Status = StatusError
		result.Err = errors.New(lastError.Message)
	case waitErr != nil:
		result.Status = StatusError
		// Surface stderr if present to make sidecar crashes debuggable.
		if stderr.Len() > 0 {
			result.Err = fmt.Errorf("%w: exit=%v stderr=%s", ErrSidecarFailed, waitErr, stderr.String())
		} else {
			result.Err = fmt.Errorf("%w: %v", ErrSidecarFailed, waitErr)
		}
	case scanErr != nil:
		result.Status = StatusError
		result.Err = fmt.Errorf("agent: scanner error: %w", scanErr)
	case gotResult && lastResult.StopReason == "budget_exceeded":
		result.Status = StatusError
		result.Err = ErrBudgetExceeded
	case gotResult && lastResult.StopReason == "max_turns":
		// max_turns reached is success-ish: the SDK terminated cleanly within bounds.
		// Distinguish in the result so callers can flag it for the operator.
		result.Status = StatusSuccess
		result.Err = ErrMaxTurnsReached
	case gotResult:
		result.Status = StatusSuccess
	default:
		// Sidecar exited cleanly but never emitted a "result" event.
		// Treat as success with zeroed metrics; the orchestrator can flag.
		result.Status = StatusSuccess
	}

	return result, result.Err
}

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
	"strings"
	"syscall"
	"time"
)

// Sidecar locates and exec's the breadbox-agent binary. Implements Runner.
//
// Concurrency: the orchestrator (internal/service/agent_orchestrator.go)
// owns the server-wide concurrency cap via its semaphore. Sidecar itself
// is stateless and safe to call from multiple goroutines — earlier
// iterations held an internal mutex "as a safety net" but that mutex
// silently capped real concurrency at 1, contradicting the operator-
// configurable `agent.max_concurrent` setting.
type Sidecar struct {
	// BinaryPath, when set, overrides binary discovery. Wire this from
	// app_config.agent.runtime_path at startup.
	BinaryPath string

	// TranscriptDir is the root directory for NDJSON transcripts.
	// One file per run: <TranscriptDir>/<runID>.ndjson. Created if missing.
	TranscriptDir string
}

// resolveBinary finds the sidecar binary via the shared LocateBinary helper.
func (s *Sidecar) resolveBinary() (string, error) {
	return LocateBinary(s.BinaryPath)
}

// LocateBinary finds the breadbox-agent sidecar binary using the same
// priority order Sidecar.Run uses at exec time. Exported so the `breadbox
// doctor` check + the admin Settings page can share the discovery
// semantics.
//
//	1. explicit path (e.g. app_config.agent.runtime_path)
//	2. $BREADBOX_AGENT_BIN
//	3. ./bin/breadbox-agent (process cwd; dev `make agent-sidecar` output)
//	4. ~/.breadbox/agent-bin/breadbox-agent (per-user release install)
//	5. PATH lookup (`breadbox-agent`; Docker image + install scripts)
//
// Returns ErrBinaryNotFound when none of the above hit.
func LocateBinary(explicitPath string) (string, error) {
	if explicitPath != "" {
		return explicitPath, nil
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
	if home, err := os.UserHomeDir(); err == nil {
		userInstall := filepath.Join(home, ".breadbox", "agent-bin", "breadbox-agent")
		if _, err := os.Stat(userInstall); err == nil {
			return userInstall, nil
		}
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
		// Sidecar.Run is the single source of truth for the transcript
		// path. Built from (TranscriptDir, spec.RunID — the UUID, not
		// the short_id) so it's stable across the run's lifecycle. The
		// result.TranscriptPath we surface back to the orchestrator and
		// the spec.TranscriptPath we forward to the TS sidecar process
		// both reference this exact `p` — DO NOT set spec.TranscriptPath
		// elsewhere (it gets clobbered here just before json.Marshal).
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
	// Auth flows via env vars set HERE, not via the JSON spec (Token is
	// `json:"-"` for exactly this reason — keep plaintext secrets out of the
	// wire format and out of any future log capture of the spec). The Go side
	// is the single source of truth for which env var is set; the sidecar
	// process inherits exactly one of {ANTHROPIC_API_KEY, CLAUDE_CODE_OAUTH_TOKEN}.
	cmd.Env = authEnvFor(os.Environ(), spec.Auth)

	// Process-group lifecycle. The sidecar (a bun-compiled binary) spawns
	// the SDK's bundled Node child for cli.js. With the stdlib default
	// (Process.Kill on ctx cancel), only the bun parent receives SIGKILL —
	// the Node child becomes orphaned to init and keeps running, finishing
	// MCP work after the orchestrator has already moved on (and after the
	// per-run API key may already be revoked). See incident: run RK7U4E06,
	// 2026-05-25.
	//
	// Setpgid puts the sidecar in its own group; cmd.Cancel + cmd.WaitDelay
	// (Go 1.20+) replace the default Kill-the-parent with a group-targeted
	// SIGKILL that takes down the Node grandchild too.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		// Kill the entire process group (`pgid == cmd.Process.Pid` because
		// Setpgid made the sidecar its own group leader). SIGKILL rather
		// than SIGTERM: the SDK doesn't have meaningful cleanup work to
		// honor here, and the rest of the runtime is already past the
		// point of caring (ctx canceled means we're tearing the run down).
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	// WaitDelay bounds how long Wait blocks after Cancel runs. Without it
	// a child that swallowed its pipe close could keep us waiting forever.
	cmd.WaitDelay = 5 * time.Second

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
				// Kill the whole group, not just the parent — same reasoning
				// as cmd.Cancel above. The handler-error path is rare (only
				// fires if the caller's event handler returns non-nil) but
				// shares the orphan-grandchild risk.
				if cmd.Process != nil {
					_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				}
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

// authEnvFor returns a copy of `inherited` with both Anthropic auth env vars
// stripped and exactly one re-added based on auth.Mode. Centralizes the
// "exactly one auth var, never both" rule on the Go side so a future
// sidecar regression (or a different sidecar runtime entirely) can't expose
// the precedence trap.
//
// auth.Token may be empty (e.g. unit tests, smoke before auth configured) —
// in that case neither var is set and the SDK will fail with its own clear
// "no auth" error rather than us silently sneaking in an inherited value.
func authEnvFor(inherited []string, auth AuthConfig) []string {
	out := make([]string, 0, len(inherited)+1)
	for _, e := range inherited {
		if strings.HasPrefix(e, "ANTHROPIC_API_KEY=") || strings.HasPrefix(e, "CLAUDE_CODE_OAUTH_TOKEN=") {
			continue
		}
		out = append(out, e)
	}
	if auth.Token == "" {
		return out
	}
	switch auth.Mode {
	case "api_key":
		out = append(out, "ANTHROPIC_API_KEY="+auth.Token)
	case "subscription":
		out = append(out, "CLAUDE_CODE_OAUTH_TOKEN="+auth.Token)
	}
	return out
}

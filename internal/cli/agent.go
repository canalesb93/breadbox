//go:build !lite

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"breadbox/internal/agent"
	"breadbox/internal/app"
	"breadbox/internal/appconfig"
	"breadbox/internal/config"
	"breadbox/internal/service"

	"github.com/spf13/cobra"
)

// silentError wraps an error so cobra won't print the "Error:" prefix
// after our handler has already emitted a user-friendly message. The
// underlying error still flows out for MapExitCode to inspect.
type silentError struct{ err error }

func (s *silentError) Error() string { return s.err.Error() }
func (s *silentError) Unwrap() error { return s.err }
func silentlyFail(err error) error   { return &silentError{err: err} }

// AddAgentCmd registers `breadbox agent <subcommand>` — the local-scope
// parent for agent-subsystem diagnostics.
func AddAgentCmd(root *cobra.Command) {
	parent := &cobra.Command{
		Use:   "agent",
		Short: "Diagnose and test the Claude Agent SDK subsystem",
	}

	test := &cobra.Command{
		Use:   "test",
		Short: "End-to-end smoke test of the agent sidecar + auth + binary discovery",
		Long: `Spawn the breadbox-agent sidecar with a tiny "say OK" prompt to verify
the full chain works:

  - Anthropic credential is configured in app_config
  - breadbox-agent binary is discoverable
  - sidecar can spawn and reach the SDK
  - SDK can reach Anthropic and produce a response

No agent definition is registered, no MCP servers attached, no
agent_runs row written. Cost is bounded to ~5¢ via the diagnostic
budget cap.

Exit codes:
  0  test succeeded
  3  no Anthropic credential configured
  5  agent binary not found
  1  test ran but model returned an error / sidecar crashed
`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentTest(cmd.Context())
		},
	}
	parent.AddCommand(test)

	run := &cobra.Command{
		Use:   "run <slug>",
		Short: "Trigger an immediate run of a named agent",
		Long: `Run one agent end-to-end against the live system: mints a scoped API
key, spawns the sidecar with the definition's prompt + schedule, calls
the MCP server, persists the resulting agent_runs row, revokes the key.

Equivalent to clicking "Run now" in the v2 SPA — useful for cron/shell
automation or local debugging.

Exit codes:
  0  run completed (status may be success or error — see status field)
  3  no Anthropic credential configured
  5  agent slug not found / agent binary not found
  1  runner crashed or model returned an unexpected error
`,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Flags().GetBool("json")
			prefix, _ := cmd.Flags().GetString("prefix")
			return runAgentRun(cmd.Context(), args[0], jsonOut, prefix)
		},
	}
	run.Flags().Bool("json", false, "emit the run result as JSON instead of human-readable")
	run.Flags().String("prefix", "", "optional operator prompt prefix to prepend for this run only")
	parent.AddCommand(run)

	list := &cobra.Command{
		Use:   "list",
		Short: "List configured agents",
		Long: `Print every agent definition with status, schedule, model, and
last-run info. Useful for SSH-based ops without the v2 SPA.

  breadbox agent list           # human-readable table
  breadbox agent list --json    # full JSON for piping into jq

Exit codes:
  0  success
  1  failed to load config or query DB
`,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Flags().GetBool("json")
			return runAgentList(cmd.Context(), jsonOut)
		},
	}
	list.Flags().Bool("json", false, "emit the full agent definitions as JSON instead of a table")
	parent.AddCommand(list)

	root.AddCommand(parent)
}

func runAgentRun(parent context.Context, slug string, jsonOut bool, promptPrefix string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	a, err := app.New(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer a.DB.Close()

	def, err := a.Service.GetAgentDefinition(ctx, slug)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "agent %q not found\n", slug)
			return silentlyFail(agent.ErrBinaryNotFound) // map to ExitValidation (5)
		}
		return fmt.Errorf("resolve agent: %w", err)
	}

	// Build a one-shot orchestrator with concurrency=1 (CLI is a single
	// process; the in-memory semaphore is just belt-and-suspenders).
	sidecar := &agent.Sidecar{
		BinaryPath:    appconfig.String(ctx, a.Queries, appconfig.KeyAgentRuntimePath, ""),
		TranscriptDir: appconfig.String(ctx, a.Queries, appconfig.KeyAgentTranscriptDir, "transcripts/agents"),
	}
	orch := service.NewOrchestrator(a.Service, sidecar, 1, cfg.EncryptionKey, logger)

	if !jsonOut {
		fmt.Fprintf(os.Stdout, "▶  Running %s (%s)…\n", def.Name, def.Slug)
	}
	runResp, runErr := orch.RunNow(ctx, def, promptPrefix)
	if runErr != nil {
		switch {
		case errors.Is(runErr, agent.ErrAuthNotConfigured):
			fmt.Fprintln(os.Stderr, "auth not configured — paste a token in Settings → Agents or run `breadbox agent test` for diagnostic detail")
			return silentlyFail(runErr)
		case errors.Is(runErr, agent.ErrBinaryNotFound):
			fmt.Fprintln(os.Stderr, "breadbox-agent binary not found — download breadbox-agent-<os>-<arch> from the latest GitHub release and place it on your PATH or at ~/.breadbox/agent-bin/, set agent.runtime_path, or build from source via `make agent-sidecar`")
			return silentlyFail(runErr)
		case errors.Is(runErr, agent.ErrConcurrencyLocked):
			// Shouldn't happen with a fresh per-CLI orchestrator, but
			// surface clearly if it does.
			fmt.Fprintln(os.Stderr, "another agent run is in progress on this server — retry shortly")
			return silentlyFail(runErr)
		}
		// Runner errored but the row was still written; print the result
		// then bubble the error for non-zero exit.
		if runResp != nil {
			printAgentRunResult(runResp, jsonOut)
		}
		fmt.Fprintf(os.Stderr, "\nrun failed: %v\n", runErr)
		return silentlyFail(runErr)
	}
	printAgentRunResult(runResp, jsonOut)
	return nil
}

func printAgentRunResult(r *service.AgentRunResponse, jsonOut bool) {
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(r)
		return
	}
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintf(os.Stdout, "  status     %s\n", r.Status)
	fmt.Fprintf(os.Stdout, "  trigger    %s\n", r.Trigger)
	fmt.Fprintf(os.Stdout, "  short_id   %s\n", r.ShortID)
	if r.DurationMs != nil {
		fmt.Fprintf(os.Stdout, "  duration   %dms\n", *r.DurationMs)
	}
	if r.TotalCostUSD != nil {
		fmt.Fprintf(os.Stdout, "  cost       $%.6f\n", *r.TotalCostUSD)
	}
	if r.TurnCount != nil {
		fmt.Fprintf(os.Stdout, "  turns      %d\n", *r.TurnCount)
	}
	if r.NumToolCalls != nil {
		fmt.Fprintf(os.Stdout, "  tool calls %d\n", *r.NumToolCalls)
	}
	if r.ErrorMessage != nil {
		fmt.Fprintf(os.Stdout, "  error      %s\n", *r.ErrorMessage)
	}
	if r.TranscriptPath != nil {
		fmt.Fprintf(os.Stdout, "  transcript %s\n", *r.TranscriptPath)
	}
}

func runAgentTest(parent context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	a, err := app.New(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer a.DB.Close()

	binaryPath := appconfig.String(ctx, a.Queries, appconfig.KeyAgentRuntimePath, "")
	transcriptDir := appconfig.String(ctx, a.Queries, appconfig.KeyAgentTranscriptDir, "transcripts/agents")
	sidecar := &agent.Sidecar{
		BinaryPath:    binaryPath,
		TranscriptDir: transcriptDir,
	}

	fmt.Fprintln(os.Stdout, "🔎 breadbox agent test")
	fmt.Fprintln(os.Stdout, "")

	result, err := agent.SmokeTest(ctx, a.Queries, cfg.EncryptionKey, sidecar, binaryPath)
	if err != nil {
		switch {
		case errors.Is(err, agent.ErrAuthNotConfigured):
			fmt.Fprintln(os.Stdout, "  ✗ auth          not configured")
			fmt.Fprintln(os.Stdout, "")
			fmt.Fprintln(os.Stdout, "Open the v2 SPA → Settings → Agents and paste an Anthropic credential.")
			fmt.Fprintln(os.Stdout, "For a subscription token: run `claude setup-token` on any machine, then paste the sk-ant-oat01-… into Settings.")
			return silentlyFail(err)
		case errors.Is(err, agent.ErrBinaryNotFound):
			fmt.Fprintln(os.Stdout, "  ✗ binary        not found")
			fmt.Fprintln(os.Stdout, "")
			fmt.Fprintln(os.Stdout, "From a release: download `breadbox-agent-<os>-<arch>` from https://github.com/canalesb93/breadbox/releases/latest")
			fmt.Fprintln(os.Stdout, "  and place it on your PATH or at ~/.breadbox/agent-bin/breadbox-agent.")
			fmt.Fprintln(os.Stdout, "Docker users: the published image already includes it.")
			fmt.Fprintln(os.Stdout, "From source: `make agent-sidecar` (writes ./bin/breadbox-agent).")
			fmt.Fprintln(os.Stdout, "Or set an explicit path: `breadbox config set agent.runtime_path /path/to/breadbox-agent`.")
			return silentlyFail(err)
		default:
			fmt.Fprintln(os.Stdout, "  ✗ smoke run failed")
			fmt.Fprintf(os.Stdout, "\n%v\n", err)
			return silentlyFail(err)
		}
	}

	fmt.Fprintf(os.Stdout, "  ✓ auth          %s\n", result.AuthMode)
	if result.BinaryPath == "" {
		fmt.Fprintln(os.Stdout, "  ✓ binary        auto-discovered (./bin/breadbox-agent, ~/.breadbox/agent-bin/, or $PATH)")
	} else {
		fmt.Fprintf(os.Stdout, "  ✓ binary        %s\n", result.BinaryPath)
	}
	fmt.Fprintf(os.Stdout, "  ✓ model         %s\n", result.Model)
	fmt.Fprintf(os.Stdout, "  ✓ duration      %dms\n", result.DurationMs)
	fmt.Fprintf(os.Stdout, "  ✓ cost          $%.6f (%d in / %d out tokens)\n", result.TotalCostUSD, result.InputTokens, result.OutputTokens)
	if result.AssistantText != "" {
		fmt.Fprintf(os.Stdout, "  ✓ response      %q\n", result.AssistantText)
	}
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "Smoke test passed. The agent subsystem is ready to run real definitions.")
	return nil
}

func runAgentList(parent context.Context, jsonOut bool) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	a, err := app.New(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer a.DB.Close()

	defs, err := a.Service.ListAgentDefinitions(ctx)
	if err != nil {
		return fmt.Errorf("list agents: %w", err)
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(defs)
	}

	if len(defs) == 0 {
		fmt.Fprintln(os.Stdout, "No agents configured. The fresh-install seed should have added 5 starters — run `breadbox migrate` if you skipped it, or check the v2 SPA at /v2/agents to confirm.")
		return nil
	}

	// Compute column widths from data so the table breathes.
	slugW, nameW := len("slug"), len("name")
	for _, d := range defs {
		if len(d.Slug) > slugW {
			slugW = len(d.Slug)
		}
		if len(d.Name) > nameW {
			nameW = len(d.Name)
		}
	}

	fmt.Fprintf(os.Stdout, "  %-*s  %-*s  %-9s  %-22s  %-21s  %s\n",
		slugW, "slug", nameW, "name", "enabled", "model", "schedule", "next fire")
	fmt.Fprintf(os.Stdout, "  %s\n",
		strings.Repeat("-", slugW+nameW+9+22+21+25+12))
	for _, d := range defs {
		schedule := "manual"
		if d.ScheduleCron != nil && *d.ScheduleCron != "" {
			schedule = *d.ScheduleCron
		}
		enabled := "no"
		if d.Enabled {
			enabled = "yes"
		}
		nextFire := "—"
		if d.NextFireAt != nil {
			nextFire = *d.NextFireAt
		}
		fmt.Fprintf(os.Stdout, "  %-*s  %-*s  %-9s  %-22s  %-21s  %s\n",
			slugW, d.Slug, nameW, d.Name, enabled, d.Model, schedule, nextFire)
	}
	fmt.Fprintf(os.Stdout, "\n%d agent(s). Tip: `breadbox agent run <slug>` to fire one now.\n", len(defs))
	return nil
}


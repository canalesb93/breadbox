//go:build !lite

package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"breadbox/internal/app"
	"breadbox/internal/config"
	"breadbox/internal/db"
	breadboxmcp "breadbox/internal/mcp"
	"breadbox/internal/service"

	"github.com/jackc/pgx/v5"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

// AddMCPCmd registers `breadbox mcp` (the cobra-friendly rename of the
// legacy `mcp-stdio`). The old name is registered as a hidden alias so
// existing Claude Desktop configs that spawn `breadbox mcp-stdio` keep
// working — a one-line deprecation notice goes to stderr.
func AddMCPCmd(root *cobra.Command) {
	mcp := &cobra.Command{
		Use:     "mcp",
		Short:   "Run the MCP server over stdio (for Claude Desktop and friends)",
		Aliases: []string{},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPStdio(cmd.Context(), Flags(cmd).Version)
		},
	}
	root.AddCommand(mcp)

	// Hidden back-compat: `breadbox mcp-stdio` keeps working but warns to
	// stderr so users can migrate without surprise.
	legacy := &cobra.Command{
		Use:    "mcp-stdio",
		Short:  "Deprecated alias for `breadbox mcp`",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(os.Stderr, "warning: `mcp-stdio` is deprecated; use `breadbox mcp` instead")
			return runMCPStdio(cmd.Context(), Flags(cmd).Version)
		},
	}
	root.AddCommand(legacy)
}

// runMCPStdio launches the stdio MCP server. Process-start attaches
// the "Local MCP" fallback agent identity to the root ctx so any code
// path that runs before the MCP `initialize` handshake completes (or
// comes from a client that omits clientInfo entirely) has a valid
// actor. The dispatcher (`MCPServer.makeToolDefLogged`) upgrades the
// ctx to a per-client agent identity on each tool call once clientInfo
// is available, so tool-call-level attribution stays sharp without
// touching this bootstrap.
//
// The fallback row is the relabelled stdio singleton (see migration
// 20260526092106_api_keys_client_fingerprint.sql) — same UUID, same
// key_prefix as the pre-PR singleton, now with actor_type='agent',
// actor_name='Local MCP', and client_fingerprint='unknown@@stdio'.
// Pre-PR annotations that reference that UUID re-render correctly
// through the agent branch without any data backfill.
func runMCPStdio(parent context.Context, version string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	a, err := app.New(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer a.DB.Close()

	// Attach the Local MCP fallback identity. The dispatcher swaps in
	// per-client keys once clientInfo arrives; this is just the safe
	// floor for anything that runs before then.
	fallbackKey, err := ensureLocalMCPFallbackKey(ctx, a.Queries)
	if err != nil {
		return fmt.Errorf("ensure local mcp fallback key: %w", err)
	}
	ctx = service.ContextWithAPIKey(ctx, fallbackKey)

	mcpServer := breadboxmcp.NewMCPServer(a.Service, version)

	mcpCfg, err := a.Service.GetMCPConfig(ctx)
	if err != nil {
		logger.Warn("failed to load MCP config, using defaults", "error", err)
		mcpCfg = &service.MCPConfig{
			Mode:          "read_write",
			DisabledTools: []string{},
		}
	}

	server := mcpServer.BuildServer(breadboxmcp.MCPServerConfig{
		Mode:          mcpCfg.Mode,
		DisabledTools: mcpCfg.DisabledTools,
		Instructions:  mcpCfg.Instructions,
		APIKeyScope:   "full_access", // stdio has no API key surface
	})

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	logger.Info("starting MCP stdio server", "version", version)
	return server.Run(ctx, &mcpsdk.StdioTransport{})
}

// stdioSystemKeyPrefix is the prefix of the legacy stdio singleton row,
// kept as a stable lookup handle. The migration relabels its actor
// fields in place; new installs (no migration history to relabel)
// create the row fresh below in the agent-typed shape.
const stdioSystemKeyPrefix = "bb_stdio_singleton"

// ensureLocalMCPFallbackKey returns the "Local MCP" fallback agent
// identity row that gets attached to the root ctx at process start.
// Used by anything that runs before the MCP `initialize` handshake
// completes — the dispatcher upgrades to a per-client agent identity
// per tool call once clientInfo is available.
//
// On an existing install the migration has already relabelled the
// stdio singleton to actor_type='agent' / actor_name='Local MCP' /
// client_fingerprint='unknown@@stdio'; this helper just looks it up.
// On a fresh install with no prior stdio history the row doesn't
// exist yet, so we create it in the agent-typed shape directly.
func ensureLocalMCPFallbackKey(ctx context.Context, q *db.Queries) (*db.ApiKey, error) {
	key, err := q.GetApiKeyByPrefix(ctx, stdioSystemKeyPrefix)
	if err == nil {
		return &key, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	row, err := q.CreateMCPClientApiKey(ctx, db.CreateMCPClientApiKeyParams{
		Name:              "mcp-client:" + service.MCPClientFallbackFingerprint,
		KeyHash:           "mcp-client-not-a-real-credential:" + service.MCPClientFallbackFingerprint,
		KeyPrefix:         stdioSystemKeyPrefix,
		ActorName:         pgText(service.MCPClientFallbackActorName),
		ClientFingerprint: pgText(service.MCPClientFallbackFingerprint),
	})
	if err != nil {
		return nil, err
	}
	return &row, nil
}


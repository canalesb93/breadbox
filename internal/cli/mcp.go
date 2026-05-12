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

// runMCPStdio launches the stdio MCP server. The body is the same as
// cmd/breadbox/main.go::runMCPStdio with one change: instead of fabricating
// an in-memory `agent` context, it ensures a singleton system-actor row
// exists in api_keys and attaches that to the context. The new check
// constraint on api_keys.actor_type rejects a synthetic id of "stdio"
// because no row matches it; the singleton key gives every stdio audit
// row a real `system` actor to point at.
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

	// Ensure a system-actor API key row exists so ContextWithAPIKey can
	// point at a real `actor_type='system'` record. The plaintext is never
	// exposed externally — this row is purely a DB-side anchor for the
	// audit trail.
	systemKey, err := ensureStdioSystemKey(ctx, a.Queries)
	if err != nil {
		return fmt.Errorf("ensure stdio system key: %w", err)
	}
	ctx = service.ContextWithAPIKey(ctx, systemKey)

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

// stdioSystemKeyPrefix is the unique prefix of the singleton stdio key
// row. It's looked up by prefix on each startup; missing rows are
// inserted with actor_type='system', actor_name='stdio'.
const stdioSystemKeyPrefix = "bb_stdio_singleton"

// ensureStdioSystemKey looks up (or creates) the stdio singleton api_keys
// row that ContextWithAPIKey attaches to all stdio MCP calls. The
// returned db.ApiKey lets the actor be attributed as system/stdio in the
// audit log.
func ensureStdioSystemKey(ctx context.Context, q *db.Queries) (*db.ApiKey, error) {
	key, err := q.GetApiKeyByPrefix(ctx, stdioSystemKeyPrefix)
	if err == nil {
		return &key, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	// The hash is a fixed sentinel — the row is never used as a valid
	// credential at the HTTP layer (no client can present a "key" whose
	// SHA-256 equals this literal string). Its only job is to be a
	// stable api_keys row for ContextWithAPIKey to attach to.
	row, err := q.CreateApiKey(ctx, db.CreateApiKeyParams{
		Name:      "MCP Stdio",
		KeyHash:   "stdio-singleton-not-a-real-credential",
		KeyPrefix: stdioSystemKeyPrefix,
		Scope:     "full_access",
		ActorType: "system",
		ActorName: pgText("stdio"),
	})
	if err != nil {
		return nil, err
	}
	return &row, nil
}


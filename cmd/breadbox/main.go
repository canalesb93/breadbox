package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"breadbox/internal/api"
	"breadbox/internal/app"
	"breadbox/internal/config"
	"breadbox/internal/db"
	breadboxmcp "breadbox/internal/mcp"
	"breadbox/internal/seed"
	"breadbox/internal/service"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: breadbox <command>")
		fmt.Fprintln(os.Stderr, "commands: serve, migrate, seed, mcp-stdio, api-keys, version")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		if err := runServe(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "migrate":
		if err := runMigrate(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "seed":
		if err := runSeed(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "mcp-stdio":
		if err := runMCPStdio(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "api-keys":
		if err := runAPIKeys(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "version":
		fmt.Println(version)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func newLogger(env string) *slog.Logger {
	var handler slog.Handler
	if env == "docker" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	}
	return slog.New(handler)
}

func runServe() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := newLogger(cfg.Environment)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a, err := app.New(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer a.DB.Close()

	// Merge app_config table values into config.
	if err := config.LoadWithDB(ctx, cfg, a.DB); err != nil {
		logger.Warn("failed to load app_config", "error", err)
	}

	router := api.NewRouter(a, version)

	srv := &http.Server{
		Addr:    ":" + cfg.ServerPort,
		Handler: router,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("received signal, shutting down", "signal", sig)

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("http server shutdown error", "error", err)
		}
		cancel()
	}()

	logger.Info("starting server", "addr", srv.Addr, "version", version)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}

	return nil
}

func runMigrate() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := newLogger(cfg.Environment)

	if cfg.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required for migrations")
	}

	goose.SetBaseFS(db.Migrations)

	sqlDB, err := goose.OpenDBWithDriver("pgx", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer sqlDB.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}

	if err := goose.Up(sqlDB, "migrations"); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	logger.Info("migrations applied successfully")
	return nil
}

func runMCPStdio() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Log to stderr so stdout is reserved for MCP JSON-RPC.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a, err := app.New(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer a.DB.Close()

	mcpServer := breadboxmcp.NewMCPServer(a.Service, version)

	// Graceful shutdown on SIGINT/SIGTERM.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	logger.Info("starting MCP stdio server", "version", version)
	return mcpServer.Server().Run(ctx, &mcpsdk.StdioTransport{})
}

func runAPIKeys() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	queries := db.New(pool)
	svc := service.New(queries, pool, nil, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	// Sub-action: "create <name>" or default to "list"
	action := "list"
	if len(os.Args) > 2 {
		action = os.Args[2]
	}

	switch action {
	case "list":
		keys, err := svc.ListAPIKeys(ctx)
		if err != nil {
			return fmt.Errorf("list api keys: %w", err)
		}
		if len(keys) == 0 {
			fmt.Println("No API keys found. Create one with: breadbox api-keys create <name>")
			return nil
		}
		fmt.Printf("%-38s  %-20s  %-12s  %-10s  %s\n", "ID", "NAME", "PREFIX", "STATUS", "LAST USED")
		for _, k := range keys {
			status := "active"
			if k.RevokedAt != nil {
				status = "revoked"
			}
			lastUsed := "never"
			if k.LastUsedAt != nil {
				lastUsed = *k.LastUsedAt
			}
			fmt.Printf("%-38s  %-20s  %-12s  %-10s  %s\n", k.ID, k.Name, k.KeyPrefix+"...", status, lastUsed)
		}

	case "create":
		name := "cli"
		if len(os.Args) > 3 {
			name = os.Args[3]
		}
		result, err := svc.CreateAPIKey(ctx, name)
		if err != nil {
			return fmt.Errorf("create api key: %w", err)
		}
		fmt.Printf("Created API key: %s\n", result.Name)
		fmt.Printf("Key: %s\n", result.PlaintextKey)
		fmt.Println("\nSave this key now — it cannot be retrieved again.")

	default:
		return fmt.Errorf("unknown api-keys action: %s (use 'list' or 'create <name>')", action)
	}

	return nil
}

func runSeed() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := newLogger(cfg.Environment)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	return seed.Run(ctx, pool, logger)
}

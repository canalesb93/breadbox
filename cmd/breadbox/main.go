package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"breadbox/internal/api"
	"breadbox/internal/app"
	"breadbox/internal/config"
	"breadbox/internal/db"
	breadboxmcp "breadbox/internal/mcp"
	"breadbox/internal/seed"
	"breadbox/internal/service"
	"breadbox/internal/sync"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"golang.org/x/crypto/bcrypt"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: breadbox <command>")
		fmt.Fprintln(os.Stderr, "commands: serve, migrate, seed, mcp-stdio, api-keys, create-admin, reset-password, version")
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
	case "create-admin":
		if err := runCreateAdmin(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "reset-password":
		if err := runResetPassword(); err != nil {
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

func newLogger(cfg *config.Config) *slog.Logger {
	level := slog.LevelInfo
	if cfg.Environment != "docker" {
		level = slog.LevelDebug
	}

	// LOG_LEVEL overrides environment-based default
	if cfg.LogLevel != "" {
		switch strings.ToLower(cfg.LogLevel) {
		case "debug":
			level = slog.LevelDebug
		case "info":
			level = slog.LevelInfo
		case "warn":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		default:
			// Will log warning after logger is created
		}
	}

	var handler slog.Handler
	if cfg.Environment == "docker" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	}
	logger := slog.New(handler)

	if cfg.LogLevel != "" {
		valid := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
		if !valid[strings.ToLower(cfg.LogLevel)] {
			logger.Warn("invalid LOG_LEVEL value, using default", "log_level", cfg.LogLevel, "default", level.String())
		}
	}

	return logger
}

func runServe() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	cfg.Version = version
	cfg.StartTime = time.Now()

	logger := newLogger(cfg)

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

	// Validate ENCRYPTION_KEY when bank providers are configured.
	if cfg.EncryptionKey == nil && (cfg.PlaidClientID != "" || cfg.TellerAppID != "") {
		return fmt.Errorf("ENCRYPTION_KEY is required when Plaid or Teller providers are configured. Generate one with: openssl rand -hex 32")
	}

	// Clean up orphaned sync logs from previous crashes.
	result, err := a.Queries.CleanupOrphanedSyncLogs(ctx)
	if err != nil {
		logger.Warn("failed to clean up orphaned sync logs", "error", err)
	} else if n := result.RowsAffected(); n > 0 {
		logger.Info("cleaned up orphaned sync logs", "count", n)
	}

	// Create and start the cron scheduler.
	syncTimeout := time.Duration(cfg.SyncTimeoutSeconds) * time.Second
	scheduler := sync.NewScheduler(a.SyncEngine, a.Queries, logger, syncTimeout)
	scheduler.Start(cfg.SyncIntervalMinutes)
	a.Scheduler = scheduler

	// Run startup sync for stale connections in background.
	go scheduler.RunStartupSync(ctx, cfg.SyncIntervalMinutes)

	router := api.NewRouter(a, version)

	srv := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      router,
		ReadTimeout:  time.Duration(cfg.ReadTimeoutS) * time.Second,
		WriteTimeout: time.Duration(cfg.WriteTimeoutS) * time.Second,
		IdleTimeout:  time.Duration(cfg.IdleTimeoutS) * time.Second,
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
		scheduler.Stop()
		cancel()
	}()

	// Startup banner.
	webhookStatus := "disabled"
	if cfg.WebhookURL != "" {
		webhookStatus = cfg.WebhookURL
	}
	plaidStatus := "not configured"
	if cfg.PlaidClientID != "" {
		plaidStatus = cfg.PlaidEnv
	}
	tellerStatus := "not configured"
	if cfg.TellerAppID != "" {
		tellerStatus = fmt.Sprintf("configured (%s)", cfg.TellerEnv)
	}
	encryptionStatus := "configured"
	if cfg.EncryptionKey == nil {
		encryptionStatus = "NOT SET"
	}
	adminStatus := "none"
	adminCount, err := a.Queries.CountAdminAccounts(ctx)
	if err != nil {
		logger.Warn("failed to check admin accounts", "error", err)
	} else if adminCount > 0 {
		adminStatus = "exists"
	}
	logger.Info("breadbox starting",
		"version", version,
		"addr", srv.Addr,
		"environment", cfg.Environment,
		"plaid", plaidStatus,
		"teller", tellerStatus,
		"encryption_key", encryptionStatus,
		"admin", adminStatus,
		"sync_interval", fmt.Sprintf("%dm", cfg.SyncIntervalMinutes),
		"webhook", webhookStatus,
		"db_pool", fmt.Sprintf("max=%d min=%d lifetime=%dm", cfg.DBMaxConns, cfg.DBMinConns, cfg.DBMaxConnLifetimeM),
	)
	if cfg.EncryptionKey == nil {
		logger.Warn("ENCRYPTION_KEY not set — encrypted provider credentials will not work")
	}
	if adminCount == 0 {
		logger.Warn("no admin account — create one at /admin/setup or via 'breadbox create-admin'")
	}
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

	logger := newLogger(cfg)

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

func runResetPassword() error {
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

	// Check if any admin account exists.
	count, err := queries.CountAdminAccounts(ctx)
	if err != nil {
		return fmt.Errorf("check admin accounts: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("no admin account found — run the setup wizard first")
	}

	// Get the first admin account.
	var adminID pgtype.UUID
	var adminUsername string
	row := pool.QueryRow(ctx, "SELECT id, username FROM admin_accounts ORDER BY created_at LIMIT 1")
	if err := row.Scan(&adminID, &adminUsername); err != nil {
		return fmt.Errorf("get admin account: %w", err)
	}

	// Get password from --password flag or prompt.
	var password string
	if len(os.Args) > 2 && os.Args[2] == "--password" && len(os.Args) > 3 {
		password = os.Args[3]
	} else {
		fmt.Printf("Resetting password for admin: %s\n", adminUsername)
		fmt.Print("New password (min 8 characters): ")
		var input string
		fmt.Scanln(&input)
		password = input

		fmt.Print("Confirm password: ")
		var confirm string
		fmt.Scanln(&confirm)
		if password != confirm {
			return fmt.Errorf("passwords do not match")
		}
	}

	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	// Hash password with bcrypt.
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	// Update password.
	if err := queries.UpdateAdminPassword(ctx, db.UpdateAdminPasswordParams{
		ID:             adminID,
		HashedPassword: hashedPassword,
	}); err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	fmt.Printf("Password updated successfully for admin: %s\n", adminUsername)
	return nil
}

func runSeed() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := newLogger(cfg)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	return seed.Run(ctx, pool, logger)
}

//go:build !lite

package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"breadbox/internal/agent"
	"breadbox/internal/api"
	"breadbox/internal/app"
	"breadbox/internal/appconfig"
	"breadbox/internal/config"
	"breadbox/internal/db"
	"breadbox/internal/service"
	"breadbox/internal/sync"
	versionpkg "breadbox/internal/version"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/spf13/cobra"
)

// AddServeCmd registers `breadbox serve` on root. The command is L-scoped
// (talks to the local DB and embedded migrations) so it does not need a
// configured host.
func AddServeCmd(root *cobra.Command) {
	var noDashboard bool
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP server (REST + MCP-over-HTTP + dashboard)",
		Long: `Run the full breadbox server: REST API, MCP-over-HTTP, OAuth, hosted-link, ` +
			`webhooks, and the admin dashboard. Use --no-dashboard (or BREADBOX_NO_DASHBOARD=true) ` +
			`to disable the dashboard, v2 SPA, and /web/v1 routes while keeping the API up.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(cmd.Context(), Flags(cmd).Version, noDashboard)
		},
	}
	cmd.Flags().BoolVar(&noDashboard, "no-dashboard", os.Getenv("BREADBOX_NO_DASHBOARD") == "true", "disable the admin dashboard, v2 SPA, and /web/v1 routes (REST + MCP + OAuth stay up)")
	root.AddCommand(cmd)
}

// runServe is the same body as the old cmd/breadbox/main.go::runServe;
// only the entry shape changed. Inline comments are unchanged so future
// `git blame` traces back to the original implementation.
func runServe(_ context.Context, version string, noDashboardFlag bool) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	// CLI flag wins; env (BREADBOX_NO_DASHBOARD) is the default surfaced
	// via the flag's default value when AddServeCmd builds the cobra cmd.
	cfg.NoDashboard = cfg.NoDashboard || noDashboardFlag

	cfg.Version = version
	cfg.StartTime = time.Now()

	dockerSocketAvailable := false
	if _, err := os.Stat("/var/run/docker.sock"); err == nil {
		dockerSocketAvailable = true
	}

	logger := newLogger(cfg)

	if cfg.DatabaseURL != "" {
		logger.Info("running database migrations...")
		if err := migrateDB(cfg.DatabaseURL); err != nil {
			return fmt.Errorf("auto-migrate: %w", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a, err := app.New(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer a.DB.Close()

	a.VersionChecker = versionpkg.NewChecker(version, logger)
	a.DockerSocketAvailable = dockerSocketAvailable

	if err := config.LoadWithDB(ctx, cfg, a.DB); err != nil {
		logger.Warn("failed to load app_config", "error", err)
	}

	if a.Providers["plaid"] == nil && cfg.PlaidClientID != "" {
		if err := a.ReinitProvider("plaid"); err != nil {
			logger.Warn("failed to reinit plaid provider from db config", "error", err)
		}
	}
	if a.Providers["teller"] == nil && cfg.TellerAppID != "" {
		if err := a.ReinitProvider("teller"); err != nil {
			logger.Warn("failed to reinit teller provider from db config", "error", err)
		}
	}

	if cfg.EncryptionKey == nil && (cfg.PlaidClientID != "" || cfg.TellerAppID != "") {
		return fmt.Errorf("ENCRYPTION_KEY is required when Plaid or Teller providers are configured. Generate one with: openssl rand -hex 32")
	}

	result, err := a.Queries.CleanupOrphanedSyncLogs(ctx)
	if err != nil {
		logger.Warn("failed to clean up orphaned sync logs", "error", err)
	} else if n := result.RowsAffected(); n > 0 {
		logger.Info("cleaned up orphaned sync logs", "count", n)
	}

	syncTimeout := time.Duration(cfg.SyncTimeoutSeconds) * time.Second
	scheduler := sync.NewScheduler(a.SyncEngine, a.Queries, logger, syncTimeout)
	scheduler.Start(cfg.SyncIntervalMinutes)
	a.Scheduler = scheduler

	// Agent orchestrator + scheduler — drives Claude Agent SDK runs against
	// the MCP server. Both subscription-token and Anthropic API-key auth
	// modes are supported (see app_config agent.auth_mode).
	if cleanupResult, cerr := a.Queries.CleanupOrphanedAgentRuns(ctx); cerr != nil {
		logger.Warn("failed to clean up orphaned agent runs", "error", cerr)
	} else if n := cleanupResult.RowsAffected(); n > 0 {
		logger.Info("cleaned up orphaned agent runs", "count", n)
	}
	// Operators can raise (or lower) in Settings → Agents. The orchestrator's
	// mint-and-revoke + semaphore handle concurrent contention safely.
	agentMaxConcurrent := appconfig.Int(ctx, a.Queries, appconfig.KeyAgentMaxConcurrent, 3)
	agentRuntimePath := appconfig.String(ctx, a.Queries, appconfig.KeyAgentRuntimePath, "")
	// Default transcripts to ./transcripts/agents (relative to the cwd
	// `breadbox serve` was launched from). An empty value silently drops
	// transcripts — run rows end up with transcript_path="" and the v2 SPA's
	// "open transcript" 404s. Operators can override via Settings → Agents.
	agentTranscriptDir := appconfig.String(ctx, a.Queries, appconfig.KeyAgentTranscriptDir, "transcripts/agents")
	agentSidecar := &agent.Sidecar{
		BinaryPath:    agentRuntimePath,
		TranscriptDir: agentTranscriptDir,
	}
	agentOrch := service.NewOrchestrator(a.Service, agentSidecar, agentMaxConcurrent, cfg.EncryptionKey, logger)
	agentSched := service.NewAgentScheduler(agentOrch, a.Service, logger)
	agentOrch.AttachScheduler(agentSched)
	// Wire the post-sync hook so trigger_on_sync_complete agents fire
	// after every successful sync. The orchestrator dispatches asynchronously
	// so the sync engine returns immediately.
	a.SyncEngine.OnSyncComplete = func(ctx context.Context, _ pgtype.UUID) {
		agentOrch.FireSyncCompleteAgents(ctx)
	}
	if err := agent.SeedDefaults(ctx, a.Queries, logger); err != nil {
		logger.Warn("agent seed failed", "error", err)
	}
	agentSched.Start(ctx)
	a.AgentOrchestrator = agentOrch
	a.AgentScheduler = agentSched
	a.Service.OnDefinitionChanged = agentOrch.NotifyDefinitionChanged

	if _, err := exec.LookPath("pg_dump"); err == nil {
		backupDir := os.Getenv("BACKUP_DIR")
		if backupDir == "" {
			if cfg.Environment == "docker" {
				backupDir = "/var/lib/breadbox/backups"
			} else {
				backupDir = filepath.Join(".", "backups")
			}
		}
		bs := service.NewBackupService(cfg.DatabaseURL, backupDir, logger)
		a.BackupService = bs

		backupQueries := a.Queries
		if err := scheduler.AddFunc("0 * * * *", func() {
			runScheduledBackup(bs, backupQueries, logger)
		}); err != nil {
			logger.Error("failed to add backup cron job", "error", err)
		}

		logger.Info("backup service initialized", "backup_dir", backupDir)
	} else {
		logger.Warn("pg_dump not found — backup service disabled")
	}

	go scheduler.RunStartupSync(ctx, cfg.SyncIntervalMinutes)

	router := api.NewRouter(a, version)

	srv := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      router,
		ReadTimeout:  time.Duration(cfg.ReadTimeoutS) * time.Second,
		WriteTimeout: time.Duration(cfg.WriteTimeoutS) * time.Second,
		IdleTimeout:  time.Duration(cfg.IdleTimeoutS) * time.Second,
	}

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
	adminCount, err := a.Queries.CountAuthAccounts(ctx)
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
	if cfg.NoDashboard {
		logger.Info("dashboard disabled", "mode", "headless-runtime")
	} else if adminCount == 0 {
		logger.Warn("no admin account — create one at /setup or via 'breadbox create-admin'")
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}

	return nil
}

// runScheduledBackup is unchanged from cmd/breadbox/main.go.
func runScheduledBackup(bs *service.BackupService, queries *db.Queries, logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}) {
	ctx := context.Background()

	schedule := appconfig.String(ctx, queries, "backup_schedule", "")
	if schedule == "" {
		return
	}

	now := time.Now()
	hour := now.Hour()
	weekday := now.Weekday()

	shouldRun := false
	switch schedule {
	case "daily_2am":
		shouldRun = hour == 2
	case "daily_3am":
		shouldRun = hour == 3
	case "daily_4am":
		shouldRun = hour == 4
	case "weekly":
		shouldRun = hour == 2 && weekday == time.Sunday
	}

	if !shouldRun {
		return
	}

	logger.Info("scheduled backup starting", "schedule", schedule)

	filename, err := bs.CreateBackup(ctx, "scheduled")
	if err != nil {
		logger.Error("scheduled backup failed", "error", err)
		return
	}

	logger.Info("scheduled backup completed", "filename", filename)

	retentionDays := appconfig.Int(ctx, queries, "backup_retention_days", 7)
	if retentionDays <= 0 {
		retentionDays = 7
	}

	deleted, err := bs.CleanupOldBackups(retentionDays)
	if err != nil {
		logger.Error("backup cleanup failed", "error", err)
	} else if deleted > 0 {
		logger.Info("backup cleanup completed", "deleted", deleted, "retention_days", retentionDays)
	}
}

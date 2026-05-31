//go:build !lite

package cli

import (
	"fmt"

	"breadbox/internal/config"
	"breadbox/internal/db"

	"github.com/pressly/goose/v3"
	"github.com/spf13/cobra"
)

// AddMigrateCmd registers `breadbox migrate` (L-scoped: talks to the local
// DB directly via goose; there is no remote migration endpoint).
func AddMigrateCmd(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Apply pending database migrations (local-only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMigrate()
		},
	}
	root.AddCommand(cmd)
}

// runMigrate is the verbatim body of cmd/breadbox/main.go::runMigrate,
// rehoused so the cmd shim stays empty.
func runMigrate() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required for migrations")
	}
	if err := migrateDB(cfg.DatabaseURL); err != nil {
		return err
	}
	logger := newLogger(cfg)
	logger.Info("migrations applied successfully")
	return nil
}

// migrateDB runs goose migrations against the given database URL. Used by
// both runServe (auto-migrate) and runMigrate.
func migrateDB(databaseURL string) error {
	goose.SetBaseFS(db.Migrations)

	sqlDB, err := goose.OpenDBWithDriver("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer sqlDB.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}

	// WithAllowMissing: apply out-of-order ("missing") migrations instead of
	// aborting. Breadbox runs several feature sprints in parallel, each on its
	// own branch; two sprints can pick migration timestamps that interleave, so
	// after both merge a lower-timestamped migration can land *after* a
	// higher-timestamped one is already applied. Plain goose.Up treats that as
	// "found N missing migrations before current version" and refuses to run,
	// which crash-loops the app on deploy (the #1647 Workflows incident:
	// versions 20260531062826/070344/074239 were behind the already-applied
	// recurring_series_type 20260531074852). Migrations here are additive-only
	// (.claude/rules/migrations.md), so applying a straggler out of order is
	// safe — far better than wedging the deploy.
	if err := goose.Up(sqlDB, "migrations", goose.WithAllowMissing()); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}

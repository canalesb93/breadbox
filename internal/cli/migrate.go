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

	if err := goose.Up(sqlDB, "migrations"); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}

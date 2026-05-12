//go:build !lite

package cli

import (
	"context"
	"fmt"

	"breadbox/internal/config"
	"breadbox/internal/seed"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

// AddSeedCmd registers `breadbox seed`. Hidden in the help index because
// it's a dev affordance, not a user-facing command. L-scoped.
func AddSeedCmd(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:    "seed",
		Short:  "Insert development seed data into the local DB",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSeed()
		},
	}
	root.AddCommand(cmd)
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

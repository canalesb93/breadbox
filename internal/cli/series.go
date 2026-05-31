//go:build !lite

package cli

import (
	"context"
	"fmt"

	"breadbox/internal/config"
	"breadbox/internal/db"
	"breadbox/internal/service"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

// AddSeriesCmd registers `breadbox series ...` — recurring-series maintenance.
// L-scoped: talks to the DB/service layer directly, no API round-trip.
func AddSeriesCmd(root *cobra.Command) {
	series := &cobra.Command{
		Use:   "series",
		Short: "Recurring-series (subscriptions) maintenance",
	}
	backfill := &cobra.Command{
		Use:   "backfill",
		Short: "Detect recurring series across all transaction history (one-time)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSeriesBackfill()
		},
	}
	reinferTypes := &cobra.Command{
		Use:   "reinfer-types",
		Short: "Re-infer the type of default-'subscription' series from member categories (one-time)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSeriesReinferTypes()
		},
	}
	series.AddCommand(backfill, reinferTypes)
	root.AddCommand(series)
}

func runSeriesBackfill() error {
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

	svc := service.New(db.New(pool), pool, nil, logger)
	n, err := svc.BackfillSeriesDetection(ctx)
	if err != nil {
		return fmt.Errorf("series backfill: %w", err)
	}
	logger.Info("recurring-series backfill complete", "candidates", n)
	fmt.Printf("Detected %d recurring-series candidate(s).\n", n)
	return nil
}

func runSeriesReinferTypes() error {
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

	svc := service.New(db.New(pool), pool, nil, logger)
	n, err := svc.ReinferSeriesTypes(ctx)
	if err != nil {
		return fmt.Errorf("series reinfer-types: %w", err)
	}
	logger.Info("recurring-series type re-inference complete", "retyped", n)
	fmt.Printf("Re-typed %d recurring series.\n", n)
	return nil
}

package app

import (
	"context"
	"fmt"
	"log/slog"

	"breadbox/internal/config"
	"breadbox/internal/db"
	"breadbox/internal/provider"
	plaidprovider "breadbox/internal/provider/plaid"
	"breadbox/internal/service"
	"breadbox/internal/sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

// App is the single dependency container for the process. It is constructed
// once at startup and passed to every handler and worker.
type App struct {
	DB         *pgxpool.Pool
	Queries    *db.Queries
	Config     *config.Config
	Logger     *slog.Logger
	Providers  map[string]provider.Provider
	SyncEngine *sync.Engine
	Service    *service.Service
}

// New creates a new App. It connects to the database, creates a Queries
// instance, and initializes the providers map.
func New(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*App, error) {
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	queries := db.New(pool)

	providers := make(map[string]provider.Provider)

	if cfg.PlaidClientID != "" && cfg.PlaidSecret != "" {
		plaidClient := plaidprovider.NewPlaidClient(cfg.PlaidClientID, cfg.PlaidSecret, cfg.PlaidEnv)
		plaidProv := plaidprovider.NewProvider(plaidClient, cfg.EncryptionKey, cfg.WebhookURL, logger)
		providers["plaid"] = plaidProv
		logger.Info("plaid provider initialized", "env", cfg.PlaidEnv)
	}

	syncEngine := sync.NewEngine(queries, pool, providers, logger)
	svc := service.New(queries, pool, syncEngine, logger)

	return &App{
		DB:         pool,
		Queries:    queries,
		Config:     cfg,
		Logger:     logger,
		Providers:  providers,
		SyncEngine: syncEngine,
		Service:    svc,
	}, nil
}

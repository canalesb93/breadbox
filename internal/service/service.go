package service

import (
	"log/slog"

	"breadbox/internal/db"
	"breadbox/internal/sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	Queries    *db.Queries
	Pool       *pgxpool.Pool
	SyncEngine *sync.Engine
	Logger     *slog.Logger
}

func New(queries *db.Queries, pool *pgxpool.Pool, syncEngine *sync.Engine, logger *slog.Logger) *Service {
	return &Service{
		Queries:    queries,
		Pool:       pool,
		SyncEngine: syncEngine,
		Logger:     logger,
	}
}

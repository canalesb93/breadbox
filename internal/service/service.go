//go:build !lite

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

	// EncryptionKey is the 32-byte AES key (ENCRYPTION_KEY) used to encrypt
	// at-rest secrets the service owns — currently custom-connector header
	// secrets. Set by the app/router wiring after New; nil when no providers
	// are configured, in which case storing a connector secret is rejected.
	EncryptionKey []byte

	// OnDefinitionChanged is invoked after any agent_definition CRUD mutation.
	// Set at startup (in serve.go) to the agent scheduler's Reload trigger so
	// new/edited/deleted definitions are picked up immediately. Nil = no-op.
	OnDefinitionChanged func()
}

func New(queries *db.Queries, pool *pgxpool.Pool, syncEngine *sync.Engine, logger *slog.Logger) *Service {
	return &Service{
		Queries:    queries,
		Pool:       pool,
		SyncEngine: syncEngine,
		Logger:     logger,
	}
}

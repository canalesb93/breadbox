//go:build !lite

package cli

import (
	"context"
	"fmt"

	"breadbox/internal/cli/config"
	"breadbox/internal/client"
	bbconfig "breadbox/internal/config"
	"breadbox/internal/db"
	"breadbox/internal/service"

	"github.com/jackc/pgx/v5/pgxpool"
)

func runAuthBootstrap(ctx context.Context, version, baseURL string) error {
	cfg, err := bbconfig.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is not set; either set it or use `breadbox auth login --host=URL --token=...`")
	}

	hosts, err := config.Load()
	if err != nil {
		return fmt.Errorf("load hosts config: %w", err)
	}
	// Idempotent: if an existing `local` host already validates, no-op.
	if existing, ok := hosts.Hosts["local"]; ok && existing.Token != "" {
		c := client.New(existing, version)
		if _, err := c.Version(ctx); err == nil {
			fmt.Println("auth bootstrap: 'local' host already configured and reachable; no-op")
			return nil
		}
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	queries := db.New(pool)
	svc := service.New(queries, pool, nil, nil)

	result, err := svc.CreateAPIKey(ctx, service.CreateAPIKeyParams{
		Name:      "cli-bootstrap",
		Scope:     "full_access",
		ActorType: "user",
		ActorName: "cli-bootstrap",
	})
	if err != nil {
		return fmt.Errorf("mint api key: %w", err)
	}

	host := config.Host{BaseURL: baseURL, Token: result.PlaintextKey}
	if err := hosts.Set("local", host); err != nil {
		return err
	}
	if err := hosts.SetDefault("local"); err != nil {
		return err
	}
	if err := hosts.Save(); err != nil {
		return err
	}

	fmt.Printf("auth bootstrap: minted key %s and saved as host 'local'\n", result.KeyPrefix)
	fmt.Printf("token: %s\n", result.PlaintextKey)
	fmt.Println("(stored in hosts.toml; you don't need to copy it elsewhere)")
	return nil
}

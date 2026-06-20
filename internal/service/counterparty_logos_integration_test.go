//go:build integration && !lite

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/appconfig"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"
)

// TestCounterpartyLogoSettings exercises the env → app_config → default
// precedence for the counterparty logo.dev toggle + token.
func TestCounterpartyLogoSettings(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()

	// Default (no env, no DB row): enabled, no token.
	if enabled, token := svc.CounterpartyLogoSettings(ctx); !enabled || token != "" {
		t.Fatalf("default = (%v, %q), want (true, \"\")", enabled, token)
	}

	// app_config disables + sets a token.
	mustSet := func(key, val string) {
		t.Helper()
		if err := queries.SetAppConfig(ctx, db.SetAppConfigParams{Key: key, Value: pgconv.Text(val)}); err != nil {
			t.Fatalf("SetAppConfig(%s): %v", key, err)
		}
	}
	mustSet(appconfig.KeyCounterpartyLogos, "false")
	mustSet(appconfig.KeyLogoDevToken, "pk_db_token")

	if enabled, token := svc.CounterpartyLogoSettings(ctx); enabled || token != "pk_db_token" {
		t.Fatalf("db = (%v, %q), want (false, \"pk_db_token\")", enabled, token)
	}

	// Re-enable via app_config.
	mustSet(appconfig.KeyCounterpartyLogos, "true")
	if enabled, _ := svc.CounterpartyLogoSettings(ctx); !enabled {
		t.Fatalf("db re-enable = %v, want true", enabled)
	}

	// Env overrides app_config (both directions).
	t.Setenv("BREADBOX_COUNTERPARTY_LOGOS", "false")
	t.Setenv("LOGO_DEV_TOKEN", "pk_env_token")
	if enabled, token := svc.CounterpartyLogoSettings(ctx); enabled || token != "pk_env_token" {
		t.Fatalf("env = (%v, %q), want (false, \"pk_env_token\")", enabled, token)
	}

	t.Setenv("BREADBOX_COUNTERPARTY_LOGOS", "1")
	if enabled, _ := svc.CounterpartyLogoSettings(ctx); !enabled {
		t.Fatalf("env=1 = %v, want true", enabled)
	}
}

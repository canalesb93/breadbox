package app

import (
	"fmt"
	"log/slog"

	"breadbox/internal/config"
	"breadbox/internal/provider"
	plaidprovider "breadbox/internal/provider/plaid"
	tellerprovider "breadbox/internal/provider/teller"
)

// initTellerProvider creates the Teller provider from config (file paths or PEM bytes).
func initTellerProvider(cfg *config.Config, providers map[string]provider.Provider, logger *slog.Logger) error {
	if cfg.TellerAppID == "" {
		return nil
	}

	// Prefer file paths (from env) over PEM bytes (from DB).
	if cfg.TellerCertPath != "" && cfg.TellerKeyPath != "" {
		client, err := tellerprovider.NewClient(cfg.TellerCertPath, cfg.TellerKeyPath)
		if err != nil {
			return fmt.Errorf("create teller client: %w", err)
		}
		providers["teller"] = tellerprovider.NewProvider(client, cfg.TellerAppID, cfg.TellerEnv, cfg.TellerWebhookSecret, cfg.EncryptionKey, logger)
		logger.Info("teller provider initialized", "env", cfg.TellerEnv, "source", "file")
		return nil
	}

	if len(cfg.TellerCertPEM) > 0 && len(cfg.TellerKeyPEM) > 0 {
		client, err := tellerprovider.NewClientFromPEM(cfg.TellerCertPEM, cfg.TellerKeyPEM)
		if err != nil {
			return fmt.Errorf("create teller client from PEM: %w", err)
		}
		providers["teller"] = tellerprovider.NewProvider(client, cfg.TellerAppID, cfg.TellerEnv, cfg.TellerWebhookSecret, cfg.EncryptionKey, logger)
		logger.Info("teller provider initialized", "env", cfg.TellerEnv, "source", "pem")
		return nil
	}

	logger.Warn("teller app ID set but certificate/key not available, teller provider not initialized")
	return nil
}

// ReinitProvider re-creates (or removes) a provider in the live Providers map
// based on the current Config values. The sync engine shares the same map
// reference, so changes are visible immediately.
func (a *App) ReinitProvider(name string) error {
	switch name {
	case "plaid":
		if a.Config.PlaidClientID != "" && a.Config.PlaidSecret != "" {
			client := plaidprovider.NewPlaidClient(a.Config.PlaidClientID, a.Config.PlaidSecret, a.Config.PlaidEnv)
			a.Providers["plaid"] = plaidprovider.NewProvider(client, a.Config.EncryptionKey, a.Config.WebhookURL, a.Logger)
			a.Logger.Info("plaid provider reinitialized", "env", a.Config.PlaidEnv)
		} else {
			delete(a.Providers, "plaid")
			a.Logger.Info("plaid provider removed (credentials cleared)")
		}

	case "teller":
		delete(a.Providers, "teller")
		if err := initTellerProvider(a.Config, a.Providers, a.Logger); err != nil {
			return err
		}

	default:
		return fmt.Errorf("unknown provider: %s", name)
	}
	return nil
}

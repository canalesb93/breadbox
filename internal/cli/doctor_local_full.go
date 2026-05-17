//go:build !lite

package cli

import (
	"context"
	"crypto/aes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"breadbox/internal/agent"
	"breadbox/internal/appconfig"
	bbconfig "breadbox/internal/config"
	"breadbox/internal/crypto"
	"breadbox/internal/db"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
	"github.com/robfig/cron/v3"
)

func runDoctorLocal(ctx context.Context, jsonOut bool, skipExternal bool) error {
	cfg, cfgErr := bbconfig.Load()
	checks := []doctorCheck{}

	if cfgErr != nil {
		checks = append(checks, doctorCheck{
			Name:        "config load",
			Status:      doctorStatusFail,
			Message:     cfgErr.Error(),
			Remediation: "fix the offending environment variable or .local.env entry",
		})
		return emitDoctor(checks, jsonOut)
	}
	checks = append(checks, doctorCheck{
		Name:    "config load",
		Status:  doctorStatusPass,
		Message: fmt.Sprintf("environment=%s", cfg.Environment),
	})

	dbCheck, migCheck, pool := checkDatabase(ctx, cfg)
	checks = append(checks, dbCheck, migCheck)
	if pool != nil {
		defer pool.Close()
	}

	if pool != nil {
		if err := bbconfig.LoadWithDB(ctx, cfg, pool); err != nil {
			checks = append(checks, doctorCheck{
				Name:    "app_config",
				Status:  doctorStatusWarn,
				Message: "could not merge app_config: " + err.Error(),
			})
		}
	}

	checks = append(checks, checkEncryptionKey(cfg))

	if cfg.PlaidClientID != "" {
		checks = append(checks, checkPlaid(cfg))
	}
	if cfg.TellerAppID != "" {
		checks = append(checks, checkTeller(cfg))
	}

	if pool != nil && cfg.EncryptionKey != nil {
		checks = append(checks, checkProviderCredentialsDecrypt(ctx, pool, cfg))
	}
	if pool != nil {
		checks = append(checks, checkAdminAccount(ctx, pool))
	}
	checks = append(checks, checkCronConfig(cfg))
	checks = append(checks, checkPublicURL(skipExternal))
	checks = append(checks, checkAgentSubsystem(ctx, pool))

	return emitDoctor(checks, jsonOut)
}

// checkAgentSubsystem reports whether the Claude Agent SDK integration is
// ready to run. Cheap + side-effect free: looks up the credential presence
// in app_config and locates the sidecar binary on disk — does NOT spawn
// the sidecar. Use `breadbox agent test` for the live round-trip.
func checkAgentSubsystem(ctx context.Context, pool *pgxpool.Pool) doctorCheck {
	if pool == nil {
		return doctorCheck{
			Name:    "agent subsystem",
			Status:  doctorStatusSkip,
			Message: "skipped — no database connection",
		}
	}
	queries := db.New(pool)
	authMode := appconfig.String(ctx, queries, appconfig.KeyAgentAuthMode, appconfig.AuthModeSubscription)
	tokenKey := appconfig.KeyAgentSubscriptionToken
	if authMode == appconfig.AuthModeAPIKey {
		tokenKey = appconfig.KeyAgentAnthropicAPIKey
	}
	stored, _ := appconfig.Read(ctx, queries, tokenKey)
	binaryPath := appconfig.String(ctx, queries, appconfig.KeyAgentRuntimePath, "")
	resolved, binErr := agent.LocateBinary(binaryPath)
	return agentSubsystemCheck(authMode, stored != "", resolved, binErr == nil)
}

// agentSubsystemCheck is the pure decision logic separated for testability.
// authMode is the resolved app_config.agent.auth_mode value; authPresent is
// whether a non-empty value sits at the matching token key; binaryPath +
// binaryReady are the result of agent.LocateBinary.
func agentSubsystemCheck(authMode string, authPresent bool, binaryPath string, binaryReady bool) doctorCheck {
	switch {
	case authPresent && binaryReady:
		return doctorCheck{
			Name:    "agent subsystem",
			Status:  doctorStatusPass,
			Message: fmt.Sprintf("ready (auth=%s, binary=%s) — `breadbox agent test` for live diagnostic", authMode, binaryPath),
		}
	case authPresent && !binaryReady:
		return doctorCheck{
			Name:        "agent subsystem",
			Status:      doctorStatusWarn,
			Message:     "auth configured but breadbox-agent binary not found",
			Remediation: "build with `make agent-sidecar` or set `agent.runtime_path` in Settings → Agents",
		}
	case !authPresent && binaryReady:
		return doctorCheck{
			Name:        "agent subsystem",
			Status:      doctorStatusWarn,
			Message:     fmt.Sprintf("binary at %s but no Anthropic credential configured (auth_mode=%s)", binaryPath, authMode),
			Remediation: "paste a credential in Settings → Agents — or run `claude setup-token` and use the result for subscription mode",
		}
	default:
		return doctorCheck{
			Name:    "agent subsystem",
			Status:  doctorStatusSkip,
			Message: "not configured (optional — see docs/agents.md)",
		}
	}
}

func emitDoctor(checks []doctorCheck, asJSON bool) error {
	ok := true
	for _, c := range checks {
		if c.Status == doctorStatusFail {
			ok = false
		}
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(doctorReport{Checks: checks, OK: ok})
	} else {
		renderDoctorTable(checks)
		fmt.Println()
		if ok {
			fmt.Println("OK — all checks passed.")
		} else {
			fmt.Println("FAIL — one or more checks failed. See remediation hints above.")
		}
	}

	if !ok {
		return fmt.Errorf("doctor: one or more checks failed")
	}
	return nil
}

func renderDoctorTable(checks []doctorCheck) {
	nameW := len("Check")
	for _, c := range checks {
		if len(c.Name) > nameW {
			nameW = len(c.Name)
		}
	}
	fmt.Printf("  %s  %-*s  %s\n", " ", nameW, "Check", "Status")
	fmt.Printf("  %s  %s  %s\n", "-", strings.Repeat("-", nameW), strings.Repeat("-", 40))
	for _, c := range checks {
		sym := statusSymbol(c.Status)
		fmt.Printf("  %s  %-*s  %s\n", sym, nameW, c.Name, c.Message)
		if c.Status == doctorStatusFail && c.Remediation != "" {
			fmt.Printf("     %s  → %s\n", strings.Repeat(" ", nameW), c.Remediation)
		}
	}
}

func statusSymbol(status string) string {
	switch status {
	case doctorStatusPass:
		return "ok"
	case doctorStatusFail:
		return "!!"
	case doctorStatusSkip:
		return "--"
	case doctorStatusWarn:
		return " !"
	default:
		return " ?"
	}
}

func checkDatabase(ctx context.Context, cfg *bbconfig.Config) (doctorCheck, doctorCheck, *pgxpool.Pool) {
	if cfg.DatabaseURL == "" {
		return doctorCheck{
				Name:        "database",
				Status:      doctorStatusFail,
				Message:     "DATABASE_URL is not set",
				Remediation: "export DATABASE_URL=postgres://user:pass@host:5432/breadbox?sslmode=disable",
			}, doctorCheck{
				Name:    "migrations",
				Status:  doctorStatusSkip,
				Message: "skipped — no database connection",
			}, nil
	}

	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(connectCtx, cfg.DatabaseURL)
	if err != nil {
		return doctorCheck{
			Name:        "database",
			Status:      doctorStatusFail,
			Message:     "connect failed: " + err.Error(),
			Remediation: "verify DATABASE_URL and that Postgres is running",
		}, doctorCheck{Name: "migrations", Status: doctorStatusSkip, Message: "skipped — no database connection"}, nil
	}
	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		return doctorCheck{
			Name:        "database",
			Status:      doctorStatusFail,
			Message:     "ping failed: " + err.Error(),
			Remediation: "verify DATABASE_URL and that Postgres is running",
		}, doctorCheck{Name: "migrations", Status: doctorStatusSkip, Message: "skipped — no database connection"}, nil
	}

	return doctorCheck{
		Name:    "database",
		Status:  doctorStatusPass,
		Message: "connected and reachable",
	}, checkMigrations(ctx, cfg.DatabaseURL), pool
}

func checkMigrations(ctx context.Context, databaseURL string) doctorCheck {
	latest, err := db.LatestEmbeddedMigration()
	if err != nil {
		return doctorCheck{Name: "migrations", Status: doctorStatusFail, Message: "could not read embedded migrations: " + err.Error(), Remediation: "rebuild the binary — embedded migrations are corrupt"}
	}
	goose.SetBaseFS(db.Migrations)
	sqlDB, err := goose.OpenDBWithDriver("pgx", databaseURL)
	if err != nil {
		return doctorCheck{Name: "migrations", Status: doctorStatusFail, Message: "open: " + err.Error(), Remediation: "verify DATABASE_URL"}
	}
	defer sqlDB.Close()
	if err := goose.SetDialect("postgres"); err != nil {
		return doctorCheck{Name: "migrations", Status: doctorStatusFail, Message: "set dialect: " + err.Error()}
	}
	applied, err := goose.GetDBVersionContext(ctx, sqlDB)
	if err != nil {
		return doctorCheck{Name: "migrations", Status: doctorStatusFail, Message: "could not read goose_db_version: " + err.Error(), Remediation: "run 'breadbox migrate' to initialize the schema"}
	}
	if applied < latest {
		return doctorCheck{Name: "migrations", Status: doctorStatusFail, Message: fmt.Sprintf("applied=%d, embedded=%d (behind by %d)", applied, latest, latest-applied), Remediation: "run 'breadbox migrate' to apply pending migrations"}
	}
	if applied > latest {
		return doctorCheck{Name: "migrations", Status: doctorStatusWarn, Message: fmt.Sprintf("applied=%d, embedded=%d — db is ahead (downgraded binary?)", applied, latest), Remediation: "upgrade the binary to the version matching your schema"}
	}
	return doctorCheck{Name: "migrations", Status: doctorStatusPass, Message: fmt.Sprintf("up-to-date (version %d)", applied)}
}

func checkEncryptionKey(cfg *bbconfig.Config) doctorCheck {
	raw := os.Getenv("ENCRYPTION_KEY")
	providerConfigured := cfg.PlaidClientID != "" || cfg.TellerAppID != ""
	if raw == "" {
		if providerConfigured {
			return doctorCheck{Name: "encryption key", Status: doctorStatusFail, Message: "ENCRYPTION_KEY is required when a bank provider is configured", Remediation: "generate one with: openssl rand -hex 32"}
		}
		return doctorCheck{Name: "encryption key", Status: doctorStatusSkip, Message: "not set (no bank provider configured)"}
	}
	key, err := hex.DecodeString(raw)
	if err != nil {
		return doctorCheck{Name: "encryption key", Status: doctorStatusFail, Message: "ENCRYPTION_KEY is not valid hex", Remediation: "regenerate with: openssl rand -hex 32"}
	}
	if len(key) != 32 {
		return doctorCheck{Name: "encryption key", Status: doctorStatusFail, Message: fmt.Sprintf("ENCRYPTION_KEY must decode to 32 bytes, got %d", len(key)), Remediation: "regenerate with: openssl rand -hex 32"}
	}
	if _, err := aes.NewCipher(key); err != nil {
		return doctorCheck{Name: "encryption key", Status: doctorStatusFail, Message: "key rejected by AES cipher: " + err.Error(), Remediation: "regenerate with: openssl rand -hex 32"}
	}
	return doctorCheck{Name: "encryption key", Status: doctorStatusPass, Message: "set and 32 bytes"}
}

func checkPlaid(cfg *bbconfig.Config) doctorCheck {
	if cfg.PlaidSecret == "" {
		return doctorCheck{Name: "plaid", Status: doctorStatusFail, Message: "PLAID_CLIENT_ID is set but PLAID_SECRET is missing", Remediation: "set PLAID_SECRET or remove PLAID_CLIENT_ID"}
	}
	env := cfg.PlaidEnv
	switch env {
	case "sandbox", "development", "production":
		return doctorCheck{Name: "plaid", Status: doctorStatusPass, Message: "client/secret set, env=" + env}
	case "":
		return doctorCheck{Name: "plaid", Status: doctorStatusFail, Message: "PLAID_ENV is empty", Remediation: "set PLAID_ENV to sandbox, development, or production"}
	default:
		return doctorCheck{Name: "plaid", Status: doctorStatusFail, Message: "PLAID_ENV=" + env + " is not one of sandbox/development/production", Remediation: "set PLAID_ENV to sandbox, development, or production"}
	}
}

func checkTeller(cfg *bbconfig.Config) doctorCheck {
	env := cfg.TellerEnv
	switch env {
	case "sandbox", "development", "production":
	default:
		return doctorCheck{Name: "teller", Status: doctorStatusFail, Message: "TELLER_ENV=" + env + " is not one of sandbox/development/production", Remediation: "set TELLER_ENV appropriately"}
	}
	for _, p := range []struct {
		label string
		path  string
	}{
		{"TELLER_CERT_PATH", cfg.TellerCertPath},
		{"TELLER_KEY_PATH", cfg.TellerKeyPath},
	} {
		if p.path == "" {
			continue
		}
		abs, _ := filepath.Abs(p.path)
		f, err := os.Open(p.path)
		if err != nil {
			return doctorCheck{Name: "teller", Status: doctorStatusFail, Message: fmt.Sprintf("%s=%s is not readable: %v", p.label, abs, err), Remediation: "verify the file exists and the process can read it"}
		}
		f.Close()
	}
	if cfg.TellerCertPath == "" && cfg.TellerKeyPath == "" && len(cfg.TellerCertPEM) == 0 && len(cfg.TellerKeyPEM) == 0 {
		return doctorCheck{Name: "teller", Status: doctorStatusFail, Message: "TELLER_APP_ID is set but no cert/key material is available", Remediation: "set TELLER_CERT_PATH and TELLER_KEY_PATH, or upload certificates via the admin dashboard"}
	}
	return doctorCheck{Name: "teller", Status: doctorStatusPass, Message: "cert/key present, env=" + env}
}

func checkProviderCredentialsDecrypt(ctx context.Context, pool *pgxpool.Pool, cfg *bbconfig.Config) doctorCheck {
	rows, err := pool.Query(ctx, `
		SELECT id::text, provider, encrypted_credentials
		FROM bank_connections
		WHERE status != 'disconnected' AND encrypted_credentials IS NOT NULL
	`)
	if err != nil {
		return doctorCheck{Name: "provider credentials", Status: doctorStatusWarn, Message: "could not query bank_connections: " + err.Error()}
	}
	defer rows.Close()
	var total, failed int
	var firstFailID, firstFailProvider string
	for rows.Next() {
		var id, provider string
		var cred []byte
		if err := rows.Scan(&id, &provider, &cred); err != nil {
			return doctorCheck{Name: "provider credentials", Status: doctorStatusWarn, Message: "scan error: " + err.Error()}
		}
		total++
		if _, err := crypto.Decrypt(cred, cfg.EncryptionKey); err != nil {
			failed++
			if firstFailID == "" {
				firstFailID = id
				firstFailProvider = provider
			}
		}
	}
	if total == 0 {
		return doctorCheck{Name: "provider credentials", Status: doctorStatusSkip, Message: "no active bank connections to check"}
	}
	if failed > 0 {
		return doctorCheck{Name: "provider credentials", Status: doctorStatusFail, Message: fmt.Sprintf("%d/%d connections failed to decrypt (first: %s %s)", failed, total, firstFailProvider, firstFailID), Remediation: "ENCRYPTION_KEY appears to have changed since these connections were created — restore the original key or re-link the affected connections"}
	}
	return doctorCheck{Name: "provider credentials", Status: doctorStatusPass, Message: fmt.Sprintf("%d connection(s) decrypt cleanly", total)}
}

func checkAdminAccount(ctx context.Context, pool *pgxpool.Pool) doctorCheck {
	q := db.New(pool)
	count, err := q.CountAuthAccounts(ctx)
	if err != nil {
		return doctorCheck{Name: "admin account", Status: doctorStatusFail, Message: "could not query auth_accounts: " + err.Error(), Remediation: "check database connectivity and that migrations ran"}
	}
	if count == 0 {
		return doctorCheck{Name: "admin account", Status: doctorStatusFail, Message: "no admin account exists", Remediation: "visit /setup in the running server or run 'breadbox create-admin'"}
	}
	return doctorCheck{Name: "admin account", Status: doctorStatusPass, Message: fmt.Sprintf("%d account(s) present", count)}
}

func checkCronConfig(cfg *bbconfig.Config) doctorCheck {
	if cfg.SyncIntervalMinutes < 0 {
		return doctorCheck{Name: "scheduler", Status: doctorStatusFail, Message: fmt.Sprintf("sync_interval_minutes=%d is negative", cfg.SyncIntervalMinutes), Remediation: "set sync_interval_minutes to a positive integer in app_config"}
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	for _, envVar := range []string{"BACKUP_CRON", "SYNC_CRON"} {
		spec := os.Getenv(envVar)
		if spec == "" {
			continue
		}
		if _, err := parser.Parse(spec); err != nil {
			return doctorCheck{Name: "scheduler", Status: doctorStatusFail, Message: fmt.Sprintf("%s=%q does not parse: %v", envVar, spec, err), Remediation: "use a standard 5-field cron expression, e.g. '0 2 * * *'"}
		}
	}
	return doctorCheck{Name: "scheduler", Status: doctorStatusPass, Message: fmt.Sprintf("sync every %dm; cron expressions valid", cfg.SyncIntervalMinutes)}
}

func checkPublicURL(skipExternal bool) doctorCheck {
	publicURL := strings.TrimSpace(os.Getenv("PUBLIC_URL"))
	domain := strings.TrimSpace(os.Getenv("DOMAIN"))
	if publicURL == "" && domain == "" {
		return doctorCheck{Name: "public url", Status: doctorStatusSkip, Message: "neither PUBLIC_URL nor DOMAIN set"}
	}
	if skipExternal {
		return doctorCheck{Name: "public url", Status: doctorStatusSkip, Message: "--skip-external set"}
	}
	target := publicURL
	if target == "" {
		target = "https://" + domain
	}
	u, err := url.Parse(target)
	if err != nil || u.Host == "" {
		return doctorCheck{Name: "public url", Status: doctorStatusFail, Message: fmt.Sprintf("invalid URL %q", target), Remediation: "set PUBLIC_URL to a full URL like https://breadbox.example.com"}
	}
	host := u.Hostname()
	if _, err := net.LookupHost(host); err != nil {
		return doctorCheck{Name: "public url", Status: doctorStatusFail, Message: fmt.Sprintf("DNS lookup for %s failed: %v", host, err), Remediation: "verify your DNS A/AAAA records point at this host"}
	}
	readyURL := strings.TrimRight(target, "/") + "/health/ready"
	hc := &http.Client{Timeout: 5 * time.Second}
	resp, err := hc.Get(readyURL)
	if err != nil {
		return doctorCheck{Name: "public url", Status: doctorStatusFail, Message: fmt.Sprintf("GET %s failed: %v", readyURL, err), Remediation: "ensure the server is running and reachable from this host, or pass --skip-external"}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return doctorCheck{Name: "public url", Status: doctorStatusFail, Message: fmt.Sprintf("%s returned HTTP %d", readyURL, resp.StatusCode), Remediation: "check server logs for why /health/ready is failing"}
	}
	return doctorCheck{Name: "public url", Status: doctorStatusPass, Message: fmt.Sprintf("DNS resolves, %s → %d", readyURL, resp.StatusCode)}
}

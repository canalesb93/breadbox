package main

import (
	"context"
	"crypto/aes"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"breadbox/internal/config"
	"breadbox/internal/crypto"
	"breadbox/internal/db"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
	"github.com/robfig/cron/v3"
)

// Check statuses for the doctor command.
const (
	doctorStatusPass = "pass"
	doctorStatusFail = "fail"
	doctorStatusSkip = "skip"
	doctorStatusWarn = "warn"
)

// doctorCheck is the structured result of a single preflight check.
type doctorCheck struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

// doctorReport is the structured output for --json mode.
type doctorReport struct {
	Checks []doctorCheck `json:"checks"`
	OK     bool          `json:"ok"`
}

// runDoctor validates configuration and connectivity without booting the server.
// Exits non-zero if any check fails so install scripts can gate on it.
func runDoctor() error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "emit structured JSON instead of a pretty table")
	skipExternal := fs.Bool("skip-external", false, "skip DNS/HTTP reachability checks (air-gapped installs)")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}

	ctx := context.Background()

	cfg, cfgErr := config.Load()
	checks := []doctorCheck{}

	// Config parse
	if cfgErr != nil {
		checks = append(checks, doctorCheck{
			Name:        "config load",
			Status:      doctorStatusFail,
			Message:     cfgErr.Error(),
			Remediation: "fix the offending environment variable or .local.env entry",
		})
		return emitDoctor(checks, *jsonOut)
	}
	checks = append(checks, doctorCheck{
		Name:    "config load",
		Status:  doctorStatusPass,
		Message: fmt.Sprintf("environment=%s", cfg.Environment),
	})

	// DATABASE_URL + connectivity + migrations
	dbCheck, migCheck, pool := checkDatabase(ctx, cfg)
	checks = append(checks, dbCheck, migCheck)
	if pool != nil {
		defer pool.Close()
	}

	// After DB is up, merge app_config values so provider checks see the full picture.
	if pool != nil {
		if err := config.LoadWithDB(ctx, cfg, pool); err != nil {
			checks = append(checks, doctorCheck{
				Name:    "app_config",
				Status:  doctorStatusWarn,
				Message: "could not merge app_config: " + err.Error(),
			})
		}
	}

	// ENCRYPTION_KEY
	checks = append(checks, checkEncryptionKey(cfg))

	// Providers (only if configured)
	if cfg.PlaidClientID != "" {
		checks = append(checks, checkPlaid(cfg))
	}
	if cfg.TellerAppID != "" {
		checks = append(checks, checkTeller(cfg))
	}

	// Provider credential decrypt: only meaningful when both the key and DB are up.
	if pool != nil && cfg.EncryptionKey != nil {
		checks = append(checks, checkProviderCredentialsDecrypt(ctx, pool, cfg))
	}

	// Admin account
	if pool != nil {
		checks = append(checks, checkAdminAccount(ctx, pool))
	}

	// Scheduler/cron config
	checks = append(checks, checkCronConfig(cfg))

	// DOMAIN / PUBLIC_URL reachability
	checks = append(checks, checkPublicURL(*skipExternal))

	return emitDoctor(checks, *jsonOut)
}

// emitDoctor writes the report and returns a non-nil error when any check failed
// so main() exits non-zero. json mode still returns an error on failure.
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

// renderDoctorTable prints a padded table with status symbols, names, and messages.
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
		return "✓" // ✓
	case doctorStatusFail:
		return "✗" // ✗
	case doctorStatusSkip:
		return "⊘" // ⊘
	case doctorStatusWarn:
		return "!"
	default:
		return "?"
	}
}

// checkDatabase validates DATABASE_URL, reachability, and migration version.
// Returns the DB-check result, the migrations-check result, and the open pool
// (so subsequent checks can reuse it). Pool is nil if connect failed.
func checkDatabase(ctx context.Context, cfg *config.Config) (doctorCheck, doctorCheck, *pgxpool.Pool) {
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

	dbCheck := doctorCheck{
		Name:    "database",
		Status:  doctorStatusPass,
		Message: "connected and reachable",
	}

	migCheck := checkMigrations(ctx, cfg.DatabaseURL)
	return dbCheck, migCheck, pool
}

// checkMigrations compares the applied goose_db_version against embedded migration files.
func checkMigrations(ctx context.Context, databaseURL string) doctorCheck {
	latest, err := latestEmbeddedMigration()
	if err != nil {
		return doctorCheck{
			Name:        "migrations",
			Status:      doctorStatusFail,
			Message:     "could not read embedded migrations: " + err.Error(),
			Remediation: "rebuild the binary — embedded migrations are corrupt",
		}
	}

	goose.SetBaseFS(db.Migrations)
	sqlDB, err := goose.OpenDBWithDriver("pgx", databaseURL)
	if err != nil {
		return doctorCheck{
			Name:        "migrations",
			Status:      doctorStatusFail,
			Message:     "open: " + err.Error(),
			Remediation: "verify DATABASE_URL",
		}
	}
	defer sqlDB.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		return doctorCheck{
			Name:    "migrations",
			Status:  doctorStatusFail,
			Message: "set dialect: " + err.Error(),
		}
	}

	applied, err := goose.GetDBVersionContext(ctx, sqlDB)
	if err != nil {
		return doctorCheck{
			Name:        "migrations",
			Status:      doctorStatusFail,
			Message:     "could not read goose_db_version: " + err.Error(),
			Remediation: "run 'breadbox migrate' to initialize the schema",
		}
	}

	if applied < latest {
		return doctorCheck{
			Name:        "migrations",
			Status:      doctorStatusFail,
			Message:     fmt.Sprintf("applied=%d, embedded=%d (behind by %d)", applied, latest, latest-applied),
			Remediation: "run 'breadbox migrate' to apply pending migrations",
		}
	}
	if applied > latest {
		return doctorCheck{
			Name:        "migrations",
			Status:      doctorStatusWarn,
			Message:     fmt.Sprintf("applied=%d, embedded=%d — db is ahead (downgraded binary?)", applied, latest),
			Remediation: "upgrade the binary to the version matching your schema",
		}
	}
	return doctorCheck{
		Name:    "migrations",
		Status:  doctorStatusPass,
		Message: fmt.Sprintf("up-to-date (version %d)", applied),
	}
}

// latestEmbeddedMigration returns the highest numeric prefix among embedded migrations.
func latestEmbeddedMigration() (int64, error) {
	entries, err := fs.ReadDir(db.Migrations, "migrations")
	if err != nil {
		return 0, err
	}
	prefix := regexp.MustCompile(`^(\d+)_`)
	var versions []int64
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		m := prefix.FindStringSubmatch(e.Name())
		if len(m) != 2 {
			continue
		}
		var v int64
		_, err := fmt.Sscanf(m[1], "%d", &v)
		if err != nil {
			continue
		}
		versions = append(versions, v)
	}
	if len(versions) == 0 {
		return 0, fmt.Errorf("no migrations found")
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })
	return versions[len(versions)-1], nil
}

// checkEncryptionKey validates ENCRYPTION_KEY is set and hex-decodes to 32 bytes.
// TODO(#688): once we persist a key fingerprint, compare it against the live key here
// to catch silent key rotations that would break decrypt for existing connections.
func checkEncryptionKey(cfg *config.Config) doctorCheck {
	raw := os.Getenv("ENCRYPTION_KEY")
	providerConfigured := cfg.PlaidClientID != "" || cfg.TellerAppID != ""

	if raw == "" {
		if providerConfigured {
			return doctorCheck{
				Name:        "encryption key",
				Status:      doctorStatusFail,
				Message:     "ENCRYPTION_KEY is required when a bank provider is configured",
				Remediation: "generate one with: openssl rand -hex 32",
			}
		}
		return doctorCheck{
			Name:    "encryption key",
			Status:  doctorStatusSkip,
			Message: "not set (no bank provider configured)",
		}
	}

	key, err := hex.DecodeString(raw)
	if err != nil {
		return doctorCheck{
			Name:        "encryption key",
			Status:      doctorStatusFail,
			Message:     "ENCRYPTION_KEY is not valid hex",
			Remediation: "regenerate with: openssl rand -hex 32",
		}
	}
	if len(key) != 32 {
		return doctorCheck{
			Name:        "encryption key",
			Status:      doctorStatusFail,
			Message:     fmt.Sprintf("ENCRYPTION_KEY must decode to 32 bytes, got %d", len(key)),
			Remediation: "regenerate with: openssl rand -hex 32",
		}
	}
	if _, err := aes.NewCipher(key); err != nil {
		return doctorCheck{
			Name:        "encryption key",
			Status:      doctorStatusFail,
			Message:     "key rejected by AES cipher: " + err.Error(),
			Remediation: "regenerate with: openssl rand -hex 32",
		}
	}
	return doctorCheck{
		Name:    "encryption key",
		Status:  doctorStatusPass,
		Message: "set and 32 bytes",
	}
}

// checkPlaid validates that PLAID_ENV is a recognized value and client/secret are present.
func checkPlaid(cfg *config.Config) doctorCheck {
	if cfg.PlaidSecret == "" {
		return doctorCheck{
			Name:        "plaid",
			Status:      doctorStatusFail,
			Message:     "PLAID_CLIENT_ID is set but PLAID_SECRET is missing",
			Remediation: "set PLAID_SECRET or remove PLAID_CLIENT_ID",
		}
	}
	env := cfg.PlaidEnv
	switch env {
	case "sandbox", "development", "production":
		return doctorCheck{
			Name:    "plaid",
			Status:  doctorStatusPass,
			Message: "client/secret set, env=" + env,
		}
	case "":
		return doctorCheck{
			Name:        "plaid",
			Status:      doctorStatusFail,
			Message:     "PLAID_ENV is empty",
			Remediation: "set PLAID_ENV to sandbox, development, or production",
		}
	default:
		return doctorCheck{
			Name:        "plaid",
			Status:      doctorStatusFail,
			Message:     "PLAID_ENV=" + env + " is not one of sandbox/development/production",
			Remediation: "set PLAID_ENV to sandbox, development, or production",
		}
	}
}

// checkTeller validates Teller env and that any cert/key paths exist and are readable.
func checkTeller(cfg *config.Config) doctorCheck {
	env := cfg.TellerEnv
	switch env {
	case "sandbox", "development", "production":
	default:
		return doctorCheck{
			Name:        "teller",
			Status:      doctorStatusFail,
			Message:     "TELLER_ENV=" + env + " is not one of sandbox/development/production",
			Remediation: "set TELLER_ENV appropriately",
		}
	}

	// If file paths are set, they must be readable.
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
			return doctorCheck{
				Name:        "teller",
				Status:      doctorStatusFail,
				Message:     fmt.Sprintf("%s=%s is not readable: %v", p.label, abs, err),
				Remediation: "verify the file exists and the process can read it",
			}
		}
		f.Close()
	}

	// Either env paths are set OR the PEM bytes came from the DB (populated by LoadWithDB).
	if cfg.TellerCertPath == "" && cfg.TellerKeyPath == "" && len(cfg.TellerCertPEM) == 0 && len(cfg.TellerKeyPEM) == 0 {
		return doctorCheck{
			Name:        "teller",
			Status:      doctorStatusFail,
			Message:     "TELLER_APP_ID is set but no cert/key material is available",
			Remediation: "set TELLER_CERT_PATH and TELLER_KEY_PATH, or upload certificates via the admin dashboard",
		}
	}

	return doctorCheck{
		Name:    "teller",
		Status:  doctorStatusPass,
		Message: "cert/key present, env=" + env,
	}
}

// checkProviderCredentialsDecrypt walks all active bank_connections and verifies
// encrypted_credentials decrypts cleanly with the current ENCRYPTION_KEY. Any
// decryption failure is a hard fail — it means the stored key no longer matches
// the data it was supposed to protect.
func checkProviderCredentialsDecrypt(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config) doctorCheck {
	rows, err := pool.Query(ctx, `
		SELECT id::text, provider, encrypted_credentials
		FROM bank_connections
		WHERE status != 'disconnected' AND encrypted_credentials IS NOT NULL
	`)
	if err != nil {
		return doctorCheck{
			Name:    "provider credentials",
			Status:  doctorStatusWarn,
			Message: "could not query bank_connections: " + err.Error(),
		}
	}
	defer rows.Close()

	var total, failed int
	var firstFailID, firstFailProvider string
	for rows.Next() {
		var id, provider string
		var cred []byte
		if err := rows.Scan(&id, &provider, &cred); err != nil {
			return doctorCheck{
				Name:    "provider credentials",
				Status:  doctorStatusWarn,
				Message: "scan error: " + err.Error(),
			}
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
		return doctorCheck{
			Name:    "provider credentials",
			Status:  doctorStatusSkip,
			Message: "no active bank connections to check",
		}
	}
	if failed > 0 {
		return doctorCheck{
			Name:        "provider credentials",
			Status:      doctorStatusFail,
			Message:     fmt.Sprintf("%d/%d connections failed to decrypt (first: %s %s)", failed, total, firstFailProvider, firstFailID),
			Remediation: "ENCRYPTION_KEY appears to have changed since these connections were created — restore the original key or re-link the affected connections",
		}
	}
	return doctorCheck{
		Name:    "provider credentials",
		Status:  doctorStatusPass,
		Message: fmt.Sprintf("%d connection(s) decrypt cleanly", total),
	}
}

// checkAdminAccount verifies at least one admin row exists in auth_accounts.
func checkAdminAccount(ctx context.Context, pool *pgxpool.Pool) doctorCheck {
	q := db.New(pool)
	count, err := q.CountAuthAccounts(ctx)
	if err != nil {
		return doctorCheck{
			Name:        "admin account",
			Status:      doctorStatusFail,
			Message:     "could not query auth_accounts: " + err.Error(),
			Remediation: "check database connectivity and that migrations ran",
		}
	}
	if count == 0 {
		return doctorCheck{
			Name:        "admin account",
			Status:      doctorStatusFail,
			Message:     "no admin account exists",
			Remediation: "visit /setup in the running server or run 'breadbox create-admin'",
		}
	}
	return doctorCheck{
		Name:    "admin account",
		Status:  doctorStatusPass,
		Message: fmt.Sprintf("%d account(s) present", count),
	}
}

// checkCronConfig verifies the sync interval and any BACKUP_SCHEDULE-style cron values parse.
// SYNC_INTERVAL_MINUTES isn't a cron expression, but we validate it's a positive int if set.
func checkCronConfig(cfg *config.Config) doctorCheck {
	// SyncIntervalMinutes is always positive once LoadWithDB ran; defend anyway.
	if cfg.SyncIntervalMinutes < 0 {
		return doctorCheck{
			Name:        "scheduler",
			Status:      doctorStatusFail,
			Message:     fmt.Sprintf("sync_interval_minutes=%d is negative", cfg.SyncIntervalMinutes),
			Remediation: "set sync_interval_minutes to a positive integer in app_config",
		}
	}

	// Validate any explicit cron expressions set via env (backup schedule, custom crons).
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	for _, envVar := range []string{"BACKUP_CRON", "SYNC_CRON"} {
		spec := os.Getenv(envVar)
		if spec == "" {
			continue
		}
		if _, err := parser.Parse(spec); err != nil {
			return doctorCheck{
				Name:        "scheduler",
				Status:      doctorStatusFail,
				Message:     fmt.Sprintf("%s=%q does not parse: %v", envVar, spec, err),
				Remediation: "use a standard 5-field cron expression, e.g. '0 2 * * *'",
			}
		}
	}

	return doctorCheck{
		Name:    "scheduler",
		Status:  doctorStatusPass,
		Message: fmt.Sprintf("sync every %dm; cron expressions valid", cfg.SyncIntervalMinutes),
	}
}

// checkPublicURL resolves DOMAIN / PUBLIC_URL and probes /health/ready over HTTP.
// Skipped entirely if --skip-external or neither env is set.
func checkPublicURL(skipExternal bool) doctorCheck {
	publicURL := strings.TrimSpace(os.Getenv("PUBLIC_URL"))
	domain := strings.TrimSpace(os.Getenv("DOMAIN"))
	if publicURL == "" && domain == "" {
		return doctorCheck{
			Name:    "public url",
			Status:  doctorStatusSkip,
			Message: "neither PUBLIC_URL nor DOMAIN set",
		}
	}
	if skipExternal {
		return doctorCheck{
			Name:    "public url",
			Status:  doctorStatusSkip,
			Message: "--skip-external set",
		}
	}

	target := publicURL
	if target == "" {
		target = "https://" + domain
	}
	u, err := url.Parse(target)
	if err != nil || u.Host == "" {
		return doctorCheck{
			Name:        "public url",
			Status:      doctorStatusFail,
			Message:     fmt.Sprintf("invalid URL %q", target),
			Remediation: "set PUBLIC_URL to a full URL like https://breadbox.example.com",
		}
	}

	host := u.Hostname()
	if _, err := net.LookupHost(host); err != nil {
		return doctorCheck{
			Name:        "public url",
			Status:      doctorStatusFail,
			Message:     fmt.Sprintf("DNS lookup for %s failed: %v", host, err),
			Remediation: "verify your DNS A/AAAA records point at this host",
		}
	}

	readyURL := strings.TrimRight(target, "/") + "/health/ready"
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(readyURL)
	if err != nil {
		return doctorCheck{
			Name:        "public url",
			Status:      doctorStatusFail,
			Message:     fmt.Sprintf("GET %s failed: %v", readyURL, err),
			Remediation: "ensure the server is running and reachable from this host, or pass --skip-external",
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return doctorCheck{
			Name:        "public url",
			Status:      doctorStatusFail,
			Message:     fmt.Sprintf("%s returned HTTP %d", readyURL, resp.StatusCode),
			Remediation: "check server logs for why /health/ready is failing",
		}
	}
	return doctorCheck{
		Name:    "public url",
		Status:  doctorStatusPass,
		Message: fmt.Sprintf("DNS resolves, %s → %d", readyURL, resp.StatusCode),
	}
}

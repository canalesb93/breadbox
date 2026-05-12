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

	"breadbox/internal/cli/config"
	"breadbox/internal/cli/output"
	"breadbox/internal/client"
	bbconfig "breadbox/internal/config"
	"breadbox/internal/crypto"
	"breadbox/internal/db"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
	"github.com/robfig/cron/v3"
	"github.com/spf13/cobra"
)

// Check statuses for the doctor command.
const (
	doctorStatusPass = "pass"
	doctorStatusFail = "fail"
	doctorStatusSkip = "skip"
	doctorStatusWarn = "warn"
)

// doctorCheck is one row of the local-mode preflight report.
type doctorCheck struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

type doctorReport struct {
	Checks []doctorCheck `json:"checks"`
	OK     bool          `json:"ok"`
}

// AddDoctorCmd registers `breadbox doctor`. The command has two modes:
//
//   - **Local** (default when no --host is configured): runs the same
//     environment + DB + provider preflight checks the legacy
//     `breadbox doctor` did before the cobra port. No HTTP call.
//   - **Remote** (when --host is set or BREADBOX_HOST is in env): calls
//     `GET /api/v1/headless/bootstrap` and renders a readable report.
//
// The brief asked for the remote mode; the local mode is preserved because
// it's the only way to validate ENCRYPTION_KEY / provider creds without a
// running server, and `breadbox doctor` is documented in install scripts.
func AddDoctorCmd(root *cobra.Command) {
	var skipExternal bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Health check: local environment or remote /headless/bootstrap",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := Flags(cmd)
			// Remote mode if the user explicitly named a host. Local mode
			// otherwise — there's no harm running both, but defaulting to
			// remote would surprise install-script callers who relied on
			// the historical local behavior.
			if flags.Host != "" || hostConfigured() {
				return runDoctorRemote(cmd.Context(), flags)
			}
			return runDoctorLocal(cmd.Context(), flags.JSON, skipExternal)
		},
	}
	cmd.Flags().BoolVar(&skipExternal, "skip-external", false, "skip DNS/HTTP reachability checks (local-mode only)")
	root.AddCommand(cmd)
}

// hostConfigured reports whether any host exists in hosts.toml — used to
// decide whether `breadbox doctor` (no flags) defaults to local or remote.
func hostConfigured() bool {
	h, err := config.Load()
	if err != nil {
		return false
	}
	return len(h.Hosts) > 0
}

// runDoctorRemote calls the HTTP bootstrap endpoint and renders the report.
func runDoctorRemote(ctx context.Context, flags *FlagBag) error {
	hosts, err := config.Load()
	if err != nil {
		return fmt.Errorf("load hosts config: %w", err)
	}
	host, name, err := hosts.Get(flags.Host)
	if err != nil {
		return err
	}
	c := client.New(host, flags.Version)

	resp, err := c.HeadlessBootstrap(ctx)
	if err != nil {
		return err
	}

	if flags.JSON || flags.NDJSON {
		return output.PrintJSON(os.Stdout, resp)
	}
	renderRemoteDoctorReport(os.Stdout, name, host.BaseURL, resp)
	return nil
}

// renderRemoteDoctorReport prints a human-readable summary of the
// /api/v1/headless/bootstrap payload. Generic-map iteration is fine here
// — the shape is small and stable.
func renderRemoteDoctorReport(w *os.File, hostName, baseURL string, resp map[string]any) {
	fmt.Fprintf(w, "breadbox doctor — %s (%s)\n\n", hostName, baseURL)

	get := func(k string) any { return resp[k] }
	getBool := func(k string) bool { v, _ := resp[k].(bool); return v }
	getInt := func(k string) int64 {
		switch x := resp[k].(type) {
		case float64:
			return int64(x)
		case int64:
			return x
		}
		return 0
	}

	dbRaw, _ := get("database").(map[string]any)
	dbConn, _ := dbRaw["connected"].(bool)
	migCurrent, _ := dbRaw["migrations_current"].(bool)
	var migVer int64
	if v, ok := dbRaw["migration_version"].(float64); ok {
		migVer = int64(v)
	}

	mark := func(ok bool) string {
		if ok {
			return "ok  "
		}
		return "FAIL"
	}

	fmt.Fprintf(w, "  %s server reachable (v%v)\n", mark(true), get("version"))
	fmt.Fprintf(w, "  %s database connected\n", mark(dbConn))
	fmt.Fprintf(w, "  %s migrations current (v%d)\n", mark(migCurrent), migVer)
	fmt.Fprintf(w, "  %s encryption key set\n", mark(getBool("encryption_key_set")))
	fmt.Fprintf(w, "  -- %d household user(s)\n", getInt("users_count"))
	fmt.Fprintf(w, "  -- %d login account(s)\n", getInt("login_accounts_count"))
	fmt.Fprintf(w, "  -- %d API key(s)\n", getInt("api_keys_count"))
	fmt.Fprintf(w, "  -- %d active connection(s)\n", getInt("active_connections_count"))
	fmt.Fprintf(w, "  %s scheduler running\n", mark(getBool("scheduler_running")))

	provs, _ := get("providers").([]any)
	if len(provs) == 0 {
		fmt.Fprintln(w, "  !! no providers configured")
	} else {
		for _, p := range provs {
			pm, _ := p.(map[string]any)
			name, _ := pm["name"].(string)
			conf, _ := pm["configured"].(bool)
			env, _ := pm["env"].(string)
			label := "configured"
			if !conf {
				label = "not configured"
			} else if env != "" {
				label = fmt.Sprintf("configured (%s)", env)
			}
			fmt.Fprintf(w, "  %s provider %s — %s\n", mark(conf), name, label)
		}
	}

	if getBool("first_run") {
		fmt.Fprintln(w, "\nfirst_run = true: no login accounts yet; visit /setup or run `breadbox create-admin`.")
	}
}

// ----- Local mode (unchanged from cmd/breadbox/doctor.go) -----

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

	return emitDoctor(checks, jsonOut)
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

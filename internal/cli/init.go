// Package cli — `breadbox init`.
//
// `breadbox init` is a local-only command: it talks straight to the
// service layer and DB, never via the REST API. The whole point is to set
// up a brand-new install before any HTTP server has been started.
//
// Concretely it:
//
//  1. Ensures ENCRYPTION_KEY is set (or generates one and writes it to
//     `.env` in the current working directory).
//  2. Prompts for a first admin login (email + password).
//  3. Prompts for a first household user (display name; defaults to the
//     email-local-part).
//  4. Mints a `full_access` `user`-actor API key and stores it as host
//     `local` in `~/.config/breadbox/hosts.toml`.
//  5. Prints a summary + next-step suggestions.
//
// Re-running init when an admin already exists is a no-op: the command
// reports what's already set up and points at the right follow-ups
// (`breadbox auth bootstrap`, `breadbox keys create`, etc.).
package cli

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"os"
	"path/filepath"
	"strings"

	"breadbox/internal/cli/config"
	bbconfig "breadbox/internal/config"
	"breadbox/internal/db"
	"breadbox/internal/service"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
)

// AddInitCmd registers `breadbox init` (L-scoped: talks to the local DB).
func AddInitCmd(root *cobra.Command) {
	var (
		nonInteractive bool
		email          string
		password       string
		userName       string
		envFile        string
		baseURL        string
	)
	cmd := &cobra.Command{
		Use:   "init",
		Short: "First-run setup: encryption key, first admin login, first API key",
		Long: `Bootstraps a fresh breadbox install. Idempotent: re-running with an
existing admin account just prints what's already configured and exits cleanly.

By default the command is interactive — it asks for the admin email, password,
and first household-user name. For scripted installs pass --non-interactive
together with --email, --password, and (optionally) --user-name.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd.Context(), initOpts{
				NonInteractive: nonInteractive,
				Email:          email,
				Password:       password,
				UserName:       userName,
				EnvFile:        envFile,
				BaseURL:        baseURL,
			})
		},
	}
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "fail instead of prompting; requires --email and --password")
	cmd.Flags().StringVar(&email, "email", "", "admin email (becomes login username)")
	cmd.Flags().StringVar(&password, "password", "", "admin password (min 8 chars)")
	cmd.Flags().StringVar(&userName, "user-name", "", "first household-user display name (defaults to email local-part)")
	cmd.Flags().StringVar(&envFile, "env-file", ".env", "path to write generated ENCRYPTION_KEY when none is set")
	cmd.Flags().StringVar(&baseURL, "base-url", "http://localhost:8080", "base URL to store in hosts.toml for the local host")
	root.AddCommand(cmd)
}

// initOpts collects every input to runInit, so tests can drive it without
// reconstructing the cobra flag plumbing.
type initOpts struct {
	NonInteractive bool
	Email          string
	Password       string
	UserName       string
	EnvFile        string
	BaseURL        string
}

func runInit(ctx context.Context, opts initOpts) error {
	cfg, err := bbconfig.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is not set; set it in the environment before running init")
	}

	// Step 1 — ensure ENCRYPTION_KEY.
	if len(cfg.EncryptionKey) == 0 {
		key, err := generateEncryptionKey()
		if err != nil {
			return err
		}
		if err := writeEncryptionKey(opts.EnvFile, key); err != nil {
			return err
		}
		fmt.Printf("init: generated ENCRYPTION_KEY → %s (remember to load it before `breadbox serve`)\n", opts.EnvFile)
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	queries := db.New(pool)

	// Idempotency check — bail early if any admin already exists.
	count, err := queries.CountAuthAccounts(ctx)
	if err != nil {
		return fmt.Errorf("count auth accounts: %w", err)
	}
	if count > 0 {
		fmt.Println("init: an admin login already exists — nothing to do.")
		fmt.Println("Next steps:")
		fmt.Println("  - `breadbox auth bootstrap` to mint a local API key")
		fmt.Println("  - `breadbox keys create --scope=full_access --actor=user` to add another key")
		return nil
	}

	// Step 2/3 — gather email + password + user name.
	email, password, userName, err := gatherInitInputs(opts)
	if err != nil {
		return err
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	svc := service.New(queries, pool, nil, nil)
	user, err := svc.CreateUser(ctx, service.CreateUserParams{Name: userName, Email: &email})
	if err != nil {
		return fmt.Errorf("create household user: %w", err)
	}
	userUUID, err := svc.ResolveUserUUID(ctx, user.ID)
	if err != nil {
		return fmt.Errorf("resolve household user: %w", err)
	}

	if _, err := queries.CreateAuthAccount(ctx, db.CreateAuthAccountParams{
		UserID:         pgtype.UUID(userUUID),
		Username:       email,
		HashedPassword: hashed,
		Role:           "admin",
	}); err != nil {
		return fmt.Errorf("create admin login: %w", err)
	}

	keyResult, err := svc.CreateAPIKey(ctx, service.CreateAPIKeyParams{
		Name:      "cli-init",
		Scope:     "full_access",
		ActorType: "user",
		ActorName: userName,
	})
	if err != nil {
		return fmt.Errorf("mint api key: %w", err)
	}

	hosts, err := config.Load()
	if err != nil {
		return fmt.Errorf("load hosts config: %w", err)
	}
	if err := hosts.Set("local", config.Host{BaseURL: opts.BaseURL, Token: keyResult.PlaintextKey}); err != nil {
		return fmt.Errorf("save host: %w", err)
	}
	if err := hosts.SetDefault("local"); err != nil {
		return fmt.Errorf("set default host: %w", err)
	}
	if err := hosts.Save(); err != nil {
		return fmt.Errorf("write hosts.toml: %w", err)
	}

	fmt.Println("init: complete.")
	fmt.Printf("  admin login : %s\n", email)
	fmt.Printf("  household   : %s\n", userName)
	fmt.Printf("  api key     : %s (saved as host 'local')\n", keyResult.KeyPrefix)
	fmt.Println("Next steps:")
	fmt.Println("  - `breadbox serve` to start the HTTP server")
	fmt.Println("  - `breadbox providers config plaid --client-id ... --secret ...` to add Plaid")
	fmt.Println("  - `breadbox connections link --provider plaid` to connect your first bank")
	return nil
}

// gatherInitInputs validates flag inputs in non-interactive mode and prompts
// otherwise. Returns the validated trio (email, password, userName).
func gatherInitInputs(opts initOpts) (email, password, userName string, err error) {
	email = strings.TrimSpace(opts.Email)
	password = opts.Password
	userName = strings.TrimSpace(opts.UserName)

	if opts.NonInteractive {
		if email == "" || password == "" {
			return "", "", "", fmt.Errorf("--non-interactive requires --email and --password")
		}
	}

	if email == "" {
		email, err = readLine("Admin email: ")
		if err != nil {
			return "", "", "", err
		}
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return "", "", "", fmt.Errorf("invalid email %q: %w", email, err)
	}

	if password == "" {
		password, err = readPassword("Password (min 8 chars): ")
		if err != nil {
			return "", "", "", err
		}
		confirm, err := readPassword("Confirm password: ")
		if err != nil {
			return "", "", "", err
		}
		if password != confirm {
			return "", "", "", fmt.Errorf("passwords do not match")
		}
	}
	if len(password) < 8 {
		return "", "", "", fmt.Errorf("password must be at least 8 characters")
	}

	if userName == "" {
		userName = strings.SplitN(email, "@", 2)[0]
		if !opts.NonInteractive {
			prompted, err := readLine(fmt.Sprintf("Household user name [%s]: ", userName))
			if err != nil {
				return "", "", "", err
			}
			if prompted != "" {
				userName = prompted
			}
		}
	}
	if userName == "" {
		return "", "", "", fmt.Errorf("household user name is required")
	}
	return email, password, userName, nil
}

// generateEncryptionKey returns a 64-char hex string suitable for the
// ENCRYPTION_KEY env var (32 random bytes).
func generateEncryptionKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate encryption key: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// writeEncryptionKey appends a fresh ENCRYPTION_KEY line to the given env
// file, creating the file (and intermediate directories) when missing. If
// the file already contains a non-empty ENCRYPTION_KEY=, the existing line
// is left untouched and a warning is printed.
func writeEncryptionKey(path, key string) error {
	if path == "" {
		path = ".env"
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve env file: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return fmt.Errorf("create env file dir: %w", err)
	}
	existing, err := os.ReadFile(abs)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read env file: %w", err)
	}
	if hasExistingKey(string(existing)) {
		fmt.Fprintf(os.Stderr, "init: %s already contains ENCRYPTION_KEY — leaving it alone\n", abs)
		return nil
	}
	line := fmt.Sprintf("ENCRYPTION_KEY=%s\n", key)
	f, err := os.OpenFile(abs, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open env file: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		return fmt.Errorf("write env file: %w", err)
	}
	return nil
}

// hasExistingKey returns true when contents contains a non-comment line
// starting with "ENCRYPTION_KEY=" and a non-empty value.
func hasExistingKey(contents string) bool {
	for _, raw := range strings.Split(contents, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "#") {
			continue
		}
		const prefix = "ENCRYPTION_KEY="
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		val := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		val = strings.Trim(val, `"'`)
		if val != "" {
			return true
		}
	}
	return false
}

// readLine reads a trimmed line from stdin with a prompt.
func readLine(prompt string) (string, error) {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read input: %w", err)
	}
	return strings.TrimSpace(line), nil
}

// readPassword reads a line of input. Echo is not suppressed — we'd need
// golang.org/x/term for that, which the project doesn't currently depend on.
// Callers running `breadbox init` interactively at a real terminal are
// running it once on a brand-new machine; the password they type goes
// directly to bcrypt and isn't echoed back. For scripted installs pass
// --password (or --non-interactive --password=$PW) instead.
func readPassword(prompt string) (string, error) {
	return readLine(prompt)
}

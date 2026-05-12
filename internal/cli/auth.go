package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"

	"breadbox/internal/cli/config"
	"breadbox/internal/cli/output"
	"breadbox/internal/client"
	bbconfig "breadbox/internal/config"
	"breadbox/internal/db"
	"breadbox/internal/service"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

// AddAuthCmd wires `breadbox auth` and its leaf commands. None of the
// children are marked as RequiresHost because their job is to set the
// host config up in the first place (or to operate against an
// explicitly-passed host).
func AddAuthCmd(root *cobra.Command) {
	auth := &cobra.Command{
		Use:   "auth",
		Short: "Manage breadbox host credentials and identity",
	}

	auth.AddCommand(newAuthBootstrapCmd())
	auth.AddCommand(newAuthLoginCmd())
	auth.AddCommand(newAuthLogoutCmd())
	auth.AddCommand(newAuthStatusCmd())
	auth.AddCommand(newAuthUseCmd())
	auth.AddCommand(newAuthWhoamiCmd())

	root.AddCommand(auth)
}

// newAuthBootstrapCmd: local-only, mints a `user`-typed key via the
// service layer and stores it under the `local` host in hosts.toml.
func newAuthBootstrapCmd() *cobra.Command {
	var baseURL string
	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Mint a local admin key directly via the DB and save as host 'local'",
		Long: `Bootstrap a CLI session against a local breadbox install without going through ` +
			`the dashboard's first-run flow. Requires DATABASE_URL to be reachable. Idempotent: ` +
			`if a working 'local' host already exists in hosts.toml, it is left alone.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthBootstrap(cmd.Context(), Flags(cmd).Version, baseURL)
		},
	}
	cmd.Flags().StringVar(&baseURL, "base-url", "http://localhost:8080", "base URL to store in hosts.toml")
	return cmd
}

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

// newAuthLoginCmd wires `breadbox auth login`. Two modes:
//
//   - `--token` paste flow (no browser round-trip; ideal for headless
//     pipelines that already have a key handy).
//   - Interactive device-code flow (default when --token is omitted) —
//     the CLI calls /api/v1/auth/device-code and polls for approval
//     while the operator visits the verification_url in a browser.
func newAuthLoginCmd() *cobra.Command {
	var token string
	var hostURL string
	var name string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Add a host to hosts.toml; paste a token with --token or run the device-code flow interactively",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthLogin(cmd.Context(), Flags(cmd).Version, hostURL, token, name)
		},
	}
	cmd.Flags().StringVar(&hostURL, "host", "", "base URL of the breadbox instance")
	cmd.Flags().StringVar(&token, "token", "", "API key (bb_...) to paste; without it, the device-code flow runs interactively")
	cmd.Flags().StringVar(&name, "name", "", "name to store the host under (defaults to the URL hostname)")
	return cmd
}

func runAuthLogin(ctx context.Context, version, hostURL, token, name string) error {
	if hostURL == "" {
		return UsageErrorf("--host is required")
	}

	if name == "" {
		name = deriveHostName(hostURL)
	}

	if token == "" {
		// Device-code flow — open a session against the host without a
		// token, walk the polling loop, then drop the resulting bb_...
		// into hosts.toml just like the paste mode would.
		c := client.New(config.Host{BaseURL: hostURL}, version)
		minted, err := runDeviceCodeFlow(ctx, c)
		if err != nil {
			return err
		}
		token = minted
	}

	host := config.Host{BaseURL: hostURL, Token: token}

	// Validate the token before saving by hitting /version.
	c := client.New(host, version)
	if _, err := c.Version(ctx); err != nil {
		return fmt.Errorf("validate token: %w", err)
	}

	hosts, err := config.Load()
	if err != nil {
		return err
	}
	if err := hosts.Set(name, host); err != nil {
		return err
	}
	if err := hosts.Save(); err != nil {
		return err
	}
	fmt.Printf("auth login: saved host %q (%s)\n", name, hostURL)
	if hosts.Default == name {
		fmt.Println("set as default host.")
	}
	return nil
}

// deviceCodePollIntervalOverride lets tests collapse the polling sleep
// to a tiny value without having to fiddle with the server-reported
// `interval`. Production code path never sets this — the CLI always
// honors the server's advertised interval (with a 2s floor).
var deviceCodePollIntervalOverride time.Duration

// runDeviceCodeFlow drives the device-code dance against the configured
// host. On a successful approval it returns the plaintext bb_... token;
// on any other terminal status (expired, denied) it returns a typed
// error the caller maps to the right exit code.
func runDeviceCodeFlow(ctx context.Context, c *client.Client) (string, error) {
	init, err := c.InitiateDeviceCode(ctx)
	if err != nil {
		return "", fmt.Errorf("initiate device-code: %w", err)
	}

	// Everything except the final token write happens on stderr so
	// stdout stays free for a future `--json`-style mode and CI logs
	// don't interleave the polling dots with the saved-host receipt.
	fmt.Fprintf(os.Stderr, "Visit %s\n", init.VerificationURL)
	fmt.Fprintf(os.Stderr, "Enter code: %s\n\n", init.UserCode)
	fmt.Fprint(os.Stderr, "Waiting")

	interval := time.Duration(init.Interval) * time.Second
	if interval <= 0 {
		interval = 2 * time.Second
	}
	if deviceCodePollIntervalOverride > 0 {
		interval = deviceCodePollIntervalOverride
	}

	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(os.Stderr)
			return "", ctx.Err()
		case <-time.After(interval):
		}
		res, err := c.PollDeviceCode(ctx, init.DeviceCode)
		if err != nil {
			fmt.Fprintln(os.Stderr)
			var apiErr *client.APIError
			if errors.As(err, &apiErr) {
				switch apiErr.Code {
				case "EXPIRED":
					return "", &deviceCodeExpiredError{}
				case "DENIED":
					return "", &deviceCodeDeniedError{}
				}
			}
			return "", fmt.Errorf("poll device-code: %w", err)
		}
		if res.Status == "approved" {
			fmt.Fprintln(os.Stderr, " ok")
			fmt.Fprintln(os.Stderr, "auth login: device approved on the server")
			return res.Token, nil
		}
		// pending — pulse the dot and keep going.
		fmt.Fprint(os.Stderr, ".")
	}
}

// deviceCodeExpiredError maps to exit code 4 (upstream) so agents can
// distinguish "user denied" from "user took too long".
type deviceCodeExpiredError struct{}

func (*deviceCodeExpiredError) Error() string {
	return "device code expired before approval; re-run `breadbox auth login --host=...`"
}

// deviceCodeDeniedError maps to exit code 3 (auth) — the operator
// actively refused this CLI invocation.
type deviceCodeDeniedError struct{}

func (*deviceCodeDeniedError) Error() string {
	return "device code denied by the operator"
}

// deriveHostName turns a URL into a stable short name. Falls back to the
// raw URL if parsing fails.
func deriveHostName(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	h := u.Hostname()
	if h == "localhost" || h == "127.0.0.1" {
		return "local"
	}
	return h
}

func newAuthLogoutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout [name]",
		Short: "Remove a host from hosts.toml",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := Flags(cmd)
			name := flags.Host
			if len(args) > 0 {
				name = args[0]
			}
			hosts, err := config.Load()
			if err != nil {
				return err
			}
			if name == "" {
				name = hosts.Default
			}
			if name == "" {
				return UsageErrorf("pass a host name or set BREADBOX_HOST")
			}
			if err := hosts.Remove(name); err != nil {
				return err
			}
			if err := hosts.Save(); err != nil {
				return err
			}
			fmt.Printf("auth logout: removed host %q\n", name)
			return nil
		},
	}
	return cmd
}

func newAuthStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "List configured hosts and the current default",
		RunE: func(cmd *cobra.Command, args []string) error {
			hosts, err := config.Load()
			if err != nil {
				return err
			}
			flags := Flags(cmd)
			if flags.JSON || flags.NDJSON {
				return output.PrintJSON(os.Stdout, hosts)
			}
			if len(hosts.Hosts) == 0 {
				fmt.Println("no hosts configured. run `breadbox auth bootstrap` or `breadbox auth login --host=URL --token=KEY`")
				return nil
			}
			tbl := output.NewTable(os.Stdout, []string{"NAME", "BASE_URL", "DEFAULT"})
			for _, n := range hosts.Names() {
				h := hosts.Hosts[n]
				marker := ""
				if n == hosts.Default {
					marker = "*"
				}
				tbl.AddRow(n, h.BaseURL, marker)
			}
			return tbl.Flush()
		},
	}
	return cmd
}

func newAuthUseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "use <name>",
		Short: "Set the default host",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hosts, err := config.Load()
			if err != nil {
				return err
			}
			if err := hosts.SetDefault(args[0]); err != nil {
				return err
			}
			if err := hosts.Save(); err != nil {
				return err
			}
			fmt.Printf("auth use: default host set to %q\n", args[0])
			return nil
		},
	}
	return cmd
}

func newAuthWhoamiCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Print the API key identity for the configured host",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := Flags(cmd)
			hosts, err := config.Load()
			if err != nil {
				return err
			}
			host, hostName, err := hosts.Get(flags.Host)
			if err != nil {
				if errors.Is(err, config.ErrNoHosts) {
					return fmt.Errorf("%w: run `breadbox auth bootstrap` first", err)
				}
				return err
			}
			c := client.New(host, flags.Version)
			who, err := c.Whoami(cmd.Context())
			if err != nil {
				return err
			}
			if flags.JSON || flags.NDJSON {
				return output.PrintJSON(os.Stdout, who)
			}
			actorName := ""
			if who.ActorName != nil {
				actorName = *who.ActorName
			}
			fmt.Printf("host:        %s (%s)\n", hostName, host.BaseURL)
			fmt.Printf("key prefix:  %s\n", who.KeyPrefix)
			fmt.Printf("scope:       %s\n", who.Scope)
			fmt.Printf("actor type:  %s\n", who.ActorType)
			if actorName != "" {
				fmt.Printf("actor name:  %s\n", actorName)
			}
			fmt.Printf("created at:  %s\n", who.CreatedAt)
			return nil
		},
	}
	return cmd
}


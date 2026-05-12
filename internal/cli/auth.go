package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"

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

// newAuthLoginCmd: paste-mode `--token` is fully supported in this PR;
// interactive device-code is a Stage-2 deliverable and surfaces a friendly
// error here so users know what to expect.
func newAuthLoginCmd() *cobra.Command {
	var token string
	var hostURL string
	var name string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Add a host to hosts.toml; --token for paste mode (device-code: Stage 2)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthLogin(cmd.Context(), Flags(cmd).Version, hostURL, token, name)
		},
	}
	cmd.Flags().StringVar(&hostURL, "host", "", "base URL of the breadbox instance")
	cmd.Flags().StringVar(&token, "token", "", "API key (bb_...) to paste; without it, device-code is planned for Stage 2")
	cmd.Flags().StringVar(&name, "name", "", "name to store the host under (defaults to the URL hostname)")
	return cmd
}

func runAuthLogin(ctx context.Context, version, hostURL, token, name string) error {
	if hostURL == "" {
		return UsageErrorf("--host is required")
	}
	if token == "" {
		fmt.Fprintln(os.Stderr, "interactive device-code login lands in Stage 2; use `breadbox auth login --host=URL --token=KEY` for now")
		return UsageErrorf("--token is required (device-code: Stage 2)")
	}

	if name == "" {
		name = deriveHostName(hostURL)
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


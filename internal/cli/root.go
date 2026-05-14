// Package cli implements the breadbox cobra command tree. cmd/breadbox is
// a thin shim that just calls NewRootCmd(version).Execute() and maps the
// resulting error to an exit code via MapExitCode.
//
// Standard persistent flags (`--host`, `--json`, `--ndjson`, `--fields`,
// `--limit`, `--all`, `--quiet`, `--debug`) live on the root and are read
// by subcommands through the FlagBag stashed on the command context.
//
// Subcommands fall into two buckets:
//   - L-scope (server, migrate, mcp, seed, create-admin, reset-password,
//     init, backup): talk to the local DB. Skip the host requirement.
//   - R/W-scope (auth whoami, doctor, all Stage-2+ data commands):
//     resolve a *client.Client via the persistent pre-run.
//
// Each subcommand registers itself via a small Add*Cmd(parent, ...) func.
// Bodies of the ported `runServe`/`runMigrate`/etc. live in cli/*.go so
// cmd/breadbox stays a one-screen file.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"breadbox/internal/cli/config"
	"breadbox/internal/client"

	"github.com/spf13/cobra"
)

// FlagBag carries the resolved values of the persistent flags. Subcommands
// pull it out of the cobra context to avoid threading nine flag pointers
// through every helper.
type FlagBag struct {
	Host    string
	JSON    bool
	NDJSON  bool
	Fields  string
	Limit   int
	All     bool
	Quiet   bool
	Debug   bool
	Version string
}

type ctxKey int

const (
	ctxKeyFlags ctxKey = iota
	ctxKeyClient
	ctxKeyHostName
)

// Flags returns the resolved FlagBag from the cobra command's context.
// Panics if the bag isn't set — which would mean cmd was constructed
// outside NewRootCmd and is a programming error.
func Flags(cmd *cobra.Command) *FlagBag {
	v, ok := cmd.Context().Value(ctxKeyFlags).(*FlagBag)
	if !ok {
		panic("cli: FlagBag missing from command context")
	}
	return v
}

// ClientFromContext returns the resolved REST client. Subcommands that
// declare RequiresHost: true via a cobra annotation are guaranteed a
// non-nil client; everyone else should nil-check.
func ClientFromContext(ctx context.Context) (*client.Client, string) {
	c, _ := ctx.Value(ctxKeyClient).(*client.Client)
	name, _ := ctx.Value(ctxKeyHostName).(string)
	return c, name
}

// withClient stuffs a resolved client into the command's context so the
// downstream RunE doesn't have to re-resolve hosts.
func withClient(ctx context.Context, c *client.Client, hostName string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyClient, c)
	ctx = context.WithValue(ctx, ctxKeyHostName, hostName)
	return ctx
}

// Annotation key for subcommands that require a configured host. Set this
// to "true" via cmd.Annotations so PersistentPreRunE knows to resolve a
// client and 3-exit if none is configured.
const annotRequiresHost = "breadbox.requires_host"

// MarkRequiresHost flags a command (and its children) as needing a
// configured host. Used by data commands; skipped by `serve`, `migrate`,
// `mcp`, etc.
func MarkRequiresHost(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[annotRequiresHost] = "true"
}

// requiresHost looks up the annotation, walking parents as a courtesy so a
// `breadbox auth ...` subtree can be tagged once on the auth group.
func requiresHost(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Annotations[annotRequiresHost] == "true" {
			return true
		}
	}
	return false
}

// NewRootCmd builds the full breadbox cobra command tree. version is the
// build-time version string the binary was linked with.
func NewRootCmd(version string) *cobra.Command {
	flags := &FlagBag{Version: version}

	root := &cobra.Command{
		Use:           "breadbox",
		Short:         "Self-hosted financial data aggregator",
		Long:          "Breadbox runs as a server, an MCP stdio host, or a CLI driving a local or remote breadbox over HTTP.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.WithValue(cmd.Context(), ctxKeyFlags, flags)

			if !requiresHost(cmd) {
				cmd.SetContext(ctx)
				return nil
			}

			hosts, err := config.Load()
			if err != nil {
				return fmt.Errorf("load hosts config: %w", err)
			}
			host, name, err := hosts.Get(flags.Host)
			if err != nil {
				if errors.Is(err, config.ErrNoHosts) {
					return fmt.Errorf("%w: run `breadbox auth bootstrap` (local) or `breadbox auth login --host=URL --token=...`", err)
				}
				return err
			}
			c := client.New(host, version)
			cmd.SetContext(withClient(ctx, c, name))
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	root.PersistentFlags().StringVar(&flags.Host, "host", os.Getenv("BREADBOX_HOST"), "host name from hosts.toml (defaults to BREADBOX_HOST or the configured default)")
	root.PersistentFlags().BoolVar(&flags.JSON, "json", false, "emit JSON output (default when stdout is not a TTY)")
	root.PersistentFlags().BoolVar(&flags.NDJSON, "ndjson", false, "emit NDJSON streaming output")
	root.PersistentFlags().StringVar(&flags.Fields, "fields", "", "comma-separated field selection passed through to the API")
	root.PersistentFlags().IntVar(&flags.Limit, "limit", 0, "max rows for list commands (0 = server default)")
	root.PersistentFlags().BoolVar(&flags.All, "all", false, "walk every page for list commands")
	root.PersistentFlags().BoolVar(&flags.Quiet, "quiet", false, "suppress non-essential stderr output")
	root.PersistentFlags().BoolVar(&flags.Debug, "debug", false, "verbose stderr logging")

	// R/W-scope commands — all use the HTTP client. These are always
	// registered, regardless of build tag.
	AddAuthCmd(root)
	AddDoctorCmd(root)
	AddAccountsCmd(root)
	AddTransactionsCmd(root)
	AddUsersCmd(root)
	AddLoginsCmd(root)
	AddKeysCmd(root)
	AddProvidersCmd(root)
	AddConfigCmd(root)
	AddWebhooksCmd(root)
	AddCategoriesCmd(root)
	AddTagsCmd(root)
	AddRulesCmd(root)
	AddReportsCmd(root)
	AddConnectionsCmd(root)
	AddSyncCmd(root)
	AddCSVCmd(root)
	AddVersionCmd(root, version)

	// L-scope commands (serve, migrate, mcp-stdio, seed, create-admin,
	// reset-password, init, backup) only register under non-lite builds —
	// they need internal/db, internal/service, etc. which are stripped
	// from CLI-only binaries.
	registerServerCommands(root)

	return root
}

package cli

import (
	"fmt"
	"os"

	"breadbox/internal/cli/output"
	"breadbox/internal/client"

	"github.com/spf13/cobra"
)

// AddKeysCmd wires `breadbox keys` — API key minting and revocation. Even
// listing is sensitive (prefix + actor metadata reveal who's connected), so
// the server gates the whole subtree behind full_access.
func AddKeysCmd(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "Manage API keys",
	}
	MarkRequiresHost(cmd)
	cmd.AddCommand(newKeysListCmd())
	cmd.AddCommand(newKeysCreateCmd())
	cmd.AddCommand(newKeysRevokeCmd())
	root.AddCommand(cmd)
}

func newKeysListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List API keys",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			keys, err := c.ListAPIKeys(cmd.Context())
			if err != nil {
				return err
			}
			return renderKeys(cmd, keys)
		},
	}
}

func newKeysCreateCmd() *cobra.Command {
	var scope, actor, name string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Mint a new API key",
		Long: `Mints a new API key on the server. The plaintext is printed to stdout
exactly once and can never be recovered — copy it from the create
response or store it via your favourite secret manager immediately.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return UsageErrorf("--name is required")
			}
			if scope == "" {
				scope = "full_access"
			}
			if scope != "full_access" && scope != "read_only" {
				return UsageErrorf("--scope must be one of: full_access, read_only")
			}
			if actor == "" {
				actor = "agent"
			}
			if actor != "user" && actor != "agent" && actor != "system" {
				return UsageErrorf("--actor must be one of: user, agent, system")
			}
			c, _ := ClientFromContext(cmd.Context())
			result, err := c.CreateAPIKey(cmd.Context(), client.CreateAPIKeyRequest{
				Name:      name,
				Scope:     scope,
				ActorType: actor,
				ActorName: name,
			})
			if err != nil {
				return err
			}
			return renderKeyCreate(cmd, result)
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "full_access", "scope: full_access or read_only")
	cmd.Flags().StringVar(&actor, "actor", "agent", "actor type: user, agent, or system")
	cmd.Flags().StringVar(&name, "name", "", "human-readable label for the key (required)")
	return cmd
}

func newKeysRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <id>",
		Short: "Revoke an API key by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			if err := c.RevokeAPIKey(cmd.Context(), args[0]); err != nil {
				return err
			}
			flags := Flags(cmd)
			if !flags.Quiet {
				fmt.Fprintf(os.Stderr, "revoked key %s\n", args[0])
			}
			return nil
		},
	}
}

func renderKeys(cmd *cobra.Command, keys []client.APIKey) error {
	flags := Flags(cmd)
	mode := output.Resolve(flags.JSON, flags.NDJSON, os.Stdout)
	switch mode {
	case output.ModeJSON:
		return output.PrintJSON(os.Stdout, keys)
	case output.ModeNDJSON:
		items := make([]any, len(keys))
		for i, k := range keys {
			items[i] = k
		}
		return output.PrintNDJSON(os.Stdout, items)
	default:
		tbl := output.NewTable(os.Stdout, []string{"ID", "PREFIX", "NAME", "SCOPE", "ACTOR_TYPE", "ACTOR_NAME", "REVOKED"})
		for _, k := range keys {
			actorName := ""
			if k.ActorName != nil {
				actorName = *k.ActorName
			}
			revoked := ""
			if k.RevokedAt != nil {
				revoked = *k.RevokedAt
			}
			tbl.AddRow(k.ID, k.KeyPrefix, k.Name, k.Scope, k.ActorType, actorName, revoked)
		}
		return tbl.Flush()
	}
}

// renderKeyCreate prints the new key. The plaintext lands on stdout (one
// time only) so shell pipelines can capture it directly. The "this is your
// only chance" warning goes to stderr so it doesn't pollute the captured
// value.
func renderKeyCreate(cmd *cobra.Command, k *client.CreateAPIKeyResult) error {
	flags := Flags(cmd)
	mode := output.Resolve(flags.JSON, flags.NDJSON, os.Stdout)
	if !flags.Quiet && mode == output.ModeTable {
		fmt.Fprintln(os.Stderr, "API key follows on stdout. This is your only chance to capture it.")
	}
	if mode == output.ModeJSON || mode == output.ModeNDJSON {
		return output.PrintJSON(os.Stdout, k)
	}
	// Print the plaintext alone in table mode so users can pipe to a
	// secret manager (`breadbox keys create --name=... | pbcopy`).
	fmt.Println(k.PlaintextKey)
	return nil
}

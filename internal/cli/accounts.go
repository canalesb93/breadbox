// Package cli — accounts noun group.
//
// `breadbox accounts ...` exposes the household's bank accounts and the
// per-account-link reconciliation surface. Read commands route through
// GET /api/v1/accounts and friends; write commands hit PATCH and the
// /account-links sub-tree. Each subcommand reuses the shared output
// formatter + FlagBag set up by NewRootCmd.
//
// The cobra spec in docs/cli-commands.md describes account-links as
// "user-links" (which household member owns an account). The actual REST
// surface is the account-link reconciliation model (primary ↔ dependent
// account matching), which is what this CLI exposes — the doc text is
// slightly aspirational. See the PR body for the follow-up.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"breadbox/internal/cli/output"
	"breadbox/internal/client"

	"github.com/spf13/cobra"
)

// AddAccountsCmd registers `breadbox accounts` and its children.
func AddAccountsCmd(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "accounts",
		Short: "Manage household bank accounts",
	}
	MarkRequiresHost(cmd)

	cmd.AddCommand(newAccountsListCmd())
	cmd.AddCommand(newAccountsGetCmd())
	cmd.AddCommand(newAccountsDetailCmd())
	cmd.AddCommand(newAccountsUpdateCmd())
	cmd.AddCommand(newAccountsLinksCmd())

	root.AddCommand(cmd)
}

func newAccountsListCmd() *cobra.Command {
	var userID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List household bank accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			accts, err := c.ListAccounts(cmd.Context(), flags.Fields, userID)
			if err != nil {
				return err
			}
			return renderAccountsList(flags, accts)
		},
	}
	cmd.Flags().StringVar(&userID, "user", "", "filter to accounts owned by this user (uuid or short_id)")
	return cmd
}

func newAccountsGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Show a single account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			acct, err := c.GetAccount(cmd.Context(), args[0], flags.Fields)
			if err != nil {
				return err
			}
			return renderAccount(flags, acct)
		},
	}
	return cmd
}

func newAccountsDetailCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "detail <id>",
		Short: "Show full account detail (balances + last 25 transactions)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			detail, err := c.GetAccountDetail(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return renderAccountDetail(flags, detail)
		},
	}
	return cmd
}

func newAccountsUpdateCmd() *cobra.Command {
	var (
		name             string
		nameSet          bool
		excluded         bool
		excludedSet      bool
		dependentLinked  bool
		dependentLinkSet bool
	)
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Patch an account's display name and flags",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Detect which flags the user actually set so we can omit the
			// other fields from the PATCH body.
			f := cmd.Flags()
			if f.Changed("name") {
				nameSet = true
			}
			if f.Changed("excluded") {
				excludedSet = true
			}
			if f.Changed("dependent-linked") {
				dependentLinkSet = true
			}
			if !nameSet && !excludedSet && !dependentLinkSet {
				return UsageErrorf("update needs at least one of --name, --excluded, --dependent-linked")
			}

			patch := client.AccountPatch{}
			if nameSet {
				v := name
				patch.DisplayName = &v
			}
			if excludedSet {
				v := excluded
				patch.IsExcluded = &v
			}
			if dependentLinkSet {
				v := dependentLinked
				patch.IsDependentLinked = &v
			}

			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			acct, err := c.UpdateAccount(cmd.Context(), args[0], patch)
			if err != nil {
				return err
			}
			if flags.Quiet {
				return nil
			}
			return renderAccount(flags, acct)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "set display_name (use empty string to clear)")
	cmd.Flags().BoolVar(&excluded, "excluded", false, "set is_excluded (hide from default views)")
	cmd.Flags().BoolVar(&dependentLinked, "dependent-linked", false, "set is_dependent_linked (exclude from household totals)")
	return cmd
}

// newAccountsLinksCmd wires `breadbox accounts links {list,add,remove}`.
// The server-side model is account-to-account reconciliation links — see
// the package-level comment for context.
func newAccountsLinksCmd() *cobra.Command {
	links := &cobra.Command{
		Use:   "links",
		Short: "Manage account-link reconciliation rows",
	}

	links.AddCommand(&cobra.Command{
		Use:   "list <account-id>",
		Short: "List account-link rows touching this account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			all, err := c.ListAccountLinks(cmd.Context())
			if err != nil {
				return err
			}
			target := args[0]
			filtered := make([]client.AccountLink, 0, len(all))
			for _, l := range all {
				if l.PrimaryAccountID == target || l.DependentAccountID == target ||
					strings.HasPrefix(l.PrimaryAccountID, target) ||
					strings.HasPrefix(l.DependentAccountID, target) {
					filtered = append(filtered, l)
				}
			}
			return renderAccountLinks(flags, filtered)
		},
	})

	add := &cobra.Command{
		Use:   "add <primary-account-id>",
		Short: "Link a dependent account to a primary account for reconciliation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dep, _ := cmd.Flags().GetString("dependent")
			strategy, _ := cmd.Flags().GetString("strategy")
			tolerance, _ := cmd.Flags().GetInt("tolerance-days")
			if dep == "" {
				return UsageErrorf("--dependent is required")
			}
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			link, err := c.CreateAccountLink(cmd.Context(), client.CreateAccountLinkParams{
				PrimaryAccountID:   args[0],
				DependentAccountID: dep,
				MatchStrategy:      strategy,
				MatchToleranceDays: tolerance,
			})
			if err != nil {
				return err
			}
			if flags.Quiet {
				return nil
			}
			return renderAccountLink(flags, link)
		},
	}
	add.Flags().String("dependent", "", "dependent account id (uuid or short_id) — required")
	add.Flags().String("strategy", "date_amount_name", "match strategy (defaults to date_amount_name)")
	add.Flags().Int("tolerance-days", 0, "match tolerance window in days")
	links.AddCommand(add)

	links.AddCommand(&cobra.Command{
		Use:   "remove <link-id>",
		Short: "Delete an account-link row",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			if err := c.DeleteAccountLink(cmd.Context(), args[0]); err != nil {
				return err
			}
			if !flags.Quiet {
				fmt.Println(args[0])
			}
			return nil
		},
	})

	return links
}

// --- rendering helpers ---

func renderAccountsList(flags *FlagBag, accts []client.Account) error {
	switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
	case output.ModeJSON:
		return output.PrintJSON(os.Stdout, accts)
	case output.ModeNDJSON:
		items := make([]any, 0, len(accts))
		for _, a := range accts {
			items = append(items, a)
		}
		return output.PrintNDJSON(os.Stdout, items)
	default:
		tbl := output.NewTable(os.Stdout, []string{"SHORT_ID", "NAME", "TYPE", "BALANCE", "CCY", "STATUS"})
		for _, a := range accts {
			tbl.AddRow(a.ShortID, a.Name, a.Type, formatFloatPtr(a.BalanceCurrent), strPtr(a.IsoCurrencyCode), strPtr(a.ConnectionStatus))
		}
		return tbl.Flush()
	}
}

func renderAccount(flags *FlagBag, acct *client.Account) error {
	switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
	case output.ModeJSON:
		return output.PrintJSON(os.Stdout, acct)
	case output.ModeNDJSON:
		return output.PrintNDJSON(os.Stdout, []any{acct})
	default:
		tbl := output.NewTable(os.Stdout, []string{"SHORT_ID", "NAME", "TYPE", "BALANCE", "CCY", "STATUS"})
		tbl.AddRow(acct.ShortID, acct.Name, acct.Type, formatFloatPtr(acct.BalanceCurrent), strPtr(acct.IsoCurrencyCode), strPtr(acct.ConnectionStatus))
		return tbl.Flush()
	}
}

func renderAccountDetail(flags *FlagBag, d *client.AccountDetail) error {
	// detail responses are dense — JSON is the only reasonable shape.
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(d)
	_ = flags
	return nil
}

func renderAccountLinks(flags *FlagBag, links []client.AccountLink) error {
	switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
	case output.ModeJSON:
		return output.PrintJSON(os.Stdout, links)
	case output.ModeNDJSON:
		items := make([]any, 0, len(links))
		for _, l := range links {
			items = append(items, l)
		}
		return output.PrintNDJSON(os.Stdout, items)
	default:
		tbl := output.NewTable(os.Stdout, []string{"SHORT_ID", "PRIMARY", "DEPENDENT", "STRATEGY", "MATCHED", "UNMATCHED"})
		for _, l := range links {
			tbl.AddRow(l.ShortID, l.PrimaryAccountName, l.DependentAccountName, l.MatchStrategy,
				fmt.Sprintf("%d", l.MatchCount), fmt.Sprintf("%d", l.UnmatchedDependentCount))
		}
		return tbl.Flush()
	}
}

func renderAccountLink(flags *FlagBag, l *client.AccountLink) error {
	switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
	case output.ModeJSON, output.ModeNDJSON:
		return output.PrintJSON(os.Stdout, l)
	default:
		tbl := output.NewTable(os.Stdout, []string{"SHORT_ID", "PRIMARY", "DEPENDENT", "STRATEGY", "MATCHED", "UNMATCHED"})
		tbl.AddRow(l.ShortID, l.PrimaryAccountName, l.DependentAccountName, l.MatchStrategy,
			fmt.Sprintf("%d", l.MatchCount), fmt.Sprintf("%d", l.UnmatchedDependentCount))
		return tbl.Flush()
	}
}

// strPtr renders an optional string field, emitting "-" when nil/empty.
func strPtr(s *string) string {
	if s == nil || *s == "" {
		return "-"
	}
	return *s
}

// formatFloatPtr renders an optional float, returning "-" for nil.
func formatFloatPtr(f *float64) string {
	if f == nil {
		return "-"
	}
	return fmt.Sprintf("%.2f", *f)
}

// _ ensures the context import survives if the build prunes it.
var _ = context.Background

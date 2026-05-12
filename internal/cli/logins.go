package cli

import (
	"fmt"
	"os"

	"breadbox/internal/cli/output"
	"breadbox/internal/client"

	"github.com/spf13/cobra"
)

// AddLoginsCmd wires `breadbox logins` — the admin login-account surface.
// Listing is sensitive (it returns identity fields), so even read-shaped
// verbs require a full_access key server-side.
//
// Note on `logins update`: the underlying service only exposes role as a
// mutable field today. The original CLI spec called for `--email`; that
// would require schema changes (logins.username is unique and rooted in the
// admin auth flow), so this PR exposes `--role` instead. The doc table is
// updated accordingly.
func AddLoginsCmd(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "logins",
		Short: "Manage admin login accounts",
	}
	MarkRequiresHost(cmd)
	cmd.AddCommand(newLoginsListCmd())
	cmd.AddCommand(newLoginsCreateCmd())
	cmd.AddCommand(newLoginsUpdateCmd())
	cmd.AddCommand(newLoginsDeleteCmd())
	cmd.AddCommand(newLoginsResetPasswordCmd())
	root.AddCommand(cmd)
}

func newLoginsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List admin login accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			logins, err := c.ListLoginAccounts(cmd.Context())
			if err != nil {
				return err
			}
			return renderLogins(cmd, logins)
		},
	}
}

func newLoginsCreateCmd() *cobra.Command {
	var email, userID, role string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a login account for an existing household user",
		Long: `Creates a new admin login account linked to a household user.
The response includes a one-time setup_token the user redeems to set their
password. The token is printed to stdout once and never retrievable again
— if lost, run ` + "`breadbox logins reset-password <id>`" + `.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if userID == "" {
				return UsageErrorf("--user is required (household user id or short_id)")
			}
			if email == "" {
				return UsageErrorf("--email is required (used as the login username)")
			}
			if role == "" {
				// Default to admin — the most common case for headless
				// bootstraps. Override with --role for editor/viewer.
				role = "admin"
			}
			if role != "admin" && role != "editor" && role != "viewer" {
				return UsageErrorf("--role must be one of: admin, editor, viewer")
			}
			c, _ := ClientFromContext(cmd.Context())
			login, err := c.CreateLoginAccount(cmd.Context(), userID, client.CreateLoginRequest{
				Username: email,
				Role:     role,
			})
			if err != nil {
				return err
			}
			return renderLoginWithToken(cmd, login)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "email address used as the login username (required)")
	cmd.Flags().StringVar(&userID, "user", "", "household user id or short_id (required)")
	cmd.Flags().StringVar(&role, "role", "", "role on the new login: admin (default), editor, or viewer")
	return cmd
}

func newLoginsUpdateCmd() *cobra.Command {
	var role string
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a login account (role only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("role") {
				return UsageErrorf("--role is required (admin, editor, or viewer)")
			}
			if role != "admin" && role != "editor" && role != "viewer" {
				return UsageErrorf("--role must be one of: admin, editor, viewer")
			}
			// We need the parent user_id; resolve it by listing.
			c, _ := ClientFromContext(cmd.Context())
			all, err := c.ListLoginAccounts(cmd.Context())
			if err != nil {
				return err
			}
			var match *client.LoginAccount
			for i := range all {
				if all[i].ID == args[0] {
					match = &all[i]
					break
				}
			}
			if match == nil {
				return fmt.Errorf("login %s not found", args[0])
			}
			login, err := c.UpdateLoginAccount(cmd.Context(), match.UserID, args[0], client.UpdateLoginRequest{Role: role})
			if err != nil {
				return err
			}
			return renderLogin(cmd, login)
		},
	}
	cmd.Flags().StringVar(&role, "role", "", "new role: admin, editor, or viewer")
	return cmd
}

func newLoginsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a login account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			if err := c.DeleteLoginAccount(cmd.Context(), args[0]); err != nil {
				return err
			}
			flags := Flags(cmd)
			if !flags.Quiet {
				fmt.Fprintf(os.Stderr, "deleted login %s\n", args[0])
			}
			return nil
		},
	}
}

func newLoginsResetPasswordCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset-password <id>",
		Short: "Issue a one-time setup token for an existing login",
		Long: `Generates a fresh setup token the user redeems to set a new password.
The token replaces any previous one, and only logins without a password
set are eligible. The plaintext token is printed to stdout exactly once —
copy it now or run this command again to mint another.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			resp, err := c.ResetLoginPassword(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			flags := Flags(cmd)
			mode := output.Resolve(flags.JSON, flags.NDJSON, os.Stdout)
			if !flags.Quiet && mode == output.ModeTable {
				fmt.Fprintln(os.Stderr, "Setup token follows on stdout. This is your only chance to capture it.")
			}
			if mode == output.ModeJSON || mode == output.ModeNDJSON {
				return output.PrintJSON(os.Stdout, resp)
			}
			// Bare-token output for table mode keeps shell piping clean.
			fmt.Println(resp.SetupToken)
			return nil
		},
	}
}

func renderLogins(cmd *cobra.Command, logins []client.LoginAccount) error {
	flags := Flags(cmd)
	mode := output.Resolve(flags.JSON, flags.NDJSON, os.Stdout)
	switch mode {
	case output.ModeJSON:
		return output.PrintJSON(os.Stdout, logins)
	case output.ModeNDJSON:
		items := make([]any, len(logins))
		for i, l := range logins {
			items[i] = l
		}
		return output.PrintNDJSON(os.Stdout, items)
	default:
		tbl := output.NewTable(os.Stdout, []string{"ID", "USERNAME", "USER", "ROLE", "HAS_PASSWORD"})
		for _, l := range logins {
			hp := "no"
			if l.HasPassword {
				hp = "yes"
			}
			tbl.AddRow(l.ID, l.Username, l.UserName, l.Role, hp)
		}
		return tbl.Flush()
	}
}

func renderLogin(cmd *cobra.Command, l *client.LoginAccount) error {
	flags := Flags(cmd)
	mode := output.Resolve(flags.JSON, flags.NDJSON, os.Stdout)
	if mode == output.ModeJSON || mode == output.ModeNDJSON {
		return output.PrintJSON(os.Stdout, l)
	}
	tbl := output.NewTable(os.Stdout, []string{"FIELD", "VALUE"})
	tbl.AddRow("id", l.ID)
	tbl.AddRow("user_id", l.UserID)
	tbl.AddRow("user_name", l.UserName)
	tbl.AddRow("username", l.Username)
	tbl.AddRow("role", l.Role)
	hp := "no"
	if l.HasPassword {
		hp = "yes"
	}
	tbl.AddRow("has_password", hp)
	tbl.AddRow("created_at", l.CreatedAt)
	return tbl.Flush()
}

// renderLoginWithToken is the create-path renderer. The setup token gets
// the same "this is your only chance" treatment as `keys create` — warn on
// stderr, print to stdout so pipe-redirect of the secret stays clean.
func renderLoginWithToken(cmd *cobra.Command, l *client.LoginAccount) error {
	flags := Flags(cmd)
	mode := output.Resolve(flags.JSON, flags.NDJSON, os.Stdout)
	if !flags.Quiet && mode == output.ModeTable && l.SetupToken != "" {
		fmt.Fprintln(os.Stderr, "Setup token included in output. This is your only chance to capture it.")
	}
	if mode == output.ModeJSON || mode == output.ModeNDJSON {
		return output.PrintJSON(os.Stdout, l)
	}
	tbl := output.NewTable(os.Stdout, []string{"FIELD", "VALUE"})
	tbl.AddRow("id", l.ID)
	tbl.AddRow("user_id", l.UserID)
	tbl.AddRow("username", l.Username)
	tbl.AddRow("role", l.Role)
	tbl.AddRow("setup_token", l.SetupToken)
	if l.SetupTokenExpiresAt != nil {
		tbl.AddRow("setup_token_expires_at", *l.SetupTokenExpiresAt)
	}
	tbl.AddRow("created_at", l.CreatedAt)
	return tbl.Flush()
}

package cli

import (
	"fmt"
	"os"

	"breadbox/internal/cli/output"
	"breadbox/internal/client"

	"github.com/spf13/cobra"
)

// AddUsersCmd wires `breadbox users` and its leaves. All children require a
// configured host — they hit /api/v1/users.
//
// Note on `--role`: the original CLI spec listed `--role` for `users create`
// and `users update`. The server schema does not carry a role on the users
// table (roles live on login accounts via `breadbox logins`), so this PR
// omits the flag rather than silently accept and discard it.
func AddUsersCmd(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "Manage household members",
	}
	MarkRequiresHost(cmd)
	cmd.AddCommand(newUsersListCmd())
	cmd.AddCommand(newUsersGetCmd())
	cmd.AddCommand(newUsersCreateCmd())
	cmd.AddCommand(newUsersUpdateCmd())
	cmd.AddCommand(newUsersDeleteCmd())
	root.AddCommand(cmd)
}

func newUsersListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List household members",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			users, err := c.ListUsers(cmd.Context())
			if err != nil {
				return err
			}
			return renderUsers(cmd, users)
		},
	}
}

func newUsersGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Show a single household member",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			u, err := c.GetUser(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return renderUser(cmd, u)
		},
	}
}

func newUsersCreateCmd() *cobra.Command {
	var name, email string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Add a household member",
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return UsageErrorf("--name is required")
			}
			req := client.CreateUserRequest{Name: name}
			if email != "" {
				e := email
				req.Email = &e
			}
			c, _ := ClientFromContext(cmd.Context())
			u, err := c.CreateUser(cmd.Context(), req)
			if err != nil {
				return err
			}
			return renderUser(cmd, u)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "display name (required)")
	cmd.Flags().StringVar(&email, "email", "", "email address (optional)")
	return cmd
}

func newUsersUpdateCmd() *cobra.Command {
	var name, email string
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a household member",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("name") && !cmd.Flags().Changed("email") {
				return UsageErrorf("at least one of --name or --email must be set")
			}
			var req client.UpdateUserRequest
			if cmd.Flags().Changed("name") {
				n := name
				req.Name = &n
			}
			if cmd.Flags().Changed("email") {
				e := email
				req.Email = &e
			}
			c, _ := ClientFromContext(cmd.Context())
			u, err := c.UpdateUser(cmd.Context(), args[0], req)
			if err != nil {
				return err
			}
			return renderUser(cmd, u)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "new display name")
	cmd.Flags().StringVar(&email, "email", "", "new email (pass an empty string to clear)")
	return cmd
}

func newUsersDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Remove a household member",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			if err := c.DeleteUser(cmd.Context(), args[0]); err != nil {
				return err
			}
			flags := Flags(cmd)
			if !flags.Quiet {
				fmt.Fprintf(os.Stderr, "deleted user %s\n", args[0])
			}
			return nil
		},
	}
}

// renderUsers picks JSON / NDJSON / table based on the user's flags. We
// don't hand a stdin-pipe consumer a table — output.Resolve handles that.
func renderUsers(cmd *cobra.Command, users []client.User) error {
	flags := Flags(cmd)
	mode := output.Resolve(flags.JSON, flags.NDJSON, os.Stdout)
	switch mode {
	case output.ModeJSON:
		return output.PrintJSON(os.Stdout, users)
	case output.ModeNDJSON:
		items := make([]any, len(users))
		for i, u := range users {
			items[i] = u
		}
		return output.PrintNDJSON(os.Stdout, items)
	default:
		tbl := output.NewTable(os.Stdout, []string{"SHORT_ID", "NAME", "EMAIL", "CREATED"})
		for _, u := range users {
			email := ""
			if u.Email != nil {
				email = *u.Email
			}
			tbl.AddRow(u.ShortID, u.Name, email, u.CreatedAt)
		}
		return tbl.Flush()
	}
}

func renderUser(cmd *cobra.Command, u *client.User) error {
	flags := Flags(cmd)
	mode := output.Resolve(flags.JSON, flags.NDJSON, os.Stdout)
	if mode == output.ModeJSON || mode == output.ModeNDJSON {
		return output.PrintJSON(os.Stdout, u)
	}
	tbl := output.NewTable(os.Stdout, []string{"FIELD", "VALUE"})
	tbl.AddRow("id", u.ID)
	tbl.AddRow("short_id", u.ShortID)
	tbl.AddRow("name", u.Name)
	email := ""
	if u.Email != nil {
		email = *u.Email
	}
	tbl.AddRow("email", email)
	tbl.AddRow("created_at", u.CreatedAt)
	tbl.AddRow("updated_at", u.UpdatedAt)
	return tbl.Flush()
}


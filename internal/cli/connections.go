// Package cli — connections noun group.
//
// `breadbox connections ...` exposes the bank-connection management
// surface, including the hosted-link flow that PRs #1043–#1047 introduced.
// The centerpiece is `connections link` — it mints a hosted-link session
// via POST /connections/link and prints the URL that an end-user (or
// agent driving a browser) opens to actually run the Plaid / Teller
// onboarding. `--wait` polls the session until it reaches a terminal
// status (completed / failed / expired) or the 5-minute timeout fires.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"breadbox/internal/cli/output"
	"breadbox/internal/client"

	"github.com/spf13/cobra"
)

// hostedLinkPollInterval is the cadence at which `--wait` polls the
// session. 2s matches the spec.
const hostedLinkPollInterval = 2 * time.Second

// hostedLinkWaitTimeout is the upper bound on `--wait`. Anything longer
// suggests the user walked away or the page is stuck; exit 4 so agents
// can branch.
const hostedLinkWaitTimeout = 5 * time.Minute

// AddConnectionsCmd registers `breadbox connections` and its children.
func AddConnectionsCmd(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "connections",
		Short: "Manage bank connections (Plaid / Teller / CSV)",
	}
	MarkRequiresHost(cmd)

	cmd.AddCommand(newConnectionsListCmd())
	cmd.AddCommand(newConnectionsGetCmd())
	cmd.AddCommand(newConnectionsLinkCmd())
	cmd.AddCommand(newConnectionsRelinkCmd())
	cmd.AddCommand(newConnectionsDisconnectCmd())
	cmd.AddCommand(newConnectionsDeleteCmd())

	root.AddCommand(cmd)
}

func newConnectionsListCmd() *cobra.Command {
	var userID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List bank connections",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			conns, err := c.ListConnections(cmd.Context(), userID)
			if err != nil {
				return err
			}
			return renderConnectionsList(flags, conns)
		},
	}
	cmd.Flags().StringVar(&userID, "user", "", "filter to connections owned by this user (uuid or short_id)")
	return cmd
}

func newConnectionsGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Show a single connection's detail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			conn, err := c.GetConnection(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return renderConnectionDetail(flags, conn)
		},
	}
	return cmd
}

// newConnectionsLinkCmd wires `breadbox connections link [--provider]
// [--user] [--wait]` plus the `link get <session-id>` subcommand.
//
// The output is shaped to be friendly: print the URL big and obvious in
// table mode, fall back to the raw `{url, session_id, expires_at}` JSON
// for agents.
func newConnectionsLinkCmd() *cobra.Command {
	var (
		provider string
		userID   string
		wait     bool
		label    string
	)
	cmd := &cobra.Command{
		Use:   "link",
		Short: "Mint a hosted-link session for connecting a bank",
		Long: `Mint a hosted-link URL that an end-user (or browser-driving agent) opens to
run the Plaid / Teller onboarding flow.

By default this prints { url, session_id, expires_at }. With --wait the
CLI blocks until the session reaches a terminal state (completed / failed
/ expired) and prints the final session payload — including the IDs of
any connections that were created.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			if userID == "" {
				return UsageErrorf("--user is required (uuid or short_id of the household user owning the connection)")
			}
			params := client.CreateHostedLinkParams{
				UserID:   userID,
				Provider: provider,
				Label:    label,
			}
			res, err := c.CreateHostedLink(cmd.Context(), params)
			if err != nil {
				return err
			}

			mode := output.Resolve(flags.JSON, flags.NDJSON, os.Stdout)
			if !wait {
				return renderHostedLinkCreate(mode, res)
			}

			// Print the URL up front so the user can act on it while
			// we poll. Polling progress goes to stderr to avoid mixing
			// it with the final JSON on stdout.
			printHostedLinkBanner(res, true)
			final, err := waitForHostedLink(cmd.Context(), c, res.ID, hostedLinkWaitTimeout)
			if err != nil {
				return err
			}
			return renderHostedLinkSession(mode, final)
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", "plaid | teller (omit to let the hosted page show a picker)")
	cmd.Flags().StringVar(&userID, "user", "", "household user that will own the resulting connection (uuid or short_id)")
	cmd.Flags().BoolVar(&wait, "wait", false, "block until the session reaches a terminal status (5-minute timeout)")
	cmd.Flags().StringVar(&label, "label", "", "human-readable label shown on the hosted page")

	cmd.AddCommand(&cobra.Command{
		Use:   "get <session-id>",
		Short: "Fetch the current state of a hosted-link session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			s, err := c.GetHostedLinkSession(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return renderHostedLinkSession(output.Resolve(flags.JSON, flags.NDJSON, os.Stdout), s)
		},
	})

	return cmd
}

func newConnectionsRelinkCmd() *cobra.Command {
	var wait bool
	cmd := &cobra.Command{
		Use:   "relink <connection-id>",
		Short: "Mint a re-auth hosted-link session for an existing connection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			res, err := c.CreateRelink(cmd.Context(), args[0], client.CreateRelinkParams{})
			if err != nil {
				return err
			}
			mode := output.Resolve(flags.JSON, flags.NDJSON, os.Stdout)
			if !wait {
				return renderHostedLinkCreate(mode, res)
			}
			printHostedLinkBanner(res, true)
			final, err := waitForHostedLink(cmd.Context(), c, res.ID, hostedLinkWaitTimeout)
			if err != nil {
				return err
			}
			return renderHostedLinkSession(mode, final)
		},
	}
	cmd.Flags().BoolVar(&wait, "wait", false, "block until the session reaches a terminal status (5-minute timeout)")
	return cmd
}

// newConnectionsDisconnectCmd is the canonical write-side endpoint —
// DELETE /connections/{id} performs the soft-disconnect.
func newConnectionsDisconnectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disconnect <id>",
		Short: "Soft-disconnect a connection (preserves history)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			if err := c.DisconnectConnection(cmd.Context(), args[0]); err != nil {
				return err
			}
			if !flags.Quiet {
				fmt.Println(args[0])
			}
			return nil
		},
	}
	return cmd
}

// newConnectionsDeleteCmd. There is no hard-delete REST endpoint today —
// DELETE /connections/{id} already performs the soft-disconnect that the
// `disconnect` verb maps to. We keep `delete` as an alias so the verb
// shape promised in docs/cli-commands.md works; the table doc notes the
// deviation.
func newConnectionsDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a connection (currently aliases disconnect — no hard delete yet)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			if err := c.DisconnectConnection(cmd.Context(), args[0]); err != nil {
				return err
			}
			if !flags.Quiet {
				fmt.Println(args[0])
			}
			return nil
		},
	}
	return cmd
}

// waitForHostedLink polls the given session every hostedLinkPollInterval
// until the session reaches a terminal status (completed / failed /
// expired), the context is cancelled, or `timeout` elapses (returning
// ErrHostedLinkTimeout, which the exit-code mapper turns into 4).
func waitForHostedLink(ctx context.Context, c *client.Client, sessionID string, timeout time.Duration) (*client.HostedLinkSession, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(hostedLinkPollInterval)
	defer ticker.Stop()

	for {
		session, err := c.GetHostedLinkSession(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		switch session.Status {
		case "completed", "failed", "expired":
			fmt.Fprintln(os.Stderr) // newline after the dots
			return session, nil
		}

		// Surface a single progress dot per poll on stderr — keeps stdout
		// clean for the eventual JSON dump.
		fmt.Fprint(os.Stderr, ".")

		if time.Now().After(deadline) {
			fmt.Fprintln(os.Stderr)
			return nil, ErrHostedLinkTimeout
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			continue
		}
	}
}

// ErrHostedLinkTimeout is returned when `--wait` exhausts its budget. The
// exit-code mapper maps this to ExitUpstream (4).
var ErrHostedLinkTimeout = errors.New("hosted-link session did not complete within the wait timeout")

// printHostedLinkBanner emits the friendly URL message on stderr (so
// stdout stays clean for the eventual JSON payload in non-TTY mode).
func printHostedLinkBanner(res *client.CreateHostedLinkResponse, isWaiting bool) {
	fmt.Fprintln(os.Stderr, "Hosted-link session ready. Open this URL in a browser:")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "    "+res.URL)
	fmt.Fprintln(os.Stderr)
	if res.ExpiresAt != "" {
		fmt.Fprintln(os.Stderr, "Expires at "+res.ExpiresAt)
	}
	if isWaiting {
		fmt.Fprintln(os.Stderr, "Polling for completion (Ctrl+C to abort)…")
	} else {
		fmt.Fprintln(os.Stderr, "Re-run with --wait to block until completed.")
	}
}

// renderHostedLinkCreate prints the create-time response: in TTY mode we
// show the friendly banner on stderr + a tiny stdout summary; in JSON
// mode we dump { url, session_id, expires_at } as documented.
func renderHostedLinkCreate(mode output.Mode, res *client.CreateHostedLinkResponse) error {
	switch mode {
	case output.ModeJSON, output.ModeNDJSON:
		return output.PrintJSON(os.Stdout, map[string]string{
			"url":        res.URL,
			"session_id": res.ID,
			"expires_at": res.ExpiresAt,
		})
	default:
		printHostedLinkBanner(res, false)
		return nil
	}
}

// renderHostedLinkSession prints a session payload — used both by `link
// get <id>` and by the post-wait final dump.
func renderHostedLinkSession(mode output.Mode, s *client.HostedLinkSession) error {
	switch mode {
	case output.ModeJSON, output.ModeNDJSON:
		return output.PrintJSON(os.Stdout, s)
	default:
		tbl := output.NewTable(os.Stdout, []string{"SHORT_ID", "PROVIDER", "ACTION", "STATUS", "CONNECTIONS", "EXPIRES_AT"})
		conns := "-"
		if len(s.ResultConnectionIDs) > 0 {
			conns = fmt.Sprintf("%d", len(s.ResultConnectionIDs))
		}
		tbl.AddRow(s.ShortID, valOrDash(s.Provider), s.Action, s.Status, conns, s.ExpiresAt)
		return tbl.Flush()
	}
}

func renderConnectionsList(flags *FlagBag, conns []client.Connection) error {
	switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
	case output.ModeJSON:
		return output.PrintJSON(os.Stdout, conns)
	case output.ModeNDJSON:
		items := make([]any, 0, len(conns))
		for _, c := range conns {
			items = append(items, c)
		}
		return output.PrintNDJSON(os.Stdout, items)
	default:
		tbl := output.NewTable(os.Stdout, []string{"SHORT_ID", "PROVIDER", "INSTITUTION", "STATUS", "USER", "LAST_SYNCED"})
		for _, c := range conns {
			tbl.AddRow(c.ShortID, c.Provider, strPtr(c.InstitutionName), c.Status, strPtr(c.UserName), strPtr(c.LastSyncedAt))
		}
		return tbl.Flush()
	}
}

func renderConnectionDetail(flags *FlagBag, c *client.ConnectionDetail) error {
	switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
	case output.ModeJSON, output.ModeNDJSON:
		return output.PrintJSON(os.Stdout, c)
	default:
		tbl := output.NewTable(os.Stdout, []string{"SHORT_ID", "PROVIDER", "INSTITUTION", "STATUS", "ACCOUNTS", "PAUSED"})
		tbl.AddRow(c.ShortID, c.Provider, strPtr(c.InstitutionName), c.Status, fmt.Sprintf("%d", c.AccountCount), fmt.Sprintf("%v", c.Paused))
		return tbl.Flush()
	}
}

// valOrDash renders a string field, returning "-" when empty.
func valOrDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

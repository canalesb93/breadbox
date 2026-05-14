// Package cli — webhooks noun group.
//
// `breadbox webhooks tail` walks /api/v1/webhook-events (most recent first)
// and prints either a human table or NDJSON for piping into jq. `breadbox
// webhooks replay <id>` POSTs to the replay endpoint to re-kick a sync.
package cli

import (
	"fmt"
	"os"

	"breadbox/internal/cli/output"
	"breadbox/internal/client"

	"github.com/spf13/cobra"
)

// AddWebhooksCmd registers `breadbox webhooks` and its children.
func AddWebhooksCmd(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "webhooks",
		Short: "Inspect and replay webhook events",
	}
	MarkRequiresHost(cmd)

	cmd.AddCommand(newWebhooksTailCmd())
	cmd.AddCommand(newWebhooksReplayCmd())

	root.AddCommand(cmd)
}

func newWebhooksTailCmd() *cobra.Command {
	var (
		provider string
		status   string
	)
	cmd := &cobra.Command{
		Use:   "tail",
		Short: "List the most recent webhook events",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			limit := flags.Limit
			if limit <= 0 {
				limit = 20
			}
			res, err := c.ListWebhookEvents(cmd.Context(), client.WebhookEventFilters{
				Provider: provider,
				Status:   status,
				Limit:    limit,
			})
			if err != nil {
				return err
			}
			return renderWebhookEvents(flags, res)
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", "filter by provider (plaid | teller)")
	cmd.Flags().StringVar(&status, "status", "", "filter by status (received | processed | error)")
	return cmd
}

func newWebhooksReplayCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "replay <id>",
		Short: "Re-trigger the sync the named webhook event would have caused",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			res, err := c.ReplayWebhookEvent(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if flags.JSON || flags.NDJSON {
				return output.PrintJSON(os.Stdout, res)
			}
			switch {
			case res.Triggered:
				fmt.Printf("replayed: webhook=%s connection=%s\n", res.WebhookEventID, res.ConnectionID)
			default:
				fmt.Printf("skipped: %s\n", res.Message)
			}
			return nil
		},
	}
	return cmd
}

func renderWebhookEvents(flags *FlagBag, res *client.WebhookEventList) error {
	switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
	case output.ModeJSON:
		return output.PrintJSON(os.Stdout, res)
	case output.ModeNDJSON:
		items := make([]any, 0, len(res.WebhookEvents))
		for _, e := range res.WebhookEvents {
			items = append(items, e)
		}
		return output.PrintNDJSON(os.Stdout, items)
	default:
		tbl := output.NewTable(os.Stdout, []string{"ID", "PROVIDER", "EVENT", "STATUS", "CONNECTION", "CREATED_AT"})
		for _, e := range res.WebhookEvents {
			conn := "-"
			if e.InstitutionName != nil && *e.InstitutionName != "" {
				conn = *e.InstitutionName
			} else if e.ConnectionID != nil {
				conn = *e.ConnectionID
			}
			created := "-"
			if e.CreatedAt != nil {
				created = *e.CreatedAt
			}
			tbl.AddRow(e.ID, e.Provider, e.EventType, e.Status, conn, created)
		}
		return tbl.Flush()
	}
}

// Package cli — sync noun group.
//
// `breadbox sync ...` drives the manual-sync, health, and history surface
// (POST /sync, GET /sync/health, GET /sync/logs). `sync logs --follow`
// uses straightforward polling — the server doesn't (yet) expose SSE/WS
// for sync events, so we emit fresh rows every 2s and stop on SIGINT.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"breadbox/internal/cli/output"
	"breadbox/internal/client"

	"github.com/spf13/cobra"
)

// syncFollowPollInterval is how often `sync logs --follow` polls for new
// rows. Matches the hosted-link cadence for consistency.
const syncFollowPollInterval = 2 * time.Second

// AddSyncCmd registers `breadbox sync` and its children.
func AddSyncCmd(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Trigger syncs and inspect sync state",
	}
	MarkRequiresHost(cmd)

	cmd.AddCommand(newSyncTriggerCmd())
	cmd.AddCommand(newSyncStatusCmd())
	cmd.AddCommand(newSyncLogsCmd())

	root.AddCommand(cmd)
}

func newSyncTriggerCmd() *cobra.Command {
	var connectionID string
	var accountID string
	cmd := &cobra.Command{
		Use:   "trigger",
		Short: "Kick a manual sync (all connections, or one)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			// --account is accepted for parity with the spec but the
			// REST endpoint scopes by connection_id, so we surface a
			// usage hint if only account is passed.
			if accountID != "" && connectionID == "" {
				return UsageErrorf("sync trigger does not yet support --account; use --connection instead")
			}
			res, err := c.TriggerSync(cmd.Context(), connectionID)
			if err != nil {
				return err
			}
			if flags.Quiet {
				return nil
			}
			return output.PrintJSON(os.Stdout, res)
		},
	}
	cmd.Flags().StringVar(&connectionID, "connection", "", "scope to a single connection (uuid or short_id)")
	cmd.Flags().StringVar(&accountID, "account", "", "(reserved) scope to a single account — not yet supported")
	return cmd
}

func newSyncStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show aggregate sync health (last 24h)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			status, err := c.SyncStatus(cmd.Context())
			if err != nil {
				return err
			}
			switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
			case output.ModeJSON, output.ModeNDJSON:
				return output.PrintJSON(os.Stdout, status)
			default:
				tbl := output.NewTable(os.Stdout, []string{"OVERALL", "LAST_STATUS", "RECENT_SYNCS", "SUCCESS_RATE", "LAST_SYNC"})
				last := "-"
				if status.LastSyncTime != nil {
					last = *status.LastSyncTime
				}
				tbl.AddRow(status.OverallHealth, status.LastSyncStatus,
					fmt.Sprintf("%d", status.RecentSyncCount),
					fmt.Sprintf("%.2f", status.RecentSuccessRate), last)
				return tbl.Flush()
			}
		},
	}
	return cmd
}

func newSyncLogsCmd() *cobra.Command {
	var (
		connectionID string
		follow       bool
	)
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show recent sync log entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)

			filters := client.SyncLogFilters{ConnectionID: connectionID}

			if follow {
				return followSyncLogs(cmd.Context(), c, filters, flags.Limit)
			}

			res, err := c.ListSyncLogs(cmd.Context(), filters, "", flags.Limit)
			if err != nil {
				return err
			}
			return renderSyncLogs(flags, res)
		},
	}
	cmd.Flags().StringVar(&connectionID, "connection", "", "scope to a single connection (uuid or short_id)")
	cmd.Flags().BoolVar(&follow, "follow", false, "tail new log rows — emits NDJSON, stops on Ctrl+C")
	return cmd
}

// followSyncLogs implements `--follow`: poll /sync/logs every 2s and emit
// any rows whose `id` we haven't already seen as NDJSON. Stops cleanly on
// SIGINT / SIGTERM (exit code 0).
func followSyncLogs(ctx context.Context, c *client.Client, filters client.SyncLogFilters, limit int) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	enc := json.NewEncoder(os.Stdout)
	seen := map[string]struct{}{}

	// Seed `seen` with the current top of the log so we only print rows
	// that arrive AFTER the user invoked the command.
	first, err := c.ListSyncLogs(ctx, filters, "", limit)
	if err != nil {
		return err
	}
	for _, row := range first.SyncLogs {
		seen[row.ID] = struct{}{}
	}

	ticker := time.NewTicker(syncFollowPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		page, err := c.ListSyncLogs(ctx, filters, "", limit)
		if err != nil {
			// Context cancellation surfaces here as an http error — treat
			// it as a clean stop rather than a noisy non-zero exit.
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		// /sync/logs is sorted newest-first; emit oldest-first so the
		// terminal reads chronologically.
		for i := len(page.SyncLogs) - 1; i >= 0; i-- {
			row := page.SyncLogs[i]
			if _, ok := seen[row.ID]; ok {
				continue
			}
			seen[row.ID] = struct{}{}
			_ = enc.Encode(row)
		}
	}
}

func renderSyncLogs(flags *FlagBag, res *client.SyncLogsResult) error {
	switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
	case output.ModeJSON:
		return output.PrintJSON(os.Stdout, res)
	case output.ModeNDJSON:
		items := make([]any, 0, len(res.SyncLogs))
		for _, r := range res.SyncLogs {
			items = append(items, r)
		}
		return output.PrintNDJSON(os.Stdout, items)
	default:
		tbl := output.NewTable(os.Stdout, []string{"INSTITUTION", "TRIGGER", "STATUS", "ADDED", "MODIFIED", "REMOVED", "STARTED_AT"})
		for _, r := range res.SyncLogs {
			started := "-"
			if r.StartedAt != nil {
				started = *r.StartedAt
			}
			tbl.AddRow(r.InstitutionName, r.Trigger, r.Status,
				fmt.Sprintf("%d", r.AddedCount),
				fmt.Sprintf("%d", r.ModifiedCount),
				fmt.Sprintf("%d", r.RemovedCount),
				started)
		}
		return tbl.Flush()
	}
}

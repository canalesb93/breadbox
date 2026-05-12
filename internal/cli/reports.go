// Package cli — reports noun group.
//
// `breadbox reports ...` is a thin shell over /api/v1/reports. Submit
// posts a JSON body authored by an agent; the CLI applies a minimal
// client-side sanity check (file is JSON with a `title` and `body`) and
// forwards the rest verbatim. The optional `--kind` flag is added to the
// payload as the `priority` field — the canonical agent_report schema
// doesn't have a `kind` column, but the spec uses that term as a synonym
// for priority/category. Tags filtering on list is applied client-side.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"breadbox/internal/cli/output"
	"breadbox/internal/client"

	"github.com/spf13/cobra"
)

// AddReportsCmd registers `breadbox reports` and its children.
func AddReportsCmd(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "reports",
		Short: "Manage agent reports",
	}
	MarkRequiresHost(cmd)

	cmd.AddCommand(newReportsListCmd())
	cmd.AddCommand(newReportsGetCmd())
	cmd.AddCommand(newReportsSubmitCmd())
	cmd.AddCommand(newReportsReadCmd())
	cmd.AddCommand(newReportsUnreadCmd())

	root.AddCommand(cmd)
}

func newReportsListCmd() *cobra.Command {
	var (
		kind   string
		status string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List agent reports",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			reports, err := c.ListReports(cmd.Context())
			if err != nil {
				return err
			}
			// The server returns the full list; --kind / --status are applied
			// client-side because the public endpoint doesn't accept them.
			filtered := reports[:0]
			for _, r := range reports {
				if kind != "" && !strings.EqualFold(r.Priority, kind) {
					continue
				}
				if status != "" {
					read := r.ReadAt != nil && *r.ReadAt != ""
					if strings.EqualFold(status, "unread") && read {
						continue
					}
					if strings.EqualFold(status, "read") && !read {
						continue
					}
				}
				filtered = append(filtered, r)
			}
			return renderReportsList(flags, filtered)
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "filter by priority/kind (info, warning, critical)")
	cmd.Flags().StringVar(&status, "status", "", "filter by read state (read | unread)")
	return cmd
}

func newReportsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Show a single report",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			r, err := c.GetReport(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return renderReport(flags, r)
		},
	}
}

func newReportsSubmitCmd() *cobra.Command {
	var (
		kind string
		file string
	)
	cmd := &cobra.Command{
		Use:   "submit",
		Short: "Submit a new report (--kind --json <file>)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if kind == "" {
				return UsageErrorf("--kind is required (info | warning | critical)")
			}
			if file == "" {
				return UsageErrorf("--json <file> is required")
			}
			raw, err := readFileOrStdin(file)
			if err != nil {
				return err
			}
			var payload map[string]any
			if err := json.Unmarshal(raw, &payload); err != nil {
				return UsageErrorf("report file must be a JSON object: %v", err)
			}
			if _, ok := payload["title"]; !ok {
				return UsageErrorf("report file missing required keys: title")
			}
			if _, ok := payload["body"]; !ok {
				return UsageErrorf("report file missing required keys: body")
			}
			// Inject --kind as priority when the file doesn't already set it.
			if _, ok := payload["priority"]; !ok {
				payload["priority"] = kind
			}
			body, err := json.Marshal(payload)
			if err != nil {
				return fmt.Errorf("marshal report payload: %w", err)
			}
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			r, err := c.CreateReport(cmd.Context(), json.RawMessage(body))
			if err != nil {
				return err
			}
			if flags.Quiet {
				return nil
			}
			return renderReport(flags, r)
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "report priority/kind (info | warning | critical)")
	cmd.Flags().StringVar(&file, "json", "", "path to report JSON body (use `-` for stdin)")
	return cmd
}

func newReportsReadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "read <id>",
		Short: "Mark a report as read",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			if err := c.MarkReportRead(cmd.Context(), args[0]); err != nil {
				return err
			}
			if !flags.Quiet {
				fmt.Println(args[0])
			}
			return nil
		},
	}
}

func newReportsUnreadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unread <id>",
		Short: "Mark a report as unread",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			if err := c.MarkReportUnread(cmd.Context(), args[0]); err != nil {
				return err
			}
			if !flags.Quiet {
				fmt.Println(args[0])
			}
			return nil
		},
	}
}

// --- rendering helpers ---

func renderReportsList(flags *FlagBag, reports []client.Report) error {
	switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
	case output.ModeJSON:
		return output.PrintJSON(os.Stdout, reports)
	case output.ModeNDJSON:
		items := make([]any, 0, len(reports))
		for _, r := range reports {
			items = append(items, r)
		}
		return output.PrintNDJSON(os.Stdout, items)
	default:
		tbl := output.NewTable(os.Stdout, []string{"SHORT_ID", "TITLE", "PRIORITY", "AUTHOR", "READ", "CREATED"})
		for _, r := range reports {
			read := "no"
			if r.ReadAt != nil && *r.ReadAt != "" {
				read = "yes"
			}
			tbl.AddRow(r.ShortID, truncate(r.Title, 60), r.Priority, r.CreatedByName, read, r.CreatedAt)
		}
		return tbl.Flush()
	}
}

func renderReport(flags *FlagBag, r *client.Report) error {
	switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
	case output.ModeJSON, output.ModeNDJSON:
		return output.PrintJSON(os.Stdout, r)
	default:
		tbl := output.NewTable(os.Stdout, []string{"SHORT_ID", "TITLE", "PRIORITY", "AUTHOR", "READ", "CREATED"})
		read := "no"
		if r.ReadAt != nil && *r.ReadAt != "" {
			read = "yes"
		}
		tbl.AddRow(r.ShortID, truncate(r.Title, 60), r.Priority, r.CreatedByName, read, r.CreatedAt)
		return tbl.Flush()
	}
}

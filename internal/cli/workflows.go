// Package cli — workflows noun group.
//
// `breadbox workflows ...` is a tag-free, HTTP-driven shell over the
// /api/v1/workflows surface (the subsystem formerly called "agents"). Like
// every other noun command it resolves a *client.Client via the persistent
// pre-run and renders through the shared output formatter, so it works
// against a local or remote breadbox over the network and compiles into the
// lite CLI build.
//
// Subcommands:
//   - list     GET /api/v1/workflows         configured workflows
//   - runs     GET /api/v1/workflows/runs     the cross-workflow run feed
//   - presets  GET /api/v1/workflow-presets   the code-defined gallery catalog
package cli

import (
	"fmt"
	"os"

	"breadbox/internal/cli/output"
	"breadbox/internal/client"

	"github.com/spf13/cobra"
)

// AddWorkflowsCmd registers `breadbox workflows` and its children.
func AddWorkflowsCmd(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "workflows",
		Short: "Inspect configured workflows, their runs, and the preset gallery",
	}
	MarkRequiresHost(cmd)

	cmd.AddCommand(newWorkflowsListCmd())
	cmd.AddCommand(newWorkflowsRunsCmd())
	cmd.AddCommand(newWorkflowsPresetsCmd())

	root.AddCommand(cmd)
}

func newWorkflowsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured workflows",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			wfs, err := c.ListWorkflows(cmd.Context())
			if err != nil {
				return err
			}
			return renderWorkflowsList(flags, wfs)
		},
	}
}

func newWorkflowsRunsCmd() *cobra.Command {
	var (
		workflow string
		status   string
		trigger  string
		offset   int
	)
	cmd := &cobra.Command{
		Use:   "runs",
		Short: "List the cross-workflow run feed (offset-paginated)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			res, err := c.ListWorkflowRuns(cmd.Context(), client.WorkflowRunListParams{
				Workflow: workflow,
				Status:   status,
				Trigger:  trigger,
				Limit:    flags.Limit,
				Offset:   offset,
			})
			if err != nil {
				return err
			}
			return renderWorkflowRuns(flags, res)
		},
	}
	cmd.Flags().StringVar(&workflow, "workflow", "", "filter to one workflow (slug, short_id, or uuid)")
	cmd.Flags().StringVar(&status, "status", "", "filter by status (success|error|in_progress|skipped|timeout)")
	cmd.Flags().StringVar(&trigger, "trigger", "", "filter by trigger (cron|manual|webhook)")
	cmd.Flags().IntVar(&offset, "offset", 0, "row offset for the next page (pair with --limit)")
	return cmd
}

func newWorkflowsPresetsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "presets",
		Short: "List the code-defined workflow preset gallery",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			presets, err := c.ListWorkflowPresets(cmd.Context())
			if err != nil {
				return err
			}
			return renderWorkflowPresets(flags, presets)
		},
	}
}

// --- rendering helpers ---

func renderWorkflowsList(flags *FlagBag, wfs []client.Workflow) error {
	switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
	case output.ModeJSON:
		return output.PrintJSON(os.Stdout, wfs)
	case output.ModeNDJSON:
		items := make([]any, 0, len(wfs))
		for _, w := range wfs {
			items = append(items, w)
		}
		return output.PrintNDJSON(os.Stdout, items)
	default:
		tbl := output.NewTable(os.Stdout, []string{"SLUG", "NAME", "SCOPE", "MODEL", "ENABLED", "SCHEDULE"})
		for _, w := range wfs {
			tbl.AddRow(w.Slug, w.Name, w.ToolScope, w.Model, boolYN(w.Enabled), workflowSchedule(w.ScheduleCron))
		}
		return tbl.Flush()
	}
}

func renderWorkflowRuns(flags *FlagBag, res *client.WorkflowRunListResult) error {
	switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
	case output.ModeJSON:
		return output.PrintJSON(os.Stdout, res)
	case output.ModeNDJSON:
		items := make([]any, 0, len(res.Runs))
		for _, r := range res.Runs {
			items = append(items, r)
		}
		return output.PrintNDJSON(os.Stdout, items)
	default:
		tbl := output.NewTable(os.Stdout, []string{"SHORT_ID", "WORKFLOW", "STATUS", "TRIGGER", "STARTED", "COST"})
		for _, r := range res.Runs {
			tbl.AddRow(r.ShortID, r.WorkflowSlug, r.Status, r.Trigger, r.StartedAt, formatCostPtr(r.TotalCostUSD))
		}
		if err := tbl.Flush(); err != nil {
			return err
		}
		if res.HasMore {
			fmt.Fprintf(os.Stderr, "(more rows; pass --offset %d to continue)\n", res.Offset+len(res.Runs))
		}
		return nil
	}
}

func renderWorkflowPresets(flags *FlagBag, presets []client.WorkflowPreset) error {
	switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
	case output.ModeJSON:
		return output.PrintJSON(os.Stdout, presets)
	case output.ModeNDJSON:
		items := make([]any, 0, len(presets))
		for _, p := range presets {
			items = append(items, p)
		}
		return output.PrintNDJSON(os.Stdout, items)
	default:
		tbl := output.NewTable(os.Stdout, []string{"SLUG", "NAME", "CATEGORY", "ENABLED"})
		for _, p := range presets {
			tbl.AddRow(p.Slug, p.Name, p.Category, boolYN(p.Enabled))
		}
		return tbl.Flush()
	}
}

// workflowSchedule renders a workflow's cron, falling back to "manual" when
// no schedule is set — matching `breadbox agent list`'s column semantics.
func workflowSchedule(cron *string) string {
	if cron == nil || *cron == "" {
		return "manual"
	}
	return *cron
}

// formatCostPtr renders an optional USD cost, returning "-" for nil.
func formatCostPtr(f *float64) string {
	if f == nil {
		return "-"
	}
	return fmt.Sprintf("$%.4f", *f)
}

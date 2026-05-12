// Package cli — tags noun group.
//
// `breadbox tags ...` is a thin shell over /api/v1/tags. Tags are bounded
// reference data (no cursor pagination); slug is the canonical handle —
// uuid and short_id are accepted for parity with other endpoints.
package cli

import (
	"fmt"
	"os"

	"breadbox/internal/cli/output"
	"breadbox/internal/client"

	"github.com/spf13/cobra"
)

// AddTagsCmd registers `breadbox tags` and its children.
func AddTagsCmd(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "tags",
		Short: "Manage transaction tags",
	}
	MarkRequiresHost(cmd)

	cmd.AddCommand(newTagsListCmd())
	cmd.AddCommand(newTagsGetCmd())
	cmd.AddCommand(newTagsCreateCmd())
	cmd.AddCommand(newTagsUpdateCmd())
	cmd.AddCommand(newTagsDeleteCmd())

	root.AddCommand(cmd)
}

func newTagsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all tags",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			tags, err := c.ListTags(cmd.Context())
			if err != nil {
				return err
			}
			return renderTagsList(flags, tags)
		},
	}
}

func newTagsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <slug>",
		Short: "Show a single tag (accepts slug, uuid, or short_id)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			tag, err := c.GetTag(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return renderTag(flags, tag)
		},
	}
}

func newTagsCreateCmd() *cobra.Command {
	var (
		label string
		desc  string
		color string
		icon  string
	)
	cmd := &cobra.Command{
		Use:   "create <slug>",
		Short: "Create a new tag",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			displayName := label
			if displayName == "" {
				displayName = args[0]
			}
			params := client.CreateTagParams{
				Slug:        args[0],
				DisplayName: displayName,
				Description: desc,
			}
			if color != "" {
				v := color
				params.Color = &v
			}
			if icon != "" {
				v := icon
				params.Icon = &v
			}
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			out, err := c.CreateTag(cmd.Context(), params)
			if err != nil {
				return err
			}
			if flags.Quiet {
				return nil
			}
			return renderTag(flags, out)
		},
	}
	cmd.Flags().StringVar(&label, "label", "", "human-readable display name (defaults to slug)")
	cmd.Flags().StringVar(&desc, "description", "", "long-form description")
	cmd.Flags().StringVar(&color, "color", "", "hex color")
	cmd.Flags().StringVar(&icon, "icon", "", "icon name")
	return cmd
}

func newTagsUpdateCmd() *cobra.Command {
	var (
		label     string
		desc      string
		color     string
		icon      string
		lifecycle string
	)
	cmd := &cobra.Command{
		Use:   "update <slug>",
		Short: "Update an existing tag (slug is immutable)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("label") && !cmd.Flags().Changed("description") &&
				!cmd.Flags().Changed("color") && !cmd.Flags().Changed("icon") &&
				!cmd.Flags().Changed("lifecycle") {
				return UsageErrorf("update needs at least one of --label, --description, --color, --icon, --lifecycle")
			}
			params := client.UpdateTagParams{}
			if cmd.Flags().Changed("label") {
				v := label
				params.DisplayName = &v
			}
			if cmd.Flags().Changed("description") {
				v := desc
				params.Description = &v
			}
			if cmd.Flags().Changed("color") {
				v := color
				params.Color = &v
			}
			if cmd.Flags().Changed("icon") {
				v := icon
				params.Icon = &v
			}
			if cmd.Flags().Changed("lifecycle") {
				v := lifecycle
				params.Lifecycle = &v
			}
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			out, err := c.UpdateTag(cmd.Context(), args[0], params)
			if err != nil {
				return err
			}
			if flags.Quiet {
				return nil
			}
			return renderTag(flags, out)
		},
	}
	cmd.Flags().StringVar(&label, "label", "", "display name")
	cmd.Flags().StringVar(&desc, "description", "", "long-form description")
	cmd.Flags().StringVar(&color, "color", "", "hex color")
	cmd.Flags().StringVar(&icon, "icon", "", "icon name")
	cmd.Flags().StringVar(&lifecycle, "lifecycle", "", "lifecycle state (e.g. active, archived)")
	return cmd
}

func newTagsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <slug>",
		Short: "Delete a tag",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			if err := c.DeleteTag(cmd.Context(), args[0]); err != nil {
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

func renderTagsList(flags *FlagBag, tags []client.Tag) error {
	switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
	case output.ModeJSON:
		return output.PrintJSON(os.Stdout, tags)
	case output.ModeNDJSON:
		items := make([]any, 0, len(tags))
		for _, t := range tags {
			items = append(items, t)
		}
		return output.PrintNDJSON(os.Stdout, items)
	default:
		tbl := output.NewTable(os.Stdout, []string{"SHORT_ID", "SLUG", "LABEL", "LIFECYCLE", "COLOR"})
		for _, t := range tags {
			tbl.AddRow(t.ShortID, t.Slug, t.DisplayName, t.Lifecycle, strPtr(t.Color))
		}
		return tbl.Flush()
	}
}

func renderTag(flags *FlagBag, t *client.Tag) error {
	switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
	case output.ModeJSON, output.ModeNDJSON:
		return output.PrintJSON(os.Stdout, t)
	default:
		tbl := output.NewTable(os.Stdout, []string{"SHORT_ID", "SLUG", "LABEL", "LIFECYCLE", "COLOR"})
		tbl.AddRow(t.ShortID, t.Slug, t.DisplayName, t.Lifecycle, strPtr(t.Color))
		return tbl.Flush()
	}
}

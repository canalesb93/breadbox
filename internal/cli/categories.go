// Package cli — categories noun group.
//
// `breadbox categories ...` is a thin shell over /api/v1/categories. It
// adds list/get/create/update/delete/merge plus TSV export/import. The
// service treats categories as bounded reference data, so the list
// command returns a bare array (no cursor pagination).
package cli

import (
	"fmt"
	"os"

	"breadbox/internal/cli/output"
	"breadbox/internal/client"

	"github.com/spf13/cobra"
)

// AddCategoriesCmd registers `breadbox categories` and its children.
func AddCategoriesCmd(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "categories",
		Short: "Manage transaction categories",
	}
	MarkRequiresHost(cmd)

	cmd.AddCommand(newCategoriesListCmd())
	cmd.AddCommand(newCategoriesGetCmd())
	cmd.AddCommand(newCategoriesCreateCmd())
	cmd.AddCommand(newCategoriesUpdateCmd())
	cmd.AddCommand(newCategoriesDeleteCmd())
	cmd.AddCommand(newCategoriesMergeCmd())
	cmd.AddCommand(newCategoriesExportCmd())
	cmd.AddCommand(newCategoriesImportCmd())

	root.AddCommand(cmd)
}

func newCategoriesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all categories",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			cats, err := c.ListCategories(cmd.Context())
			if err != nil {
				return err
			}
			return renderCategoriesList(flags, cats)
		},
	}
}

func newCategoriesGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Show a single category",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			cat, err := c.GetCategory(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return renderCategory(flags, cat)
		},
	}
}

func newCategoriesCreateCmd() *cobra.Command {
	var (
		name   string
		slug   string
		parent string
		icon   string
		color  string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new category",
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return UsageErrorf("--name is required")
			}
			params := client.CreateCategoryParams{DisplayName: name, Slug: slug}
			if parent != "" {
				v := parent
				params.ParentID = &v
			}
			if icon != "" {
				v := icon
				params.Icon = &v
			}
			if color != "" {
				v := color
				params.Color = &v
			}
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			out, err := c.CreateCategory(cmd.Context(), params)
			if err != nil {
				return err
			}
			if flags.Quiet {
				return nil
			}
			return renderCategory(flags, out)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "display name (required)")
	cmd.Flags().StringVar(&slug, "slug", "", "stable slug (auto-generated when omitted)")
	cmd.Flags().StringVar(&parent, "parent", "", "parent category id (uuid or short_id)")
	cmd.Flags().StringVar(&icon, "icon", "", "icon name")
	cmd.Flags().StringVar(&color, "color", "", "hex color (e.g. #ff0000)")
	return cmd
}

func newCategoriesUpdateCmd() *cobra.Command {
	var (
		name      string
		icon      string
		color     string
		sortOrder int32
		hidden    bool
	)
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update an existing category",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("name") && !cmd.Flags().Changed("icon") &&
				!cmd.Flags().Changed("color") && !cmd.Flags().Changed("sort-order") &&
				!cmd.Flags().Changed("hidden") {
				return UsageErrorf("update needs at least one of --name, --icon, --color, --sort-order, --hidden")
			}
			c, _ := ClientFromContext(cmd.Context())
			// The server's PUT requires display_name; fetch the current row
			// when --name is omitted so we preserve it.
			if !cmd.Flags().Changed("name") {
				cur, err := c.GetCategory(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				name = cur.DisplayName
			}
			params := client.UpdateCategoryParams{DisplayName: name, SortOrder: sortOrder, Hidden: hidden}
			if cmd.Flags().Changed("icon") {
				v := icon
				params.Icon = &v
			}
			if cmd.Flags().Changed("color") {
				v := color
				params.Color = &v
			}
			flags := Flags(cmd)
			out, err := c.UpdateCategory(cmd.Context(), args[0], params)
			if err != nil {
				return err
			}
			if flags.Quiet {
				return nil
			}
			return renderCategory(flags, out)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "display name")
	cmd.Flags().StringVar(&icon, "icon", "", "icon name")
	cmd.Flags().StringVar(&color, "color", "", "hex color")
	cmd.Flags().Int32Var(&sortOrder, "sort-order", 0, "sort order")
	cmd.Flags().BoolVar(&hidden, "hidden", false, "hide from default category pickers")
	return cmd
}

func newCategoriesDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a category (server blocks if transactions reference it)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			affected, err := c.DeleteCategory(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if flags.Quiet {
				return nil
			}
			if flags.JSON || flags.NDJSON || !isTerminal(os.Stdout) {
				return output.PrintJSON(os.Stdout, map[string]any{
					"id":                    args[0],
					"affected_transactions": affected,
				})
			}
			fmt.Println(args[0])
			return nil
		},
	}
}

func newCategoriesMergeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "merge <from> <to>",
		Short: "Merge `from` into `to` (migrate transactions, drop source)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			if err := c.MergeCategories(cmd.Context(), args[0], args[1]); err != nil {
				return err
			}
			if !flags.Quiet {
				fmt.Println(args[0])
			}
			return nil
		},
	}
}

func newCategoriesExportCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Dump categories (--format tsv|json)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			switch format {
			case "", "tsv":
				body, err := c.ExportCategoriesTSV(cmd.Context())
				if err != nil {
					return err
				}
				fmt.Print(body)
				return nil
			case "json":
				cats, err := c.ListCategories(cmd.Context())
				if err != nil {
					return err
				}
				return renderCategoriesList(flags, cats)
			default:
				return UsageErrorf("--format must be tsv or json (got %q)", format)
			}
		},
	}
	cmd.Flags().StringVar(&format, "format", "tsv", "tsv | json")
	return cmd
}

func newCategoriesImportCmd() *cobra.Command {
	var replace bool
	cmd := &cobra.Command{
		Use:   "import <file>",
		Short: "Import categories from a TSV file (use `-` for stdin)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := readFileOrStdin(args[0])
			if err != nil {
				return err
			}
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			res, err := c.ImportCategoriesTSV(cmd.Context(), body, replace)
			if err != nil {
				return err
			}
			if flags.Quiet {
				return nil
			}
			return output.PrintJSON(os.Stdout, res)
		},
	}
	cmd.Flags().BoolVar(&replace, "replace", false, "drop existing non-system categories before importing")
	return cmd
}

// --- rendering helpers ---

func renderCategoriesList(flags *FlagBag, cats []client.Category) error {
	switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
	case output.ModeJSON:
		return output.PrintJSON(os.Stdout, cats)
	case output.ModeNDJSON:
		items := make([]any, 0, len(cats))
		for _, c := range cats {
			items = append(items, c)
		}
		return output.PrintNDJSON(os.Stdout, items)
	default:
		tbl := output.NewTable(os.Stdout, []string{"SHORT_ID", "SLUG", "NAME", "PARENT", "HIDDEN"})
		for _, c := range cats {
			renderCategoryRow(tbl, c, "")
		}
		return tbl.Flush()
	}
}

func renderCategoryRow(tbl *output.Table, c client.Category, prefix string) {
	parent := "-"
	if c.ParentSlug != nil && *c.ParentSlug != "" {
		parent = *c.ParentSlug
	}
	hidden := "no"
	if c.Hidden {
		hidden = "yes"
	}
	tbl.AddRow(c.ShortID, c.Slug, prefix+c.DisplayName, parent, hidden)
	for _, ch := range c.Children {
		renderCategoryRow(tbl, ch, prefix+"  ")
	}
}

func renderCategory(flags *FlagBag, c *client.Category) error {
	switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
	case output.ModeJSON, output.ModeNDJSON:
		return output.PrintJSON(os.Stdout, c)
	default:
		tbl := output.NewTable(os.Stdout, []string{"SHORT_ID", "SLUG", "NAME", "PARENT", "HIDDEN"})
		parent := "-"
		if c.ParentSlug != nil && *c.ParentSlug != "" {
			parent = *c.ParentSlug
		}
		hidden := "no"
		if c.Hidden {
			hidden = "yes"
		}
		tbl.AddRow(c.ShortID, c.Slug, c.DisplayName, parent, hidden)
		return tbl.Flush()
	}
}

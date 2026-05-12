// Package cli — config noun group.
//
// `breadbox config ...` reads + writes rows in the server-side `app_config`
// table. Secret values (anything matching the server's denylist) come back
// masked unless --reveal is set; some keys (ENCRYPTION_KEY, Teller PEM
// blobs) refuse to be revealed at all.
package cli

import (
	"fmt"
	"os"

	"breadbox/internal/cli/output"
	"breadbox/internal/client"

	"github.com/spf13/cobra"
)

// AddConfigCmd registers `breadbox config` and its children.
func AddConfigCmd(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Read and write server-side configuration (app_config table)",
	}
	MarkRequiresHost(cmd)

	cmd.AddCommand(newConfigListCmd())
	cmd.AddCommand(newConfigGetCmd())
	cmd.AddCommand(newConfigSetCmd())
	cmd.AddCommand(newConfigUnsetCmd())

	root.AddCommand(cmd)
}

func newConfigListCmd() *cobra.Command {
	var reveal bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List every app_config entry and its source (env / db / default)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			entries, err := c.ListConfig(cmd.Context(), reveal)
			if err != nil {
				return err
			}
			return renderConfigList(flags, entries)
		},
	}
	cmd.Flags().BoolVar(&reveal, "reveal", false, "show full secret values (still blocked for hardcoded denylist)")
	return cmd
}

func newConfigGetCmd() *cobra.Command {
	var reveal bool
	cmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Print a single config value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			entry, err := c.GetConfig(cmd.Context(), args[0], reveal)
			if err != nil {
				return err
			}
			return renderConfigEntry(flags, entry)
		},
	}
	cmd.Flags().BoolVar(&reveal, "reveal", false, "show the full value if secret-flagged")
	return cmd
}

func newConfigSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Write a value to the app_config table",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			entry, err := c.SetConfig(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			if flags.Quiet {
				return nil
			}
			return renderConfigEntry(flags, entry)
		},
	}
	return cmd
}

func newConfigUnsetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unset <key>",
		Short: "Drop a key from app_config (falls back to env / compile-in default)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			if err := c.DeleteConfig(cmd.Context(), args[0]); err != nil {
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

// renderConfigList prints the list either as a table or JSON depending on
// the output mode.
func renderConfigList(flags *FlagBag, entries []client.ConfigEntry) error {
	switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
	case output.ModeJSON:
		return output.PrintJSON(os.Stdout, entries)
	case output.ModeNDJSON:
		items := make([]any, 0, len(entries))
		for _, e := range entries {
			items = append(items, e)
		}
		return output.PrintNDJSON(os.Stdout, items)
	default:
		tbl := output.NewTable(os.Stdout, []string{"KEY", "VALUE", "SOURCE", "SECRET"})
		for _, e := range entries {
			val := "-"
			if e.Value != nil {
				val = *e.Value
			}
			secret := "-"
			if e.Secret {
				secret = "yes"
			}
			tbl.AddRow(e.Key, val, e.Source, secret)
		}
		return tbl.Flush()
	}
}

func renderConfigEntry(flags *FlagBag, e *client.ConfigEntry) error {
	switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
	case output.ModeJSON, output.ModeNDJSON:
		return output.PrintJSON(os.Stdout, e)
	default:
		val := ""
		if e.Value != nil {
			val = *e.Value
		}
		fmt.Println(val)
		return nil
	}
}

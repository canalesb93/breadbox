// Package cli — rules noun group.
//
// `breadbox rules ...` is a thin shell over /api/v1/rules. Create and
// update accept a JSON file matching the canonical rule-DSL spec
// (docs/rule-dsl.md). The CLI does a minimal client-side check (file is
// JSON, has required top-level keys) and forwards the rest verbatim;
// the server validates the full grammar.
package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"breadbox/internal/cli/output"
	"breadbox/internal/client"

	"github.com/spf13/cobra"
)

// AddRulesCmd registers `breadbox rules` and its children.
func AddRulesCmd(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "rules",
		Short: "Manage transaction rules",
	}
	MarkRequiresHost(cmd)

	cmd.AddCommand(newRulesListCmd())
	cmd.AddCommand(newRulesGetCmd())
	cmd.AddCommand(newRulesCreateCmd())
	cmd.AddCommand(newRulesUpdateCmd())
	cmd.AddCommand(newRulesDeleteCmd())
	cmd.AddCommand(newRulesPreviewCmd())
	cmd.AddCommand(newRulesApplyCmd())
	cmd.AddCommand(newRulesBatchCmd())

	root.AddCommand(cmd)
}

func newRulesListCmd() *cobra.Command {
	var (
		enabled bool
		cursor  string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List transaction rules (cursor-paginated)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			params := client.RuleListParams{
				Cursor: cursor,
				Limit:  flags.Limit,
			}
			if cmd.Flags().Changed("enabled") {
				v := enabled
				params.Enabled = &v
			}
			res, err := c.ListRules(cmd.Context(), params)
			if err != nil {
				return err
			}
			return renderRulesList(flags, res)
		},
	}
	cmd.Flags().BoolVar(&enabled, "enabled", true, "filter by enabled state (true|false)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "opaque cursor from a previous page")
	return cmd
}

func newRulesGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Show a single rule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			r, err := c.GetRule(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return renderRule(flags, r)
		},
	}
}

// validateRuleFile sanity-checks that the file contents are JSON and
// contain a `name` plus either `conditions` + `actions` or a
// `category_slug` shortcut. The server is the source of truth for the
// rest of the DSL grammar.
func validateRuleFile(raw []byte) (json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, UsageErrorf("rule file must be a JSON object: %v", err)
	}
	if _, ok := obj["name"]; !ok {
		return nil, UsageErrorf("rule file missing required keys: name")
	}
	_, hasActions := obj["actions"]
	_, hasCategorySlug := obj["category_slug"]
	if !hasActions && !hasCategorySlug {
		return nil, UsageErrorf("rule file missing required keys: actions (or category_slug)")
	}
	return json.RawMessage(raw), nil
}

func newRulesCreateCmd() *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a rule from a DSL JSON file (--json)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return UsageErrorf("--json <file> is required")
			}
			raw, err := readFileOrStdin(file)
			if err != nil {
				return err
			}
			body, err := validateRuleFile(raw)
			if err != nil {
				return err
			}
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			r, err := c.CreateRule(cmd.Context(), body)
			if err != nil {
				return err
			}
			if flags.Quiet {
				return nil
			}
			return renderRule(flags, r)
		},
	}
	cmd.Flags().StringVar(&file, "json", "", "path to rule DSL JSON file (use `-` for stdin)")
	return cmd
}

func newRulesUpdateCmd() *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a rule from a DSL JSON file (--json)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return UsageErrorf("--json <file> is required")
			}
			raw, err := readFileOrStdin(file)
			if err != nil {
				return err
			}
			// Update accepts partial patches, so we only check it's an object.
			var probe map[string]json.RawMessage
			if err := json.Unmarshal(raw, &probe); err != nil {
				return UsageErrorf("rule file must be a JSON object: %v", err)
			}
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			r, err := c.UpdateRule(cmd.Context(), args[0], json.RawMessage(raw))
			if err != nil {
				return err
			}
			if flags.Quiet {
				return nil
			}
			return renderRule(flags, r)
		},
	}
	cmd.Flags().StringVar(&file, "json", "", "path to rule DSL patch JSON file (use `-` for stdin)")
	return cmd
}

func newRulesDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a rule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			if err := c.DeleteRule(cmd.Context(), args[0]); err != nil {
				return err
			}
			if !flags.Quiet {
				fmt.Println(args[0])
			}
			return nil
		},
	}
}

func newRulesPreviewCmd() *cobra.Command {
	var sample int
	cmd := &cobra.Command{
		Use:   "preview <id>",
		Short: "Preview which transactions a rule would match",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			// PreviewRule expects a Condition body, so resolve the rule's
			// stored conditions first and forward them to the server.
			r, err := c.GetRule(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			cond := r.Conditions
			if len(cond) == 0 {
				cond = json.RawMessage("{}")
			}
			req := client.PreviewRuleRequest{Conditions: cond, SampleSize: sample}
			res, err := c.PreviewRule(cmd.Context(), req)
			if err != nil {
				return err
			}
			return output.PrintJSON(os.Stdout, res)
		},
	}
	cmd.Flags().IntVar(&sample, "sample-size", 0, "max number of matched rows the server returns")
	return cmd
}

func newRulesApplyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "apply <id>",
		Short: "Apply a rule retroactively to existing transactions",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			res, err := c.ApplyRule(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if flags.Quiet {
				return nil
			}
			return output.PrintJSON(os.Stdout, res)
		},
	}
}

func newRulesBatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "batch <file>",
		Short: "Create many rules in one call (JSON array or {rules:[...]})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := readFileOrStdin(args[0])
			if err != nil {
				return err
			}
			// Accept either a bare array of rule objects or an envelope
			// {"rules":[...], "on_error":"continue|abort"} matching the
			// server's POST /rules/batch shape.
			trimmed := raw
			for len(trimmed) > 0 && (trimmed[0] == ' ' || trimmed[0] == '\n' || trimmed[0] == '\t' || trimmed[0] == '\r') {
				trimmed = trimmed[1:]
			}
			var body []byte
			if len(trimmed) > 0 && trimmed[0] == '[' {
				wrap := map[string]json.RawMessage{"rules": json.RawMessage(raw)}
				b, err := json.Marshal(wrap)
				if err != nil {
					return fmt.Errorf("wrap batch array: %w", err)
				}
				body = b
			} else {
				// Ensure it parses as an object up front so we return a usage
				// error rather than a 400 from the server.
				var probe map[string]json.RawMessage
				if err := json.Unmarshal(raw, &probe); err != nil {
					return UsageErrorf("batch file must be a JSON array or {rules:[...]} object: %v", err)
				}
				body = raw
			}
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			res, err := c.BatchCreateRules(cmd.Context(), json.RawMessage(body))
			if err != nil {
				return err
			}
			if flags.Quiet {
				return nil
			}
			return output.PrintJSON(os.Stdout, res)
		},
	}
	return cmd
}

// --- rendering helpers ---

func renderRulesList(flags *FlagBag, res *client.RuleListResult) error {
	switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
	case output.ModeJSON:
		return output.PrintJSON(os.Stdout, res)
	case output.ModeNDJSON:
		items := make([]any, 0, len(res.Rules))
		for _, r := range res.Rules {
			items = append(items, r)
		}
		return output.PrintNDJSON(os.Stdout, items)
	default:
		tbl := output.NewTable(os.Stdout, []string{"SHORT_ID", "NAME", "TRIGGER", "PRIORITY", "ENABLED", "HITS"})
		for _, r := range res.Rules {
			tbl.AddRow(r.ShortID, r.Name, r.Trigger, fmt.Sprintf("%d", r.Priority),
				boolYN(r.Enabled), fmt.Sprintf("%d", r.HitCount))
		}
		if err := tbl.Flush(); err != nil {
			return err
		}
		if res.HasMore && res.NextCursor != "" {
			fmt.Fprintf(os.Stderr, "(more rows; pass --cursor %s to continue)\n", res.NextCursor)
		}
		return nil
	}
}

func renderRule(flags *FlagBag, r *client.Rule) error {
	switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
	case output.ModeJSON, output.ModeNDJSON:
		return output.PrintJSON(os.Stdout, r)
	default:
		tbl := output.NewTable(os.Stdout, []string{"SHORT_ID", "NAME", "TRIGGER", "PRIORITY", "ENABLED", "HITS"})
		tbl.AddRow(r.ShortID, r.Name, r.Trigger, fmt.Sprintf("%d", r.Priority),
			boolYN(r.Enabled), fmt.Sprintf("%d", r.HitCount))
		return tbl.Flush()
	}
}

func boolYN(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

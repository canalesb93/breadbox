// Package cli — transactions noun group.
//
// `breadbox transactions ...` exposes the read+write surface for the
// transactions table: list, get, count, summary, atomic per-row
// updates, soft-delete + restore, tag attach/detach, comments, and the
// activity-timeline annotations feed. Filter parsing is centralised in
// transactionFiltersFromFlags so list/count/summary share one query
// string.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"breadbox/internal/cli/output"
	"breadbox/internal/client"

	"github.com/spf13/cobra"
)

// AddTransactionsCmd registers `breadbox transactions` and its children.
func AddTransactionsCmd(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "transactions",
		Short: "Query and mutate transactions",
	}
	MarkRequiresHost(cmd)

	cmd.AddCommand(newTxnListCmd())
	cmd.AddCommand(newTxnGetCmd())
	cmd.AddCommand(newTxnCountCmd())
	cmd.AddCommand(newTxnSummaryCmd())
	cmd.AddCommand(newTxnUpdateCmd())
	cmd.AddCommand(newTxnBatchCmd())
	cmd.AddCommand(newTxnCategorizeCmd())
	cmd.AddCommand(newTxnUncategorizeCmd())
	cmd.AddCommand(newTxnRecategorizeCmd())
	cmd.AddCommand(newTxnDeleteCmd())
	cmd.AddCommand(newTxnRestoreCmd())
	cmd.AddCommand(newTxnTagCmd())
	cmd.AddCommand(newTxnUntagCmd())
	cmd.AddCommand(newTxnAnnotationsCmd())
	cmd.AddCommand(newTxnCommentsCmd())

	root.AddCommand(cmd)
}

// txnFilterFlags binds the shared filter flag set to a cobra command and
// returns a TransactionFilters value resolved from those flags. The same
// shape powers list / count / summary.
func txnFilterFlags(cmd *cobra.Command) {
	cmd.Flags().String("account", "", "filter by account (uuid or short_id)")
	cmd.Flags().String("category", "", "filter by category slug")
	cmd.Flags().String("from", "", "start date (YYYY-MM-DD, inclusive)")
	cmd.Flags().String("to", "", "end date (YYYY-MM-DD, exclusive)")
	cmd.Flags().String("search", "", "name/merchant search term (>=2 chars)")
	cmd.Flags().String("search-mode", "", "contains | words | fuzzy")
	cmd.Flags().String("exclude-search", "", "exclude rows matching this substring")
	cmd.Flags().String("user", "", "filter by household user (uuid or short_id)")
	cmd.Flags().StringSlice("tag", nil, "filter to rows carrying ALL given tags (repeatable or comma-separated)")
	cmd.Flags().StringSlice("any-tag", nil, "filter to rows carrying AT LEAST ONE of these tags")
	cmd.Flags().Bool("has-comment", false, "(reserved) require at least one comment")
}

func filtersFromCmd(cmd *cobra.Command) client.TransactionFilters {
	f := client.TransactionFilters{}
	f.Account, _ = cmd.Flags().GetString("account")
	f.Category, _ = cmd.Flags().GetString("category")
	f.From, _ = cmd.Flags().GetString("from")
	f.To, _ = cmd.Flags().GetString("to")
	f.Search, _ = cmd.Flags().GetString("search")
	f.SearchMode, _ = cmd.Flags().GetString("search-mode")
	f.ExcludeSearch, _ = cmd.Flags().GetString("exclude-search")
	f.User, _ = cmd.Flags().GetString("user")
	if v, _ := cmd.Flags().GetStringSlice("tag"); len(v) > 0 {
		f.Tags = expandCSV(v)
	}
	if v, _ := cmd.Flags().GetStringSlice("any-tag"); len(v) > 0 {
		f.AnyTags = expandCSV(v)
	}
	if cmd.Flags().Changed("has-comment") {
		b, _ := cmd.Flags().GetBool("has-comment")
		f.HasComment = &b
	}
	return f
}

// expandCSV expands `["a,b", "c"]` → `["a","b","c"]` so the user can pass
// either repeated flags or one comma-joined value.
func expandCSV(in []string) []string {
	out := make([]string, 0, len(in))
	for _, raw := range in {
		for _, p := range strings.Split(raw, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

func newTxnListCmd() *cobra.Command {
	var cursor string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List transactions (cursor-paginated)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			filters := filtersFromCmd(cmd)

			mode := output.Resolve(flags.JSON, flags.NDJSON, os.Stdout)

			if flags.All {
				return walkAllTransactions(cmd.Context(), c, filters, flags, mode)
			}
			res, err := c.ListTransactions(cmd.Context(), filters, cursor, flags.Limit, flags.Fields)
			if err != nil {
				return err
			}
			if flags.Debug && res.NextCursor != "" {
				fmt.Fprintf(os.Stderr, "next_cursor=%s has_more=%v\n", res.NextCursor, res.HasMore)
			}
			return renderTransactionPage(mode, res)
		},
	}
	cmd.Flags().StringVar(&cursor, "cursor", "", "opaque cursor from a previous page's next_cursor")
	txnFilterFlags(cmd)
	return cmd
}

func walkAllTransactions(ctx context.Context, c *client.Client, filters client.TransactionFilters, flags *FlagBag, mode output.Mode) error {
	enc := json.NewEncoder(os.Stdout)
	cursor := ""
	first := true
	if mode == output.ModeJSON {
		// Stream everything into one big array so JSON readers can slurp it.
		fmt.Print("[")
		defer fmt.Println("]")
	}
	for {
		page, err := c.ListTransactions(ctx, filters, cursor, flags.Limit, flags.Fields)
		if err != nil {
			return err
		}
		if flags.Debug {
			fmt.Fprintf(os.Stderr, "page rows=%d next_cursor=%s has_more=%v\n", len(page.Transactions), page.NextCursor, page.HasMore)
		}
		switch mode {
		case output.ModeNDJSON:
			for _, t := range page.Transactions {
				_ = enc.Encode(t)
			}
		case output.ModeJSON:
			for _, t := range page.Transactions {
				if !first {
					fmt.Print(",")
				}
				first = false
				b, _ := json.Marshal(t)
				_, _ = os.Stdout.Write(b)
			}
		default:
			tbl := buildTransactionTable(os.Stdout, page.Transactions)
			_ = tbl.Flush()
		}
		if !page.HasMore || page.NextCursor == "" {
			return nil
		}
		cursor = page.NextCursor
	}
}

func newTxnGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Fetch a single transaction",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			t, err := c.GetTransaction(cmd.Context(), args[0], flags.Fields)
			if err != nil {
				return err
			}
			return renderTransaction(flags, t)
		},
	}
	return cmd
}

func newTxnCountCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "count",
		Short: "Count transactions matching the filters",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			n, err := c.CountTransactions(cmd.Context(), filtersFromCmd(cmd))
			if err != nil {
				return err
			}
			if flags.JSON || flags.NDJSON || !isTerminal(os.Stdout) {
				return output.PrintJSON(os.Stdout, map[string]int64{"count": n})
			}
			fmt.Println(n)
			return nil
		},
	}
	txnFilterFlags(cmd)
	return cmd
}

func newTxnSummaryCmd() *cobra.Command {
	var by string
	cmd := &cobra.Command{
		Use:   "summary",
		Short: "Aggregate transactions by category, month, week, or day",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			if by == "" {
				return UsageErrorf("--by is required (category, month, week, day, or category_month)")
			}
			res, err := c.TransactionSummary(cmd.Context(), by, filtersFromCmd(cmd))
			if err != nil {
				return err
			}
			return output.PrintJSON(os.Stdout, res)
		},
	}
	cmd.Flags().StringVar(&by, "by", "", "group_by: category | month | week | day | category_month")
	txnFilterFlags(cmd)
	return cmd
}

func newTxnUpdateCmd() *cobra.Command {
	var (
		category string
		reset    bool
		note     string
		tags     []string
	)
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Atomically update category, tags, and/or attach a comment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)

			op := client.UpdateTransactionOp{TransactionID: args[0]}
			if cmd.Flags().Changed("category") {
				v := category
				op.CategorySlug = &v
			}
			if cmd.Flags().Changed("reset-category") {
				op.ResetCategory = reset
			}
			if cmd.Flags().Changed("note") {
				v := note
				op.Comment = &v
			}
			for _, slug := range expandCSV(tags) {
				op.TagsToAdd = append(op.TagsToAdd, client.UpdateTransactionTag{Slug: slug})
			}

			if op.CategorySlug == nil && !op.ResetCategory && op.Comment == nil && len(op.TagsToAdd) == 0 {
				return UsageErrorf("update needs at least one of --category, --reset-category, --note, --tag")
			}

			res, err := c.UpdateTransactions(cmd.Context(), client.UpdateTransactionsRequest{
				Operations: []client.UpdateTransactionOp{op},
			})
			if err != nil {
				return err
			}
			if flags.Quiet {
				return nil
			}
			return output.PrintJSON(os.Stdout, res)
		},
	}
	cmd.Flags().StringVar(&category, "category", "", "set category slug (overrides rules)")
	cmd.Flags().BoolVar(&reset, "reset-category", false, "clear the category override")
	cmd.Flags().StringVar(&note, "note", "", "attach this string as a comment")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "tag slugs to add (repeatable or comma-separated)")
	return cmd
}

func newTxnBatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "batch <file>",
		Short: "Run /transactions/update with operations from a JSON file (max 50)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			raw, err := readFileOrStdin(args[0])
			if err != nil {
				return err
			}
			var req client.UpdateTransactionsRequest
			if err := json.Unmarshal(raw, &req); err != nil {
				// allow a bare array of operations as a convenience.
				var ops []client.UpdateTransactionOp
				if err2 := json.Unmarshal(raw, &ops); err2 != nil {
					return fmt.Errorf("parse batch JSON: %w", err)
				}
				req.Operations = ops
			}
			res, err := c.UpdateTransactions(cmd.Context(), req)
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

func newTxnCategorizeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "categorize <id> <category-id>",
		Short: "Set a manual category override on a transaction",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			if err := c.SetTransactionCategory(cmd.Context(), args[0], args[1]); err != nil {
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

func newTxnUncategorizeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uncategorize <id>",
		Short: "Clear the manual category override",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			if err := c.ResetTransactionCategory(cmd.Context(), args[0]); err != nil {
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

func newTxnRecategorizeCmd() *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:   "recategorize",
		Short: "Server-side bulk recategorize by filter",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			if target == "" {
				return UsageErrorf("--category is required (target category slug)")
			}
			f := filtersFromCmd(cmd)
			req := client.BulkRecategorizeRequest{
				TargetCategorySlug: target,
				StartDate:          f.From,
				EndDate:            f.To,
				AccountID:          f.Account,
				UserID:             f.User,
				CategorySlug:       f.Category,
				Search:             f.Search,
				MinAmount:          f.MinAmount,
				MaxAmount:          f.MaxAmount,
				Pending:            f.Pending,
			}
			res, err := c.BulkRecategorize(cmd.Context(), req)
			if err != nil {
				return err
			}
			if flags.Quiet {
				return nil
			}
			return output.PrintJSON(os.Stdout, res)
		},
	}
	cmd.Flags().StringVar(&target, "category", "", "target category slug (required)")
	txnFilterFlags(cmd)
	return cmd
}

func newTxnDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Soft-delete a transaction",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			if err := c.DeleteTransaction(cmd.Context(), args[0]); err != nil {
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

func newTxnRestoreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restore <id>",
		Short: "Restore a soft-deleted transaction",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			if err := c.RestoreTransaction(cmd.Context(), args[0]); err != nil {
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

func newTxnTagCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tag <id> <slug>",
		Short: "Attach a tag to a transaction (auto-creates if missing)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			res, err := c.AddTransactionTag(cmd.Context(), args[0], args[1])
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

func newTxnUntagCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "untag <id> <slug>",
		Short: "Detach a tag from a transaction",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			res, err := c.RemoveTransactionTag(cmd.Context(), args[0], args[1])
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

func newTxnAnnotationsCmd() *cobra.Command {
	var kinds []string
	cmd := &cobra.Command{
		Use:   "annotations <id>",
		Short: "Activity-timeline rows for a transaction",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			res, err := c.ListAnnotations(cmd.Context(), args[0], expandCSV(kinds), flags.Limit)
			if err != nil {
				return err
			}
			return output.PrintJSON(os.Stdout, res)
		},
	}
	cmd.Flags().StringSliceVar(&kinds, "kind", nil, "filter by annotation kind (comment, rule_applied, review, ...)")
	return cmd
}

func newTxnCommentsCmd() *cobra.Command {
	comments := &cobra.Command{
		Use:   "comments",
		Short: "Manage comments on a transaction",
	}

	comments.AddCommand(&cobra.Command{
		Use:   "add <id> <message>",
		Short: "Add a comment to a transaction",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			com, err := c.CreateComment(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			if flags.Quiet {
				return nil
			}
			return output.PrintJSON(os.Stdout, com)
		},
	})

	comments.AddCommand(&cobra.Command{
		Use:   "list <id>",
		Short: "List comments on a transaction",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			coms, err := c.ListComments(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
			case output.ModeJSON:
				return output.PrintJSON(os.Stdout, coms)
			case output.ModeNDJSON:
				items := make([]any, 0, len(coms))
				for _, c := range coms {
					items = append(items, c)
				}
				return output.PrintNDJSON(os.Stdout, items)
			default:
				tbl := output.NewTable(os.Stdout, []string{"SHORT_ID", "AUTHOR", "CREATED_AT", "CONTENT"})
				for _, com := range coms {
					tbl.AddRow(com.ShortID, com.AuthorName, com.CreatedAt, truncate(com.Content, 80))
				}
				return tbl.Flush()
			}
		},
	})

	comments.AddCommand(&cobra.Command{
		Use:   "edit <id> <comment-id> <message>",
		Short: "Edit a comment",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			com, err := c.UpdateComment(cmd.Context(), args[0], args[1], args[2])
			if err != nil {
				return err
			}
			if flags.Quiet {
				return nil
			}
			return output.PrintJSON(os.Stdout, com)
		},
	})

	comments.AddCommand(&cobra.Command{
		Use:   "delete <id> <comment-id>",
		Short: "Delete a comment",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			if err := c.DeleteComment(cmd.Context(), args[0], args[1]); err != nil {
				return err
			}
			if !flags.Quiet {
				fmt.Println(args[1])
			}
			return nil
		},
	})

	return comments
}

// --- rendering helpers ---

func renderTransactionPage(mode output.Mode, res *client.TransactionListResult) error {
	switch mode {
	case output.ModeJSON:
		return output.PrintJSON(os.Stdout, res)
	case output.ModeNDJSON:
		items := make([]any, 0, len(res.Transactions))
		for _, t := range res.Transactions {
			items = append(items, t)
		}
		return output.PrintNDJSON(os.Stdout, items)
	default:
		tbl := buildTransactionTable(os.Stdout, res.Transactions)
		if err := tbl.Flush(); err != nil {
			return err
		}
		if res.HasMore && res.NextCursor != "" {
			fmt.Fprintf(os.Stderr, "(more rows; pass --cursor %s to continue or --all to walk every page)\n", res.NextCursor)
		}
		return nil
	}
}

func renderTransaction(flags *FlagBag, t *client.Transaction) error {
	switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
	case output.ModeJSON, output.ModeNDJSON:
		return output.PrintJSON(os.Stdout, t)
	default:
		tbl := buildTransactionTable(os.Stdout, []client.Transaction{*t})
		return tbl.Flush()
	}
}

func buildTransactionTable(w io.Writer, rows []client.Transaction) *output.Table {
	tbl := output.NewTable(w, []string{"SHORT_ID", "DATE", "AMOUNT", "CCY", "NAME", "CATEGORY", "ACCOUNT"})
	for _, t := range rows {
		cat := "-"
		if t.Category != nil && t.Category.DisplayName != nil {
			cat = *t.Category.DisplayName
		}
		acct := strPtr(t.AccountName)
		tbl.AddRow(t.ShortID, t.Date, fmt.Sprintf("%.2f", t.Amount), strPtr(t.IsoCurrencyCode), t.ProviderName, cat, acct)
	}
	return tbl
}

// truncate cuts a string to n runes with an ellipsis, leaving stdout
// readable when comment bodies are long.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n < 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}

// readFileOrStdin reads a file path or `-` from stdin.
func readFileOrStdin(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

// isTerminal exposes output's TTY check for callers that don't need the full
// Resolve dispatch (count, for instance).
func isTerminal(w *os.File) bool {
	info, err := w.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

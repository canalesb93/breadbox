// Package cli — csv noun group.
//
// `breadbox csv {preview,import}` uploads CSV files via the
// /connections/csv/preview and /connections/csv/import endpoints
// (multipart/form-data). The handler infers a column mapping during
// preview; import requires the user to pass one back (or to have an
// existing connection_id).
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"breadbox/internal/cli/output"
	"breadbox/internal/client"

	"github.com/spf13/cobra"
)

// AddCSVCmd registers `breadbox csv` and its children.
func AddCSVCmd(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "csv",
		Short: "Preview and import bank CSVs",
	}
	MarkRequiresHost(cmd)

	cmd.AddCommand(newCSVPreviewCmd())
	cmd.AddCommand(newCSVImportCmd())

	root.AddCommand(cmd)
}

func newCSVPreviewCmd() *cobra.Command {
	var accountID string
	cmd := &cobra.Command{
		Use:   "preview <file>",
		Short: "Parse a CSV and return the inferred column mapping + preview rows",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			data, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read CSV: %w", err)
			}
			opts := client.CSVOptions{ConnectionID: accountID}
			res, err := c.PreviewCSV(cmd.Context(), filepath.Base(args[0]), data, opts)
			if err != nil {
				return err
			}
			return output.PrintJSON(os.Stdout, res)
		},
	}
	cmd.Flags().StringVar(&accountID, "account", "", "(reserved) hint that we're appending to an existing CSV connection")
	return cmd
}

func newCSVImportCmd() *cobra.Command {
	var (
		accountID    string
		userID       string
		accountName  string
		connectionID string
		dateFormat   string
	)
	cmd := &cobra.Command{
		Use:   "import <file>",
		Short: "Import a CSV into Breadbox (creates a new csv connection or appends)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			data, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read CSV: %w", err)
			}
			// Run preview first to grab an inferred mapping — saves the
			// user from passing --column-mapping by hand. Auto-detection
			// is per-file and matches the admin upload flow.
			preview, err := c.PreviewCSV(cmd.Context(), filepath.Base(args[0]), data, client.CSVOptions{})
			if err != nil {
				return err
			}
			mapping, _ := preview["inferred_mapping"].(map[string]any)
			columnMapping := map[string]int{}
			for k, v := range mapping {
				if n, ok := v.(float64); ok {
					columnMapping[k] = int(n)
				}
			}

			conn := connectionID
			if conn == "" && accountID != "" {
				// `--account` is the spec's flag name; on import without
				// an explicit connection we pass it through as the
				// connection hint (matches the server's preference).
				conn = accountID
			}

			opts := client.CSVOptions{
				UserID:        userID,
				AccountName:   accountName,
				ConnectionID:  conn,
				DateFormat:    dateFormat,
				ColumnMapping: columnMapping,
			}
			// Auto-populate template-detected toggles when available so the
			// import lines up with the same defaults the admin UI uses.
			if v, ok := preview["positive_is_debit"].(bool); ok {
				opts.PositiveIsDebit = v
			}
			if v, ok := preview["has_debit_credit"].(bool); ok {
				opts.HasDebitCredit = v
			}
			if v, ok := preview["date_format"].(string); ok && opts.DateFormat == "" {
				opts.DateFormat = v
			}

			res, err := c.ImportCSV(cmd.Context(), filepath.Base(args[0]), data, opts)
			if err != nil {
				return err
			}
			if flags.Quiet {
				return nil
			}
			return output.PrintJSON(os.Stdout, res)
		},
	}
	cmd.Flags().StringVar(&accountID, "account", "", "existing CSV connection id (uuid or short_id) to append to")
	cmd.Flags().StringVar(&userID, "user", "", "household user that will own the import (uuid or short_id)")
	cmd.Flags().StringVar(&accountName, "account-name", "", "human-readable account name when creating a new csv connection")
	cmd.Flags().StringVar(&connectionID, "connection", "", "alias for --account; existing csv connection id")
	cmd.Flags().StringVar(&dateFormat, "date-format", "", "override Go date format string (e.g. 2006-01-02)")
	return cmd
}

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// AddVersionCmd registers `breadbox version`. Prints just the version
// string — matching the legacy behavior of `breadbox version`. Detailed
// upgrade info is exposed via `GET /api/v1/version` and `breadbox doctor`.
func AddVersionCmd(root *cobra.Command, version string) {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the breadbox build version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println(version)
			return nil
		},
	}
	root.AddCommand(cmd)
}

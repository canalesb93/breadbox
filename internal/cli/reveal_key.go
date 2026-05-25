//go:build !lite

package cli

import (
	"encoding/hex"
	"fmt"

	bbconfig "breadbox/internal/config"

	"github.com/spf13/cobra"
)

// AddRevealKeyCmd wires `breadbox reveal-key` — prints the resolved
// ENCRYPTION_KEY exactly once to stdout.
//
// L-scoped: never talks to the API. The whole point is recovery — you
// run this on the server (SSH, console, container exec) when you need
// the key to import into a password manager or transplant to a new
// host. It reads the same env/.env precedence the running server does
// (see internal/config.Load), so the value matches whatever
// `breadbox serve` is using.
func AddRevealKeyCmd(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "reveal-key",
		Short: "Print the configured ENCRYPTION_KEY to stdout",
		Long: `Prints the ENCRYPTION_KEY currently resolved from environment
(typically loaded from .env on the server) so you can copy it into a
password manager or move it to a new host. Reads the same precedence as
` + "`breadbox serve`" + ` — no DB lookup, no API call.

Output is the 64-character hex string only, followed by a newline. Pipe to
` + "`pbcopy` / `xclip`" + ` to copy without it landing in shell history.

Exit codes:
  0  printed key
  1  ENCRYPTION_KEY is not set in the current environment`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := bbconfig.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if len(cfg.EncryptionKey) == 0 {
				return fmt.Errorf("ENCRYPTION_KEY is not set in the current environment")
			}
			fmt.Fprintln(cmd.OutOrStdout(), hex.EncodeToString(cfg.EncryptionKey))
			return nil
		},
	}
	root.AddCommand(cmd)
}

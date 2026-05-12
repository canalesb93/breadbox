//go:build lite

// Smoke test for the lite (CLI-only) build. Verifies that under
// -tags=lite the cobra command tree exposes the HTTP-client subcommands
// (auth, accounts, transactions, …) while the L-scope server commands
// (serve, migrate, mcp, seed, init, backup, create-admin, reset-password)
// are physically absent.
//
// Runs ONLY under -tags=lite — the default and headless builds exercise
// the inverse via the regular test suite (e.g. internal/cli/init_test.go
// references runInit, which is `!lite`).

package cli_test

import (
	"strings"
	"testing"

	"breadbox/internal/cli"

	"github.com/spf13/cobra"
)

func TestLiteBuildOmitsServerCommands(t *testing.T) {
	root := cli.NewRootCmd("test")

	forbidden := []string{
		"serve",
		"migrate",
		"seed",
		"mcp-stdio",
		"init",
		"backup",
		"create-admin",
		"reset-password",
	}
	for _, name := range forbidden {
		if findChild(root, name) != nil {
			t.Errorf("lite build should not expose %q", name)
		}
	}

	required := []string{
		"auth",
		"accounts",
		"transactions",
		"categories",
		"tags",
		"rules",
		"reports",
		"connections",
		"sync",
		"csv",
		"users",
		"logins",
		"keys",
		"providers",
		"config",
		"webhooks",
		"doctor",
		"version",
	}
	for _, name := range required {
		if findChild(root, name) == nil {
			t.Errorf("lite build is missing required command %q", name)
		}
	}
}

// findChild looks up a direct child of root by its first-token Use name.
// cobra's Find walks the tree and falls back to the deepest matched node
// (it returns root itself for unknown names), which is the wrong behavior
// for an existence check — hence this explicit child walk.
func findChild(root *cobra.Command, name string) *cobra.Command {
	for _, c := range root.Commands() {
		first := c.Use
		if i := strings.IndexByte(first, ' '); i >= 0 {
			first = first[:i]
		}
		if first == name {
			return c
		}
	}
	return nil
}

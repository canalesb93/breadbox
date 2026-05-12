//go:build lite

package cli

import "github.com/spf13/cobra"

// registerServerCommands is a no-op on lite builds. The L-scope commands
// (serve, migrate, mcp, seed, create-admin, reset-password, init, backup)
// require internal/db, internal/service, etc. — none of which compile under
// -tags=lite. The cobra command tree therefore omits them entirely; the
// lite smoke test (lite_smoke_test.go) pins that contract.
func registerServerCommands(_ *cobra.Command) {}

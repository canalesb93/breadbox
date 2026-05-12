//go:build !lite

package cli

import "github.com/spf13/cobra"

// registerServerCommands wires the L-scope subcommands that need server
// runtime packages (internal/db, internal/service, internal/sync,
// internal/app, internal/mcp, internal/seed, etc.).
//
// Lite builds compile a sibling stub (registerServerCommands in
// root_lite.go) that is a no-op — those commands are physically absent
// from the binary.
func registerServerCommands(root *cobra.Command) {
	AddServeCmd(root)
	AddMigrateCmd(root)
	AddSeedCmd(root)
	AddMCPCmd(root)
	AddCreateAdminCmd(root)
	AddResetPasswordCmd(root)
	AddInitCmd(root)
	AddBackupCmd(root)
}

// Package main is the breadbox binary entrypoint. The actual command tree
// lives in internal/cli; this file is a 20-line shim so the binary stays a
// single artifact across server, MCP, and CLI roles.
package main

import (
	"fmt"
	"os"

	"breadbox/internal/cli"

	// pgx stdlib driver registration is needed for goose's database/sql
	// migrations regardless of which subcommand runs.
	_ "github.com/jackc/pgx/v5/stdlib"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cmd := cli.NewRootCmd(version)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(cli.MapExitCode(err))
	}
}

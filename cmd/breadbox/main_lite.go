//go:build lite

// Package main is the breadbox binary entrypoint (CLI-only build, `-tags=lite`).
// Same shim as main_full.go without the goose/pgx stdlib driver
// registration — lite builds never run migrations or open a database.
// Built as `breadbox-cli` via `go build -tags=lite -o breadbox-cli ./cmd/breadbox`.
package main

import (
	"fmt"
	"os"

	"breadbox/internal/cli"
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

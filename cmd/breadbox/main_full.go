//go:build !lite

// Package main is the breadbox binary entrypoint (full build). The actual
// command tree lives in internal/cli; this file is a thin shim so the
// binary stays a single artifact across server, MCP, and CLI roles.
//
// The lite build (cmd/breadbox/main_lite.go) skips the goose/pgx stdlib
// driver registration and ships the same shim under -tags=lite, producing
// a smaller CLI-only binary (typically built as `breadbox-cli`).
package main

import (
	"fmt"
	"os"

	"breadbox/internal/cli"

	// pgx stdlib driver registration is needed for goose's database/sql
	// migrations regardless of which subcommand runs.
	_ "github.com/jackc/pgx/v5/stdlib"

	// Embed the IANA tzdata into the binary so time.LoadLocation always
	// resolves named zones (e.g. "America/Los_Angeles") regardless of
	// whether the host ships /usr/share/zoneinfo. Without this, a deploy on
	// a minimal image (scratch/distroless, or a bare static binary) silently
	// falls back to time.Local for the viewer's bb_tz cookie zone, rendering
	// every absolute timestamp in the server's timezone. Cron schedule
	// timezones depend on the same lookup. ~450KB; correctness over size.
	_ "time/tzdata"
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

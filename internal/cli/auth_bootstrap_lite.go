//go:build lite

package cli

import (
	"context"
	"fmt"
)

// runAuthBootstrap on a lite (CLI-only) build cannot mint a key via the
// service layer because internal/db and internal/service are stripped from
// the binary. Direct the user to the device-code flow instead.
func runAuthBootstrap(_ context.Context, _, _ string) error {
	return fmt.Errorf("auth bootstrap requires a server-side build; from a CLI-only build use `breadbox auth login --host=URL` (device-code flow) or `--token=...`")
}

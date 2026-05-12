//go:build lite

package cli

import (
	"context"
	"fmt"
)

// runDoctorLocal is unreachable from CLI-only builds — db/service/sync/etc.
// are stripped. Operators on a lite build configure a host and rely on the
// remote-mode doctor.
func runDoctorLocal(_ context.Context, _ bool, _ bool) error {
	return fmt.Errorf("local doctor mode is unavailable in the CLI-only build; configure a host with `breadbox auth login --host=...` and re-run `breadbox doctor`")
}

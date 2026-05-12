//go:build headless

// Force NoDashboard=true on headless builds. The dashboard assets
// (internal/admin, internal/templates, web/) are stripped by build tags;
// flipping the runtime gate as well guarantees api/router.go never reaches
// the dashboard mount block, so even the dummy stubs in
// internal/admin/stubs_headless.go are never invoked.

package config

import "os"

func init() {
	// Use the same env-var path as the runtime flag so the existing
	// Load() pipeline sees it. Setting this before any Load() call (init
	// time is fine — config.Load is called from main()) makes parseBool
	// return true regardless of what the operator passed.
	_ = os.Setenv("BREADBOX_NO_DASHBOARD", "1")
}

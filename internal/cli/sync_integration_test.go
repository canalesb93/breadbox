//go:build integration && !lite

package cli_test

import (
	"context"
	"testing"

	"breadbox/internal/client"
)

// TestSyncTrigger_Returns202 is deliberately skipped here — the trigger
// handler returns 202 synchronously and spawns the actual sync work on a
// background goroutine that calls into svc.SyncEngine. The test env
// passes nil for that engine (we don't want to wire up a real sync run
// from a CLI integration test), so the spawned goroutine would panic
// after the request returns. The handler is exercised exhaustively in
// internal/api/*_integration_test.go.

func TestSyncStatus_ReturnsHealth(t *testing.T) {
	env := setupConnEnv(t)
	// No connections is fine — handler still returns a summary row.
	status, err := env.Client.SyncStatus(context.Background())
	if err != nil {
		t.Fatalf("SyncStatus: %v", err)
	}
	if status.OverallHealth == "" {
		t.Error("expected non-empty overall_health")
	}
}

func TestSyncLogs_ReturnsPage(t *testing.T) {
	env := setupConnEnv(t)
	// No fixtures — just assert the empty page shape lands as expected.
	res, err := env.Client.ListSyncLogs(context.Background(), client.SyncLogFilters{}, "", 5)
	if err != nil {
		t.Fatalf("ListSyncLogs: %v", err)
	}
	if res.Limit != 5 {
		t.Errorf("limit = %d, want 5", res.Limit)
	}
}

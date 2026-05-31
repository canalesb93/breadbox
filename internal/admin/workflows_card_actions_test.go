//go:build !headless && !lite

package admin

import (
	"testing"

	"breadbox/internal/service"
)

// strPtrF1 is a tiny local helper to take the address of a string literal in
// table cases. Prefixed to avoid clashing with sibling test helpers.
func strPtrF1(s string) *string { return &s }

// TestF1GalleryBuildWorkflowLastRun covers the pure mapping from a service run
// summary to the gallery card's last-run view: nil passthrough, the
// CompletedAt-over-StartedAt preference, the in-progress fallback to StartedAt,
// and graceful handling of an unparseable timestamp (zero FinishedAt, status
// still carried).
func TestF1GalleryBuildWorkflowLastRun(t *testing.T) {
	t.Run("nil run returns nil", func(t *testing.T) {
		if got := buildWorkflowLastRun(nil); got != nil {
			t.Fatalf("buildWorkflowLastRun(nil) = %+v, want nil", got)
		}
	})

	t.Run("prefers CompletedAt", func(t *testing.T) {
		run := &service.AgentRunSummary{
			ShortID:     "run12345",
			Status:      "success",
			StartedAt:   "2026-05-31T10:00:00Z",
			CompletedAt: strPtrF1("2026-05-31T10:05:00Z"),
		}
		got := buildWorkflowLastRun(run)
		if got == nil {
			t.Fatal("buildWorkflowLastRun returned nil for a non-nil run")
		}
		if got.ShortID != "run12345" || got.Status != "success" {
			t.Fatalf("identity not carried: %+v", got)
		}
		if got.FinishedAt.IsZero() {
			t.Fatal("FinishedAt is zero; expected the parsed CompletedAt")
		}
		if got.FinishedAt.Format("15:04") != "10:05" {
			t.Fatalf("FinishedAt = %s, want CompletedAt (10:05)", got.FinishedAt.Format("15:04"))
		}
	})

	t.Run("falls back to StartedAt when not completed", func(t *testing.T) {
		run := &service.AgentRunSummary{
			ShortID:   "inprog01",
			Status:    "in_progress",
			StartedAt: "2026-05-31T09:30:00Z",
			// CompletedAt nil — an in-progress run.
		}
		got := buildWorkflowLastRun(run)
		if got == nil {
			t.Fatal("buildWorkflowLastRun returned nil")
		}
		if got.FinishedAt.IsZero() {
			t.Fatal("FinishedAt is zero; expected the parsed StartedAt")
		}
		if got.FinishedAt.Format("15:04") != "09:30" {
			t.Fatalf("FinishedAt = %s, want StartedAt (09:30)", got.FinishedAt.Format("15:04"))
		}
	})

	t.Run("empty CompletedAt string falls back to StartedAt", func(t *testing.T) {
		run := &service.AgentRunSummary{
			Status:      "success",
			StartedAt:   "2026-05-31T08:00:00Z",
			CompletedAt: strPtrF1(""), // present but empty — must not win
		}
		got := buildWorkflowLastRun(run)
		if got == nil || got.FinishedAt.IsZero() {
			t.Fatalf("expected StartedAt fallback, got %+v", got)
		}
		if got.FinishedAt.Format("15:04") != "08:00" {
			t.Fatalf("FinishedAt = %s, want StartedAt (08:00)", got.FinishedAt.Format("15:04"))
		}
	})

	t.Run("unparseable timestamp leaves zero time but carries status", func(t *testing.T) {
		run := &service.AgentRunSummary{
			ShortID:   "badtime0",
			Status:    "error",
			StartedAt: "not-a-timestamp",
		}
		got := buildWorkflowLastRun(run)
		if got == nil {
			t.Fatal("buildWorkflowLastRun returned nil")
		}
		if !got.FinishedAt.IsZero() {
			t.Fatalf("FinishedAt = %v, want zero for unparseable input", got.FinishedAt)
		}
		if got.Status != "error" || got.ShortID != "badtime0" {
			t.Fatalf("status/short id not carried on parse failure: %+v", got)
		}
	})
}

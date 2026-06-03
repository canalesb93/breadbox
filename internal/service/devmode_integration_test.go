//go:build integration && !lite

package service_test

import (
	"context"
	"strings"
	"testing"

	"breadbox/internal/service"
)

func TestDevModeSettings_RoundTrip(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	got, err := svc.GetDevModeSettings(ctx)
	if err != nil {
		t.Fatalf("GetDevModeSettings: %v", err)
	}
	if got.Enabled {
		t.Error("expected developer mode off by default")
	}
	if got.IssueLabel != "dev-report" {
		t.Errorf("default label = %q, want dev-report", got.IssueLabel)
	}
	if got.GithubRepo != "canalesb93/breadbox" {
		t.Errorf("default repo = %q, want canalesb93/breadbox", got.GithubRepo)
	}

	enabled := true
	repo := "acme/widgets"
	label := "internal-report"
	updated, err := svc.UpdateDevModeSettings(ctx, service.UpdateDevModeSettingsParams{
		Enabled:    &enabled,
		GithubRepo: &repo,
		IssueLabel: &label,
	})
	if err != nil {
		t.Fatalf("UpdateDevModeSettings: %v", err)
	}
	if !updated.Enabled || updated.GithubRepo != repo || updated.IssueLabel != label {
		t.Errorf("unexpected updated settings: %+v", updated)
	}
	if !svc.DevModeEnabled(ctx) {
		t.Error("DevModeEnabled should be true")
	}

	// Malformed repo is rejected.
	bad := "not-a-repo"
	if _, err := svc.UpdateDevModeSettings(ctx, service.UpdateDevModeSettingsParams{GithubRepo: &bad}); err == nil {
		t.Error("expected error for malformed repo")
	}
}

func TestCreateDevReport_BuildsDraft(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	repo := "acme/widgets"
	if _, err := svc.UpdateDevModeSettings(ctx, service.UpdateDevModeSettingsParams{GithubRepo: &repo}); err != nil {
		t.Fatalf("set repo: %v", err)
	}

	res, err := svc.CreateDevReport(ctx, service.CreateDevReportInput{
		Type:        "task",
		Title:       "Add a CSV export button",
		Description: "Would be handy.",
		PageURL:     "http://localhost:8080/transactions",
		PagePath:    "/transactions",
		Metadata:    map[string]any{"theme": "dark", "viewport": "1280×800"},
		CreatedBy:   "admin@example.com",
	})
	if err != nil {
		t.Fatalf("CreateDevReport: %v", err)
	}
	if res.Status != "draft" {
		t.Errorf("status = %q, want draft", res.Status)
	}
	if !strings.Contains(res.DraftURL, "github.com/acme/widgets/issues/new") {
		t.Errorf("draft URL = %q", res.DraftURL)
	}
	if !strings.Contains(res.DraftURL, "Task") {
		t.Errorf("draft URL should carry the [Task] title: %q", res.DraftURL)
	}
}

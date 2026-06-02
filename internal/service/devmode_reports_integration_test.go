//go:build integration && !lite

package service_test

import (
	"bytes"
	"context"
	"testing"

	"breadbox/internal/service"
)

// devModeTestKey is a fixed 32-byte AES-256 key for the encrypted token tests.
var devModeTestKey = bytes.Repeat([]byte("k"), 32)

func TestDevModeSettings_RoundTrip(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// Defaults when nothing is set.
	got, err := svc.GetDevModeSettings(ctx, devModeTestKey)
	if err != nil {
		t.Fatalf("GetDevModeSettings: %v", err)
	}
	if got.Enabled {
		t.Error("expected developer mode off by default")
	}
	if got.IssueLabel != "dev-report" {
		t.Errorf("default label = %q, want dev-report", got.IssueLabel)
	}

	enabled := true
	repo := "canalesb93/breadbox"
	label := "internal-report"
	token := "ghp_supersecret_value_1234567890"
	updated, err := svc.UpdateDevModeSettings(ctx, service.UpdateDevModeSettingsParams{
		Enabled:     &enabled,
		GithubRepo:  &repo,
		IssueLabel:  &label,
		GithubToken: &token,
	}, devModeTestKey)
	if err != nil {
		t.Fatalf("UpdateDevModeSettings: %v", err)
	}
	if !updated.Enabled || updated.GithubRepo != repo || updated.IssueLabel != label {
		t.Errorf("unexpected updated settings: %+v", updated)
	}
	if !updated.HasToken {
		t.Error("expected HasToken true after setting a token")
	}
	if updated.TokenMask == nil || *updated.TokenMask == token {
		t.Errorf("token must be masked, not echoed: %v", updated.TokenMask)
	}
	if !svc.DevModeEnabled(ctx) {
		t.Error("DevModeEnabled should be true")
	}

	// Empty token = keep current.
	empty := ""
	kept, err := svc.UpdateDevModeSettings(ctx, service.UpdateDevModeSettingsParams{GithubToken: &empty}, devModeTestKey)
	if err != nil {
		t.Fatalf("UpdateDevModeSettings (keep): %v", err)
	}
	if !kept.HasToken {
		t.Error("empty token submit should keep the current token")
	}

	// Clear token.
	cleared, err := svc.UpdateDevModeSettings(ctx, service.UpdateDevModeSettingsParams{ClearGithubToken: true}, devModeTestKey)
	if err != nil {
		t.Fatalf("UpdateDevModeSettings (clear): %v", err)
	}
	if cleared.HasToken {
		t.Error("ClearGithubToken should remove the token")
	}

	// Invalid repo is rejected.
	bad := "not-a-repo"
	if _, err := svc.UpdateDevModeSettings(ctx, service.UpdateDevModeSettingsParams{GithubRepo: &bad}, devModeTestKey); err == nil {
		t.Error("expected error for malformed repo")
	}
}

func TestCreateDevReport_PersistsAndServesArtifacts(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	// Ensure no GitHub config so the call takes the "saved locally" path.
	emptyRepo := ""
	if _, err := svc.UpdateDevModeSettings(ctx, service.UpdateDevModeSettingsParams{
		GithubRepo:       &emptyRepo,
		ClearGithubToken: true,
	}, devModeTestKey); err != nil {
		t.Fatalf("clear config: %v", err)
	}

	png := []byte("\x89PNG\r\n\x1a\n-fake-bytes")
	in := service.CreateDevReportInput{
		Type:                  "task",
		Title:                 "Add a CSV export button",
		Description:           "Would be handy to export the filtered list.",
		PageURL:               "http://localhost:8080/transactions",
		PagePath:              "/transactions",
		ScreenshotData:        png,
		ScreenshotContentType: "image/png",
		HTMLSnapshot:          "<html><body>snapshot</body></html>",
		Metadata:              map[string]any{"theme": "dark", "viewport": "1280×800"},
		CreatedBy:             "admin@example.com",
	}

	res, err := svc.CreateDevReport(ctx, in, devModeTestKey)
	if err != nil {
		t.Fatalf("CreateDevReport: %v", err)
	}
	if res.ShortID == "" {
		t.Fatal("expected a short id")
	}
	if res.Status != "saved" {
		t.Errorf("status = %q, want saved (no GitHub config)", res.Status)
	}
	if res.Error == "" {
		t.Error("expected a non-empty note explaining the local-only save")
	}

	// Screenshot artifact round-trips.
	data, ct, err := svc.GetDevReportArtifact(ctx, res.ShortID)
	if err != nil {
		t.Fatalf("GetDevReportArtifact: %v", err)
	}
	if !bytes.Equal(data, png) {
		t.Error("screenshot bytes did not round-trip")
	}
	if ct != "image/png" {
		t.Errorf("content type = %q, want image/png", ct)
	}

	// HTML snapshot round-trips.
	html, err := svc.GetDevReportSnapshot(ctx, res.ShortID)
	if err != nil {
		t.Fatalf("GetDevReportSnapshot: %v", err)
	}
	if html != in.HTMLSnapshot {
		t.Errorf("snapshot = %q", html)
	}

	// The report shows up in the history list.
	list, err := svc.ListDevReports(ctx, 50)
	if err != nil {
		t.Fatalf("ListDevReports: %v", err)
	}
	var found *service.DevReportSummary
	for i := range list {
		if list[i].ShortID == res.ShortID {
			found = &list[i]
			break
		}
	}
	if found == nil {
		t.Fatal("created report not present in ListDevReports")
	}
	if found.Type != "task" || found.Title != in.Title || found.Status != "saved" {
		t.Errorf("unexpected summary: %+v", found)
	}
}

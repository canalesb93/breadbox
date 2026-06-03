//go:build integration && !lite

package service_test

import (
	"context"
	"errors"
	"testing"

	"breadbox/internal/service"
)

// TestUpdateNotificationChannel covers the edit-drawer semantics: non-secret
// fields update, a blank URL/token keeps the stored secret, and a provided
// URL/token replaces it.
func TestUpdateNotificationChannel(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	created, err := svc.AddNotificationChannel(ctx, service.AddNotificationChannelParams{
		Name:        "Family",
		URL:         "https://ntfy.sh/secret-topic",
		Format:      "ntfy",
		MinPriority: "info",
		NtfyToken:   "tk_original",
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	// Blank URL + token (nil) → keep secrets; other fields change.
	if _, err := svc.UpdateNotificationChannel(ctx, created.ID, service.UpdateNotificationChannelParams{
		Name:        "Family alerts",
		Format:      "ntfy",
		MinPriority: "critical",
		Enabled:     false,
		// URL and NtfyToken left nil → keep current.
	}); err != nil {
		t.Fatalf("update (keep secrets): %v", err)
	}
	got := getChannel(t, svc, created.ID)
	if got.Name != "Family alerts" || got.MinPriority != "critical" || got.Enabled {
		t.Fatalf("non-secret fields not updated: %+v", got)
	}
	if got.URL != "https://ntfy.sh/secret-topic" {
		t.Fatalf("blank URL should keep current, got %q", got.URL)
	}
	if got.NtfyToken != "tk_original" {
		t.Fatalf("blank token should keep current, got %q", got.NtfyToken)
	}

	// Provided URL + token → replace.
	newURL := "https://ntfy.sh/new-topic"
	newToken := "tk_rotated"
	if _, err := svc.UpdateNotificationChannel(ctx, created.ID, service.UpdateNotificationChannelParams{
		Name:        "Family alerts",
		URL:         &newURL,
		Format:      "ntfy",
		MinPriority: "critical",
		NtfyToken:   &newToken,
		Enabled:     true,
	}); err != nil {
		t.Fatalf("update (replace secrets): %v", err)
	}
	got = getChannel(t, svc, created.ID)
	if got.URL != newURL || got.NtfyToken != newToken || !got.Enabled {
		t.Fatalf("secrets/enabled not replaced: %+v", got)
	}

	// Unknown id → ErrNotFound.
	if _, err := svc.UpdateNotificationChannel(ctx, "nope1234", service.UpdateNotificationChannelParams{Format: "auto", MinPriority: "info"}); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("unknown id err = %v, want ErrNotFound", err)
	}
}

func getChannel(t *testing.T, svc *service.Service, id string) service.NotificationChannel {
	t.Helper()
	chans, err := svc.GetNotificationChannels(context.Background())
	if err != nil {
		t.Fatalf("get channels: %v", err)
	}
	for _, c := range chans {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("channel %s not found", id)
	return service.NotificationChannel{}
}

package pages

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestFeedEmptyStateVariants asserts the right empty-state copy + CTA
// renders for each of the four scenarios the dispatcher in feedTimeline
// covers. Each variant is constructed with a minimal FeedProps and
// rendered to a strings.Builder; we then probe for distinguishing text.
//
// This is a no-DB unit test — pure templ rendering — so it lives in the
// pages package alongside the component it exercises and runs under
// `go test ./...` without any integration build tag.
func TestFeedEmptyStateVariants(t *testing.T) {
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name        string
		props       FeedProps
		mustContain []string
		mustOmit    []string
	}{
		{
			name: "first_run_no_connections",
			props: FeedProps{
				Now:            now,
				WindowDays:     3,
				HasConnections: false,
			},
			mustContain: []string{
				"Welcome to Breadbox",
				"Connect your first bank to start filling your feed.",
				`href="/connections/new"`,
				"btn-primary",
			},
			mustOmit: []string{
				"Quiet around here",
				"Clear filter",
				"feedSyncNow",
			},
		},
		{
			name: "no_recent_admin_with_last_sync",
			props: FeedProps{
				Now:            now,
				WindowDays:     3,
				HasConnections: true,
				LastSyncAt:     now.Add(-2 * time.Hour),
				IsAdmin:        true,
			},
			mustContain: []string{
				"Quiet around here",
				"Last sync was",
				"Sync now",
				// The sync-all kickoff lives in static/js/admin/components/feed.js;
				// the page wires it in via x-data="feedSyncNow" on the empty-state
				// card and a <script src=…feed.js> tag immediately above. Asserting
				// on `feedSyncNow` covers both ends without coupling the test to
				// the inline URL string.
				"feedSyncNow",
				`/static/js/admin/components/feed.js`,
				`href="/transactions"`,
			},
			mustOmit: []string{
				"Welcome to Breadbox",
				"Clear filter",
			},
		},
		{
			name: "no_recent_member_no_sync_button",
			props: FeedProps{
				Now:            now,
				WindowDays:     3,
				HasConnections: true,
				LastSyncAt:     now.Add(-26 * time.Hour),
				IsAdmin:        false,
			},
			mustContain: []string{
				"Quiet around here",
				`href="/transactions"`,
			},
			mustOmit: []string{
				// non-admin members never see the sync trigger — neither the
				// button copy nor the wiring to the sync-all kickoff factory.
				"Sync now",
				"feedSyncNow",
			},
		},
		{
			name: "no_recent_no_sync_yet",
			props: FeedProps{
				Now:            now,
				WindowDays:     3,
				HasConnections: true,
				IsAdmin:        true,
			},
			mustContain: []string{
				"Quiet around here",
				"Try syncing to pull the latest",
				"Sync now",
			},
			mustOmit: []string{
				"Last sync was",
			},
		},
		{
			name: "filtered_no_match_comments",
			props: FeedProps{
				Now:            now,
				WindowDays:     3,
				HasConnections: true,
				Filter:         "comments",
				LastSyncAt:     now.Add(-1 * time.Hour),
			},
			mustContain: []string{
				"No comments in the last 3 days",
				"Clear filter",
				`href="/feed"`,
			},
			mustOmit: []string{
				"Welcome to Breadbox",
				"Quiet around here",
				"Sync now",
				"feedSyncNow",
			},
		},
		{
			name: "filtered_no_match_reports",
			props: FeedProps{
				Now:            now,
				WindowDays:     3,
				HasConnections: true,
				Filter:         "reports",
			},
			mustContain: []string{
				"No reports in the last 3 days",
				"Clear filter",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf strings.Builder
			if err := Feed(tc.props).Render(context.Background(), &buf); err != nil {
				t.Fatalf("Render returned error: %v", err)
			}
			html := buf.String()
			for _, want := range tc.mustContain {
				if !strings.Contains(html, want) {
					t.Errorf("expected rendered HTML to contain %q\nrendered (%d bytes):\n%s", want, buf.Len(), html)
				}
			}
			for _, omit := range tc.mustOmit {
				if strings.Contains(html, omit) {
					t.Errorf("expected rendered HTML to omit %q (was found)", omit)
				}
			}
		})
	}
}

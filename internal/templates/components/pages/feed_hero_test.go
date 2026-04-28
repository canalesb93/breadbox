package pages

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestFeedHeroNextSyncSubLine asserts the next-sync ETA sub-line under the
// Last Sync hero tile is gated on (a) a non-empty NextSyncRel and (b) a
// non-error LastSyncStatus. The tile renders the existing "Wells Fargo ·
// healthy" line in either case — what we toggle is the second muted line
// that points users at the upcoming cron fire.
//
// Pure templ render, no DB — runs under `go test ./...`.
func TestFeedHeroNextSyncSubLine(t *testing.T) {
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name        string
		hero        FeedHero
		mustContain []string
		mustOmit    []string
	}{
		{
			name: "healthy_with_next_sync_eta",
			hero: FeedHero{
				LastSyncAt:          now.Add(-38 * time.Minute),
				LastSyncRel:         "38 minutes ago",
				LastSyncStatus:      "success",
				LastSyncInstitution: "Wells Fargo",
				NextSyncRel:         "in ~6h",
			},
			mustContain: []string{
				"Wells Fargo",
				"healthy",
				"Next sync in ~6h",
				`data-lucide="clock"`,
			},
		},
		{
			name: "failing_hides_next_sync",
			hero: FeedHero{
				LastSyncAt:          now.Add(-2 * time.Hour),
				LastSyncRel:         "2 hours ago",
				LastSyncStatus:      "error",
				LastSyncInstitution: "Wells Fargo",
				NextSyncRel:         "in ~6h",
			},
			mustContain: []string{
				"Wells Fargo",
				"failing",
			},
			mustOmit: []string{
				"Next sync",
			},
		},
		{
			name: "healthy_without_eta_hides_sub_line",
			hero: FeedHero{
				LastSyncAt:          now.Add(-1 * time.Hour),
				LastSyncRel:         "1 hour ago",
				LastSyncStatus:      "success",
				LastSyncInstitution: "Wells Fargo",
				NextSyncRel:         "",
			},
			mustContain: []string{
				"healthy",
			},
			mustOmit: []string{
				"Next sync",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			props := FeedProps{
				Now:            now,
				WindowDays:     3,
				HasConnections: true,
				Hero:           tc.hero,
			}
			var buf strings.Builder
			if err := Feed(props).Render(context.Background(), &buf); err != nil {
				t.Fatalf("Render returned error: %v", err)
			}
			html := buf.String()
			for _, want := range tc.mustContain {
				if !strings.Contains(html, want) {
					t.Errorf("expected rendered HTML to contain %q\nrendered (%d bytes)", want, buf.Len())
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

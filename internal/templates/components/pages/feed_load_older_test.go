package pages

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestFeedLoadOlderButton asserts the footer "Load older activity"
// pagination affordance renders the right shape for each branch:
//
//   - empty rail (zero items, OldestVisible zero) → no button
//   - in-window (oldest event < 30 days old) → button with the oldest
//     timestamp threaded into `?before=…`, filter preserved
//   - past the 30-day cap (AtMaxLookback) → "End of feed" sentence, no button
//
// Pure templ render, no DB — runs under `go test ./...`.
func TestFeedLoadOlderButton(t *testing.T) {
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)

	// One sync row in the rendered window — gives `feedTimeline` something
	// to render so the load-older footer actually emits. Empty days would
	// take the empty-state branch and skip the footer entirely.
	syncDay := func(ts time.Time) []FeedDay {
		return []FeedDay{{
			Key:   ts.Format("2006-01-02"),
			Label: "Today",
			First: true,
			Items: []FeedItem{{
				Type:         "sync",
				Timestamp:    ts,
				TimestampStr: ts.UTC().Format(time.RFC3339),
				Sync: &FeedSync{
					SyncLogID:       "sync-1",
					InstitutionName: "Wells Fargo",
					Provider:        "plaid",
					Status:          "success",
					AddedCount:      3,
					StartedAt:       ts,
				},
			}},
		}}
	}

	cases := []struct {
		name        string
		props       FeedProps
		mustContain []string
		mustOmit    []string
	}{
		{
			name: "in_window_renders_button_with_before_param",
			props: FeedProps{
				Now:            now,
				WindowDays:     3,
				HasConnections: true,
				Days:           syncDay(now.Add(-2 * time.Hour)),
				TotalItems:     1,
				OldestVisible:  now.Add(-2 * time.Hour),
				AtMaxLookback:  false,
			},
			mustContain: []string{
				"Load older activity",
				`href="/feed?before=2026-04-28T10:00:00Z"`,
				"Showing the last 3 days",
			},
			mustOmit: []string{
				"End of feed",
			},
		},
		{
			name: "preserves_active_filter_in_load_older_href",
			props: FeedProps{
				Now:            now,
				WindowDays:     3,
				HasConnections: true,
				Days:           syncDay(now.Add(-90 * time.Minute)),
				TotalItems:     1,
				OldestVisible:  now.Add(-90 * time.Minute),
				Filter:         "syncs",
			},
			mustContain: []string{
				"Load older activity",
				`href="/feed?filter=syncs&amp;before=2026-04-28T10:30:00Z"`,
			},
		},
		{
			name: "past_30_day_cap_shows_end_of_feed",
			props: FeedProps{
				Now:            now,
				WindowDays:     3,
				HasConnections: true,
				Days:           syncDay(now.Add(-31 * 24 * time.Hour)),
				TotalItems:     1,
				OldestVisible:  now.Add(-31 * 24 * time.Hour),
				AtMaxLookback:  true,
			},
			mustContain: []string{
				"End of feed",
			},
			mustOmit: []string{
				"Load older activity",
			},
		},
		{
			name: "empty_rail_no_button",
			props: FeedProps{
				Now:            now,
				WindowDays:     3,
				HasConnections: true,
				Days:           nil,
				TotalItems:     0,
			},
			mustOmit: []string{
				"Load older activity",
				"End of feed",
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

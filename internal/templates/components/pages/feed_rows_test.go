//go:build !headless && !lite

package pages

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestFeedRows covers the inline-pagination partial render (FeedRows) used by
// the home feed's "Load older activity" append flow:
//
//   - each day bucket renders a day separator plus its feed rows;
//   - the leading day separator is omitted when its key matches
//     OmitLeadingDayKey (the tail day already on the client) so appended rows
//     continue under the existing heading — but that day's item rows still
//     render;
//   - the leading separator is kept when the key doesn't match.
//
// Pure templ render, no DB — runs under `go test ./...`.
func TestFeedRows(t *testing.T) {
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)

	syncItem := func(ts time.Time, inst string) FeedItem {
		return FeedItem{
			Type:         "sync",
			Timestamp:    ts,
			TimestampStr: ts.UTC().Format(time.RFC3339),
			Sync: &FeedSync{
				SyncLogID:       "sync-" + inst,
				InstitutionName: inst,
				Provider:        "plaid",
				Status:          "success",
				AddedCount:      2,
				StartedAt:       ts,
			},
		}
	}

	days := []FeedDay{
		{
			Key:   "2026-04-25",
			Label: "Apr 25",
			Items: []FeedItem{syncItem(now.Add(-3*24*time.Hour), "Wells Fargo")},
		},
		{
			Key:   "2026-04-24",
			Label: "Apr 24",
			Items: []FeedItem{syncItem(now.Add(-4*24*time.Hour), "Chase")},
		},
	}

	t.Run("renders_both_day_separators_when_no_omit", func(t *testing.T) {
		html := renderFeedRows(t, FeedRowsProps{Days: days, Now: now})
		for _, want := range []string{
			`aria-label="Apr 25"`, // day separator only emits aria-label
			`aria-label="Apr 24"`,
			"Wells Fargo",
			"Chase",
		} {
			if !strings.Contains(html, want) {
				t.Errorf("expected rendered rows to contain %q", want)
			}
		}
	})

	t.Run("omits_leading_separator_matching_anchor", func(t *testing.T) {
		html := renderFeedRows(t, FeedRowsProps{
			Days:              days,
			Now:               now,
			OmitLeadingDayKey: "2026-04-25",
		})
		if strings.Contains(html, `aria-label="Apr 25"`) {
			t.Errorf("expected leading day separator (Apr 25) to be omitted")
		}
		// Its item rows still render — only the duplicate heading is dropped.
		if !strings.Contains(html, "Wells Fargo") {
			t.Errorf("expected the omitted day's item rows to still render")
		}
		// The next day's separator is untouched.
		if !strings.Contains(html, `aria-label="Apr 24"`) {
			t.Errorf("expected the second day separator (Apr 24) to render")
		}
	})

	t.Run("keeps_leading_separator_when_anchor_differs", func(t *testing.T) {
		html := renderFeedRows(t, FeedRowsProps{
			Days:              days,
			Now:               now,
			OmitLeadingDayKey: "2026-04-26", // not the first day's key
		})
		if !strings.Contains(html, `aria-label="Apr 25"`) {
			t.Errorf("expected leading day separator (Apr 25) to render when anchor differs")
		}
	})
}

func renderFeedRows(t *testing.T, p FeedRowsProps) string {
	t.Helper()
	var buf strings.Builder
	if err := FeedRows(p).Render(context.Background(), &buf); err != nil {
		t.Fatalf("FeedRows render: %v", err)
	}
	return buf.String()
}

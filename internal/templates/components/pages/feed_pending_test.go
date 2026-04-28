package pages

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ptrString returns &s — handy for building FeedTransactionRef.Category*
// pointers inline in test cases without intermediate variables.
func ptrString(s string) *string { return &s }

// TestFeedTxRefRowPendingPill asserts the inline transaction-reference row
// renders the canonical clock-icon pending mark when the underlying
// transaction is pending, and renders cleanly without it when posted.
//
// The clock-icon convention is the same used by `tx_row.templ` and
// `tx_row_compact.templ` — pending rows often re-show as posted later so
// the feed needs to surface the lifecycle state alongside the amount.
//
// Pure templ render, no DB. Drives the row through a sync-card so the
// outer scaffolding (Feed) exercises the same branch the page uses.
func TestFeedTxRefRowPendingPill(t *testing.T) {
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)

	feedWith := func(tx FeedTransactionRef) FeedProps {
		ts := now.Add(-2 * time.Hour)
		return FeedProps{
			Now:            now,
			WindowDays:     3,
			HasConnections: true,
			TotalItems:     1,
			Days: []FeedDay{{
				Key:   ts.Format("2006-01-02"),
				Label: "Today",
				First: true,
				Items: []FeedItem{{
					Type:         "sync",
					Timestamp:    ts,
					TimestampStr: ts.UTC().Format(time.RFC3339),
					Sync: &FeedSync{
						SyncLogID:          "sync-1",
						InstitutionName:    "Wells Fargo",
						Provider:           "plaid",
						Status:             "success",
						AddedCount:         1,
						StartedAt:          ts,
						SampleTransactions: []FeedTransactionRef{tx},
					},
				}},
			}},
		}
	}

	cases := []struct {
		name        string
		tx          FeedTransactionRef
		mustContain []string
		mustOmit    []string
	}{
		{
			name: "pending_renders_clock_icon_and_sr_text",
			tx: FeedTransactionRef{
				ShortID:      "tx12345a",
				MerchantName: "Whole Foods",
				Amount:       42.10,
				Currency:     "USD",
				AccountName:  "Chase Checking",
				Pending:      true,
			},
			mustContain: []string{
				`data-lucide="clock"`,
				"text-warning/70",
				"Pending — not yet posted",
				"(pending)",
				"Whole Foods",
			},
		},
		{
			name: "posted_omits_clock_icon",
			tx: FeedTransactionRef{
				ShortID:      "tx12345b",
				MerchantName: "Whole Foods",
				Amount:       42.10,
				Currency:     "USD",
				AccountName:  "Chase Checking",
				Pending:      false,
			},
			mustContain: []string{
				"Whole Foods",
			},
			mustOmit: []string{
				"Pending — not yet posted",
				"(pending)",
			},
		},
		{
			// All-caps merchant from a provider feed must be rendered in
			// title-case so the row matches the /transactions list. Tests
			// the `titleCase` pipeline applied to feedTxRefRow as part of
			// the tx-row consistency pass (PR #970).
			name: "all_caps_merchant_renders_title_case",
			tx: FeedTransactionRef{
				ShortID:      "tx12345c",
				MerchantName: "WATER SUPPLY COMPANY",
				Amount:       102.40,
				Currency:     "USD",
				AccountName:  "Essential Savings",
				Pending:      false,
			},
			mustContain: []string{
				"Water Supply Company",
			},
			mustOmit: []string{
				"WATER SUPPLY COMPANY",
			},
		},
		{
			// When a category icon + color are present the row must render
			// the same coloured-circle avatar used by tx_row_compact.templ
			// (the /transactions list). Letter avatar fallback otherwise.
			name: "category_avatar_renders_lucide_icon",
			tx: FeedTransactionRef{
				ShortID:             "tx12345d",
				MerchantName:        "Whole Foods",
				Amount:              42.10,
				Currency:            "USD",
				AccountName:         "Chase Checking",
				CategoryDisplayName: ptrString("Groceries"),
				CategoryColor:       ptrString("oklch(0.7 0.15 140)"),
				CategoryIcon:        ptrString("shopping-cart"),
				CategorySlug:        ptrString("food_and_drink_groceries"),
			},
			mustContain: []string{
				`data-lucide="shopping-cart"`,
				"bb-tx-avatar bb-tx-avatar--sm",
			},
			mustOmit: []string{
				"bb-tx-avatar--letter",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf strings.Builder
			if err := Feed(feedWith(tc.tx)).Render(context.Background(), &buf); err != nil {
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

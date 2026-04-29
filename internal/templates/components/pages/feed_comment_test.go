package pages

import (
	"context"
	"strings"
	"testing"
	"time"

	"breadbox/internal/templates/components"
)

// TestFeedCommentRowUsesSharedCommentTile verifies the consolidation done in
// #969: every /feed comment row renders the shared `TimelineCommentTile`
// (a neutral 24px message-square on the rail) and the actor's avatar moves
// inline before the bolded actor name. The old per-feed avatar tile has
// been retired, so neither `feedCommentAvatar` markup nor a 24px <img>
// avatar should appear inside the rail anchor anymore.
//
// Pure templ render — no DB. Drives the row through a full Feed render so
// the surrounding scaffolding exercises the same code path users hit.
func TestFeedCommentRowUsesSharedCommentTile(t *testing.T) {
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	ts := now.Add(-15 * time.Minute)

	feedWith := func(c FeedComment) FeedProps {
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
					Type:         "comment",
					Timestamp:    ts,
					TimestampStr: ts.UTC().Format(time.RFC3339),
					Comment:      &c,
				}},
			}},
		}
	}

	cases := []struct {
		name        string
		comment     FeedComment
		mustContain []string
		mustOmit    []string
	}{
		{
			name: "user_comment_renders_message_square_tile_and_inline_avatar",
			comment: FeedComment{
				ActorType:          "user",
				ActorID:            "user-abc",
				ActorName:          "Alice",
				ActorAvatarVersion: "v1",
				Transaction: FeedTransactionRef{
					ShortID:      "tx9000aa",
					MerchantName: "Whole Foods",
					Amount:       42.10,
					Currency:     "USD",
				},
			},
			mustContain: []string{
				// Rail tile is the shared message-square, not an avatar.
				`data-lucide="message-square"`,
				// Inline avatar uses the 16px treatment from
				// TimelineActorInline.
				`inline-block w-4 h-4 rounded-full object-cover border border-base-300 align-text-bottom`,
				// Bold actor name + verb.
				`Alice`,
				`commented on`,
				`Whole Foods`,
			},
			mustOmit: []string{
				// Old 24px avatar tile classes used to scaffold the
				// rail before #969.
				`w-6 h-6 rounded-full object-cover ring-4 ring-base-200`,
			},
		},
		{
			name: "agent_comment_renders_message_square_tile_and_inline_bot",
			comment: FeedComment{
				ActorType: "agent",
				ActorName: "Categorizer",
				Transaction: FeedTransactionRef{
					ShortID:      "tx9000bb",
					MerchantName: "Costco",
					Amount:       12.34,
					Currency:     "USD",
				},
			},
			mustContain: []string{
				`data-lucide="message-square"`,
				// Inline bot tile, 16px (w-4 h-4) — distinct from the
				// 24px (w-6 h-6) rail tile that used to be there.
				`inline-flex items-center justify-center w-4 h-4 rounded-full bg-primary/10`,
				`Categorizer`,
				`commented on`,
			},
			mustOmit: []string{
				`bg-primary/12 ring-4 ring-base-200`,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf strings.Builder
			if err := Feed(feedWith(tc.comment)).Render(context.Background(), &buf); err != nil {
				t.Fatalf("Render: %v", err)
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

// TestFeedAndTxDetailShareInlineActor asserts the shared
// `components.TimelineActorInline` primitive renders the same actor markup
// no matter which surface it's invoked from. We don't need to render the
// whole transaction-detail page to prove the consolidation — both
// `feedCommentActor` and the equivalent tx-detail adapter ultimately hand
// the same `TimelineActor` shape to the same primitive, so rendering
// `TimelineActorInline` directly with that shape is the byte-equivalent
// of what each page would emit.
func TestFeedAndTxDetailShareInlineActor(t *testing.T) {
	cases := []struct {
		name      string
		actor     components.TimelineActor
		mustHave  []string
		mustOmit  []string
	}{
		{
			name: "user_with_avatar",
			actor: components.TimelineActor{
				Name:      "Alice",
				AvatarURL: "/avatars/user-abc?v=v1",
			},
			mustHave: []string{
				`<img src="/avatars/user-abc?v=v1"`,
				`inline-block w-4 h-4 rounded-full object-cover border border-base-300 align-text-bottom mr-1`,
				`<strong class="font-semibold text-base-content">Alice</strong>`,
			},
		},
		{
			name: "agent_renders_bot_tile",
			actor: components.TimelineActor{
				Name:    "Categorizer",
				IsAgent: true,
			},
			mustHave: []string{
				`bg-primary/10`,
				`data-lucide="bot"`,
				`<strong class="font-semibold text-base-content">Categorizer</strong>`,
			},
			mustOmit: []string{
				`<img`,
			},
		},
		{
			name: "system_actor_renders_only_bold_name",
			actor: components.TimelineActor{
				Name: "Breadbox",
			},
			mustHave: []string{
				`<strong class="font-semibold text-base-content">Breadbox</strong>`,
			},
			mustOmit: []string{
				`<img`,
				`data-lucide="bot"`,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf strings.Builder
			if err := components.TimelineActorInline(tc.actor).Render(context.Background(), &buf); err != nil {
				t.Fatalf("Render: %v", err)
			}
			html := buf.String()
			for _, want := range tc.mustHave {
				if !strings.Contains(html, want) {
					t.Errorf("expected %q in:\n%s", want, html)
				}
			}
			for _, omit := range tc.mustOmit {
				if strings.Contains(html, omit) {
					t.Errorf("expected to omit %q in:\n%s", omit, html)
				}
			}
		})
	}
}

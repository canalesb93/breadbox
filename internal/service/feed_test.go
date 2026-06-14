//go:build !lite

package service

import (
	"testing"
	"time"
)

// TestDecodeRuleHitCounts pins the on-disk shape of the `sync_logs.rule_hits`
// column. It is written by RuleResolver.HitCountsJSON as a flat
// {ruleUUID: count} object — NOT an array of {rule_id,count} objects. A prior
// version of the feed parser unmarshalled into a []struct, which silently
// failed against the real object payload and suppressed every rule-outcome
// line on the home feed.
func TestDecodeRuleHitCounts(t *testing.T) {
	id1 := "11111111-1111-1111-1111-111111111111"
	id2 := "22222222-2222-2222-2222-222222222222"

	t.Run("canonical map payload", func(t *testing.T) {
		got := decodeRuleHitCounts([]byte(`{"` + id1 + `":3,"` + id2 + `":1}`))
		if len(got) != 2 {
			t.Fatalf("expected 2 entries, got %d (%v)", len(got), got)
		}
		if got[id1] != 3 || got[id2] != 1 {
			t.Fatalf("unexpected counts: %v", got)
		}
	})

	t.Run("empty and blank payloads yield nil", func(t *testing.T) {
		for _, p := range []string{"", "{}", "[]"} {
			if got := decodeRuleHitCounts([]byte(p)); got != nil {
				t.Fatalf("payload %q: expected nil, got %v", p, got)
			}
		}
	})

	t.Run("malformed json yields nil", func(t *testing.T) {
		if got := decodeRuleHitCounts([]byte(`not json`)); got != nil {
			t.Fatalf("expected nil for malformed json, got %v", got)
		}
	})

	t.Run("legacy array shape is rejected, not parsed", func(t *testing.T) {
		// The shape the old parser expected. It is never written by the
		// resolver; decoding it into the map must fail closed (nil) rather
		// than silently succeed.
		if got := decodeRuleHitCounts([]byte(`[{"rule_id":"` + id1 + `","count":2}]`)); got != nil {
			t.Fatalf("expected nil for legacy array shape, got %v", got)
		}
	})
}

// TestFilterFeedEvents covers the chip-driven post-filter applied by
// `ListFeedEvents` over its grouped output. The pipeline that produces the
// input slice is integration-tested elsewhere; this unit test pins the
// filter switch independently of the DB so each chip's contract is easy
// to read in isolation.
func TestFilterFeedEvents(t *testing.T) {
	now := time.Now()
	mkSync := func() FeedEvent {
		return FeedEvent{Type: "sync", Timestamp: now, Sync: &FeedSyncEvent{}}
	}
	mkComment := func(actorID string) FeedEvent {
		return FeedEvent{
			Type:      "comment",
			Timestamp: now,
			Comment:   &FeedCommentEvent{ActorID: actorID},
		}
	}
	mkSession := func(actorID string) FeedEvent {
		return FeedEvent{
			Type:         "agent_session",
			Timestamp:    now,
			AgentSession: &FeedAgentSessionEvent{ActorID: actorID},
		}
	}
	mkBulk := func(actorID string) FeedEvent {
		return FeedEvent{
			Type:       "bulk_action",
			Timestamp:  now,
			BulkAction: &FeedBulkActionEvent{ActorID: actorID},
		}
	}

	events := []FeedEvent{
		mkSync(),
		mkComment("user-A"),
		mkComment("user-B"),
		mkSession("user-A"),
		mkBulk("user-B"),
	}

	cases := []struct {
		name    string
		filter  string
		actorID string
		want    []string // expected ev.Type sequence
	}{
		{"empty filter passes all", "", "", []string{"sync", "comment", "comment", "agent_session", "bulk_action"}},
		{"unknown filter passes all", "wat", "", []string{"sync", "comment", "comment", "agent_session", "bulk_action"}},
		{"reports drops everything from service layer", "reports", "", nil},
		{"syncs keeps only sync events", "syncs", "", []string{"sync"}},
		{"comments keeps only comment events", "comments", "", []string{"comment", "comment"}},
		{"sessions keeps only agent_session events", "sessions", "", []string{"agent_session"}},
		{"me with empty actor downgrades to all", "me", "", []string{"sync", "comment", "comment", "agent_session", "bulk_action"}},
		{"me filters to comments + sessions + bulk by actor", "me", "user-A", []string{"comment", "agent_session"}},
		{"me with non-matching actor returns nothing", "me", "user-Z", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := filterFeedEvents(events, tc.filter, tc.actorID)
			if len(got) != len(tc.want) {
				t.Fatalf("len(got)=%d want=%d (got types: %v)", len(got), len(tc.want), typesOf(got))
			}
			for i, ev := range got {
				if ev.Type != tc.want[i] {
					t.Errorf("event %d: got type=%q want=%q", i, ev.Type, tc.want[i])
				}
			}
		})
	}
}

func typesOf(evs []FeedEvent) []string {
	out := make([]string, len(evs))
	for i, ev := range evs {
		out[i] = ev.Type
	}
	return out
}

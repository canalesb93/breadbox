package service

import (
	"testing"
	"time"
)

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

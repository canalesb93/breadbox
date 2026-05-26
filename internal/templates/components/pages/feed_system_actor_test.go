//go:build !headless && !lite

package pages

import (
	"testing"
)

// TestFeedActorAvatarIDDropsSystemID guards the rendering fix for the
// stdio MCP singleton bug: a `system` actor row whose actor_id points
// at an api_keys UUID (the stdio singleton's UUID) used to slip into
// /avatars/{uuid}?type=user, miss the users lookup, and fall through
// to a humanoid DiceBear seeded on the api_keys UUID. Dropping the ID
// on `system` rows forces UserAvatar into its bot-tile fallback.
func TestFeedActorAvatarIDDropsSystemID(t *testing.T) {
	cases := []struct {
		name      string
		actorType string
		actorID   string
		want      string
	}{
		{"system actor drops api_keys UUID", "system", "11111111-2222-3333-4444-555555555555", ""},
		{"system actor with empty id stays empty", "system", "", ""},
		{"agent actor keeps its id", "agent", "agent-uuid", "agent-uuid"},
		{"user actor keeps its id", "user", "user-uuid", "user-uuid"},
		{"unknown actor type passes through (no special case)", "", "raw-id", "raw-id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := feedActorAvatarID(tc.actorType, tc.actorID); got != tc.want {
				t.Fatalf("feedActorAvatarID(%q, %q) = %q, want %q", tc.actorType, tc.actorID, got, tc.want)
			}
		})
	}
}

// TestFeedCommentActorSystemRendersAsBot guards the comment-actor
// half of the same fix: a `system`-authored comment used to fall
// through the switch with IsAgent=false and a stale UserID, so the
// 16px inline avatar rendered as a humanoid DiceBear. The fix sets
// IsAgent=true and drops UserID so the bot-tile fallback wins.
func TestFeedCommentActorSystemRendersAsBot(t *testing.T) {
	c := &FeedComment{
		ActorType:          "system",
		ActorID:            "11111111-2222-3333-4444-555555555555",
		ActorName:          "Local MCP",
		ActorAvatarVersion: "v1",
	}
	got := feedCommentActor(c)
	if !got.IsAgent {
		t.Fatalf("feedCommentActor: system actor IsAgent = false, want true")
	}
	if got.UserID != "" {
		t.Fatalf("feedCommentActor: system actor UserID = %q, want empty (would render humanoid DiceBear)", got.UserID)
	}
	if got.Version != "" {
		t.Fatalf("feedCommentActor: system actor Version = %q, want empty", got.Version)
	}
}

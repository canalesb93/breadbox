package admin

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"breadbox/internal/service"
	"breadbox/internal/timefmt"
)

func TestBuildPaginationBase_URLEncodesValues(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		contains string // substring that must appear in result
		excludes string // substring that must NOT appear in result
	}{
		{
			name:     "ampersand in search",
			query:    "search=foo%26bar",
			contains: "search=foo%26bar",
			excludes: "search=foo&bar&",
		},
		{
			name:     "equals sign in search",
			query:    "search=foo%3Dbar",
			contains: "search=foo%3Dbar",
		},
		{
			name:     "plus sign in search",
			query:    "search=foo%2Bbar",
			contains: "search=foo%2Bbar",
		},
		{
			name:     "space in search",
			query:    "search=foo+bar",
			contains: "search=foo+bar",
		},
		{
			name:     "normal value passes through",
			query:    "search=groceries&pending=true",
			contains: "search=groceries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/transactions?"+tt.query, nil)
			result := buildPaginationBase(req)

			if tt.contains != "" {
				if !strings.Contains(result, tt.contains) {
					t.Errorf("expected result to contain %q, got %q", tt.contains, result)
				}
			}
			if tt.excludes != "" {
				if strings.Contains(result, tt.excludes) {
					t.Errorf("expected result NOT to contain %q, got %q", tt.excludes, result)
				}
			}
		})
	}
}

func TestBuildExportURL_URLEncodesValues(t *testing.T) {
	req, _ := http.NewRequest("GET", "/transactions?search=foo%26bar", nil)
	result := buildExportURL(req)

	if !strings.Contains(result, "search=foo%26bar") {
		t.Errorf("expected URL-encoded ampersand in export URL, got %q", result)
	}
	// The raw ampersand should not appear as a parameter separator within the value
	if strings.Contains(result, "search=foo&bar") {
		t.Errorf("raw ampersand should be encoded in export URL, got %q", result)
	}
}

// The activity timeline is built purely from annotations — comments,
// tag_added/tag_removed, rule_applied, category_set. Review resolutions are
// not a distinct timeline event type.

func TestBuildActivityTimeline_FreeStandingCommentStillEmitted(t *testing.T) {
	// CommentID must be populated from the annotation's own short_id — the
	// write path never sets payload.comment_id, so the template-gated delete
	// button only renders when we surface ShortID here. See #703.
	annotations := []service.Annotation{{
		ID:        "ann-free",
		ShortID:   "ax9Ku7Qp",
		Kind:      "comment",
		ActorName: "Alice",
		ActorType: "user",
		Payload: map[string]interface{}{
			"content": "split with Bob",
		},
		CreatedAt: "2026-04-04T12:00:00Z",
	}}

	entries := buildActivityTimeline(annotations, nil, nil)

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Type != "comment" {
		t.Errorf("expected comment entry, got type=%q", entries[0].Type)
	}
	if entries[0].CommentID != "ax9Ku7Qp" {
		t.Errorf("expected CommentID to come from annotation ShortID (ax9Ku7Qp), got %q", entries[0].CommentID)
	}
}

// Rule-sourced tag_added annotations are represented by the paired
// rule_applied annotation that sync writes alongside them. The timeline
// must dedup the tag_added so rule-driven tag applications do not render
// twice (once as "Added tag X" and once as "Rule '…' added tag X").
// Mirrors the existing category_set dedup.
func TestBuildActivityTimeline_RuleSourcedTagAddedDeduped(t *testing.T) {
	ruleID := "rule-1"
	annotations := []service.Annotation{
		{
			ID:        "ann-tag",
			Kind:      "tag_added",
			ActorName: "Auto-tag new transactions for review",
			ActorType: "system",
			ActorID:   &ruleID,
			Payload: map[string]interface{}{
				"slug":      "needs-review",
				"source":    "rule",
				"rule_id":   ruleID,
				"rule_name": "Auto-tag new transactions for review",
			},
			CreatedAt: "2026-04-04T12:00:00Z",
		},
		{
			ID:        "ann-rule",
			Kind:      "rule_applied",
			ActorName: "Auto-tag new transactions for review",
			ActorType: "system",
			ActorID:   &ruleID,
			Payload: map[string]interface{}{
				"rule_name": "Auto-tag new transactions for review",
				"how":       "tag_added",
				"slug":      "needs-review",
			},
			CreatedAt: "2026-04-04T12:00:00Z",
		},
	}

	entries := buildActivityTimeline(annotations, nil, nil)

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (rule_applied only), got %d", len(entries))
	}
	if entries[0].Type != "rule" {
		t.Errorf("expected the surviving entry to be a rule entry, got type=%q", entries[0].Type)
	}
}

// User-authored tag additions must still render as regular "Added tag …" entries.
func TestBuildActivityTimeline_UserTagAddedStillEmitted(t *testing.T) {
	userID := "user-1"
	annotations := []service.Annotation{{
		ID:        "ann-user-tag",
		Kind:      "tag_added",
		ActorName: "Alice",
		ActorType: "user",
		ActorID:   &userID,
		Payload: map[string]interface{}{
			"slug": "vacation",
			// no "source" field — user-authored
		},
		CreatedAt: "2026-04-05T09:00:00Z",
	}}

	entries := buildActivityTimeline(annotations, nil, nil)

	if len(entries) != 1 {
		t.Fatalf("expected 1 tag entry, got %d", len(entries))
	}
	if entries[0].Type != "tag" {
		t.Errorf("expected tag entry, got type=%q", entries[0].Type)
	}
	if entries[0].TagSlug != "vacation" {
		t.Errorf("expected TagSlug=vacation, got %q", entries[0].TagSlug)
	}
}

// Older rule_applied rows can have empty rule_name/rule_id (pre-ruleapply
// helper retroactive runs). Render a generic subject instead of the raw
// `Rule "" set category to food_and_drink_groceries` copy that #704 reported.
func TestBuildActivityTimeline_RuleAppliedEmptyNameFallsBack(t *testing.T) {
	annotations := []service.Annotation{{
		ID:        "ann-rule-empty",
		Kind:      "rule_applied",
		ActorName: "",
		ActorType: "system",
		Payload: map[string]interface{}{
			"rule_id":       "",
			"rule_name":     "",
			"applied_by":    "retroactive",
			"action_field":  "category",
			"action_value":  "food_and_drink_groceries",
		},
		CreatedAt: "2026-04-04T12:00:00Z",
	}}

	catLookup := func(slug string) categoryDisplay {
		if slug == "food_and_drink_groceries" {
			return categoryDisplay{DisplayName: "Food & Drink › Groceries"}
		}
		return categoryDisplay{DisplayName: slug}
	}

	entries := buildActivityTimeline(annotations, catLookup, nil)

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	want := `A rule set category to Food & Drink › Groceries`
	if entries[0].Summary != want {
		t.Errorf("summary = %q, want %q", entries[0].Summary, want)
	}
	if entries[0].RuleID != "" {
		t.Errorf("expected empty RuleID so template skips /rules/<id> link, got %q", entries[0].RuleID)
	}
	// Rule rows must not overload ActorName with a verb phrase.
	if entries[0].ActorName != "" {
		t.Errorf("rule row ActorName should be empty, got %q", entries[0].ActorName)
	}
	// Origin carries the retroactively/during-sync qualifier instead.
	if entries[0].Origin != "retroactively" {
		t.Errorf("Origin = %q, want %q", entries[0].Origin, "retroactively")
	}
}

// Named rule rows must keep their quoted-name subject and the link-eligible
// RuleID — the fallback is only for the empty-name edge case.
func TestBuildActivityTimeline_RuleAppliedNamedStillQuoted(t *testing.T) {
	ruleID := "rule-1"
	annotations := []service.Annotation{{
		ID:        "ann-rule-named",
		Kind:      "rule_applied",
		ActorName: "Walgreens auto-categorize",
		ActorType: "system",
		ActorID:   &ruleID,
		RuleID:    &ruleID,
		Payload: map[string]interface{}{
			"rule_id":      ruleID,
			"rule_name":    "Walgreens auto-categorize",
			"applied_by":   "sync",
			"action_field": "category",
			"action_value": "shopping",
		},
		CreatedAt: "2026-04-04T12:00:00Z",
	}}

	catLookup := func(slug string) categoryDisplay {
		if slug == "shopping" {
			return categoryDisplay{DisplayName: "Shopping"}
		}
		return categoryDisplay{DisplayName: slug}
	}

	entries := buildActivityTimeline(annotations, catLookup, nil)

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	want := `Rule "Walgreens auto-categorize" set category to Shopping`
	if entries[0].Summary != want {
		t.Errorf("summary = %q, want %q", entries[0].Summary, want)
	}
	if entries[0].RuleID != ruleID {
		t.Errorf("expected RuleID=%q, got %q", ruleID, entries[0].RuleID)
	}
	if entries[0].ActorName != "" {
		t.Errorf("rule row ActorName should be empty, got %q", entries[0].ActorName)
	}
	if entries[0].Origin != "during sync" {
		t.Errorf("Origin = %q, want %q", entries[0].Origin, "during sync")
	}
}

// groupActivityByDay buckets the sorted-desc activity slice into per-day
// groups so the template can render day separators. Entries without a
// parseable timestamp are dropped. Ordering within a day is preserved.
func TestGroupActivityByDay_BucketsByLocalDate(t *testing.T) {
	// Use times far from midnight so the UTC→local conversion lands on the
	// same calendar date regardless of the server's timezone offset.
	entries := []service.ActivityEntry{
		{Type: "comment", Timestamp: "2026-04-16T15:00:00Z", Summary: "a"},
		{Type: "comment", Timestamp: "2026-04-16T14:00:00Z", Summary: "b"},
		{Type: "comment", Timestamp: "2026-04-14T15:00:00Z", Summary: "c"},
	}

	groups := groupActivityByDay(entries, time.Now(), time.Local)

	if len(groups) != 2 {
		t.Fatalf("expected 2 day groups, got %d", len(groups))
	}
	if len(groups[0].Events) != 2 {
		t.Errorf("expected 2 events in first group, got %d", len(groups[0].Events))
	}
	if len(groups[1].Events) != 1 {
		t.Errorf("expected 1 event in second group, got %d", len(groups[1].Events))
	}
	// Input order (desc) is preserved within a bucket.
	if groups[0].Events[0].Summary != "a" || groups[0].Events[1].Summary != "b" {
		t.Errorf("expected events in input order, got %v, %v", groups[0].Events[0].Summary, groups[0].Events[1].Summary)
	}
	// Labels are non-empty.
	for i, g := range groups {
		if g.Label == "" {
			t.Errorf("group %d has empty Label", i)
		}
		if g.Date == "" {
			t.Errorf("group %d has empty Date", i)
		}
	}
}

func TestGroupActivityByDay_EmptyInput(t *testing.T) {
	now := time.Now()
	if groups := groupActivityByDay(nil, now, time.Local); groups != nil {
		t.Errorf("expected nil for nil input, got %v", groups)
	}
	if groups := groupActivityByDay([]service.ActivityEntry{}, now, time.Local); groups != nil {
		t.Errorf("expected nil for empty input, got %v", groups)
	}
}

func TestGroupActivityByDay_DropsUnparseableTimestamps(t *testing.T) {
	entries := []service.ActivityEntry{
		{Type: "comment", Timestamp: "2026-04-16T09:00:00Z", Summary: "ok"},
		{Type: "comment", Timestamp: "not-a-date", Summary: "bad"},
	}
	groups := groupActivityByDay(entries, time.Now(), time.Local)
	if len(groups) != 1 {
		t.Fatalf("expected 1 day group, got %d", len(groups))
	}
	if len(groups[0].Events) != 1 || groups[0].Events[0].Summary != "ok" {
		t.Errorf("expected only the parseable entry to survive, got %+v", groups[0].Events)
	}
}

// At a midnight boundary the day-bucket label and the per-row relative-time
// helper must agree because they share the same now anchor. Before this fix
// each path captured its own time.Now(), so a row at 23:55 could end up
// grouped under "Today" with a "yesterday" timestamp (or vice-versa). The
// fix threads a single now through groupActivityByDay + timefmt.RelativeAt.
func TestGroupActivityByDay_SharesNowAnchorWithRelative(t *testing.T) {
	loc := time.Local
	// Anchor: 2026-04-27 00:10:00 local. The 23:55 row sits in "yesterday"
	// (2026-04-26) and the 00:05 row in "today" (2026-04-27).
	now := time.Date(2026, 4, 27, 0, 10, 0, 0, loc)
	yesterdayLate := time.Date(2026, 4, 26, 23, 55, 0, 0, loc)
	todayEarly := time.Date(2026, 4, 27, 0, 5, 0, 0, loc)

	entries := []service.ActivityEntry{
		{Type: "comment", Timestamp: todayEarly.Format(time.RFC3339), Summary: "today-early"},
		{Type: "comment", Timestamp: yesterdayLate.Format(time.RFC3339), Summary: "yesterday-late"},
	}

	groups := groupActivityByDay(entries, now, time.Local)
	if len(groups) != 2 {
		t.Fatalf("expected 2 day groups, got %d", len(groups))
	}

	// First group is the today-early row (input order preserved).
	today := groups[0]
	yesterday := groups[1]

	if today.Label != "Today" {
		t.Errorf("today bucket label = %q, want %q", today.Label, "Today")
	}
	if yesterday.Label != "Yesterday" {
		t.Errorf("yesterday bucket label = %q, want %q", yesterday.Label, "Yesterday")
	}

	// Per-row relative time, anchored against the same now, must put the
	// 23:55 row at "15 minutes ago" (a < 1h bucket — i.e. clearly NOT
	// "1 day ago", which is what an unanchored Relative() drift could
	// produce if its time.Now() crossed midnight independently).
	yesterdayRel := timefmt.RelativeAt(yesterdayLate, now)
	if yesterdayRel != "15 minutes ago" {
		t.Errorf("yesterday-late relative = %q, want %q", yesterdayRel, "15 minutes ago")
	}
	todayRel := timefmt.RelativeAt(todayEarly, now)
	if todayRel != "5 minutes ago" {
		t.Errorf("today-early relative = %q, want %q", todayRel, "5 minutes ago")
	}
}

// update_transactions can write a tag_added with a note AND a standalone
// comment in the same call. The timeline should collapse the duplicate
// comment (the note already renders inline on the tag row) so users don't
// see twin near-identical rows.
func TestBuildActivityTimeline_TagNoteAndDuplicateCommentDeduped(t *testing.T) {
	actor := "user-1"
	note := "Please review this one again"
	annotations := []service.Annotation{
		{
			ID:        "ann-tag",
			Kind:      "tag_added",
			ActorID:   &actor,
			ActorName: "admin@example.com",
			ActorType: "user",
			Payload: map[string]interface{}{
				"slug": "needs-review",
				"note": note,
			},
			CreatedAt: "2026-04-16T03:39:34.502794Z",
		},
		{
			ID:        "ann-comment",
			Kind:      "comment",
			ActorID:   &actor,
			ActorName: "admin@example.com",
			ActorType: "user",
			Payload: map[string]interface{}{
				"content": note,
			},
			CreatedAt: "2026-04-16T03:39:34.513524Z",
		},
	}

	entries := buildActivityTimeline(annotations, nil, nil)

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (duplicate comment suppressed), got %d", len(entries))
	}
	if entries[0].Type != "tag" {
		t.Errorf("expected the tag entry to survive, got type=%q", entries[0].Type)
	}
	if entries[0].Detail != note {
		t.Errorf("expected note to render inline as Detail, got %q", entries[0].Detail)
	}
}

// A standalone comment with no matching tag note must still be emitted.
func TestBuildActivityTimeline_StandaloneCommentNotDeduped(t *testing.T) {
	actor := "user-1"
	annotations := []service.Annotation{{
		ID:        "ann-standalone",
		Kind:      "comment",
		ActorID:   &actor,
		ActorName: "admin@example.com",
		ActorType: "user",
		Payload: map[string]interface{}{
			"content": "Just a standalone note",
		},
		CreatedAt: "2026-04-16T03:39:34Z",
	}}

	entries := buildActivityTimeline(annotations, nil, nil)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Type != "comment" {
		t.Errorf("expected comment type, got %q", entries[0].Type)
	}
}

// A comment far outside the ±2s window of a matching tag_added note must NOT
// be deduped — users can legitimately comment the same text later.
func TestBuildActivityTimeline_DistantCommentNotDeduped(t *testing.T) {
	actor := "user-1"
	note := "duplicate text"
	annotations := []service.Annotation{
		{
			ID:        "ann-tag",
			Kind:      "tag_added",
			ActorID:   &actor,
			ActorName: "admin@example.com",
			ActorType: "user",
			Payload: map[string]interface{}{
				"slug": "needs-review",
				"note": note,
			},
			CreatedAt: "2026-04-16T03:39:34Z",
		},
		{
			ID:        "ann-comment-later",
			Kind:      "comment",
			ActorID:   &actor,
			ActorName: "admin@example.com",
			ActorType: "user",
			Payload: map[string]interface{}{
				"content": note,
			},
			CreatedAt: "2026-04-16T03:40:34Z", // 60s later
		},
	}

	entries := buildActivityTimeline(annotations, nil, nil)
	if len(entries) != 2 {
		t.Fatalf("expected both entries to survive (distant timestamps), got %d", len(entries))
	}
}

// Tag timeline rows now carry display name + color for chip rendering and a
// TagAction discriminator for the plus/minus icon overlay.
func TestBuildActivityTimeline_TagRowHasChipMetadata(t *testing.T) {
	color := "#ff9800"
	lookup := func(slug string) tagDisplay {
		if slug == "needs-review" {
			return tagDisplay{DisplayName: "Needs Review", Color: &color}
		}
		return tagDisplay{DisplayName: slug}
	}
	annotations := []service.Annotation{
		{
			ID:        "ann-add",
			Kind:      "tag_added",
			ActorName: "admin@example.com",
			ActorType: "user",
			Payload: map[string]interface{}{
				"slug": "needs-review",
			},
			CreatedAt: "2026-04-16T03:39:34Z",
		},
		{
			ID:        "ann-rem",
			Kind:      "tag_removed",
			ActorName: "admin@example.com",
			ActorType: "user",
			Payload: map[string]interface{}{
				"slug": "needs-review",
			},
			CreatedAt: "2026-04-16T04:00:00Z",
		},
		{
			ID:        "ann-unknown",
			Kind:      "tag_added",
			ActorName: "admin@example.com",
			ActorType: "user",
			Payload: map[string]interface{}{
				"slug": "deleted-tag-slug",
			},
			CreatedAt: "2026-04-16T05:00:00Z",
		},
	}

	entries := buildActivityTimeline(annotations, nil, lookup)
	if len(entries) != 3 {
		t.Fatalf("expected 3 tag entries, got %d", len(entries))
	}
	byKey := map[string]service.ActivityEntry{}
	for _, e := range entries {
		if e.Type != "tag" {
			t.Fatalf("expected type=tag, got %q", e.Type)
		}
		byKey[e.TagAction+"/"+e.TagSlug] = e
	}

	added := byKey["added/needs-review"]
	if added.TagDisplayName != "Needs Review" {
		t.Errorf("added.TagDisplayName = %q, want %q", added.TagDisplayName, "Needs Review")
	}
	if added.TagColor == nil || *added.TagColor != color {
		t.Errorf("added.TagColor not propagated, got %v", added.TagColor)
	}
	if added.Summary != "Added tag" {
		t.Errorf("added.Summary = %q, want just \"Added tag\" (chip renders the name)", added.Summary)
	}

	removed := byKey["removed/needs-review"]
	if removed.TagAction != "removed" {
		t.Errorf("removed.TagAction = %q", removed.TagAction)
	}
	if removed.TagColor == nil || *removed.TagColor != color {
		t.Errorf("removed.TagColor not propagated, got %v", removed.TagColor)
	}

	// Unknown/deleted tag falls back to slug as display name, no color.
	unknown := byKey["added/deleted-tag-slug"]
	if unknown.TagDisplayName != "deleted-tag-slug" {
		t.Errorf("unknown tag fallback DisplayName = %q, want slug", unknown.TagDisplayName)
	}
	if unknown.TagColor != nil {
		t.Errorf("unknown tag should have nil color, got %v", unknown.TagColor)
	}
}

func TestBuildActivityTimeline_LegacyPrefixCommentSuppressed(t *testing.T) {
	annotations := []service.Annotation{{
		ID:        "ann-legacy",
		Kind:      "comment",
		ActorName: "Legacy",
		ActorType: "system",
		Payload: map[string]interface{}{
			"content":    "[Review: Some note migrated before consolidation]",
			"comment_id": "comment-legacy",
		},
		CreatedAt: "2026-03-01T00:00:00Z",
	}}

	entries := buildActivityTimeline(annotations, nil, nil)

	if len(entries) != 0 {
		t.Fatalf("expected legacy [Review: ...] comment to be suppressed, got %d entries", len(entries))
	}
}

// ActorAvatarVersion must flow from the joined-row Annotation through into
// the ActivityEntry for actor-rendered rows (comment, tag_added, tag_removed,
// category_set). The template uses it to cache-bust the avatar URL so that an
// avatar upload invalidates the timeline image immediately rather than living
// in the browser cache for the avatar handler's 24h max-age window. System and
// agent rows must surface an empty version — no user avatar to fingerprint.
func TestBuildActivityTimeline_PlumbsAvatarVersion(t *testing.T) {
	userID := "user-1"
	annotations := []service.Annotation{
		{
			ID:                 "ann-comment",
			Kind:               "comment",
			ActorName:          "Alice",
			ActorType:          "user",
			ActorID:            &userID,
			ActorAvatarVersion: "1700000000",
			Payload:            map[string]interface{}{"content": "Looks right"},
			CreatedAt:          "2026-04-05T09:00:00Z",
		},
		{
			ID:                 "ann-tag",
			Kind:               "tag_added",
			ActorName:          "Alice",
			ActorType:          "user",
			ActorID:            &userID,
			ActorAvatarVersion: "1700000000",
			Payload:            map[string]interface{}{"slug": "vacation"},
			CreatedAt:          "2026-04-05T09:01:00Z",
		},
		{
			ID:                 "ann-tag-removed",
			Kind:               "tag_removed",
			ActorName:          "Alice",
			ActorType:          "user",
			ActorID:            &userID,
			ActorAvatarVersion: "1700000000",
			Payload:            map[string]interface{}{"slug": "vacation"},
			CreatedAt:          "2026-04-05T09:02:00Z",
		},
		{
			ID:                 "ann-cat",
			Kind:               "category_set",
			ActorName:          "Alice",
			ActorType:          "user",
			ActorID:            &userID,
			ActorAvatarVersion: "1700000000",
			Payload:            map[string]interface{}{"category_slug": "groceries"},
			CreatedAt:          "2026-04-05T09:03:00Z",
		},
		{
			// rule_applied is system-authored — no avatar version.
			ID:        "ann-rule",
			Kind:      "rule_applied",
			ActorName: "",
			ActorType: "system",
			Payload: map[string]interface{}{
				"rule_id":      "rule-1",
				"rule_name":    "Auto-categorize",
				"applied_by":   "sync",
				"action_field": "category",
				"action_value": "groceries",
			},
			CreatedAt: "2026-04-05T09:04:00Z",
		},
	}

	entries := buildActivityTimeline(annotations, nil, nil)

	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}

	// Find each entry by Type+Summary fingerprint and assert.
	for _, e := range entries {
		switch {
		case e.Type == "comment", e.Type == "tag", e.Type == "category":
			if e.ActorAvatarVersion != "1700000000" {
				t.Errorf("user-actor %s entry: ActorAvatarVersion = %q, want %q", e.Type, e.ActorAvatarVersion, "1700000000")
			}
		case e.Type == "rule":
			if e.ActorAvatarVersion != "" {
				t.Errorf("system rule entry: ActorAvatarVersion = %q, want empty", e.ActorAvatarVersion)
			}
		}
	}
}

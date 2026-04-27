package service

import (
	"testing"
)

// EnrichAnnotations is the canonical home of the dedup + summary logic that
// powers the admin activity timeline and the MCP list_annotations tool. The
// tests below cover its three responsibilities:
//
//  1. Drop rule-source structural rows (tag_added/tag_removed/category_set
//     written alongside a rule_applied) so consumers don't see the same event
//     twice.
//  2. Drop the standalone-comment half of an MCP update_transactions call
//     that wrote a tag-with-note plus an identical comment within ±2s.
//  3. Compute Action/Summary/Subject/Origin/Source/Note/Content/TagSlug/
//     CategorySlug/RuleName from the kind+payload.
//
// They are pure unit tests (no DB) — the helper takes EnrichOptions closures
// directly so we don't need s.ListTags / s.ListCategoryTree.

func tagDisplayFromMap(by map[string]string) func(string) string {
	return func(slug string) string {
		if name, ok := by[slug]; ok {
			return name
		}
		return slug
	}
}

func TestEnrichAnnotations_TagAddedHumanizesAndComposesSummary(t *testing.T) {
	rows := []Annotation{{
		ID:        "a1",
		Kind:      "tag_added",
		ActorName: "Alice",
		Payload: map[string]interface{}{
			"slug": "food",
		},
		CreatedAt: "2026-04-04T12:00:00Z",
	}}
	out := EnrichAnnotations(rows, EnrichOptions{
		TagDisplay: tagDisplayFromMap(map[string]string{"food": "Food"}),
	})
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1", len(out))
	}
	got := out[0]
	if got.Action != "added" {
		t.Errorf("Action = %q, want added", got.Action)
	}
	if got.TagSlug != "food" {
		t.Errorf("TagSlug = %q, want food", got.TagSlug)
	}
	if got.Subject != "Food" {
		t.Errorf("Subject = %q, want Food", got.Subject)
	}
	if got.Summary != "Alice added the Food tag" {
		t.Errorf("Summary = %q, want %q", got.Summary, "Alice added the Food tag")
	}
}

func TestEnrichAnnotations_TagRemovedWithNoteAppendsRationale(t *testing.T) {
	rows := []Annotation{{
		ID:        "a1",
		Kind:      "tag_removed",
		ActorName: "Bob",
		Payload: map[string]interface{}{
			"slug": "needs-review",
			"note": "wrong category",
		},
		CreatedAt: "2026-04-04T12:00:00Z",
	}}
	out := EnrichAnnotations(rows, EnrichOptions{
		TagDisplay: tagDisplayFromMap(map[string]string{"needs-review": "Needs Review"}),
	})
	got := out[0]
	if got.Action != "removed" {
		t.Errorf("Action = %q, want removed", got.Action)
	}
	if got.Note != "wrong category" {
		t.Errorf("Note = %q, want \"wrong category\"", got.Note)
	}
	want := "Bob removed the Needs Review tag — wrong category"
	if got.Summary != want {
		t.Errorf("Summary = %q, want %q", got.Summary, want)
	}
}

// Rule-source tag_added/tag_removed/category_set rows are deduped — the
// parent rule_applied row carries the same information.
func TestEnrichAnnotations_DropsRuleSourcedTagAdded(t *testing.T) {
	ruleID := "rule-1"
	rows := []Annotation{
		{
			ID:        "a-tag",
			Kind:      "tag_added",
			ActorName: "Auto-tag",
			ActorType: "system",
			ActorID:   &ruleID,
			Payload: map[string]interface{}{
				"slug":   "needs-review",
				"source": "rule",
			},
			CreatedAt: "2026-04-04T12:00:00Z",
		},
		{
			ID:        "a-rule",
			Kind:      "rule_applied",
			ActorName: "Auto-tag",
			ActorType: "system",
			ActorID:   &ruleID,
			RuleID:    &ruleID,
			Payload: map[string]interface{}{
				"rule_name":    "Auto-tag",
				"action_field": "tag",
				"action_value": "needs-review",
				"applied_by":   "sync",
			},
			CreatedAt: "2026-04-04T12:00:00Z",
		},
	}
	out := EnrichAnnotations(rows, EnrichOptions{})
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1 (rule_applied only)", len(out))
	}
	if out[0].Kind != "rule_applied" {
		t.Errorf("survivor kind = %q, want rule_applied", out[0].Kind)
	}
	if out[0].Action != "applied" {
		t.Errorf("Action = %q, want applied", out[0].Action)
	}
	if out[0].Origin != "during sync" {
		t.Errorf("Origin = %q, want \"during sync\"", out[0].Origin)
	}
	if out[0].TagSlug != "needs-review" {
		t.Errorf("TagSlug = %q, want needs-review", out[0].TagSlug)
	}
	wantSummary := `Rule "Auto-tag" added tag needs-review during sync`
	if out[0].Summary != wantSummary {
		t.Errorf("Summary = %q, want %q", out[0].Summary, wantSummary)
	}
}

// User-authored tag additions (no source=rule) survive enrichment.
func TestEnrichAnnotations_KeepsUserAuthoredTagAdded(t *testing.T) {
	rows := []Annotation{{
		ID:        "a1",
		Kind:      "tag_added",
		ActorName: "Alice",
		Payload: map[string]interface{}{
			"slug": "vacation",
		},
		CreatedAt: "2026-04-04T12:00:00Z",
	}}
	out := EnrichAnnotations(rows, EnrichOptions{})
	if len(out) != 1 || out[0].Kind != "tag_added" {
		t.Fatalf("expected user tag_added to survive, got %v", out)
	}
}

func TestEnrichAnnotations_RuleAppliedRetroactiveOrigin(t *testing.T) {
	rows := []Annotation{{
		ID:        "a1",
		Kind:      "rule_applied",
		ActorName: "",
		ActorType: "system",
		Payload: map[string]interface{}{
			"rule_name":    "Walgreens",
			"action_field": "category",
			"action_value": "shopping",
			"applied_by":   "retroactive",
		},
		CreatedAt: "2026-04-04T12:00:00Z",
	}}
	out := EnrichAnnotations(rows, EnrichOptions{
		CategoryDisplay: tagDisplayFromMap(map[string]string{"shopping": "Shopping"}),
	})
	got := out[0]
	if got.Origin != "retroactively" {
		t.Errorf("Origin = %q, want retroactively", got.Origin)
	}
	if got.CategorySlug != "shopping" {
		t.Errorf("CategorySlug = %q, want shopping", got.CategorySlug)
	}
	want := `Rule "Walgreens" set category to Shopping retroactively`
	if got.Summary != want {
		t.Errorf("Summary = %q, want %q", got.Summary, want)
	}
}

// Empty rule_name on a rule_applied row produces the "A rule" fallback
// rather than `Rule ""` (mirrors the admin-handler invariant from #704).
func TestEnrichAnnotations_RuleAppliedEmptyNameFallsBack(t *testing.T) {
	rows := []Annotation{{
		ID:        "a1",
		Kind:      "rule_applied",
		ActorType: "system",
		Payload: map[string]interface{}{
			"rule_name":    "",
			"action_field": "category",
			"action_value": "food_and_drink_groceries",
			"applied_by":   "retroactive",
		},
		CreatedAt: "2026-04-04T12:00:00Z",
	}}
	out := EnrichAnnotations(rows, EnrichOptions{
		CategoryDisplay: tagDisplayFromMap(map[string]string{"food_and_drink_groceries": "Food & Drink › Groceries"}),
	})
	want := "A rule set category to Food & Drink › Groceries retroactively"
	if out[0].Summary != want {
		t.Errorf("Summary = %q, want %q", out[0].Summary, want)
	}
}

// Comment with content identical to an adjacent tag_added.note from the
// same actor within ±2s is deduped (it's the standalone-comment half of
// an MCP update_transactions call). The tag row already inlines the note.
func TestEnrichAnnotations_DropsCommentDuplicatingAdjacentTagNote(t *testing.T) {
	actor := "user-1"
	note := "Please review this one again"
	rows := []Annotation{
		{
			ID:        "a-tag",
			Kind:      "tag_added",
			ActorID:   &actor,
			ActorName: "alice",
			Payload: map[string]interface{}{
				"slug": "needs-review",
				"note": note,
			},
			CreatedAt: "2026-04-16T03:39:34.502794Z",
		},
		{
			ID:        "a-comment",
			Kind:      "comment",
			ActorID:   &actor,
			ActorName: "alice",
			Payload: map[string]interface{}{
				"content": note,
			},
			CreatedAt: "2026-04-16T03:39:34.513524Z",
		},
	}
	out := EnrichAnnotations(rows, EnrichOptions{})
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1 (duplicate comment dropped)", len(out))
	}
	if out[0].Kind != "tag_added" {
		t.Errorf("survivor kind = %q, want tag_added", out[0].Kind)
	}
}

// Tombstoned comments must NEVER be folded into an adjacent tag-with-note
// even when the timestamps and actor match — a deletion is a distinct audit
// event and the timeline must surface it independently.
func TestEnrichAnnotations_TombstoneNeverFoldsIntoTagNote(t *testing.T) {
	actor := "user-1"
	note := "Please review this one again"
	rows := []Annotation{
		{
			ID:        "a-tag",
			Kind:      "tag_added",
			ActorID:   &actor,
			ActorName: "alice",
			Payload: map[string]interface{}{
				"slug": "needs-review",
				"note": note,
			},
			CreatedAt: "2026-04-16T03:39:34.502794Z",
		},
		{
			ID:        "a-comment",
			Kind:      "comment",
			ActorID:   &actor,
			ActorName: "alice",
			Payload: map[string]interface{}{
				"content": note,
			},
			CreatedAt: "2026-04-16T03:39:34.513524Z",
			IsDeleted: true,
		},
	}
	out := EnrichAnnotations(rows, EnrichOptions{})
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2 (tombstone must survive enrichment)", len(out))
	}
	// Tombstone Summary is the muted "<Actor> deleted a comment" sentence,
	// not the original body.
	gotComment := out[1]
	if gotComment.Kind != "comment" || !gotComment.IsDeleted {
		t.Fatalf("expected tombstoned comment to survive; got kind=%q deleted=%v", gotComment.Kind, gotComment.IsDeleted)
	}
	if gotComment.Summary != "alice deleted a comment" {
		t.Errorf("Summary = %q, want %q", gotComment.Summary, "alice deleted a comment")
	}
}

// A bare tombstone (no actor name) renders the system-shape "Comment deleted"
// sentence so the timeline still narrates the event.
func TestEnrichAnnotations_TombstoneSummaryFallsBackWithoutActor(t *testing.T) {
	rows := []Annotation{{
		ID:        "ghost",
		Kind:      "comment",
		Payload:   map[string]interface{}{"content": "old body"},
		CreatedAt: "2026-04-16T03:39:34Z",
		IsDeleted: true,
	}}
	out := EnrichAnnotations(rows, EnrichOptions{})
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1", len(out))
	}
	if out[0].Summary != "Comment deleted" {
		t.Errorf("Summary = %q, want %q", out[0].Summary, "Comment deleted")
	}
}

// A comment outside the ±2s window of a matching tag_added.note is NOT
// deduped — users can legitimately repeat the same text minutes later.
func TestEnrichAnnotations_KeepsDistantComment(t *testing.T) {
	actor := "user-1"
	note := "duplicate text"
	rows := []Annotation{
		{
			ID:        "a-tag",
			Kind:      "tag_added",
			ActorID:   &actor,
			ActorName: "alice",
			Payload: map[string]interface{}{
				"slug": "needs-review",
				"note": note,
			},
			CreatedAt: "2026-04-16T03:39:34Z",
		},
		{
			ID:        "a-comment",
			Kind:      "comment",
			ActorID:   &actor,
			ActorName: "alice",
			Payload: map[string]interface{}{
				"content": note,
			},
			CreatedAt: "2026-04-16T03:40:34Z", // 60s later
		},
	}
	out := EnrichAnnotations(rows, EnrichOptions{})
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2 (distant timestamps preserved)", len(out))
	}
}

// Legacy [Review: ...] prefixed comments are import noise from the
// pre-consolidation reviews table and get suppressed by enrichment.
func TestEnrichAnnotations_DropsLegacyReviewPrefixedComment(t *testing.T) {
	rows := []Annotation{{
		ID:        "legacy",
		Kind:      "comment",
		ActorName: "Legacy",
		ActorType: "system",
		Payload: map[string]interface{}{
			"content": "[Review: Some note migrated before consolidation]",
		},
		CreatedAt: "2026-03-01T00:00:00Z",
	}}
	out := EnrichAnnotations(rows, EnrichOptions{})
	if len(out) != 0 {
		t.Errorf("len = %d, want 0 (legacy prefix should be suppressed)", len(out))
	}
}

func TestEnrichAnnotations_CommentSummaryIncludesPreview(t *testing.T) {
	rows := []Annotation{{
		ID:        "c1",
		Kind:      "comment",
		ActorName: "Alice",
		Payload: map[string]interface{}{
			"content": "split with Bob",
		},
		CreatedAt: "2026-04-04T12:00:00Z",
	}}
	out := EnrichAnnotations(rows, EnrichOptions{})
	got := out[0]
	// Comments deliberately do NOT carry an Action — kind="comment" is
	// the discriminator, and the existing MCP integration test pins
	// this contract.
	if got.Action != "" {
		t.Errorf("Action = %q, want empty (comment kind)", got.Action)
	}
	if got.Content != "split with Bob" {
		t.Errorf("Content = %q, want \"split with Bob\"", got.Content)
	}
	want := "Alice commented: split with Bob"
	if got.Summary != want {
		t.Errorf("Summary = %q, want %q", got.Summary, want)
	}
}

func TestEnrichAnnotations_CommentSummaryTruncatesLongBodies(t *testing.T) {
	body := ""
	for i := 0; i < 200; i++ {
		body += "x"
	}
	rows := []Annotation{{
		ID:        "c1",
		Kind:      "comment",
		ActorName: "Alice",
		Payload:   map[string]interface{}{"content": body},
		CreatedAt: "2026-04-04T12:00:00Z",
	}}
	out := EnrichAnnotations(rows, EnrichOptions{})
	got := out[0]
	// Content carries the full body; Summary truncates with an ellipsis
	// at the 120-char preview budget. Rune count gives us a UTF-8-safe
	// upper bound — "…" is one rune but 3 bytes.
	if got.Content != body {
		t.Errorf("Content should be unmodified, got %d chars", len(got.Content))
	}
	runeCount := len([]rune(got.Summary))
	maxRunes := len([]rune("Alice commented: ")) + 120 + 1 // 120 preview + 1 ellipsis
	if runeCount > maxRunes {
		t.Errorf("Summary should be truncated, got %d runes (max %d): %q", runeCount, maxRunes, got.Summary)
	}
	tail := []rune(got.Summary)
	if len(tail) == 0 || tail[len(tail)-1] != '…' {
		t.Errorf("Summary should end with ellipsis, got %q", got.Summary)
	}
}

// Multi-line comment bodies preview only the first line in Summary so the
// timeline row stays single-line. Full body is on Content.
func TestEnrichAnnotations_CommentSummaryStopsAtFirstNewline(t *testing.T) {
	rows := []Annotation{{
		ID:        "c1",
		Kind:      "comment",
		ActorName: "Alice",
		Payload:   map[string]interface{}{"content": "first line\nsecond line\nthird"},
		CreatedAt: "2026-04-04T12:00:00Z",
	}}
	out := EnrichAnnotations(rows, EnrichOptions{})
	want := "Alice commented: first line…"
	if out[0].Summary != want {
		t.Errorf("Summary = %q, want %q", out[0].Summary, want)
	}
}

// Unknown kinds round-trip with their structural fields intact and an
// empty Action/Summary so future event types aren't silently dropped.
func TestEnrichAnnotations_UnknownKindRoundTrips(t *testing.T) {
	rows := []Annotation{{
		ID:        "a1",
		Kind:      "future_event",
		ActorName: "Alice",
		Payload:   map[string]interface{}{"foo": "bar"},
		CreatedAt: "2026-04-04T12:00:00Z",
	}}
	out := EnrichAnnotations(rows, EnrichOptions{})
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1", len(out))
	}
	got := out[0]
	if got.Kind != "future_event" {
		t.Errorf("Kind = %q, want future_event", got.Kind)
	}
	if got.Action != "" || got.Summary != "" {
		t.Errorf("expected empty derived fields for unknown kind, got Action=%q Summary=%q", got.Action, got.Summary)
	}
}

func TestEnrichAnnotations_PreservesOrder(t *testing.T) {
	rows := []Annotation{
		{ID: "a", Kind: "comment", ActorName: "alice", Payload: map[string]interface{}{"content": "1"}, CreatedAt: "2026-04-01T00:00:00Z"},
		{ID: "b", Kind: "comment", ActorName: "bob", Payload: map[string]interface{}{"content": "2"}, CreatedAt: "2026-04-02T00:00:00Z"},
		{ID: "c", Kind: "comment", ActorName: "cathy", Payload: map[string]interface{}{"content": "3"}, CreatedAt: "2026-04-03T00:00:00Z"},
	}
	out := EnrichAnnotations(rows, EnrichOptions{})
	if len(out) != 3 {
		t.Fatalf("len = %d, want 3", len(out))
	}
	for i, want := range []string{"a", "b", "c"} {
		if out[i].ID != want {
			t.Errorf("out[%d].ID = %q, want %q", i, out[i].ID, want)
		}
	}
}

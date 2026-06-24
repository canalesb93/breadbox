//go:build !lite

package service

import (
	"strings"
	"testing"
	"unicode/utf8"
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

// A long multibyte comment body must truncate on a rune boundary, never
// mid-character — byte slicing at the 120 budget would split a multibyte
// rune and emit invalid UTF-8 into the timeline summary.
func TestEnrichAnnotations_CommentSummaryTruncatesMultibyteOnRuneBoundary(t *testing.T) {
	// One ASCII byte followed by 3-byte runes shifts the rune grid so the
	// 120-byte budget lands *inside* a rune — a byte slice at [:120] would
	// split it and corrupt the UTF-8; a rune slice cuts cleanly.
	body := "a" + strings.Repeat("世", 200)
	rows := []Annotation{{
		ID:        "c1",
		Kind:      "comment",
		ActorName: "Alice",
		Payload:   map[string]interface{}{"content": body},
		CreatedAt: "2026-04-04T12:00:00Z",
	}}
	out := EnrichAnnotations(rows, EnrichOptions{})
	got := out[0].Summary

	if !utf8.ValidString(got) {
		t.Fatalf("Summary is not valid UTF-8 (rune split mid-character): %q", got)
	}
	if !strings.HasPrefix(got, "Alice commented: ") {
		t.Errorf("Summary lost its prefix: %q", got)
	}
	if r := []rune(got); r[len(r)-1] != '…' {
		t.Errorf("Summary should end with ellipsis, got %q", got)
	}
	// The preview must be exactly 120 runes of content plus the ellipsis —
	// proving we truncated on a rune boundary, not a byte one.
	preview := strings.TrimPrefix(got, "Alice commented: ")
	if rc := utf8.RuneCountInString(preview); rc != 121 {
		t.Errorf("preview rune count = %d, want 121 (120 + ellipsis)", rc)
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

// Series membership rows render the prebuilt Summary differentiating
// linked/unlinked, mirroring the counterparty twin below.
func TestEnrichAnnotations_SeriesMembershipSummaries(t *testing.T) {
	rows := []Annotation{
		{
			ID:        "s-assigned",
			Kind:      "series_assigned",
			ActorName: "Alice",
			Payload:   map[string]interface{}{"series_id": "SER12345", "series_name": "Netflix"},
			CreatedAt: "2026-04-04T12:00:00Z",
		},
		{
			ID:        "s-unlinked",
			Kind:      "series_unlinked",
			ActorName: "", // system-attributed
			Payload:   map[string]interface{}{"series_id": "SER12345", "series_name": "Netflix"},
			CreatedAt: "2026-04-04T12:01:00Z",
		},
	}
	out := EnrichAnnotations(rows, EnrichOptions{})
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	if out[0].Action != "linked" || out[0].Subject != "Netflix" {
		t.Errorf("assigned: Action=%q Subject=%q", out[0].Action, out[0].Subject)
	}
	if out[0].Summary != "Alice linked this to Netflix" {
		t.Errorf("assigned Summary = %q", out[0].Summary)
	}
	if out[1].Summary != "Unlinked from Netflix" {
		t.Errorf("unlinked Summary = %q", out[1].Summary)
	}
}

// Counterparty membership rows mirror series exactly: a prebuilt Summary
// differentiating set/remove, surfaced via the same Summary-fallback render
// path. (rules-substrate doctrine: counterparty events read like series.)
func TestEnrichAnnotations_CounterpartyMembershipSummaries(t *testing.T) {
	rows := []Annotation{
		{
			ID:        "cp-assigned-actor",
			Kind:      "counterparty_assigned",
			ActorName: "Alice",
			Payload:   map[string]interface{}{"counterparty_id": "CPX12345", "counterparty_name": "Netflix"},
			CreatedAt: "2026-04-04T12:00:00Z",
		},
		{
			ID:        "cp-assigned-system",
			Kind:      "counterparty_assigned",
			ActorName: "", // system-attributed (rule / detection)
			Payload:   map[string]interface{}{"counterparty_id": "CPX12345", "counterparty_name": "Netflix"},
			CreatedAt: "2026-04-04T12:01:00Z",
		},
		{
			ID:        "cp-unlinked-actor",
			Kind:      "counterparty_unlinked",
			ActorName: "Bob",
			Payload:   map[string]interface{}{"counterparty_id": "CPX12345", "counterparty_name": "Netflix"},
			CreatedAt: "2026-04-04T12:02:00Z",
		},
		{
			ID:        "cp-unlinked-system",
			Kind:      "counterparty_unlinked",
			ActorName: "",
			Payload:   map[string]interface{}{"counterparty_id": "CPX12345", "counterparty_name": "Netflix"},
			CreatedAt: "2026-04-04T12:03:00Z",
		},
	}
	out := EnrichAnnotations(rows, EnrichOptions{})
	if len(out) != 4 {
		t.Fatalf("len = %d, want 4", len(out))
	}
	if out[0].Action != "set" || out[0].Subject != "Netflix" {
		t.Errorf("assigned-actor: Action=%q Subject=%q", out[0].Action, out[0].Subject)
	}
	if out[0].Summary != "Alice set the counterparty to Netflix" {
		t.Errorf("assigned-actor Summary = %q", out[0].Summary)
	}
	if out[1].Summary != "Counterparty set to Netflix" {
		t.Errorf("assigned-system Summary = %q", out[1].Summary)
	}
	if out[2].Action != "removed" || out[2].Summary != "Bob removed the counterparty" {
		t.Errorf("unlinked-actor: Action=%q Summary=%q", out[2].Action, out[2].Summary)
	}
	if out[3].Summary != "Counterparty removed" {
		t.Errorf("unlinked-system Summary = %q", out[3].Summary)
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

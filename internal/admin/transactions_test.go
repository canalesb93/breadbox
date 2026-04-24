package admin

import (
	"net/http"
	"testing"

	"breadbox/internal/service"
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
				if !containsSubstring(result, tt.contains) {
					t.Errorf("expected result to contain %q, got %q", tt.contains, result)
				}
			}
			if tt.excludes != "" {
				if containsSubstring(result, tt.excludes) {
					t.Errorf("expected result NOT to contain %q, got %q", tt.excludes, result)
				}
			}
		})
	}
}

func TestBuildExportURL_URLEncodesValues(t *testing.T) {
	req, _ := http.NewRequest("GET", "/transactions?search=foo%26bar", nil)
	result := buildExportURL(req)

	if !containsSubstring(result, "search=foo%26bar") {
		t.Errorf("expected URL-encoded ampersand in export URL, got %q", result)
	}
	// The raw ampersand should not appear as a parameter separator within the value
	if containsSubstring(result, "search=foo&bar") {
		t.Errorf("raw ampersand should be encoded in export URL, got %q", result)
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
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

	entries := buildActivityTimeline(annotations, nil)

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

	entries := buildActivityTimeline(annotations, nil)

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

	entries := buildActivityTimeline(annotations, nil)

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

	categoryDisplay := func(slug string) string {
		if slug == "food_and_drink_groceries" {
			return "Food & Drink › Groceries"
		}
		return slug
	}

	entries := buildActivityTimeline(annotations, categoryDisplay)

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

	categoryDisplay := func(slug string) string {
		if slug == "shopping" {
			return "Shopping"
		}
		return slug
	}

	entries := buildActivityTimeline(annotations, categoryDisplay)

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

	entries := buildActivityTimeline(annotations, nil)

	if len(entries) != 0 {
		t.Fatalf("expected legacy [Review: ...] comment to be suppressed, got %d entries", len(entries))
	}
}

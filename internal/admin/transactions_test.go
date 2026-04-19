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
	annotations := []service.Annotation{{
		ID:        "ann-free",
		Kind:      "comment",
		ActorName: "Alice",
		ActorType: "user",
		Payload: map[string]interface{}{
			"content":    "split with Bob",
			"comment_id": "comment-free",
		},
		CreatedAt: "2026-04-04T12:00:00Z",
	}}

	entries := buildActivityTimeline(annotations)

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Type != "comment" {
		t.Errorf("expected comment entry, got type=%q", entries[0].Type)
	}
	if entries[0].CommentID != "comment-free" {
		t.Errorf("expected CommentID=comment-free, got %q", entries[0].CommentID)
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

	entries := buildActivityTimeline(annotations)

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

	entries := buildActivityTimeline(annotations)

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

	entries := buildActivityTimeline(annotations)

	if len(entries) != 0 {
		t.Fatalf("expected legacy [Review: ...] comment to be suppressed, got %d entries", len(entries))
	}
}

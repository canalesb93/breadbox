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

func TestBuildActivityTimeline_LinkedCommentRendersInResolution(t *testing.T) {
	reviewID := "rev-1"
	resolvedName := "Groceries"
	reviewerType := "agent"
	reviewerName := "auto-review"
	reviewedAt := "2026-04-03T10:00:00Z"
	reviews := []service.ReviewResponse{{
		ID:                          reviewID,
		ReviewType:                  "uncategorized",
		Status:                      "approved",
		ReviewerType:                &reviewerType,
		ReviewerName:                &reviewerName,
		ResolvedCategoryDisplayName: &resolvedName,
		CreatedAt:                   "2026-04-03T09:00:00Z",
		ReviewedAt:                  &reviewedAt,
	}}
	linkedID := reviewID
	comments := []service.CommentResponse{{
		ID:         "comment-linked",
		AuthorName: reviewerName,
		AuthorType: reviewerType,
		Content:    "matched recurring merchant rule",
		ReviewID:   &linkedID,
		CreatedAt:  "2026-04-03T10:00:00Z",
	}}

	entries := buildActivityTimeline(reviews, comments, nil)

	var resolution *service.ActivityEntry
	for i := range entries {
		if entries[i].Type == "comment" {
			t.Fatalf("linked review-note comment should not render as a standalone entry, got %+v", entries[i])
		}
		if entries[i].ReviewStatus == "approved" {
			resolution = &entries[i]
		}
	}
	if resolution == nil {
		t.Fatal("expected an approved resolution entry")
	}
	if resolution.Detail != "matched recurring merchant rule" {
		t.Errorf("resolution Detail = %q, want linked comment content", resolution.Detail)
	}
	if resolution.CommentID != "comment-linked" {
		t.Errorf("resolution CommentID = %q, want comment-linked (so delete affordance is preserved)", resolution.CommentID)
	}
}

func TestBuildActivityTimeline_FreeStandingCommentStillEmitted(t *testing.T) {
	comments := []service.CommentResponse{{
		ID:         "comment-free",
		AuthorName: "Alice",
		AuthorType: "user",
		Content:    "split with Bob",
		CreatedAt:  "2026-04-04T12:00:00Z",
	}}

	entries := buildActivityTimeline(nil, comments, nil)

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

func TestBuildActivityTimeline_LegacyPrefixCommentSuppressed(t *testing.T) {
	comments := []service.CommentResponse{{
		ID:         "comment-legacy",
		AuthorName: "Legacy",
		AuthorType: "system",
		Content:    "[Review: Some note migrated before consolidation]",
		CreatedAt:  "2026-03-01T00:00:00Z",
	}}

	entries := buildActivityTimeline(nil, comments, nil)

	if len(entries) != 0 {
		t.Fatalf("expected legacy [Review: ...] comment to be suppressed, got %d entries", len(entries))
	}
}

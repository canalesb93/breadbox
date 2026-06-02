//go:build !lite

package service

import (
	"strings"
	"testing"
)

func TestNormalizeReportType(t *testing.T) {
	cases := map[string]string{
		"bug":     "bug",
		"Bug":     "bug",
		"task":    "task",
		"TASK":    "task",
		" task ":  "task",
		"feature": "bug", // anything not "task" falls back to bug
		"":        "bug",
	}
	for in, want := range cases {
		if got := normalizeReportType(in); got != want {
			t.Errorf("normalizeReportType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIssueTitle(t *testing.T) {
	if got := issueTitle("bug", "Broken chart"); got != "[Bug] Broken chart" {
		t.Errorf("got %q", got)
	}
	if got := issueTitle("task", "Add export"); got != "[Task] Add export" {
		t.Errorf("got %q", got)
	}
}

func TestDedupeLabels(t *testing.T) {
	if got := dedupeLabels("dev-report", "bug"); strings.Join(got, ",") != "dev-report,bug" {
		t.Errorf("got %v", got)
	}
	// flow label collides with type → single entry
	if got := dedupeLabels("bug", "bug"); strings.Join(got, ",") != "bug" {
		t.Errorf("got %v", got)
	}
	// empty flow label drops to just the type
	if got := dedupeLabels("", "task"); strings.Join(got, ",") != "task" {
		t.Errorf("got %v", got)
	}
}

func TestMetaStr(t *testing.T) {
	m := map[string]any{
		"viewport": "1280×800",
		"dpr":      float64(2),
		"flag":     true,
		"pipe":     "a|b\nc",
	}
	if got := metaStr(m, "viewport"); got != "1280×800" {
		t.Errorf("viewport = %q", got)
	}
	if got := metaStr(m, "dpr"); got != "2" {
		t.Errorf("dpr = %q", got)
	}
	if got := metaStr(m, "flag"); got != "true" {
		t.Errorf("flag = %q", got)
	}
	// pipes escaped + newlines flattened for markdown-table safety
	if got := metaStr(m, "pipe"); got != "a\\|b c" {
		t.Errorf("pipe = %q", got)
	}
	if got := metaStr(m, "missing"); got != "" {
		t.Errorf("missing = %q", got)
	}
	if got := metaStr(nil, "x"); got != "" {
		t.Errorf("nil map = %q", got)
	}
}

func TestArtifactURL(t *testing.T) {
	if got := artifactURL("", "abc123", "screenshot"); got != "/-/dev-reports/abc123/screenshot" {
		t.Errorf("relative = %q", got)
	}
	if got := artifactURL("https://bb.example.com", "abc123", "snapshot.html"); got != "https://bb.example.com/-/dev-reports/abc123/snapshot.html" {
		t.Errorf("absolute = %q", got)
	}
}

func TestBuildDevReportIssueBody(t *testing.T) {
	in := CreateDevReportInput{
		Type:           "bug",
		Title:          "Broken",
		Description:    "It broke when I clicked.",
		PageURL:        "https://bb.example.com/transactions",
		PagePath:       "/transactions",
		ScreenshotData: []byte("not-a-real-image-but-nonempty"),
		HTMLSnapshot:   "<html></html>",
		Metadata: map[string]any{
			"viewport":   "1280×800",
			"theme":      "dark",
			"user_agent": "Mozilla/5.0",
		},
		CreatedBy: "admin@example.com",
	}

	// With a hosted image URL → embeds the <img>.
	body := buildDevReportIssueBody("bug", in, "abc123", "https://i.img402.dev/x.jpg", "https://bb.example.com")
	for _, want := range []string{
		"**Type:** Bug",
		"It broke when I clicked.",
		`<img src="https://i.img402.dev/x.jpg"`,
		"| Page | `/transactions` |",
		"| Reported by | admin@example.com |",
		"| Theme | dark |",
		"https://bb.example.com/-/dev-reports/abc123/screenshot",
		"https://bb.example.com/-/dev-reports/abc123/snapshot.html",
		"Report `abc123`",
		"Developer Mode",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("issue body missing %q\n---\n%s", want, body)
		}
	}

	// Without a hosted image but with screenshot bytes → notes the durable copy.
	body2 := buildDevReportIssueBody("task", in, "abc123", "", "")
	if strings.Contains(body2, "<img") {
		t.Error("expected no <img> when image URL is empty")
	}
	if !strings.Contains(body2, "**Type:** Task") {
		t.Error("expected Task type label")
	}
	if !strings.Contains(body2, "relative") {
		t.Error("expected a note about relative artifact links when no public base URL")
	}
}

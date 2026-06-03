//go:build !lite

package service

import (
	"strings"
	"testing"
)

func TestSplitOwnerRepo(t *testing.T) {
	cases := []struct {
		in          string
		owner, name string
		wantErr     bool
	}{
		{"canalesb93/breadbox", "canalesb93", "breadbox", false},
		{" canalesb93/breadbox ", "canalesb93", "breadbox", false},
		{"canalesb93/breadbox/", "canalesb93", "breadbox", false},
		{"breadbox", "", "", true},
		{"a/b/c", "", "", true},
		{"/breadbox", "", "", true},
		{"owner/", "", "", true},
		{"own er/repo", "", "", true},
		{"", "", "", true},
	}
	for _, c := range cases {
		owner, name, err := splitOwnerRepo(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("splitOwnerRepo(%q): expected error, got %q/%q", c.in, owner, name)
			}
			continue
		}
		if err != nil || owner != c.owner || name != c.name {
			t.Errorf("splitOwnerRepo(%q) = %q/%q err=%v, want %q/%q", c.in, owner, name, err, c.owner, c.name)
		}
	}
}

func TestNormalizeReportType(t *testing.T) {
	for in, want := range map[string]string{
		"bug": "bug", "Bug": "bug", "task": "task", "TASK": "task",
		" task ": "task", "feature": "bug", "": "bug",
	} {
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
	if got := dedupeLabels("bug", "bug"); strings.Join(got, ",") != "bug" {
		t.Errorf("got %v", got)
	}
	if got := dedupeLabels("", "task"); strings.Join(got, ",") != "task" {
		t.Errorf("got %v", got)
	}
}

func TestBuildDraftURL(t *testing.T) {
	u := buildDraftURL("acme/widgets", "[Bug] X", "some body", []string{"dev-report", "bug"})
	if !strings.HasPrefix(u, "https://github.com/acme/widgets/issues/new?") {
		t.Errorf("unexpected prefix: %q", u)
	}
	for _, want := range []string{"title=", "body=", "labels="} {
		if !strings.Contains(u, want) {
			t.Errorf("draft URL missing %q: %s", want, u)
		}
	}
	if got := buildDraftURL("not-a-repo", "t", "b", nil); got != "" {
		t.Errorf("expected empty URL for bad repo, got %q", got)
	}
	long := buildDraftURL("a/b", "t", strings.Repeat("x", 30000), nil)
	if len(long) > draftURLMaxLen+200 {
		t.Errorf("draft URL not capped: %d chars", len(long))
	}
}

func TestMetaStr(t *testing.T) {
	m := map[string]any{"viewport": "1280×800", "dpr": float64(2), "flag": true, "pipe": "a|b\nc"}
	if got := metaStr(m, "viewport"); got != "1280×800" {
		t.Errorf("viewport = %q", got)
	}
	if got := metaStr(m, "dpr"); got != "2" {
		t.Errorf("dpr = %q", got)
	}
	if got := metaStr(m, "pipe"); got != "a\\|b c" {
		t.Errorf("pipe = %q", got)
	}
	if got := metaStr(nil, "x"); got != "" {
		t.Errorf("nil map = %q", got)
	}
}

func TestBuildDevReportIssueBody(t *testing.T) {
	in := CreateDevReportInput{
		Type:           "bug",
		Title:          "Broken",
		Description:    "It broke when I clicked.",
		PageURL:        "https://bb.example.com/transactions",
		PagePath:       "/transactions",
		ScreenshotData: []byte("nonempty-bytes"),
		Metadata:       map[string]any{"viewport": "1280×800", "theme": "dark", "user_agent": "Mozilla/5.0", "redacted": true},
		CreatedBy:      "admin@example.com",
	}
	// With hosted artifact URLs → embeds the <img> + links the snapshot.
	body := buildDevReportIssueBody("bug", in,
		"https://bb-artifacts.exe.xyz/f/abc.jpg", "https://bb-artifacts.exe.xyz/f/def.html")
	for _, want := range []string{
		"**Type:** Bug",
		"It broke when I clicked.",
		`<img src="https://bb-artifacts.exe.xyz/f/abc.jpg"`,
		"[HTML snapshot](https://bb-artifacts.exe.xyz/f/def.html)",
		"| Page | `/transactions` |",
		"| Reported by | admin@example.com |",
		"| Redacted | Yes — financial data masked |",
		"Developer Mode",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("issue body missing %q\n---\n%s", want, body)
		}
	}

	// Without hosted URLs but with screenshot bytes → notes the hosting failure.
	body2 := buildDevReportIssueBody("task", in, "", "")
	if strings.Contains(body2, "<img") {
		t.Error("expected no <img> when image URL is empty")
	}
	if !strings.Contains(body2, "**Type:** Task") {
		t.Error("expected Task type label")
	}
}

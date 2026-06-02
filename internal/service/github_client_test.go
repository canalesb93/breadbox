//go:build !lite

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSplitOwnerRepo(t *testing.T) {
	cases := []struct {
		in            string
		owner, name   string
		wantErr       bool
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
		if err != nil {
			t.Errorf("splitOwnerRepo(%q): unexpected error: %v", c.in, err)
			continue
		}
		if owner != c.owner || name != c.name {
			t.Errorf("splitOwnerRepo(%q) = %q/%q, want %q/%q", c.in, owner, name, c.owner, c.name)
		}
	}
}

func TestNewGithubIssueClient_RequiresToken(t *testing.T) {
	if _, err := newGithubIssueClient("", "owner/repo"); err == nil {
		t.Fatal("expected error for empty token")
	}
	if _, err := newGithubIssueClient("tok", "bad"); err == nil {
		t.Fatal("expected error for bad repo")
	}
}

func TestEnsureLabel_CreatesWhenMissing(t *testing.T) {
	var createdBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/labels/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/labels"):
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &createdBody)
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	old := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = old }()

	c, err := newGithubIssueClient("tok", "owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ensureLabel(context.Background(), "dev-report", "8957E5", "desc"); err != nil {
		t.Fatalf("ensureLabel: %v", err)
	}
	if createdBody["name"] != "dev-report" {
		t.Errorf("created label name = %q, want dev-report", createdBody["name"])
	}
	if createdBody["color"] != "8957E5" {
		t.Errorf("created label color = %q, want 8957E5", createdBody["color"])
	}
}

func TestEnsureLabel_NoopWhenExists(t *testing.T) {
	posted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posted = true
		}
		w.WriteHeader(http.StatusOK) // GET label found
	}))
	defer srv.Close()
	old := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = old }()

	c, _ := newGithubIssueClient("tok", "owner/repo")
	if err := c.ensureLabel(context.Background(), "dev-report", "8957E5", "desc"); err != nil {
		t.Fatalf("ensureLabel: %v", err)
	}
	if posted {
		t.Error("ensureLabel POSTed a create even though the label exists")
	}
}

func TestCreateIssue_ParsesResponse(t *testing.T) {
	var gotPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/issues") {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q, want Bearer tok", got)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotPayload)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"number": 42, "html_url": "https://github.com/owner/repo/issues/42"}`))
	}))
	defer srv.Close()
	old := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = old }()

	c, _ := newGithubIssueClient("tok", "owner/repo")
	num, url, err := c.createIssue(context.Background(), "[Bug] Title", "body", []string{"dev-report", "bug"})
	if err != nil {
		t.Fatalf("createIssue: %v", err)
	}
	if num != 42 {
		t.Errorf("number = %d, want 42", num)
	}
	if url != "https://github.com/owner/repo/issues/42" {
		t.Errorf("url = %q", url)
	}
	if gotPayload["title"] != "[Bug] Title" {
		t.Errorf("title = %v", gotPayload["title"])
	}
	labels, _ := gotPayload["labels"].([]any)
	if len(labels) != 2 {
		t.Errorf("labels = %v, want 2", gotPayload["labels"])
	}
}

func TestCreateIssue_SurfacesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"message": "Validation Failed"}`))
	}))
	defer srv.Close()
	old := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = old }()

	c, _ := newGithubIssueClient("tok", "owner/repo")
	_, _, err := c.createIssue(context.Background(), "t", "b", nil)
	if err == nil || !strings.Contains(err.Error(), "Validation Failed") {
		t.Fatalf("expected error surfacing GitHub message, got %v", err)
	}
}

//go:build !lite

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// githubIssueClient is a minimal GitHub REST client scoped to one repo,
// used by Developer Mode to ensure a label exists and open an issue. It is
// deliberately small — issue filing is the only operation — and mirrors the
// outbound-HTTP conventions used by the provider clients (context-aware
// requests, explicit timeout, bounded error-body reads).
type githubIssueClient struct {
	token string
	owner string
	repo  string
	http  *http.Client
}

// githubAPIBase is the GitHub REST root. A var (not const) so tests can point
// the client at an httptest server.
var githubAPIBase = "https://api.github.com"

// newGithubIssueClient validates "owner/repo" and returns a client bound to
// the supplied token. The token needs the classic `repo` scope or a
// fine-grained token with read+write Issues permission on the repo.
func newGithubIssueClient(token, repo string) (*githubIssueClient, error) {
	owner, name, err := splitOwnerRepo(repo)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("%w: GitHub token is not configured", ErrInvalidParameter)
	}
	return &githubIssueClient{
		token: token,
		owner: owner,
		repo:  name,
		http:  &http.Client{Timeout: 20 * time.Second},
	}, nil
}

// splitOwnerRepo parses "owner/repo", rejecting blanks, extra slashes, and
// whitespace. Returned parts are safe to interpolate into an API path.
func splitOwnerRepo(repo string) (owner, name string, err error) {
	repo = strings.TrimSpace(repo)
	repo = strings.TrimSuffix(repo, "/")
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("%w: repository must be in owner/repo form", ErrInvalidParameter)
	}
	owner, name = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	if owner == "" || name == "" || strings.ContainsAny(repo, " \t") {
		return "", "", fmt.Errorf("%w: repository must be in owner/repo form", ErrInvalidParameter)
	}
	return owner, name, nil
}

func (c *githubIssueClient) newRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, githubAPIBase+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "breadbox-devmode")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// ensureLabel creates the label if it doesn't already exist. A 404 from the
// GET means "missing" → POST it; an "already_exists" race on POST is benign.
// Best-effort: the caller treats a returned error as a soft warning.
func (c *githubIssueClient) ensureLabel(ctx context.Context, name, color, description string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	getReq, err := c.newRequest(ctx, http.MethodGet,
		fmt.Sprintf("/repos/%s/%s/labels/%s", c.owner, c.repo, url.PathEscape(name)), nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(getReq)
	if err != nil {
		return fmt.Errorf("github: get label: %w", err)
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil // already exists
	}
	if resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("github: get label returned %d", resp.StatusCode)
	}

	postReq, err := c.newRequest(ctx, http.MethodPost,
		fmt.Sprintf("/repos/%s/%s/labels", c.owner, c.repo),
		map[string]string{"name": name, "color": strings.TrimPrefix(color, "#"), "description": description})
	if err != nil {
		return err
	}
	cResp, err := c.http.Do(postReq)
	if err != nil {
		return fmt.Errorf("github: create label: %w", err)
	}
	body, _ := io.ReadAll(io.LimitReader(cResp.Body, 2048))
	cResp.Body.Close()
	switch cResp.StatusCode {
	case http.StatusCreated, http.StatusOK:
		return nil
	case http.StatusUnprocessableEntity:
		// already_exists race between GET and POST — fine.
		if bytes.Contains(body, []byte("already_exists")) {
			return nil
		}
		return fmt.Errorf("github: create label: %s", strings.TrimSpace(string(body)))
	default:
		return fmt.Errorf("github: create label returned %d: %s", cResp.StatusCode, strings.TrimSpace(string(body)))
	}
}

// createIssue opens an issue and returns its number + html_url.
func (c *githubIssueClient) createIssue(ctx context.Context, title, body string, labels []string) (int, string, error) {
	payload := map[string]any{"title": title, "body": body}
	if len(labels) > 0 {
		payload["labels"] = labels
	}
	req, err := c.newRequest(ctx, http.MethodPost,
		fmt.Sprintf("/repos/%s/%s/issues", c.owner, c.repo), payload)
	if err != nil {
		return 0, "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("github: create issue: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusCreated {
		return 0, "", fmt.Errorf("github: create issue returned %d: %s", resp.StatusCode, githubErrorMessage(raw))
	}
	var out struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return 0, "", fmt.Errorf("github: decode issue response: %w", err)
	}
	return out.Number, out.HTMLURL, nil
}

// githubErrorMessage pulls the human-readable "message" out of a GitHub
// error body, falling back to the raw (bounded) text.
func githubErrorMessage(raw []byte) string {
	var e struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Message != "" {
		return e.Message
	}
	s := strings.TrimSpace(string(raw))
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}

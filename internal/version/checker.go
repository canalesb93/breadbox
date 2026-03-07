package version

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ReleaseInfo holds metadata about a GitHub release.
type ReleaseInfo struct {
	Version string // e.g. "1.2.0" (no v prefix)
	TagName string // e.g. "v1.2.0"
	URL     string // GitHub release URL
}

// Checker checks GitHub for new releases and caches the result.
type Checker struct {
	mu         sync.Mutex
	current    string
	cached     *ReleaseInfo
	cachedAt   time.Time
	cacheTTL   time.Duration
	repoOwner  string
	repoName   string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewChecker creates a Checker that compares currentVersion against the latest
// GitHub release for canalesb93/breadbox.
func NewChecker(currentVersion string, logger *slog.Logger) *Checker {
	return &Checker{
		current:   currentVersion,
		cacheTTL:  1 * time.Hour,
		repoOwner: "canalesb93",
		repoName:  "breadbox",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// githubRelease is the subset of the GitHub API response we need.
type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// GetLatest returns the latest release, using a 1-hour in-memory cache.
func (c *Checker) GetLatest(ctx context.Context) (*ReleaseInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cached != nil && time.Since(c.cachedAt) < c.cacheTTL {
		return c.cached, nil
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", c.repoOwner, c.repoName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("github API returned %d: %s", resp.StatusCode, string(body))
	}

	var gr githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	info := &ReleaseInfo{
		Version: strings.TrimPrefix(gr.TagName, "v"),
		TagName: gr.TagName,
		URL:     gr.HTMLURL,
	}

	c.cached = info
	c.cachedAt = time.Now()
	return info, nil
}

// CheckForUpdate returns whether an update is available. If the current version
// is "dev" or the GitHub API is unreachable, updateAvailable will be false/nil
// respectively.
func (c *Checker) CheckForUpdate(ctx context.Context) (updateAvailable *bool, latest *ReleaseInfo, err error) {
	if c.current == "dev" {
		f := false
		return &f, nil, nil
	}

	info, err := c.GetLatest(ctx)
	if err != nil {
		return nil, nil, err
	}

	newer := isNewer(info.Version, c.current)
	return &newer, info, nil
}

// isNewer returns true if latest is a higher semver than current.
func isNewer(latest, current string) bool {
	latestParts := parseSemver(latest)
	currentParts := parseSemver(current)
	if latestParts == nil || currentParts == nil {
		return false
	}
	for i := 0; i < 3; i++ {
		if latestParts[i] > currentParts[i] {
			return true
		}
		if latestParts[i] < currentParts[i] {
			return false
		}
	}
	return false
}

// parseSemver splits a version string like "1.2.3" into [1, 2, 3].
// Returns nil if parsing fails. Strips a leading "v" if present.
func parseSemver(v string) []int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return nil
	}
	result := make([]int, 3)
	for i, p := range parts {
		// Strip pre-release suffix (e.g. "3-beta1" → "3").
		if idx := strings.IndexAny(p, "-+"); idx != -1 {
			p = p[:idx]
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		result[i] = n
	}
	return result
}

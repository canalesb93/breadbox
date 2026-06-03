//go:build !lite

package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"breadbox/internal/appconfig"
)

// DevModeSettingsResponse is the read shape for Settings → Developer.
type DevModeSettingsResponse struct {
	Enabled    bool   `json:"enabled"`
	GithubRepo string `json:"github_repo"`
	IssueLabel string `json:"issue_label"`
}

// UpdateDevModeSettingsParams holds the writable Developer-Mode settings.
// Nil = leave untouched. An empty GithubRepo clears it (falls back to the
// default); an empty IssueLabel resets to the default.
type UpdateDevModeSettingsParams struct {
	Enabled    *bool
	GithubRepo *string
	IssueLabel *string
}

// GetDevModeSettings reads the devmode.* keys from app_config.
func (s *Service) GetDevModeSettings(ctx context.Context) (*DevModeSettingsResponse, error) {
	return &DevModeSettingsResponse{
		Enabled:    appconfig.Bool(ctx, s.Queries, appconfig.KeyDevModeEnabled, false),
		GithubRepo: appconfig.String(ctx, s.Queries, appconfig.KeyDevModeGithubRepo, appconfig.DevModeDefaultRepo),
		IssueLabel: appconfig.String(ctx, s.Queries, appconfig.KeyDevModeIssueLabel, appconfig.DevModeDefaultLabel),
	}, nil
}

// DevModeEnabled is a cheap single-key read used by the base-layout middleware
// to decide whether to render the floating reporter.
func (s *Service) DevModeEnabled(ctx context.Context) bool {
	return appconfig.Bool(ctx, s.Queries, appconfig.KeyDevModeEnabled, false)
}

// UpdateDevModeSettings validates and writes the non-nil fields, then returns
// the new state.
func (s *Service) UpdateDevModeSettings(ctx context.Context, p UpdateDevModeSettingsParams) (*DevModeSettingsResponse, error) {
	if p.Enabled != nil {
		if err := s.Queries.SetAppConfig(ctx, appconfigParam(appconfig.KeyDevModeEnabled, strconv.FormatBool(*p.Enabled))); err != nil {
			return nil, fmt.Errorf("set devmode_enabled: %w", err)
		}
	}
	if p.GithubRepo != nil {
		repo := strings.TrimSuffix(strings.TrimSpace(*p.GithubRepo), "/")
		if repo != "" {
			if _, _, err := splitOwnerRepo(repo); err != nil {
				return nil, err
			}
		}
		if err := s.Queries.SetAppConfig(ctx, appconfigParam(appconfig.KeyDevModeGithubRepo, repo)); err != nil {
			return nil, fmt.Errorf("set devmode_github_repo: %w", err)
		}
	}
	if p.IssueLabel != nil {
		label := strings.TrimSpace(*p.IssueLabel)
		if label == "" {
			label = appconfig.DevModeDefaultLabel
		}
		if err := s.Queries.SetAppConfig(ctx, appconfigParam(appconfig.KeyDevModeIssueLabel, label)); err != nil {
			return nil, fmt.Errorf("set devmode_issue_label: %w", err)
		}
	}
	return s.GetDevModeSettings(ctx)
}

// splitOwnerRepo parses "owner/repo", rejecting blanks, extra slashes, and
// whitespace. Returned parts are safe to interpolate into a GitHub URL.
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

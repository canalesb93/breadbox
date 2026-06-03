//go:build !lite

package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"breadbox/internal/appconfig"
)

// DevModeSettingsResponse is the read shape for Settings → Developer. The
// GitHub token is never returned in plaintext — only a masked hint (and a
// HasToken flag) leaves the server.
type DevModeSettingsResponse struct {
	Enabled    bool    `json:"enabled"`
	GithubRepo string  `json:"github_repo"`
	IssueLabel string  `json:"issue_label"`
	HasToken   bool    `json:"has_token"`
	TokenMask  *string `json:"token_mask,omitempty"`
}

// UpdateDevModeSettingsParams holds the writable Developer-Mode settings.
// Nil = leave untouched. An empty GithubRepo clears it; an empty IssueLabel
// resets to the default; an empty GithubToken is the "keep current" signal
// (clearing the token requires the dedicated ClearGithubToken flag).
type UpdateDevModeSettingsParams struct {
	Enabled          *bool
	GithubRepo       *string
	IssueLabel       *string
	GithubToken      *string
	ClearGithubToken bool
}

// GetDevModeSettings reads the devmode.* keys, decrypting + masking the token.
func (s *Service) GetDevModeSettings(ctx context.Context, encKey []byte) (*DevModeSettingsResponse, error) {
	token, _, err := appconfig.ReadEncrypted(ctx, s.Queries, appconfig.KeyDevModeGithubToken, encKey)
	if err != nil {
		return nil, fmt.Errorf("read github token: %w", err)
	}
	return &DevModeSettingsResponse{
		Enabled:    appconfig.Bool(ctx, s.Queries, appconfig.KeyDevModeEnabled, false),
		GithubRepo: appconfig.String(ctx, s.Queries, appconfig.KeyDevModeGithubRepo, appconfig.DevModeDefaultRepo),
		IssueLabel: appconfig.String(ctx, s.Queries, appconfig.KeyDevModeIssueLabel, appconfig.DevModeDefaultLabel),
		HasToken:   token != "",
		TokenMask:  maskToken(token),
	}, nil
}

// DevModeEnabled is a cheap single-key read used by the base-layout
// middleware to decide whether to render the floating reporter.
func (s *Service) DevModeEnabled(ctx context.Context) bool {
	return appconfig.Bool(ctx, s.Queries, appconfig.KeyDevModeEnabled, false)
}

// UpdateDevModeSettings validates and writes the non-nil fields, then returns
// the new (masked) state.
func (s *Service) UpdateDevModeSettings(ctx context.Context, p UpdateDevModeSettingsParams, encKey []byte) (*DevModeSettingsResponse, error) {
	if p.Enabled != nil {
		if err := s.Queries.SetAppConfig(ctx, appconfigParam(appconfig.KeyDevModeEnabled, strconv.FormatBool(*p.Enabled))); err != nil {
			return nil, fmt.Errorf("set devmode_enabled: %w", err)
		}
	}
	if p.GithubRepo != nil {
		repo := strings.TrimSpace(*p.GithubRepo)
		repo = strings.TrimSuffix(repo, "/")
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
	if p.ClearGithubToken {
		if err := appconfig.WriteEncrypted(ctx, s.Queries, appconfig.KeyDevModeGithubToken, "", encKey); err != nil {
			return nil, fmt.Errorf("clear github token: %w", err)
		}
	} else if p.GithubToken != nil {
		token := strings.TrimSpace(*p.GithubToken)
		if token != "" { // empty = keep current
			if err := appconfig.WriteEncrypted(ctx, s.Queries, appconfig.KeyDevModeGithubToken, token, encKey); err != nil {
				return nil, fmt.Errorf("write github token: %w", err)
			}
		}
	}
	return s.GetDevModeSettings(ctx, encKey)
}

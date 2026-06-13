//go:build !lite

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	"breadbox/internal/appconfig"
)

// bbArtifactsUploadURL is the upload endpoint for the self-hosted artifact
// store (bb-artifacts.exe.xyz) — public read, ~180-day retention, 25 MB cap,
// hosts both images and HTML. Uploads require auth — see artifactUploadToken for
// how the Bearer token is resolved (Settings → Developer, or the
// BB_ARTIFACTS_UPLOAD_TOKEN env override). The token is kept server-side and is
// never exposed to the browser. Endpoint overridable via BB_ARTIFACTS_UPLOAD_URL
// for staging/self-host. The URL is public; the token is a secret.
const bbArtifactsUploadURL = "https://bb-artifacts.exe.xyz/upload"

// bbArtifactsMaxBytes mirrors the store's per-file cap.
const bbArtifactsMaxBytes = 25 << 20

func bbArtifactsEndpoint() string {
	if v := os.Getenv("BB_ARTIFACTS_UPLOAD_URL"); v != "" {
		return v
	}
	return bbArtifactsUploadURL
}

// artifactUploadToken resolves the Bearer token for artifact uploads, with the
// project-wide precedence env → app_config → none: the BB_ARTIFACTS_UPLOAD_TOKEN
// env var wins (ops override), otherwise the encrypted token saved in
// Settings → Developer. Returns "" when neither is configured (uploads then go
// out unauthenticated and the host will 401 — best-effort, the report still files).
func (s *Service) artifactUploadToken(ctx context.Context) string {
	if v := strings.TrimSpace(os.Getenv("BB_ARTIFACTS_UPLOAD_TOKEN")); v != "" {
		return v
	}
	if len(s.EncryptionKey) == 0 {
		return ""
	}
	tok, _, err := appconfig.ReadEncrypted(ctx, s.Queries, appconfig.KeyDevModeUploadToken, s.EncryptionKey)
	if err != nil {
		return ""
	}
	return tok
}

// uploadArtifact posts a file (image or HTML) to the artifact store and returns
// the public URL. The store keys content-type off the filename extension, so
// pass "screenshot.jpg" / "snapshot.html". token is the Bearer credential (may
// be ""). Best-effort: callers fall back to omitting the artifact on error.
func uploadArtifact(ctx context.Context, data []byte, filename, token string) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("artifact: empty file")
	}
	if len(data) > bbArtifactsMaxBytes {
		return "", fmt.Errorf("artifact: %d bytes over the %d byte limit", len(data), bbArtifactsMaxBytes)
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", err
	}
	if err := mw.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, bbArtifactsEndpoint(), &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	// Auth is kept server-side: the browser POSTs to /-/dev-reports (session +
	// CSRF), and the server attaches the resolved upload token here. Never sent
	// to clients.
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("artifact: upload: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode == http.StatusUnauthorized {
		return "", fmt.Errorf("artifact: upload unauthorized (401) — set BB_ARTIFACTS_UPLOAD_TOKEN on the server")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("artifact: upload returned %d", resp.StatusCode)
	}
	var out struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.URL == "" {
		return "", fmt.Errorf("artifact: unexpected response")
	}
	return out.URL, nil
}

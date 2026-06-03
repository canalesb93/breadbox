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
	"time"
)

// bbArtifactsUploadURL is the upload endpoint for the self-hosted artifact
// store (bb-artifacts.exe.xyz) — anonymous upload, public read, ~180-day
// retention, 25 MB cap, hosts both images and HTML. Overridable via env for
// staging/self-host. A public URL, not a secret.
const bbArtifactsUploadURL = "https://bb-artifacts.exe.xyz/upload"

// bbArtifactsMaxBytes mirrors the store's per-file cap.
const bbArtifactsMaxBytes = 25 << 20

func bbArtifactsEndpoint() string {
	if v := os.Getenv("BB_ARTIFACTS_UPLOAD_URL"); v != "" {
		return v
	}
	return bbArtifactsUploadURL
}

// uploadArtifact posts a file (image or HTML) to the artifact store and returns
// the public URL. The store keys content-type off the filename extension, so
// pass "screenshot.jpg" / "snapshot.html". Best-effort: callers fall back to
// omitting the artifact from the report on error.
func uploadArtifact(ctx context.Context, data []byte, filename string) (string, error) {
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

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("artifact: upload: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
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

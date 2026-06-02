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
	"time"
)

// img402MaxBytes is the upload ceiling enforced by img402.dev's free tier.
// The Developer-Mode reporter only attempts an upload when the capture is
// at or under this size; larger captures fall back to the durable in-app
// artifact link embedded in the issue body.
const img402MaxBytes = 1 << 20 // 1 MiB

const img402UploadURL = "https://img402.dev/api/free"

// uploadToImg402 posts an image to img402.dev and returns the public,
// GitHub-renderable URL. img402 is ephemeral (7-day expiry) which is ideal
// for an embedded screenshot — the durable copy lives in Breadbox. Used
// best-effort: any error leaves the caller to fall back to the in-app link.
func uploadToImg402(ctx context.Context, data []byte, filename string) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("img402: empty image")
	}
	if len(data) > img402MaxBytes {
		return "", fmt.Errorf("img402: image is %d bytes, over the %d byte limit", len(data), img402MaxBytes)
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("image", filename)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", err
	}
	if err := mw.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, img402UploadURL, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("img402: upload: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("img402: upload returned %d", resp.StatusCode)
	}
	var out struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.URL == "" {
		return "", fmt.Errorf("img402: unexpected response")
	}
	return out.URL, nil
}

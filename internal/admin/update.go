package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"

	"breadbox/internal/app"
	"breadbox/internal/db"

	"github.com/jackc/pgx/v5/pgtype"
)

// DismissUpdateHandler handles POST /admin/api/update/dismiss.
// It stores the dismissed version in app_config so the banner is hidden until
// a newer release is published.
func DismissUpdateHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Version string `json:"version"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Version == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"error": "version is required"})
			return
		}

		_ = a.Queries.SetAppConfig(r.Context(), db.SetAppConfigParams{
			Key:   "update_dismissed_version",
			Value: pgtype.Text{String: body.Version, Valid: true},
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}
}

// TriggerUpdateHandler handles POST /admin/api/update.
// It pulls the latest Docker image via the Docker Engine API (Unix socket).
// After pulling, the admin must run `docker compose up -d` to apply the update.
func TriggerUpdateHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.DockerSocketAvailable {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"error": "Docker socket not available"})
			return
		}

		if err := pullImage(r.Context(), "ghcr.io/canalesb93/breadbox", "latest"); err != nil {
			a.Logger.Error("docker image pull failed", "error", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{"error": fmt.Sprintf("Failed to pull image: %v", err)})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"ok":      true,
			"message": "Image pulled. Run 'docker compose up -d' to complete the update.",
			"pulled":  true,
		})
	}
}

// pullImage pulls a Docker image via the Docker Engine API over the Unix socket.
func pullImage(ctx context.Context, image, tag string) error {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", "/var/run/docker.sock")
		},
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   0, // image pulls can be slow; context handles cancellation
	}

	url := fmt.Sprintf("http://localhost/v1.46/images/create?fromImage=%s&tag=%s", image, tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("docker API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("docker API returned status %d: %s", resp.StatusCode, string(body))
	}

	// The Docker pull API streams JSON progress. We must consume the entire
	// body to ensure the pull completes before we return.
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return fmt.Errorf("read pull stream: %w", err)
	}

	return nil
}

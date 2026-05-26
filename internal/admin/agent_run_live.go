//go:build !headless && !lite

package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"time"

	"breadbox/internal/agent"
	"breadbox/internal/appconfig"
	"breadbox/internal/service"
	"breadbox/internal/templates/components"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// AgentRunLivePayload is the JSON shape returned by
// GET /-/agents/runs/{shortId}/live — the live-update endpoint the
// run-detail page polls while a run is still in_progress.
//
// We send back HTML fragments rather than per-event diffs because (a)
// the templ rendering is the canonical source of truth for event
// shapes — duplicating it on the client would drift — and (b) the
// transcript is capped at 500 events, so a few KB of HTML on a 3 s
// cadence is cheaper than maintaining a JS-side renderer.
type AgentRunLivePayload struct {
	Status          string `json:"status"`
	StatusBadgeHTML string `json:"statusBadgeHTML"`
	TranscriptHTML  string `json:"transcriptHTML"`
	StatsHTML       string `json:"statsHTML"`
	EventCount      int    `json:"eventCount"`
	DurationMs      int64  `json:"durationMs,omitempty"`
}

// AgentRunLiveHandler serves the JSON snapshot the run-detail page
// polls. Same auth as the dashboard route (admin session via the /-/
// group); same transcript parsing as the full page handler so the
// in-place patch matches what the initial server-side render would
// have produced.
func AgentRunLiveHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		shortID := chi.URLParam(r, "shortId")

		run, err := svc.GetAgentRun(ctx, shortID)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				writeAdminError(w, http.StatusNotFound, "NOT_FOUND", "Run not found")
				return
			}
			writeAdminError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load run")
			return
		}

		row := agentRunRowFromResponse(*run)

		// Enrich with agent identity so the stats panel renders the
		// same label set as the initial server render.
		if run.AgentDefinitionID != nil {
			if def, derr := svc.GetAgentDefinition(ctx, *run.AgentDefinitionID); derr == nil {
				row.AgentSlug = def.Slug
				row.AgentName = def.Name
			}
		}

		if reportMap, rerr := svc.ListReportSummariesForRunIDs(ctx, []string{run.ID}); rerr == nil {
			if reps, ok := reportMap[run.ID]; ok {
				row.Reports = make([]components.AgentRunReportRef, 0, len(reps))
				for _, rep := range reps {
					row.Reports = append(row.Reports, components.AgentRunReportRef{
						ShortID:  rep.ShortID,
						Title:    rep.Title,
						Priority: rep.Priority,
					})
				}
			}
		}

		// Reuse the page handler's parser so any classification fix is
		// applied uniformly across the initial render and the poll.
		path := ""
		if run.TranscriptPath != nil {
			path = *run.TranscriptPath
		}
		if path == "" && run.ID != "" {
			dir := appconfig.String(ctx, svc.Queries, appconfig.KeyAgentTranscriptDir, agent.DefaultTranscriptDir())
			if dir != "" {
				path = filepath.Join(dir, run.ID+".ndjson")
			}
		}
		var events []pages.TranscriptEvent
		var truncated bool
		if path != "" {
			events, truncated, _ = parseTranscriptFile(path, agentRunTranscriptMaxEvents)
		}

		props := pages.AgentRunDetailProps{
			Run:            row,
			Transcript:     events,
			Truncated:      truncated,
			TranscriptPath: path,
		}

		payload := AgentRunLivePayload{
			Status:     run.Status,
			EventCount: len(events),
		}

		// Render the transcript fragment.
		var buf bytes.Buffer
		if err := pages.AgentRunTranscriptFragment(props).Render(ctx, &buf); err != nil {
			writeAdminError(w, http.StatusInternalServerError, "RENDER_ERROR", "Failed to render transcript")
			return
		}
		payload.TranscriptHTML = buf.String()

		// Render the status badge so the in_progress→success transition
		// can update without a page reload.
		buf.Reset()
		if err := components.AgentRunStatusBadge(run.Status).Render(ctx, &buf); err != nil {
			writeAdminError(w, http.StatusInternalServerError, "RENDER_ERROR", "Failed to render status badge")
			return
		}
		payload.StatusBadgeHTML = buf.String()

		if row.DurationMs > 0 {
			payload.DurationMs = row.DurationMs
		} else if !row.StartedAt.IsZero() && run.Status == "in_progress" {
			payload.DurationMs = time.Since(row.StartedAt).Milliseconds()
		}

		writeAdminJSON(w, http.StatusOK, payload)
	}
}

// writeAdminJSON is a tiny local helper — the admin package already has
// response helpers, but they live in response.go and we want to keep
// our endpoint self-contained while the surrounding admin error
// helpers are refactored.
func writeAdminJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeAdminError(w http.ResponseWriter, status int, code, message string) {
	type errBody struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	var b errBody
	b.Error.Code = code
	b.Error.Message = message
	writeAdminJSON(w, status, b)
}

// dummy reference to avoid "imported and not used" when the rest of the
// admin package vends a different ctx helper.
var _ = context.Background

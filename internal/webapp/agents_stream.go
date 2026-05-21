//go:build !headless && !lite

package webapp

import (
	"bufio"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"

	"breadbox/internal/appconfig"
	"breadbox/internal/service"
	"breadbox/internal/webapp/pages"
)

// ----------------------------------------------------------------------------
// v3 streaming surface #1: agent run live transcript (SSE)
//
// This is the first Datastar/SSE-style streaming surface in the v3 MPA. It's the
// most tractable real-time source because the work is already done for us: the
// sidecar writes the run's NDJSON transcript to disk line-by-line as the run
// executes (internal/agent/sidecar.go). We just tail that file.
//
// Contract (documented in .claude/rules/app-mpa.md):
//   - The page (agentRunDetail) renders the transcript lines already on disk at
//     first paint, so it works with JS off.
//   - When the run is still in_progress, the page marks the <pre> with
//     data-stream-live + data-stream-url. The agent-run-stream island opens an
//     EventSource to that URL.
//   - This handler emits one SSE `event: line` frame per NEW transcript line
//     (lines past the count the client already rendered), then `event: done`
//     when the run reaches a terminal status. No fragment-diffing library: the
//     payload is the raw NDJSON line and the island appends a node. Append-only
//     growth is the whole reason a hand-rolled tail beats the Datastar SDK here.
//   - Graceful degradation: no JS / no EventSource → the static server render +
//     a Refresh link still shows current state.
// ----------------------------------------------------------------------------

// terminalRunStatuses are the run states that mean "stop tailing".
var terminalRunStatuses = map[string]bool{
	"success":  true,
	"error":    true,
	"timeout":  true,
	"skipped":  true,
	"canceled": true,
}

// registerAgentStream wires the run-detail page + its SSE endpoint onto the
// authenticated agents subrouter. Registered from registerAgents so the central
// Router in handler.go stays untouched.
func (h *Handler) registerAgentStream(r chi.Router) {
	r.Get("/agents/{slug}/runs/{shortId}", h.agentRunDetail)
	r.Get("/agents/{slug}/runs/{shortId}/stream", h.agentRunStream)
}

// agentRunDetail renders one run with its transcript. In-progress runs get the
// live SSE enhancement; finished runs render the full static transcript.
func (h *Handler) agentRunDetail(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	shortID := chi.URLParam(r, "shortId")

	agent, err := h.app.Service.GetAgentDefinition(r.Context(), slug)
	if errors.Is(err, service.ErrNotFound) {
		h.notFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	run, err := h.app.Service.GetAgentRun(r.Context(), shortID)
	if errors.Is(err, service.ErrNotFound) {
		h.notFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	lines := h.readTranscriptLines(r, run)
	live := run.Status == "in_progress"

	render(w, r, http.StatusOK, pages.AgentRunDetail(h.shellData(r, "Run "+run.ShortID), pages.AgentRunData{
		Agent:     agent,
		Run:       run,
		Lines:     lines,
		Live:      live,
		StreamURL: "/app/agents/" + slug + "/runs/" + run.ShortID + "/stream",
	}))
}

// agentRunStream tails the run's NDJSON transcript and emits new lines as SSE.
// The client passes ?from=<n> (lines already rendered) so we never replay what
// the server already painted. It exits when the run reaches a terminal status
// (sending `event: done`) or the client disconnects.
func (h *Handler) agentRunStream(w http.ResponseWriter, r *http.Request) {
	shortID := chi.URLParam(r, "shortId")
	run, err := h.app.Service.GetAgentRun(r.Context(), shortID)
	if err != nil {
		// SSE clients can't read a JSON body usefully; a 404 is enough for the
		// island to give up and leave the static render in place.
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	path := h.transcriptPath(r, run)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering (nginx)
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	sent := 0
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()

	// Drain whatever's already on disk past the client's cursor, then poll for
	// growth. Re-resolving the run status each tick is the terminal-state check.
	emit := func() (done bool) {
		newLines := readTranscriptFrom(path, sent)
		for _, ln := range newLines {
			if !writeSSEEvent(w, "line", ln) {
				return true // client gone
			}
			sent++
		}
		flusher.Flush()

		fresh, err := h.app.Service.GetAgentRun(ctx, shortID)
		if err == nil && terminalRunStatuses[fresh.Status] {
			// One last drain in case the final lines + status landed together.
			for _, ln := range readTranscriptFrom(path, sent) {
				writeSSEEvent(w, "line", ln)
				sent++
			}
			writeSSEEvent(w, "done", fresh.Status)
			flusher.Flush()
			return true
		}
		return false
	}

	if emit() {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if emit() {
				return
			}
		}
	}
}

// readTranscriptLines reads all lines on disk for the initial server render.
func (h *Handler) readTranscriptLines(r *http.Request, run *service.AgentRunResponse) []string {
	return readTranscriptFrom(h.transcriptPath(r, run), 0)
}

// transcriptPath resolves the on-disk NDJSON path for a run. Mirrors the REST
// handler (internal/api/agents.go): prefer the persisted transcript_path, fall
// back to the deterministic <transcript_dir>/<run.ID>.ndjson — the column is
// only set on completion, so the fallback is what makes an in-progress run
// streamable at all.
func (h *Handler) transcriptPath(r *http.Request, run *service.AgentRunResponse) string {
	if run.TranscriptPath != nil && *run.TranscriptPath != "" {
		return *run.TranscriptPath
	}
	if run.ID == "" {
		return ""
	}
	dir := appconfig.String(r.Context(), h.app.Service.Queries, appconfig.KeyAgentTranscriptDir, "transcripts/agents")
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, run.ID+".ndjson")
}

// readTranscriptFrom returns transcript lines with index >= from. A fresh open
// per call keeps the tail simple and correct under concurrent appends; the
// files are small (one run) so this is cheap.
func readTranscriptFrom(path string, from int) []string {
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024) // tolerate long JSON lines
	idx := 0
	for sc.Scan() {
		if idx >= from {
			if line := sc.Text(); line != "" {
				out = append(out, line)
			}
		}
		idx++
	}
	if errors.Is(sc.Err(), io.EOF) || sc.Err() == nil {
		return out
	}
	return out
}

// writeSSEEvent writes one `event:`/`data:` SSE frame. Returns false if the
// connection write failed (client disconnected). Multi-line payloads are split
// into multiple `data:` lines per the SSE spec; NDJSON lines are single-line so
// this is defensive only.
func writeSSEEvent(w http.ResponseWriter, event, data string) bool {
	if _, err := io.WriteString(w, "event: "+event+"\n"); err != nil {
		return false
	}
	if _, err := io.WriteString(w, "data: "+data+"\n\n"); err != nil {
		return false
	}
	return true
}

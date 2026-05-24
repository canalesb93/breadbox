//go:build !headless && !lite

package admin

import (
	"bufio"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"breadbox/internal/appconfig"
	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// Cap how many NDJSON lines we parse into TranscriptEvent structs. A long
// run can produce thousands of lines; rendering all of them server-side
// blows up page weight and CPU. Beyond this we render a banner pointing
// at the raw file on disk.
const agentRunTranscriptMaxEvents = 500

// AgentRunsListPageHandler serves both
//
//   - GET /agents/{slug}/runs (per-agent view)
//   - GET /agents/runs        (global view)
//
// The empty slug param distinguishes the two — chi keeps it empty when
// the {slug} segment isn't part of the matched route. Filter params:
// status, trigger, hit_cap, agent (global only), start, end, limit,
// offset. Invalid filter values are silently dropped (admin convention).
func AgentRunsListPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		slug := chi.URLParam(r, "slug")
		mode := "global"
		if slug != "" {
			mode = "agent"
		}

		filters, limit, offset := parseAgentRunFilters(r)

		props := pages.AgentRunsListProps{
			Mode:      mode,
			Filters:   filters,
			Limit:     limit,
			Offset:    offset,
			CSRFToken: GetCSRFToken(r),
		}

		var agentName string

		if mode == "agent" {
			def, err := svc.GetAgentDefinition(ctx, slug)
			if err != nil {
				if errors.Is(err, service.ErrNotFound) {
					tr.RenderNotFound(w, r)
					return
				}
				tr.RenderError(w, r)
				return
			}
			props.AgentSlug = def.Slug
			props.AgentName = def.Name
			agentName = def.Name

			params := service.AgentRunListParams{
				Limit:   limit,
				Offset:  offset,
				Status:  filters.Status,
				Trigger: filters.Trigger,
				HitCap:  filters.HitCap,
				Start:   parseDateParam(r, "start"),
				End:     parseInclusiveDateParam(r, "end"),
			}
			result, err := svc.ListAgentRuns(ctx, def.Slug, params)
			if err != nil {
				tr.RenderError(w, r)
				return
			}
			props.Rows = make([]pages.AgentRunRowProps, 0, len(result.Runs))
			for _, run := range result.Runs {
				row := agentRunRowFromResponse(run)
				row.AgentSlug = def.Slug
				row.AgentName = def.Name
				props.Rows = append(props.Rows, row)
			}
			props.Total = len(result.Runs)
		} else {
			params := service.AllAgentRunListParams{
				Limit:         limit,
				Offset:        offset,
				AgentSlugOrID: filters.AgentSlug,
				Status:        filters.Status,
				Trigger:       filters.Trigger,
				HitCap:        filters.HitCap,
				Start:         parseDateParam(r, "start"),
				End:           parseInclusiveDateParam(r, "end"),
			}
			result, err := svc.ListAllAgentRuns(ctx, params)
			if err != nil {
				// Bad agent filter slug → silently drop the filter rather
				// than erroring the whole page; matches the "silently drop
				// invalid filters" admin convention.
				params.AgentSlugOrID = ""
				filters.AgentSlug = ""
				props.Filters = filters
				result, err = svc.ListAllAgentRuns(ctx, params)
				if err != nil {
					tr.RenderError(w, r)
					return
				}
			}
			props.Rows = make([]pages.AgentRunRowProps, 0, len(result.Runs))
			for _, run := range result.Runs {
				row := agentRunRowFromResponse(run.AgentRunResponse)
				row.AgentSlug = run.AgentSlug
				row.AgentName = run.AgentName
				props.Rows = append(props.Rows, row)
			}
			props.Total = len(result.Runs)

			// Populate the agent filter dropdown.
			defs, err := svc.ListAgentDefinitions(ctx)
			if err == nil {
				props.AgentOptions = make([]pages.AgentRunsAgentOption, 0, len(defs))
				for _, d := range defs {
					props.AgentOptions = append(props.AgentOptions, pages.AgentRunsAgentOption{
						Slug: d.Slug,
						Name: d.Name,
					})
				}
			}
		}

		title := "Run history"
		if agentName != "" {
			title = "Runs — " + agentName
		}
		data := BaseTemplateData(r, sm, "agents", title)
		tr.RenderWithTempl(w, r, data, pages.AgentRunsList(props))
	}
}

// AgentRunDetailPageHandler serves GET /agents/runs/{shortId}. Resolves the
// run, parses the NDJSON transcript file (best-effort; missing file just
// surfaces an Error message), and renders the detail templ.
func AgentRunDetailPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		shortID := chi.URLParam(r, "shortId")

		run, err := svc.GetAgentRun(ctx, shortID)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				tr.RenderNotFound(w, r)
				return
			}
			tr.RenderError(w, r)
			return
		}

		row := agentRunRowFromResponse(*run)

		// Try to enrich with agent identity + system/user prompts so the
		// page can show "this is what the agent was told to do".
		systemPrompt := ""
		userPrompt := ""
		var defSlug, defName string
		if run.AgentDefinitionID != nil {
			if def, derr := svc.GetAgentDefinition(ctx, *run.AgentDefinitionID); derr == nil {
				defSlug = def.Slug
				defName = def.Name
				userPrompt = def.Prompt
				if def.SystemPrompt != nil {
					systemPrompt = *def.SystemPrompt
				}
			}
		}
		row.AgentSlug = defSlug
		row.AgentName = defName

		promptPrefix := ""
		if run.PromptPrefix != nil {
			promptPrefix = *run.PromptPrefix
		}

		props := pages.AgentRunDetailProps{
			Run:          row,
			SystemPrompt: systemPrompt,
			Prompt:       userPrompt,
			PromptPrefix: promptPrefix,
			CSRFToken:    GetCSRFToken(r),
		}

		// in_progress runs auto-refresh so the transcript fills in.
		if run.Status == "in_progress" {
			props.RefreshSeconds = 5
		}

		// Resolve the transcript path. Falls back to the deterministic path
		// the same way internal/api/agents.go::GetAgentRunTranscriptHandler
		// does, so an in-progress run's transcript is readable before the
		// transcript_path column is filled in.
		path := ""
		if run.TranscriptPath != nil {
			path = *run.TranscriptPath
		}
		if path == "" && run.ID != "" {
			dir := appconfig.String(ctx, svc.Queries, appconfig.KeyAgentTranscriptDir, "transcripts/agents")
			if dir != "" {
				path = filepath.Join(dir, run.ID+".ndjson")
			}
		}
		props.TranscriptPath = path

		if path != "" {
			events, truncated, perr := parseTranscriptFile(path, agentRunTranscriptMaxEvents)
			if perr != nil {
				// Not fatal — page still renders with a non-blocking error
				// banner. Likely cases: file missing for a still-in-progress
				// run, or the file was pruned after retention.
				if run.Status != "in_progress" {
					props.Error = "Transcript file not available: " + perr.Error()
				}
			} else {
				props.Transcript = events
				props.Truncated = truncated
			}
		}

		title := "Run " + row.ShortID
		data := BaseTemplateData(r, sm, "agents", title)
		tr.RenderWithTempl(w, r, data, pages.AgentRunDetail(props))
	}
}

// parseAgentRunFilters extracts the shared filter param set out of the
// query string. Unknown enum values are silently dropped (admin handler
// convention: render an empty filter rather than a 400).
func parseAgentRunFilters(r *http.Request) (pages.AgentRunsFilterProps, int, int) {
	q := r.URL.Query()

	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > 200 {
				n = 200
			}
			limit = n
		}
	}
	offset := 0
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	f := pages.AgentRunsFilterProps{
		Status:    q.Get("status"),
		Trigger:   q.Get("trigger"),
		HitCap:    q.Get("hit_cap"),
		AgentSlug: q.Get("agent"),
		Start:     q.Get("start"),
		End:       q.Get("end"),
	}
	if !isAllowedAgentRunStatus(f.Status) {
		f.Status = ""
	}
	if !isAllowedAgentRunTrigger(f.Trigger) {
		f.Trigger = ""
	}
	if !isAllowedAgentRunHitCap(f.HitCap) {
		f.HitCap = ""
	}
	return f, limit, offset
}

func isAllowedAgentRunStatus(s string) bool {
	switch s {
	case "", "success", "error", "in_progress", "skipped", "timeout":
		return true
	}
	return false
}

func isAllowedAgentRunTrigger(s string) bool {
	switch s {
	case "", "cron", "manual", "webhook":
		return true
	}
	return false
}

func isAllowedAgentRunHitCap(s string) bool {
	switch s {
	case "", "max_turns", "max_budget", "any":
		return true
	}
	return false
}

// agentRunRowFromResponse expands a service.AgentRunResponse (lots of
// pointer fields for nullable numeric columns) into the templ-friendly
// AgentRunRowProps where every cell is a plain value. Zero values stand
// in for nil so the templ doesn't have to nil-check every cell.
func agentRunRowFromResponse(run service.AgentRunResponse) pages.AgentRunRowProps {
	row := pages.AgentRunRowProps{
		ShortID: run.ShortID,
		Status:  run.Status,
		Trigger: run.Trigger,
	}
	if t, err := time.Parse(time.RFC3339, run.StartedAt); err == nil {
		row.StartedAt = t
	}
	if run.CompletedAt != nil && *run.CompletedAt != "" {
		if t, err := time.Parse(time.RFC3339, *run.CompletedAt); err == nil {
			row.FinishedAt = &t
		}
	}
	if run.DurationMs != nil {
		row.DurationMs = int64(*run.DurationMs)
	}
	if run.TotalCostUSD != nil {
		row.CostUSD = *run.TotalCostUSD
	}
	if run.InputTokens != nil {
		row.TokensIn = int64(*run.InputTokens)
	}
	if run.OutputTokens != nil {
		row.TokensOut = int64(*run.OutputTokens)
	}
	if run.HitCap != nil {
		row.HitCap = *run.HitCap
	}
	if run.TurnCount != nil {
		row.Turns = *run.TurnCount
	}
	if run.OperatorNote != nil {
		row.Note = *run.OperatorNote
	}
	return row
}

// parseTranscriptFile opens the NDJSON transcript at path, parses up to
// maxEvents lines into TranscriptEvent structs, and returns whether the
// file had more lines beyond the cap. Lines that fail to parse as JSON
// are dropped (rare; the sidecar writes one JSON object per line). Lines
// whose shape doesn't match any recognised SDK event type are surfaced
// as "raw" events so operators can still see them.
func parseTranscriptFile(path string, maxEvents int) ([]pages.TranscriptEvent, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Some SDK events (tool_result with a large JSON blob) can exceed the
	// default 64 KB scanner buffer. Bump the max to 4 MB per line — generous
	// but still bounded.
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	out := make([]pages.TranscriptEvent, 0, maxEvents)
	truncated := false
	for scanner.Scan() {
		if len(out) >= maxEvents {
			truncated = true
			// Keep scanning to detect "are there more lines" without
			// allocating; we already have the events we'll render.
			for scanner.Scan() {
				_ = scanner.Bytes()
			}
			break
		}
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var obj map[string]any
		if jerr := json.Unmarshal(raw, &obj); jerr != nil {
			// Couldn't parse — surface raw so operators see something.
			out = append(out, pages.TranscriptEvent{
				Type:    "raw",
				RawJSON: string(raw),
			})
			continue
		}
		out = append(out, classifyTranscriptEvent(obj, string(raw)))
	}
	if serr := scanner.Err(); serr != nil {
		// Return what we got plus the error so the page can show a
		// partial transcript rather than 500.
		return out, truncated, serr
	}
	return out, truncated, nil
}

// classifyTranscriptEvent translates one parsed NDJSON object into a
// TranscriptEvent. Kept deliberately simple — when the shape doesn't
// match anything we recognise, fall through to the "raw" branch so
// operators always see *something* readable.
func classifyTranscriptEvent(obj map[string]any, raw string) pages.TranscriptEvent {
	ev := pages.TranscriptEvent{RawJSON: raw}
	if t, ok := obj["ts"].(float64); ok && t > 0 {
		// SDK writes ts as milliseconds since epoch.
		ev.Timestamp = time.UnixMilli(int64(t))
	}
	typ, _ := obj["type"].(string)
	switch typ {
	case "assistant_message":
		ev.Type = "assistant"
		ev.Role = "assistant"
		ev.Text = extractMessageText(obj)
		// An assistant_message can also carry tool_use blocks. When that
		// happens, the SDK splits the model turn into multiple events
		// (one with text, one with tool_use). We only surface the text
		// here — tool_use blocks land in their own classify branch if
		// the SDK ever emits them standalone, otherwise the user will
		// see "no text content" for the tool-only assistant message.
		// We could iterate content[] for tool_use too, but the
		// constraint says "don't gold-plate" — grouping is non-trivial.
		if toolUse, name, input := firstToolUseBlock(obj); toolUse {
			ev.Type = "tool_use"
			ev.ToolName = name
			ev.ToolInputJSON = input
		}
	case "user_message":
		ev.Type = "user"
		ev.Role = "user"
		// A user_message often carries tool_result blocks. Surface the
		// first one as a tool_result event if there is one; otherwise
		// fall back to user text.
		if hasResult, resultJSON := firstToolResultBlock(obj); hasResult {
			ev.Type = "tool_result"
			ev.ToolResultJSON = resultJSON
		} else {
			ev.Text = extractMessageText(obj)
		}
	case "tool_use":
		ev.Type = "tool_use"
		if data, ok := obj["data"].(map[string]any); ok {
			ev.ToolName, _ = data["name"].(string)
			if input, ok := data["input"]; ok {
				if b, err := json.MarshalIndent(input, "", "  "); err == nil {
					ev.ToolInputJSON = string(b)
				}
			}
		}
	case "tool_result":
		ev.Type = "tool_result"
		if data, ok := obj["data"].(map[string]any); ok {
			if content, ok := data["content"]; ok {
				if b, err := json.MarshalIndent(content, "", "  "); err == nil {
					ev.ToolResultJSON = string(b)
				}
			}
		}
	case "result":
		ev.Type = "result"
		if data, ok := obj["data"].(map[string]any); ok {
			ev.CostUSD = readFloat(data, "totalCostUsd")
			ev.TokensIn = readInt(data, "inputTokens")
			ev.TokensOut = readInt(data, "outputTokens")
			ev.CacheRead = readInt(data, "cacheReadTokens")
			ev.CacheWrite = readInt(data, "cacheCreationTokens")
		}
	case "error":
		ev.Type = "error"
		if data, ok := obj["data"].(map[string]any); ok {
			ev.Text, _ = data["message"].(string)
		}
	case "cost_cap_hit", "system":
		ev.Type = "system"
		if data, ok := obj["data"].(map[string]any); ok {
			if msg, ok := data["message"].(string); ok {
				ev.Text = msg
			}
		}
	default:
		ev.Type = "raw"
	}
	return ev
}

// extractMessageText walks data.message.content[] for blocks of type
// "text" and concatenates the .text fields with blank-line separators —
// the same shape the SDK emits.
func extractMessageText(obj map[string]any) string {
	data, ok := obj["data"].(map[string]any)
	if !ok {
		return ""
	}
	msg, ok := data["message"].(map[string]any)
	if !ok {
		return ""
	}
	content, ok := msg["content"].([]any)
	if !ok {
		return ""
	}
	out := ""
	for _, block := range content {
		bm, ok := block.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := bm["type"].(string); t == "text" {
			if txt, _ := bm["text"].(string); txt != "" {
				if out != "" {
					out += "\n\n"
				}
				out += txt
			}
		}
	}
	return out
}

func firstToolUseBlock(obj map[string]any) (found bool, name, inputJSON string) {
	data, ok := obj["data"].(map[string]any)
	if !ok {
		return false, "", ""
	}
	msg, ok := data["message"].(map[string]any)
	if !ok {
		return false, "", ""
	}
	content, ok := msg["content"].([]any)
	if !ok {
		return false, "", ""
	}
	for _, block := range content {
		bm, ok := block.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := bm["type"].(string); t == "tool_use" {
			n, _ := bm["name"].(string)
			input := bm["input"]
			b, _ := json.MarshalIndent(input, "", "  ")
			return true, n, string(b)
		}
	}
	return false, "", ""
}

func firstToolResultBlock(obj map[string]any) (found bool, contentJSON string) {
	data, ok := obj["data"].(map[string]any)
	if !ok {
		return false, ""
	}
	msg, ok := data["message"].(map[string]any)
	if !ok {
		return false, ""
	}
	content, ok := msg["content"].([]any)
	if !ok {
		return false, ""
	}
	for _, block := range content {
		bm, ok := block.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := bm["type"].(string); t == "tool_result" {
			inner := bm["content"]
			b, _ := json.MarshalIndent(inner, "", "  ")
			return true, string(b)
		}
	}
	return false, ""
}

func readFloat(m map[string]any, k string) float64 {
	switch v := m[k].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	}
	return 0
}

func readInt(m map[string]any, k string) int64 {
	switch v := m[k].(type) {
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case int64:
		return v
	}
	return 0
}

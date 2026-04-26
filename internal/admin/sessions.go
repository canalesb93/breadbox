package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"
	"breadbox/internal/timefmt"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// SessionDetailHandler serves GET /admin/agents/sessions/{id}.
func SessionDetailHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		detail, err := svc.GetMCPSessionDetail(r.Context(), id)
		if err != nil {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}

		data := BaseTemplateData(r, sm, "agents", "Session Detail")
		props := buildSessionDetailProps(detail)
		renderSessionDetail(w, r, tr, data, props)
	}
}

// renderSessionDetail mirrors renderSyncLogDetail / renderLogs: hands
// the typed SessionDetailProps to the templ component and uses
// RenderWithTempl to host it inside base.html.
func renderSessionDetail(w http.ResponseWriter, r *http.Request, tr *TemplateRenderer, data map[string]any, props pages.SessionDetailProps) {
	tr.RenderWithTempl(w, r, data, pages.SessionDetail(props))
}

// buildSessionDetailProps projects service.MCPSessionDetailResponse
// into the flat view-model the templ renders. Pre-renders relative
// time and pretty-prints request/response JSON so the templ stays
// free of admin funcMap helpers.
func buildSessionDetailProps(detail service.MCPSessionDetailResponse) pages.SessionDetailProps {
	out := pages.SessionDetailProps{
		Session: pages.SessionDetailHeader{
			Purpose:       detail.Purpose,
			AgentName:     detail.AgentName,
			APIKeyName:    detail.APIKeyName,
			CreatedAt:     relativeTimeFromRFC3339(detail.CreatedAt),
			ToolCallCount: detail.ToolCallCount,
			ErrorCount:    detail.ErrorCount,
			WriteCount:    detail.WriteCount,
			ReadCount:     detail.ReadCount,
		},
	}
	if len(detail.ToolCalls) > 0 {
		out.ToolCalls = make([]pages.SessionDetailToolCall, 0, len(detail.ToolCalls))
		for _, c := range detail.ToolCalls {
			tc := pages.SessionDetailToolCall{
				ToolName:       c.ToolName,
				Classification: c.Classification,
				IsError:        c.IsError,
				Reason:         c.Reason,
				Sequence:       c.Sequence,
				OffsetLabel:    c.OffsetLabel,
				CreatedAt:      c.CreatedAt,
				CreatedAtAbs:   timefmt.FormatRFC3339Local(c.CreatedAt, timefmt.LayoutDateTimeLocal),
				CreatedAtRel:   relativeTimeFromRFC3339(c.CreatedAt),
			}
			if c.DurationMs != nil {
				tc.DurationMs = *c.DurationMs
				tc.HasDuration = true
			}
			if c.RequestJSON != nil {
				tc.RequestPretty = prettyJSONIndent(*c.RequestJSON)
			}
			if c.ResponseJSON != nil {
				tc.ResponsePretty = prettyJSONIndent(*c.ResponseJSON)
			}
			out.ToolCalls = append(out.ToolCalls, tc)
		}
	}
	return out
}

// relativeTimeFromRFC3339 mirrors the funcMap "relativeTime" branch
// for an RFC3339 string. Returns the input unchanged when parsing
// fails (matches the funcMap fallback).
func relativeTimeFromRFC3339(s string) string {
	if s == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return relativeTime(parsed)
}

// prettyJSONIndent mirrors the funcMap "prettyJSON" branch for raw
// JSON bytes, returning the input as a string when indenting fails.
func prettyJSONIndent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return string(raw)
	}
	return buf.String()
}

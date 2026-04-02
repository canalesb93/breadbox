package admin

import (
	"net/http"

	"breadbox/internal/service"

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
		data["Session"] = detail
		data["ToolCalls"] = detail.ToolCalls
		tr.Render(w, r, "session_detail.html", data)
	}
}

package admin

import (
	"net/http"

	breadboxmcp "breadbox/internal/mcp"

	"github.com/alexedwards/scs/v2"
)

// ReviewInstructionsPageHandler serves GET /admin/review-instructions.
func ReviewInstructionsPageHandler(sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := BaseTemplateData(r, sm, "review-instructions", "Review Instructions")
		data["InitialReviewInstructions"] = breadboxmcp.InitialReviewInstructions
		data["RecurringReviewInstructions"] = breadboxmcp.RecurringReviewInstructions
		tr.Render(w, r, "review_instructions.html", data)
	}
}

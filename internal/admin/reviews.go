package admin

import (
	"encoding/json"
	"errors"
	"net/http"

	"breadbox/internal/app"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
)

// ReviewsPageHandler serves GET /admin/reviews.
//
// Phase 3 retired the review_queue table; the /reviews admin page is now a
// thin redirect to the tag-filtered transactions view. Phase 4 will replace
// this with a proper transactions-page tag filter UI. We keep the route so
// outbound links, bookmarks, and the nav badge keep resolving without 404s.
func ReviewsPageHandler(_ *app.App, _ *scs.SessionManager, _ *TemplateRenderer, _ *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/transactions?tags=needs-review", http.StatusSeeOther)
	}
}

// EnqueueReviewAdminHandler handles POST /-/reviews/enqueue. Phase 3 repoints
// this at the needs-review tag so the transaction detail page's "send to
// review" toggle keeps working. Accepts { transaction_id } and adds the
// needs-review tag on the transaction. Returns 201 on add, 409 if already
// tagged (preserving the old ErrReviewAlreadyPending semantic for the UI).
func EnqueueReviewAdminHandler(a *app.App, sm *scs.SessionManager, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor := ActorFromSession(sm, r)

		var body struct {
			TransactionID string `json:"transaction_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}
		if body.TransactionID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "transaction_id is required"})
			return
		}

		added, alreadyPresent, err := svc.AddTransactionTag(r.Context(), body.TransactionID, "needs-review", actor, "")
		if err != nil {
			switch {
			case errors.Is(err, service.ErrNotFound):
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "transaction not found"})
			case errors.Is(err, service.ErrInvalidParameter):
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			default:
				a.Logger.Error("tag transaction for review", "error", err)
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
			}
			return
		}
		if alreadyPresent {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "review already pending"})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "added": added})
	}
}

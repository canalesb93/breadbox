package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"breadbox/internal/app"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// ReviewsPageHandler serves GET /admin/reviews.
func ReviewsPageHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		statusFilter := r.URL.Query().Get("status")
		if statusFilter == "" {
			statusFilter = "pending"
		}

		defaultLimit := 50

		params := service.ReviewListParams{
			Status: &statusFilter,
			Limit:  defaultLimit,
		}

		if v := r.URL.Query().Get("review_type"); v != "" {
			params.ReviewType = &v
		}
		if v := r.URL.Query().Get("account_id"); v != "" {
			params.AccountID = &v
		}
		if v := r.URL.Query().Get("user_id"); v != "" {
			params.UserID = &v
		}
		if v := r.URL.Query().Get("cursor"); v != "" {
			params.Cursor = v
		}
		if v := r.URL.Query().Get("limit"); v != "" {
			if l, err := strconv.Atoi(v); err == nil && l > 0 {
				params.Limit = l
			}
		}

		result, err := svc.ListReviews(ctx, params)
		if err != nil {
			a.Logger.Error("list reviews", "error", err)
			tr.Render(w, r, "500.html", map[string]any{"PageTitle": "Error", "CurrentPage": "reviews"})
			return
		}

		counts, err := svc.GetReviewCounts(ctx)
		if err != nil {
			a.Logger.Error("get review counts", "error", err)
			counts = &service.ReviewCountsResponse{}
		}

		// Load accounts and users for filter dropdowns.
		accounts, _ := a.Queries.ListAccounts(ctx)
		users, _ := a.Queries.ListUsers(ctx)

		// Load category tree for the category picker component.
		categories, _ := svc.ListCategoryTree(ctx)

		// Phase 1 (Rule Actions v2): the review queue page is always enabled.
		// The enable/disable gate (review_auto_enqueue) was removed; Phase 4
		// drops the page entirely in favor of a tag-driven transactions view.
		data := BaseTemplateData(r, sm, "reviews", "Reviews")
		data["ReviewAutoEnqueue"] = true
		data["Reviews"] = result.Reviews
		data["HasMore"] = result.HasMore
		data["NextCursor"] = result.NextCursor
		data["Total"] = result.Total
		data["Counts"] = counts
		data["StatusFilter"] = statusFilter
		data["ReviewTypeFilter"] = r.URL.Query().Get("review_type")
		data["AccountIDFilter"] = r.URL.Query().Get("account_id")
		data["UserIDFilter"] = r.URL.Query().Get("user_id")
		data["ViewMode"] = "triage"
		data["Accounts"] = accounts
		data["Users"] = users
		data["Categories"] = categories

		tr.Render(w, r, "reviews.html", data)
	}
}

// SubmitReviewAdminHandler handles POST /admin/api/reviews/{id}/submit.
func SubmitReviewAdminHandler(a *app.App, sm *scs.SessionManager, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		actor := ActorFromSession(sm, r)

		var body struct {
			Decision   string  `json:"decision"`
			CategoryID *string `json:"category_id,omitempty"`
			Note       *string `json:"note,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}

		result, err := svc.SubmitReview(r.Context(), service.SubmitReviewParams{
			ReviewID:   id,
			Decision:   body.Decision,
			CategoryID: body.CategoryID,
			Note:       body.Note,
			Actor:      actor,
		})
		if err != nil {
			switch {
			case errors.Is(err, service.ErrNotFound):
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "review not found"})
			case errors.Is(err, service.ErrReviewAlreadyResolved):
				writeJSON(w, http.StatusConflict, map[string]any{"error": "review already resolved"})
			case errors.Is(err, service.ErrInvalidDecision):
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid decision"})
			case errors.Is(err, service.ErrInvalidParameter):
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			default:
				a.Logger.Error("submit review", "error", err)
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
			}
			return
		}

		writeJSON(w, http.StatusOK, result)
	}
}

// DismissReviewAdminHandler handles POST /admin/api/reviews/{id}/dismiss.
func DismissReviewAdminHandler(a *app.App, sm *scs.SessionManager, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		actor := ActorFromSession(sm, r)

		if err := svc.DismissReview(r.Context(), id, actor); err != nil {
			switch {
			case errors.Is(err, service.ErrNotFound):
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "review not found"})
			case errors.Is(err, service.ErrReviewAlreadyResolved):
				writeJSON(w, http.StatusConflict, map[string]any{"error": "review already resolved"})
			default:
				a.Logger.Error("dismiss review", "error", err)
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
			}
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// EnqueueReviewAdminHandler handles POST /admin/api/reviews/enqueue.
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

		result, err := svc.EnqueueManualReview(r.Context(), body.TransactionID, actor)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrNotFound):
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "transaction not found"})
			case errors.Is(err, service.ErrReviewAlreadyPending):
				writeJSON(w, http.StatusConflict, map[string]any{"error": "review already pending"})
			default:
				a.Logger.Error("enqueue review", "error", err)
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
			}
			return
		}

		writeJSON(w, http.StatusCreated, result)
	}
}

// DismissAllReviewsAdminHandler handles POST /admin/api/reviews/dismiss-all.
func DismissAllReviewsAdminHandler(a *app.App, sm *scs.SessionManager, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor := ActorFromSession(sm, r)

		count, err := svc.DismissAllPendingReviews(r.Context(), actor)
		if err != nil {
			a.Logger.Error("dismiss all reviews", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "dismissed": count})
	}
}

// EnqueueExistingReviewsHandler handles POST /admin/api/reviews/enqueue-existing.
func EnqueueExistingReviewsHandler(a *app.App, sm *scs.SessionManager, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		count, err := svc.EnqueueExistingUncategorized(r.Context())
		if err != nil {
			a.Logger.Error("enqueue existing uncategorized", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to enqueue reviews"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enqueued": count})
	}
}

// ReviewSettingsHandler handles POST /admin/api/reviews/settings.
//
// Phase 1 (Rule Actions v2): the review_auto_enqueue config flag was removed.
// The endpoint is kept (and returns ok) only so existing JS clients that POST
// here don't break; the body is ignored.
func ReviewSettingsHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}
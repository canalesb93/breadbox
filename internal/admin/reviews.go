package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"breadbox/internal/app"
	"breadbox/internal/db"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
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

		// Load review settings from app_config.
		reviewAutoEnqueue := false // reviews are off by default
		if cfg, err := a.Queries.GetAppConfig(ctx, "review_auto_enqueue"); err == nil && cfg.Value.Valid {
			if v, err := strconv.ParseBool(cfg.Value.String); err == nil {
				reviewAutoEnqueue = v
			}
		}

		data := BaseTemplateData(r, sm, "reviews", "Reviews")
		data["ReviewAutoEnqueue"] = reviewAutoEnqueue
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
func ReviewSettingsHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			AutoEnqueue bool `json:"auto_enqueue"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}

		ctx := r.Context()

		if err := a.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
			Key:   "review_auto_enqueue",
			Value: pgtype.Text{String: strconv.FormatBool(body.AutoEnqueue), Valid: true},
		}); err != nil {
			a.Logger.Error("save review_auto_enqueue", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to save settings"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}
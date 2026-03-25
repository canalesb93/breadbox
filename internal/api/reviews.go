package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// ListReviewsHandler returns a filtered, paginated list of review queue items.
func ListReviewsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		// Parse limit (1-200, default 50).
		limit := 50
		if v := q.Get("limit"); v != "" {
			parsed, err := strconv.Atoi(v)
			if err != nil || parsed < 1 || parsed > 200 {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "limit must be between 1 and 200")
				return
			}
			limit = parsed
		}

		var status *string
		if v := q.Get("status"); v != "" {
			status = &v
		}

		var reviewType *string
		if v := q.Get("review_type"); v != "" {
			reviewType = &v
		}

		var accountID *string
		if v := q.Get("account_id"); v != "" {
			accountID = &v
		}

		var userID *string
		if v := q.Get("user_id"); v != "" {
			userID = &v
		}

		var categoryPrimaryRaw *string
		if v := q.Get("category_primary_raw"); v != "" {
			categoryPrimaryRaw = &v
		}

		result, err := svc.ListReviews(r.Context(), service.ReviewListParams{
			Status:             status,
			ReviewType:         reviewType,
			AccountID:          accountID,
			UserID:             userID,
			CategoryPrimaryRaw: categoryPrimaryRaw,
			Limit:              limit,
			Cursor:             q.Get("cursor"),
		})
		if err != nil {
			if errors.Is(err, service.ErrInvalidCursor) {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "Invalid cursor")
				return
			}
			if errors.Is(err, service.ErrInvalidParameter) {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list reviews")
			return
		}

		writeData(w, result)
	}
}

// GetReviewHandler returns a single review by ID.
func GetReviewHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		review, err := svc.GetReview(r.Context(), id)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Review not found")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get review")
			return
		}

		writeData(w, review)
	}
}

// SubmitReviewHandler processes a single review decision.
func SubmitReviewHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var input struct {
			Decision   string  `json:"decision"`
			CategoryID *string `json:"category_id,omitempty"`
			Note       *string `json:"note,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body")
			return
		}

		actor := service.ActorFromContext(r.Context())

		review, err := svc.SubmitReview(r.Context(), service.SubmitReviewParams{
			ReviewID:   id,
			Decision:   input.Decision,
			CategoryID: input.CategoryID,
			Note:       input.Note,
			Actor:      actor,
		})
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Review not found")
				return
			}
			if errors.Is(err, service.ErrReviewAlreadyResolved) {
				mw.WriteError(w, http.StatusConflict, "REVIEW_ALREADY_RESOLVED", "Review has already been resolved")
				return
			}
			if errors.Is(err, service.ErrInvalidDecision) {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "Decision must be one of: approved, rejected, skipped")
				return
			}
			if errors.Is(err, service.ErrInvalidParameter) {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to submit review")
			return
		}

		writeData(w, review)
	}
}

// BulkSubmitReviewsHandler processes multiple review decisions at once.
func BulkSubmitReviewsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Reviews []service.BulkReviewItem `json:"reviews"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body")
			return
		}

		actor := service.ActorFromContext(r.Context())

		result, err := svc.BulkSubmitReviews(r.Context(), service.BulkSubmitReviewParams{
			Reviews: input.Reviews,
			Actor:   actor,
		})
		if err != nil {
			if errors.Is(err, service.ErrInvalidParameter) {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to bulk submit reviews")
			return
		}

		writeData(w, result)
	}
}

// EnqueueReviewHandler manually adds a transaction to the review queue.
func EnqueueReviewHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			TransactionID string `json:"transaction_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body")
			return
		}

		actor := service.ActorFromContext(r.Context())

		review, err := svc.EnqueueManualReview(r.Context(), input.TransactionID, actor)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Transaction not found")
				return
			}
			if errors.Is(err, service.ErrReviewAlreadyPending) {
				mw.WriteError(w, http.StatusConflict, "REVIEW_ALREADY_PENDING", "A pending review already exists for this transaction")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to enqueue review")
			return
		}

		writeJSON(w, http.StatusCreated, review)
	}
}

// ReviewSummaryHandler returns pending reviews grouped by category_primary_raw.
func ReviewSummaryHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := svc.GetReviewSummary(r.Context())
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get review summary")
			return
		}
		writeData(w, result)
	}
}

// AutoApproveCategorizedHandler bulk-approves reviews whose transactions already have categories.
func AutoApproveCategorizedHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor := service.ActorFromContext(r.Context())
		result, err := svc.AutoApproveCategorizedReviews(r.Context(), actor)
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to auto-approve reviews")
			return
		}
		writeData(w, result)
	}
}

// ReviewCountsHandler returns aggregate counts for the review queue.
func ReviewCountsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		counts, err := svc.GetReviewCounts(r.Context())
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get review counts")
			return
		}

		writeData(w, counts)
	}
}

// DismissReviewHandler removes a pending review item.
func DismissReviewHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		actor := service.ActorFromContext(r.Context())

		if err := svc.DismissReview(r.Context(), id, actor); err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Review not found")
				return
			}
			if errors.Is(err, service.ErrReviewAlreadyResolved) {
				mw.WriteError(w, http.StatusConflict, "REVIEW_ALREADY_RESOLVED", "Review has already been resolved")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to dismiss review")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

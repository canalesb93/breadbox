package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// ListTransactionsHandler returns a paginated, filterable list of transactions.
func ListTransactionsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		// Parse limit (1-500, default 100).
		limit := 100
		if v := q.Get("limit"); v != "" {
			parsed, err := strconv.Atoi(v)
			if err != nil || parsed < 1 || parsed > 500 {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "limit must be between 1 and 500")
				return
			}
			limit = parsed
		}

		// Parse cursor.
		cursor := q.Get("cursor")

		// Parse start_date (YYYY-MM-DD, inclusive).
		var startDate *time.Time
		if v := q.Get("start_date"); v != "" {
			t, err := time.Parse("2006-01-02", v)
			if err != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "start_date must be in YYYY-MM-DD format")
				return
			}
			startDate = &t
		}

		// Parse end_date (YYYY-MM-DD, exclusive).
		var endDate *time.Time
		if v := q.Get("end_date"); v != "" {
			t, err := time.Parse("2006-01-02", v)
			if err != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "end_date must be in YYYY-MM-DD format")
				return
			}
			endDate = &t
		}

		// Validate start_date < end_date.
		if startDate != nil && endDate != nil && !startDate.Before(*endDate) {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "start_date must be before end_date")
			return
		}

		// Parse account_id.
		var accountID *string
		if v := q.Get("account_id"); v != "" {
			accountID = &v
		}

		// Parse user_id.
		var userID *string
		if v := q.Get("user_id"); v != "" {
			userID = &v
		}

		// Parse category.
		var category *string
		if v := q.Get("category"); v != "" {
			category = &v
		}

		// Parse category_detailed.
		var categoryDetailed *string
		if v := q.Get("category_detailed"); v != "" {
			categoryDetailed = &v
		}

		// Parse min_amount.
		var minAmount *float64
		if v := q.Get("min_amount"); v != "" {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "min_amount must be a valid number")
				return
			}
			minAmount = &f
		}

		// Parse max_amount.
		var maxAmount *float64
		if v := q.Get("max_amount"); v != "" {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "max_amount must be a valid number")
				return
			}
			maxAmount = &f
		}

		// Validate min <= max.
		if minAmount != nil && maxAmount != nil && *minAmount > *maxAmount {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "min_amount must be less than or equal to max_amount")
			return
		}

		// Parse pending (true/false).
		var pending *bool
		if v := q.Get("pending"); v != "" {
			switch v {
			case "true":
				b := true
				pending = &b
			case "false":
				b := false
				pending = &b
			default:
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "pending must be true or false")
				return
			}
		}

		// Parse search (min 2 chars).
		var search *string
		if v := q.Get("search"); v != "" {
			if len(v) < 2 {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "search must be at least 2 characters")
				return
			}
			search = &v
		}

		// Parse sort_by.
		var sortBy *string
		if v := q.Get("sort_by"); v != "" {
			switch v {
			case "date", "amount", "name":
				sortBy = &v
			default:
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "sort_by must be one of: date, amount, name")
				return
			}
		}

		// Parse sort_order.
		var sortOrder *string
		if v := q.Get("sort_order"); v != "" {
			switch v {
			case "asc", "desc":
				sortOrder = &v
			default:
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "sort_order must be asc or desc")
				return
			}
		}

		params := service.TransactionListParams{
			Cursor:           cursor,
			Limit:            limit,
			StartDate:        startDate,
			EndDate:          endDate,
			AccountID:        accountID,
			UserID:           userID,
			Category:         category,
			CategoryDetailed: categoryDetailed,
			MinAmount:        minAmount,
			MaxAmount:        maxAmount,
			Pending:          pending,
			Search:           search,
			SortBy:           sortBy,
			SortOrder:        sortOrder,
		}

		result, err := svc.ListTransactions(r.Context(), params)
		if err != nil {
			if errors.Is(err, service.ErrInvalidCursor) {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_CURSOR", "The provided cursor is not valid")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list transactions")
			return
		}

		writeData(w, result)
	}
}

// CountTransactionsHandler returns the number of transactions matching optional filters.
func CountTransactionsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		var startDate *time.Time
		if v := q.Get("start_date"); v != "" {
			t, err := time.Parse("2006-01-02", v)
			if err != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "start_date must be in YYYY-MM-DD format")
				return
			}
			startDate = &t
		}

		var endDate *time.Time
		if v := q.Get("end_date"); v != "" {
			t, err := time.Parse("2006-01-02", v)
			if err != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "end_date must be in YYYY-MM-DD format")
				return
			}
			endDate = &t
		}

		if startDate != nil && endDate != nil && !startDate.Before(*endDate) {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "start_date must be before end_date")
			return
		}

		var accountID *string
		if v := q.Get("account_id"); v != "" {
			accountID = &v
		}

		var userID *string
		if v := q.Get("user_id"); v != "" {
			userID = &v
		}

		var category *string
		if v := q.Get("category"); v != "" {
			category = &v
		}

		var categoryDetailed *string
		if v := q.Get("category_detailed"); v != "" {
			categoryDetailed = &v
		}

		var minAmount *float64
		if v := q.Get("min_amount"); v != "" {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "min_amount must be a valid number")
				return
			}
			minAmount = &f
		}

		var maxAmount *float64
		if v := q.Get("max_amount"); v != "" {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "max_amount must be a valid number")
				return
			}
			maxAmount = &f
		}

		if minAmount != nil && maxAmount != nil && *minAmount > *maxAmount {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "min_amount must be less than or equal to max_amount")
			return
		}

		var pending *bool
		if v := q.Get("pending"); v != "" {
			switch v {
			case "true":
				b := true
				pending = &b
			case "false":
				b := false
				pending = &b
			default:
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "pending must be true or false")
				return
			}
		}

		var search *string
		if v := q.Get("search"); v != "" {
			if len(v) < 2 {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "search must be at least 2 characters")
				return
			}
			search = &v
		}

		params := service.TransactionCountParams{
			StartDate:        startDate,
			EndDate:          endDate,
			AccountID:        accountID,
			UserID:           userID,
			Category:         category,
			CategoryDetailed: categoryDetailed,
			MinAmount:        minAmount,
			MaxAmount:        maxAmount,
			Pending:          pending,
			Search:           search,
		}

		count, err := svc.CountTransactionsFiltered(r.Context(), params)
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to count transactions")
			return
		}

		writeJSON(w, http.StatusOK, map[string]int64{"count": count})
	}
}

// GetTransactionHandler returns a single transaction by ID.
func GetTransactionHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		txn, err := svc.GetTransaction(r.Context(), id)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Transaction not found")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get transaction")
			return
		}

		writeData(w, txn)
	}
}

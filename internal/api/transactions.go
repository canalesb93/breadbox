package api

import (
	"encoding/json"
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

		// Parse category_slug.
		var categorySlug *string
		if v := q.Get("category_slug"); v != "" {
			categorySlug = &v
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

		var excludeSearch *string
		if v := q.Get("exclude_search"); v != "" {
			if len(v) < 2 {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "exclude_search must be at least 2 characters")
				return
			}
			excludeSearch = &v
		}

		params := service.TransactionListParams{
			Cursor:        cursor,
			Limit:         limit,
			StartDate:     startDate,
			EndDate:       endDate,
			AccountID:     accountID,
			UserID:        userID,
			CategorySlug:  categorySlug,
			MinAmount:     minAmount,
			MaxAmount:     maxAmount,
			Pending:       pending,
			Search:        search,
			ExcludeSearch: excludeSearch,
			SortBy:        sortBy,
			SortOrder:     sortOrder,
		}

		// Parse fields for field selection.
		fieldSet, err := service.ParseFields(q.Get("fields"))
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_FIELDS", err.Error())
			return
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

		if fieldSet != nil {
			filtered := make([]map[string]any, len(result.Transactions))
			for i, t := range result.Transactions {
				filtered[i] = service.FilterTransactionFields(t, fieldSet)
			}
			writeData(w, map[string]any{
				"transactions": filtered,
				"next_cursor":  result.NextCursor,
				"has_more":     result.HasMore,
				"limit":        result.Limit,
			})
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

		var categorySlug *string
		if v := q.Get("category_slug"); v != "" {
			categorySlug = &v
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

		var excludeSearch *string
		if es := q.Get("exclude_search"); es != "" {
			if len(es) < 2 {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "exclude_search must be at least 2 characters")
				return
			}
			excludeSearch = &es
		}

		params := service.TransactionCountParams{
			StartDate:     startDate,
			EndDate:       endDate,
			AccountID:     accountID,
			UserID:        userID,
			CategorySlug:  categorySlug,
			MinAmount:     minAmount,
			MaxAmount:     maxAmount,
			Pending:       pending,
			Search:        search,
			ExcludeSearch: excludeSearch,
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

		fieldSet, err := service.ParseFields(r.URL.Query().Get("fields"))
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_FIELDS", err.Error())
			return
		}

		txn, err := svc.GetTransaction(r.Context(), id)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Transaction not found")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get transaction")
			return
		}

		if fieldSet != nil {
			writeData(w, service.FilterTransactionFields(*txn, fieldSet))
			return
		}

		writeData(w, txn)
	}
}

// TransactionSummaryHandler returns aggregated transaction totals.
func TransactionSummaryHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		groupBy := q.Get("group_by")
		if groupBy == "" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "group_by is required (category, month, week, day, category_month)")
			return
		}

		params := service.TransactionSummaryParams{
			GroupBy: groupBy,
		}

		if v := q.Get("start_date"); v != "" {
			t, err := time.Parse("2006-01-02", v)
			if err != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "start_date must be in YYYY-MM-DD format")
				return
			}
			params.StartDate = &t
		}

		if v := q.Get("end_date"); v != "" {
			t, err := time.Parse("2006-01-02", v)
			if err != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "end_date must be in YYYY-MM-DD format")
				return
			}
			params.EndDate = &t
		}

		if v := q.Get("account_id"); v != "" {
			params.AccountID = &v
		}
		if v := q.Get("user_id"); v != "" {
			params.UserID = &v
		}
		if v := q.Get("category"); v != "" {
			params.Category = &v
		}
		if q.Get("include_pending") == "true" {
			params.IncludePending = true
		}

		result, err := svc.GetTransactionSummary(r.Context(), params)
		if err != nil {
			if errors.Is(err, service.ErrInvalidParameter) {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get transaction summary")
			return
		}

		writeData(w, result)
	}
}

// MerchantSummaryHandler returns aggregated merchant-level statistics.
func MerchantSummaryHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		params := service.MerchantSummaryParams{}

		if v := q.Get("start_date"); v != "" {
			t, err := time.Parse("2006-01-02", v)
			if err != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "start_date must be in YYYY-MM-DD format")
				return
			}
			params.StartDate = &t
		}

		if v := q.Get("end_date"); v != "" {
			t, err := time.Parse("2006-01-02", v)
			if err != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "end_date must be in YYYY-MM-DD format")
				return
			}
			params.EndDate = &t
		}

		if v := q.Get("account_id"); v != "" {
			params.AccountID = &v
		}
		if v := q.Get("user_id"); v != "" {
			params.UserID = &v
		}
		if v := q.Get("category_slug"); v != "" {
			params.CategorySlug = &v
		}

		if v := q.Get("min_amount"); v != "" {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "min_amount must be a valid number")
				return
			}
			params.MinAmount = &f
		}

		if v := q.Get("max_amount"); v != "" {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "max_amount must be a valid number")
				return
			}
			params.MaxAmount = &f
		}

		if v := q.Get("search"); v != "" {
			params.Search = &v
		}
		if v := q.Get("exclude_search"); v != "" {
			if len(v) < 2 {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "exclude_search must be at least 2 characters")
				return
			}
			params.ExcludeSearch = &v
		}

		if v := q.Get("min_count"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "min_count must be a positive integer")
				return
			}
			params.MinCount = n
		}

		if q.Get("spending_only") == "true" {
			params.SpendingOnly = true
		}

		result, err := svc.GetMerchantSummary(r.Context(), params)
		if err != nil {
			if errors.Is(err, service.ErrInvalidParameter) {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get merchant summary")
			return
		}

		writeData(w, result)
	}
}

// SetTransactionCategoryHandler sets a manual category override on a transaction.
// PATCH /transactions/{id}/category
func SetTransactionCategoryHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var input struct {
			CategoryID string `json:"category_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body")
			return
		}
		if input.CategoryID == "" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "category_id is required")
			return
		}
		if err := svc.SetTransactionCategory(r.Context(), id, input.CategoryID); err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Transaction not found")
				return
			}
			if errors.Is(err, service.ErrCategoryNotFound) {
				mw.WriteError(w, http.StatusNotFound, "CATEGORY_NOT_FOUND", "Category not found")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to set transaction category")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ResetTransactionCategoryHandler clears the manual category override on a transaction.
// DELETE /transactions/{id}/category
func ResetTransactionCategoryHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.ResetTransactionCategory(r.Context(), id); err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Transaction not found")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to reset transaction category")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// BatchCategorizeHandler handles POST /api/v1/transactions/batch-categorize.
func BatchCategorizeHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Items []service.BatchCategorizeItem `json:"items"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body")
			return
		}

		result, err := svc.BatchSetTransactionCategory(r.Context(), input.Items)
		if err != nil {
			if errors.Is(err, service.ErrInvalidParameter) {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to batch categorize transactions")
			return
		}

		writeData(w, result)
	}
}

// BulkRecategorizeHandler handles POST /api/v1/transactions/bulk-recategorize.
func BulkRecategorizeHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			TargetCategorySlug string   `json:"target_category_slug"`
			StartDate          string   `json:"start_date"`
			EndDate            string   `json:"end_date"`
			AccountID          string   `json:"account_id"`
			UserID             string   `json:"user_id"`
			CategorySlug       string   `json:"category_slug"`
			MinAmount          *float64 `json:"min_amount"`
			MaxAmount          *float64 `json:"max_amount"`
			Pending            *bool    `json:"pending"`
			Search             string   `json:"search"`
			NameContains       string   `json:"name_contains"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body")
			return
		}

		if input.TargetCategorySlug == "" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "target_category_slug is required")
			return
		}

		params := service.BulkRecategorizeParams{
			TargetCategorySlug: input.TargetCategorySlug,
			MinAmount:          input.MinAmount,
			MaxAmount:          input.MaxAmount,
			Pending:            input.Pending,
		}

		if input.StartDate != "" {
			t, err := time.Parse("2006-01-02", input.StartDate)
			if err != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "invalid start_date format")
				return
			}
			params.StartDate = &t
		}
		if input.EndDate != "" {
			t, err := time.Parse("2006-01-02", input.EndDate)
			if err != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "invalid end_date format")
				return
			}
			params.EndDate = &t
		}
		if input.AccountID != "" {
			params.AccountID = &input.AccountID
		}
		if input.UserID != "" {
			params.UserID = &input.UserID
		}
		if input.CategorySlug != "" {
			params.CategorySlug = &input.CategorySlug
		}
		if input.Search != "" {
			params.Search = &input.Search
		}
		if input.NameContains != "" {
			params.NameContains = &input.NameContains
		}

		result, err := svc.BulkRecategorizeByFilter(r.Context(), params)
		if err != nil {
			if errors.Is(err, service.ErrInvalidParameter) {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
				return
			}
			if errors.Is(err, service.ErrCategoryNotFound) {
				mw.WriteError(w, http.StatusNotFound, "CATEGORY_NOT_FOUND", err.Error())
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to bulk recategorize transactions")
			return
		}

		writeData(w, result)
	}
}

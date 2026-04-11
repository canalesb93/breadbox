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

// parseTransactionFilters extracts the common filter parameters shared by
// ListTransactions and CountTransactions. It writes an error response and
// returns false when any parameter is invalid.
func parseTransactionFilters(w http.ResponseWriter, r *http.Request) (
	startDate, endDate *time.Time,
	accountID, userID, categorySlug *string,
	minAmount, maxAmount *float64,
	pending *bool,
	search, searchMode, excludeSearch *string,
	ok bool,
) {
	q := r.URL.Query()

	var err error

	startDate, err = parseDateParam(q, "start_date")
	if err != nil {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
		return
	}

	endDate, err = parseDateParam(q, "end_date")
	if err != nil {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
		return
	}

	if startDate != nil && endDate != nil && !startDate.Before(*endDate) {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "start_date must be before end_date")
		return
	}

	accountID = parseOptionalStringParam(q, "account_id")
	userID = parseOptionalStringParam(q, "user_id")
	categorySlug = parseOptionalStringParam(q, "category_slug")

	minAmount, err = parseFloatParam(q, "min_amount")
	if err != nil {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
		return
	}

	maxAmount, err = parseFloatParam(q, "max_amount")
	if err != nil {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
		return
	}

	if minAmount != nil && maxAmount != nil && *minAmount > *maxAmount {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "min_amount must be less than or equal to max_amount")
		return
	}

	pending, err = parseBoolParam(q, "pending")
	if err != nil {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
		return
	}

	search, err = parseMinLengthStringParam(q, "search", 2)
	if err != nil {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
		return
	}

	if v := q.Get("search_mode"); v != "" {
		if !service.ValidateSearchMode(v) {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "search_mode must be one of: contains, words, fuzzy")
			return
		}
		searchMode = &v
	}

	excludeSearch, err = parseMinLengthStringParam(q, "exclude_search", 2)
	if err != nil {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
		return
	}

	ok = true
	return
}

// ListTransactionsHandler returns a paginated, filterable list of transactions.
func ListTransactionsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		limit, err := parseIntParam(q, "limit", 100, 1, 500)
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}

		startDate, endDate, accountID, userID, categorySlug, minAmount, maxAmount, pending, search, searchMode, excludeSearch, ok := parseTransactionFilters(w, r)
		if !ok {
			return
		}

		// Parse sort_by.
		sortBy, err := parseEnumParam(q, "sort_by", []string{"date", "amount", "name"})
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
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
			Cursor:        q.Get("cursor"),
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
			SearchMode:    searchMode,
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
		startDate, endDate, accountID, userID, categorySlug, minAmount, maxAmount, pending, search, searchMode, excludeSearch, ok := parseTransactionFilters(w, r)
		if !ok {
			return
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
			SearchMode:    searchMode,
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

		var err error
		params.StartDate, err = parseDateParam(q, "start_date")
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}

		params.EndDate, err = parseDateParam(q, "end_date")
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}

		params.AccountID = parseOptionalStringParam(q, "account_id")
		params.UserID = parseOptionalStringParam(q, "user_id")
		params.Category = parseOptionalStringParam(q, "category")
		if q.Get("include_pending") == "true" {
			params.IncludePending = true
		}
		if q.Get("spending_only") == "true" {
			params.SpendingOnly = true
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

		var err error

		params.StartDate, err = parseDateParam(q, "start_date")
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}

		params.EndDate, err = parseDateParam(q, "end_date")
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}

		params.AccountID = parseOptionalStringParam(q, "account_id")
		params.UserID = parseOptionalStringParam(q, "user_id")
		params.CategorySlug = parseOptionalStringParam(q, "category_slug")

		params.MinAmount, err = parseFloatParam(q, "min_amount")
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}

		params.MaxAmount, err = parseFloatParam(q, "max_amount")
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}

		params.Search = parseOptionalStringParam(q, "search")
		if v := q.Get("search_mode"); v != "" {
			if !service.ValidateSearchMode(v) {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "search_mode must be one of: contains, words, fuzzy")
				return
			}
			params.SearchMode = &v
		}

		params.ExcludeSearch, err = parseMinLengthStringParam(q, "exclude_search", 2)
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}

		if v := q.Get("min_count"); v != "" {
			n, parseErr := strconv.Atoi(v)
			if parseErr != nil || n < 1 {
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

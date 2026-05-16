//go:build !lite

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

// transactionFilters bundles the filter values shared by ListTransactions and
// CountTransactions. Returning it via a struct keeps the parse function and
// its call sites readable as the v2 SPA adds new multi-select filters.
type transactionFilters struct {
	StartDate     *time.Time
	EndDate       *time.Time
	AccountID     *string
	UserID        *string
	CategorySlug  *string
	AccountIDs    []string
	CategorySlugs []string
	MinAmount     *float64
	MaxAmount     *float64
	Pending       *bool
	Search        *string
	SearchMode    *string
	ExcludeSearch *string
	Tags          []string
	AnyTag        []string
}

func parseTransactionFilters(w http.ResponseWriter, r *http.Request) (f transactionFilters, ok bool) {
	q := r.URL.Query()

	var err error

	f.StartDate, err = parseDateParam(q, "start_date")
	if err != nil {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
		return
	}

	f.EndDate, err = parseDateParam(q, "end_date")
	if err != nil {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
		return
	}

	if f.StartDate != nil && f.EndDate != nil && f.EndDate.Before(*f.StartDate) {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "start_date must be on or before end_date")
		return
	}

	// account_id and category_slug accept either a single value or a
	// comma-separated list; the service layer prefers the slice when present.
	if list := parseCSVParam(q, "account_id"); len(list) > 1 {
		f.AccountIDs = list
	} else {
		f.AccountID = parseOptionalStringParam(q, "account_id")
	}
	f.UserID = parseOptionalStringParam(q, "user_id")
	if list := parseCSVParam(q, "category_slug"); len(list) > 1 {
		f.CategorySlugs = list
	} else {
		f.CategorySlug = parseOptionalStringParam(q, "category_slug")
	}

	f.MinAmount, err = parseFloatParam(q, "min_amount")
	if err != nil {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
		return
	}

	f.MaxAmount, err = parseFloatParam(q, "max_amount")
	if err != nil {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
		return
	}

	if f.MinAmount != nil && f.MaxAmount != nil && *f.MinAmount > *f.MaxAmount {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "min_amount must be less than or equal to max_amount")
		return
	}

	f.Pending, err = parseBoolParam(q, "pending")
	if err != nil {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
		return
	}

	f.Search, err = parseMinLengthStringParam(q, "search", 2)
	if err != nil {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
		return
	}

	if v := q.Get("search_mode"); v != "" {
		if !service.ValidateSearchMode(v) {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "search_mode must be one of: contains, words, fuzzy")
			return
		}
		f.SearchMode = &v
	}

	f.ExcludeSearch, err = parseMinLengthStringParam(q, "exclude_search", 2)
	if err != nil {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
		return
	}

	f.Tags = parseCSVParam(q, "tags")
	f.AnyTag = parseCSVParam(q, "any_tag")

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

		f, ok := parseTransactionFilters(w, r)
		if !ok {
			return
		}

		sortBy, err := parseEnumParam(q, "sort_by", []string{"date", "amount", "provider_name"})
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}

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

		offset, err := parseIntParam(q, "offset", 0, 0, 1_000_000)
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}

		params := service.TransactionListParams{
			Cursor:        q.Get("cursor"),
			Offset:        offset,
			Limit:         limit,
			StartDate:     f.StartDate,
			EndDate:       f.EndDate,
			AccountID:     f.AccountID,
			UserID:        f.UserID,
			CategorySlug:  f.CategorySlug,
			AccountIDs:    f.AccountIDs,
			CategorySlugs: f.CategorySlugs,
			MinAmount:     f.MinAmount,
			MaxAmount:     f.MaxAmount,
			Pending:       f.Pending,
			Search:        f.Search,
			SearchMode:    f.SearchMode,
			ExcludeSearch: f.ExcludeSearch,
			SortBy:        sortBy,
			SortOrder:     sortOrder,
			Tags:          f.Tags,
			AnyTag:        f.AnyTag,
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
		f, ok := parseTransactionFilters(w, r)
		if !ok {
			return
		}

		params := service.TransactionCountParams{
			StartDate:     f.StartDate,
			EndDate:       f.EndDate,
			AccountID:     f.AccountID,
			UserID:        f.UserID,
			CategorySlug:  f.CategorySlug,
			AccountIDs:    f.AccountIDs,
			CategorySlugs: f.CategorySlugs,
			MinAmount:     f.MinAmount,
			MaxAmount:     f.MaxAmount,
			Pending:       f.Pending,
			Search:        f.Search,
			SearchMode:    f.SearchMode,
			ExcludeSearch: f.ExcludeSearch,
			Tags:          f.Tags,
			AnyTag:        f.AnyTag,
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
			writeServiceError(w, err, "Transaction not found", "Failed to get transaction")
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

		params.Search, err = parseMinLengthStringParam(q, "search", 2)
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}
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
		if !decodeJSON(w, r, &input) {
			return
		}
		if input.CategoryID == "" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "category_id is required")
			return
		}
		actor := service.ActorFromContext(r.Context())
		if err := svc.SetTransactionCategory(r.Context(), id, input.CategoryID, actor); err != nil {
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
		actor := service.ActorFromContext(r.Context())
		if err := svc.ResetTransactionCategory(r.Context(), id, actor); err != nil {
			writeServiceError(w, err, "Transaction not found", "Failed to reset transaction category")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// DeleteTransactionHandler soft-deletes a transaction by setting its
// `deleted_at` timestamp. All read paths filter on `deleted_at IS NULL`, so
// the row immediately disappears from list/get/summary/etc. The DB row is
// preserved so a subsequent restore call can bring it back.
//
// DELETE /transactions/{id}
//
// Returns 204 on success. Returns 404 NOT_FOUND when the transaction
// doesn't exist or is already soft-deleted — the call is idempotent at the
// API surface (already-gone == not found).
func DeleteTransactionHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		actor := service.ActorFromContext(r.Context())
		if err := svc.SoftDeleteTransaction(r.Context(), id, actor); err != nil {
			writeServiceError(w, err, "Transaction not found", "Failed to delete transaction")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// RestoreTransactionHandler clears the soft-delete flag on a previously
// deleted transaction, bringing it back into all read paths.
//
// POST /transactions/{id}/restore
//
// Returns 204 on success. Returns 404 NOT_FOUND when the transaction
// doesn't exist or isn't currently soft-deleted (nothing to restore).
//
// Note: the path-id must be a UUID for soft-deleted rows because the
// short_id → UUID resolver itself filters on `deleted_at IS NULL` and
// won't find a deleted row by short_id. Live (non-deleted) ids resolve
// via either form, but they fall through to the "not currently deleted"
// 404 anyway.
func RestoreTransactionHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		actor := service.ActorFromContext(r.Context())
		if err := svc.RestoreTransaction(r.Context(), id, actor); err != nil {
			writeServiceError(w, err, "Transaction not found", "Failed to restore transaction")
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
		if !decodeJSON(w, r, &input) {
			return
		}

		actor := service.ActorFromContext(r.Context())
		result, err := svc.BatchSetTransactionCategory(r.Context(), input.Items, actor)
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

// updateTransactionsRequest mirrors the MCP `update_transactions` tool's
// input shape 1:1 (snake_case, identical field names). REST sibling of
// internal/mcp/tools_update_transactions.go.
type updateTransactionsRequest struct {
	Operations []updateTransactionsOpRequest `json:"operations"`
	OnError    string                        `json:"on_error"`
}

// updateTransactionsOpRequest is a single per-transaction compound operation.
type updateTransactionsOpRequest struct {
	TransactionID string             `json:"transaction_id"`
	CategorySlug  *string            `json:"category_slug,omitempty"`
	ResetCategory bool               `json:"reset_category,omitempty"`
	TagsToAdd     []updateTagOpEntry `json:"tags_to_add,omitempty"`
	TagsToRemove  []updateTagOpEntry `json:"tags_to_remove,omitempty"`
	Comment       *string            `json:"comment,omitempty"`
}

// updateTagOpEntry is a single tag add/remove entry.
type updateTagOpEntry struct {
	Slug string `json:"slug"`
}

// UpdateTransactionsHandler is the REST sibling of the MCP `update_transactions`
// tool. Each operation can set a category (or clear an override), add/remove
// tags, and attach a comment — all atomic per transaction. Up to 50 ops per
// call. Per-op errors live inside `results[]`; the top-level call returns
// `200 OK` unless the input itself is malformed (empty/oversized operations,
// invalid `on_error`), in which case it returns `400 INVALID_PARAMETER`.
//
// POST /transactions/update
func UpdateTransactionsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input updateTransactionsRequest
		if !decodeJSON(w, r, &input) {
			return
		}

		ops := make([]service.UpdateTransactionsOp, len(input.Operations))
		for i, op := range input.Operations {
			ops[i] = service.UpdateTransactionsOp{
				TransactionID: op.TransactionID,
				CategorySlug:  op.CategorySlug,
				ResetCategory: op.ResetCategory,
				Comment:       op.Comment,
			}
			for _, t := range op.TagsToAdd {
				ops[i].TagsToAdd = append(ops[i].TagsToAdd, service.UpdateTransactionsTagOp{Slug: t.Slug})
			}
			for _, t := range op.TagsToRemove {
				ops[i].TagsToRemove = append(ops[i].TagsToRemove, service.UpdateTransactionsTagOp{Slug: t.Slug})
			}
		}

		actor := service.ActorFromContext(r.Context())
		results, err := svc.UpdateTransactions(r.Context(), service.UpdateTransactionsParams{
			Operations: ops,
			OnError:    input.OnError,
			Actor:      actor,
		})
		// Top-level validation errors (empty ops, > 50 ops, bad on_error)
		// surface as 400. Per-op errors live inside `results[]`.
		if err != nil && len(results) == 0 {
			if errors.Is(err, service.ErrInvalidParameter) {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update transactions")
			return
		}

		// abort mode with a partial-batch failure: results is populated AND
		// err is set. Mirror the MCP envelope — return 200 with the partial
		// results plus an `aborted` flag, so callers can see the per-op
		// outcomes before the rollback.
		succeeded := 0
		failed := 0
		for _, r := range results {
			if r.Status == "ok" {
				succeeded++
			} else {
				failed++
			}
		}

		payload := map[string]any{
			"results":   results,
			"succeeded": succeeded,
			"failed":    failed,
		}
		if err != nil {
			payload["aborted"] = true
			payload["error"] = err.Error()
		}
		writeData(w, payload)
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
		if !decodeJSON(w, r, &input) {
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

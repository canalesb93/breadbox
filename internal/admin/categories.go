package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// CategoriesPageHandler serves GET /admin/categories.
// Serves the combined categories + mappings page with tabs.
func CategoriesPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		categories, err := svc.ListCategoryTree(ctx)
		if err != nil {
			http.Error(w, "Failed to load categories", http.StatusInternalServerError)
			return
		}

		unmapped, err := svc.ListUnmappedCategories(ctx)
		if err != nil {
			http.Error(w, "Failed to load unmapped categories", http.StatusInternalServerError)
			return
		}

		// Parse optional provider filter for mappings tab.
		var providerPtr *string
		if p := r.URL.Query().Get("provider"); p == "plaid" || p == "teller" || p == "csv" {
			providerPtr = &p
		}

		mappings, err := svc.ListMappings(ctx, providerPtr)
		if err != nil {
			http.Error(w, "Failed to load mappings", http.StatusInternalServerError)
			return
		}

		filterProvider := ""
		if providerPtr != nil {
			filterProvider = *providerPtr
		}

		// Parse date range for spending data (default 30 days).
		spendingDays := 30
		if d := r.URL.Query().Get("days"); d != "" {
			switch d {
			case "7":
				spendingDays = 7
			case "30":
				spendingDays = 30
			case "90":
				spendingDays = 90
			case "365":
				spendingDays = 365
			}
		}
		spendingStart := time.Now().AddDate(0, 0, -spendingDays)

		// Fetch spending by category for the selected period.
		type CategorySpending struct {
			Amount           float64
			TransactionCount int64
			Percent          float64 // percentage of total spending
		}
		spendingByCategory := make(map[string]CategorySpending)
		var totalSpending float64
		var maxCategorySpend float64

		catSummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:      "category",
			StartDate:    &spendingStart,
			SpendingOnly: true,
		})
		if err == nil && catSummary != nil {
			for _, row := range catSummary.Summary {
				slug := "uncategorized"
				if row.Category != nil && *row.Category != "" {
					slug = *row.Category
				}
				spendingByCategory[slug] = CategorySpending{
					Amount:           row.TotalAmount,
					TransactionCount: row.TransactionCount,
				}
				totalSpending += row.TotalAmount
				if row.TotalAmount > maxCategorySpend {
					maxCategorySpend = row.TotalAmount
				}
			}
			// Calculate percentages.
			if totalSpending > 0 {
				for slug, cs := range spendingByCategory {
					cs.Percent = (cs.Amount / totalSpending) * 100
					spendingByCategory[slug] = cs
				}
			}
		}

		data := BaseTemplateData(r, sm, "categories", "Categories")
		data["Categories"] = categories
		data["UnmappedCount"] = len(unmapped)
		data["UnmappedCategories"] = unmapped
		data["Mappings"] = mappings
		data["FilterProvider"] = filterProvider
		data["SpendingByCategory"] = spendingByCategory
		data["TotalSpending"] = totalSpending
		data["MaxCategorySpend"] = maxCategorySpend
		data["SpendingDays"] = spendingDays
		tr.Render(w, r, "categories.html", data)
	}
}

// CreateCategoryAdminHandler handles POST /admin/api/categories.
func CreateCategoryAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			DisplayName string  `json:"display_name"`
			ParentID    *string `json:"parent_id"`
			Icon        *string `json:"icon"`
			Color       *string `json:"color"`
			SortOrder   int32   `json:"sort_order"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeCategoryError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
			return
		}
		req.DisplayName = strings.TrimSpace(req.DisplayName)
		if req.DisplayName == "" {
			writeCategoryError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Display name is required")
			return
		}

		result, err := svc.CreateCategory(r.Context(), service.CreateCategoryParams{
			DisplayName: req.DisplayName,
			ParentID:    req.ParentID,
			Icon:        req.Icon,
			Color:       req.Color,
			SortOrder:   req.SortOrder,
		})
		if err != nil {
			handleCategoryError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, result)
	}
}

// UpdateCategoryAdminHandler handles PUT /admin/api/categories/{id}.
func UpdateCategoryAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req struct {
			DisplayName string  `json:"display_name"`
			Icon        *string `json:"icon"`
			Color       *string `json:"color"`
			SortOrder   int32   `json:"sort_order"`
			Hidden      bool    `json:"hidden"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeCategoryError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
			return
		}
		req.DisplayName = strings.TrimSpace(req.DisplayName)
		if req.DisplayName == "" {
			writeCategoryError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Display name is required")
			return
		}

		result, err := svc.UpdateCategory(r.Context(), id, service.UpdateCategoryParams{
			DisplayName: req.DisplayName,
			Icon:        req.Icon,
			Color:       req.Color,
			SortOrder:   req.SortOrder,
			Hidden:      req.Hidden,
		})
		if err != nil {
			handleCategoryError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// DeleteCategoryAdminHandler handles DELETE /admin/api/categories/{id}.
func DeleteCategoryAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		affected, err := svc.DeleteCategory(r.Context(), id)
		if err != nil {
			handleCategoryError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int64{"affected_transactions": affected})
	}
}

// MergeCategoryAdminHandler handles POST /admin/api/categories/{id}/merge.
func MergeCategoryAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceID := chi.URLParam(r, "id")
		var req struct {
			TargetID string `json:"target_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeCategoryError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
			return
		}
		if req.TargetID == "" {
			writeCategoryError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "target_id is required")
			return
		}

		if err := svc.MergeCategories(r.Context(), sourceID, req.TargetID); err != nil {
			handleCategoryError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleCategoryError maps service-layer errors to HTTP responses.
func handleCategoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrCategoryNotFound):
		writeCategoryError(w, http.StatusNotFound, "NOT_FOUND", "Category not found")
	case errors.Is(err, service.ErrCategoryUndeletable):
		writeCategoryError(w, http.StatusConflict, "UNDELETABLE", "This category cannot be deleted")
	case errors.Is(err, service.ErrSlugConflict):
		writeCategoryError(w, http.StatusConflict, "SLUG_CONFLICT", "A category with this name already exists")
	case errors.Is(err, service.ErrInvalidParameter):
		writeCategoryError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		writeCategoryError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred")
	}
}

// writeCategoryError writes a JSON error envelope.
func writeCategoryError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

// SetTransactionCategoryAdminHandler handles POST /admin/api/transactions/{id}/category.
func SetTransactionCategoryAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req struct {
			CategoryID string `json:"category_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeCategoryError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
			return
		}
		if req.CategoryID == "" {
			writeCategoryError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "category_id is required")
			return
		}
		if err := svc.SetTransactionCategory(r.Context(), id, req.CategoryID); err != nil {
			if errors.Is(err, service.ErrNotFound) {
				writeCategoryError(w, http.StatusNotFound, "NOT_FOUND", "Transaction not found")
				return
			}
			if errors.Is(err, service.ErrCategoryNotFound) {
				writeCategoryError(w, http.StatusNotFound, "CATEGORY_NOT_FOUND", "Category not found")
				return
			}
			writeCategoryError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to set category")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ResetTransactionCategoryAdminHandler handles DELETE /admin/api/transactions/{id}/category.
func ResetTransactionCategoryAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.ResetTransactionCategory(r.Context(), id); err != nil {
			if errors.Is(err, service.ErrNotFound) {
				writeCategoryError(w, http.StatusNotFound, "NOT_FOUND", "Transaction not found")
				return
			}
			writeCategoryError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to reset category")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// BatchSetTransactionCategoryAdminHandler handles POST /admin/api/transactions/batch-categorize.
// Accepts {items: [{transaction_id, category_id}]} and sets category overrides on all.
func BatchSetTransactionCategoryAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Items []struct {
				TransactionID string `json:"transaction_id"`
				CategoryID    string `json:"category_id"`
			} `json:"items"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeCategoryError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
			return
		}
		if len(req.Items) == 0 {
			writeCategoryError(w, http.StatusBadRequest, "VALIDATION_ERROR", "items array is required")
			return
		}
		if len(req.Items) > 500 {
			writeCategoryError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Maximum 500 items per batch")
			return
		}

		succeeded := 0
		failed := 0
		for _, item := range req.Items {
			if item.TransactionID == "" || item.CategoryID == "" {
				failed++
				continue
			}
			if err := svc.SetTransactionCategory(r.Context(), item.TransactionID, item.CategoryID); err != nil {
				failed++
				continue
			}
			succeeded++
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"succeeded": succeeded,
			"failed":    failed,
			"total":     len(req.Items),
		})
	}
}

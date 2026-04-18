package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// CategoriesPageHandler serves GET /admin/categories.
func CategoriesPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		categories, err := svc.ListCategoryTree(ctx)
		if err != nil {
			http.Error(w, "Failed to load categories", http.StatusInternalServerError)
			return
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
		spendingByCategory := make(map[string]CategorySpending) // keyed by display name
		var totalSpending float64
		var maxCategorySpend float64

		catSummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:      "category",
			StartDate:    &spendingStart,
			SpendingOnly: true,
		})
		if err == nil && catSummary != nil {
			for _, row := range catSummary.Summary {
				name := "Uncategorized"
				if row.Category != nil && *row.Category != "" {
					name = *row.Category
				}
				spendingByCategory[name] = CategorySpending{
					Amount:           row.TotalAmount,
					TransactionCount: row.TransactionCount,
				}
				totalSpending += row.TotalAmount
			}

			// Aggregate child spending into parent totals.
			for _, parent := range categories {
				var parentAmount float64
				var parentCount int64
				// Add direct parent spending if any.
				if ps, ok := spendingByCategory[parent.DisplayName]; ok {
					parentAmount += ps.Amount
					parentCount += ps.TransactionCount
				}
				// Add children spending.
				for _, child := range parent.Children {
					if cs, ok := spendingByCategory[child.DisplayName]; ok {
						parentAmount += cs.Amount
						parentCount += cs.TransactionCount
					}
				}
				if parentAmount > 0 || parentCount > 0 {
					spendingByCategory[parent.DisplayName] = CategorySpending{
						Amount:           parentAmount,
						TransactionCount: parentCount,
					}
				}
				if parentAmount > maxCategorySpend {
					maxCategorySpend = parentAmount
				}
			}

			// Calculate percentages.
			if totalSpending > 0 {
				for name, cs := range spendingByCategory {
					cs.Percent = (cs.Amount / totalSpending) * 100
					spendingByCategory[name] = cs
				}
			}
		}

		data := BaseTemplateData(r, sm, "categories", "Categories")
		data["Categories"] = categories
		data["SpendingByCategory"] = spendingByCategory
		data["TotalSpending"] = totalSpending
		data["MaxCategorySpend"] = maxCategorySpend
		data["SpendingDays"] = spendingDays
		tr.Render(w, r, "categories.html", data)
	}
}

// CategoryNewPageHandler serves GET /categories/new — renders the empty create form.
func CategoryNewPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		categories, err := svc.ListCategoryTree(r.Context())
		if err != nil {
			tr.RenderError(w, r)
			return
		}
		data := BaseTemplateData(r, sm, "categories", "Add Category")
		data["IsEdit"] = false
		data["Categories"] = categories
		data["Breadcrumbs"] = []Breadcrumb{
			{Label: "Rules & Categories", Href: "/rules?tab=categories"},
			{Label: "Add Category"},
		}
		tr.Render(w, r, "category_form.html", data)
	}
}

// CategoryEditPageHandler serves GET /categories/{id}/edit — renders the form populated from DB.
func CategoryEditPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		category, err := svc.GetCategory(r.Context(), id)
		if err != nil {
			if errors.Is(err, service.ErrCategoryNotFound) || errors.Is(err, service.ErrNotFound) {
				tr.RenderNotFound(w, r)
				return
			}
			tr.RenderError(w, r)
			return
		}
		data := BaseTemplateData(r, sm, "categories", "Edit "+category.DisplayName)
		data["IsEdit"] = true
		data["Category"] = category
		data["Breadcrumbs"] = []Breadcrumb{
			{Label: "Rules & Categories", Href: "/rules?tab=categories"},
			{Label: category.DisplayName},
		}
		tr.Render(w, r, "category_form.html", data)
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
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
			return
		}
		req.DisplayName = strings.TrimSpace(req.DisplayName)
		if req.DisplayName == "" {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Display name is required")
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
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
			return
		}
		req.DisplayName = strings.TrimSpace(req.DisplayName)
		if req.DisplayName == "" {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Display name is required")
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
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
			return
		}
		if req.TargetID == "" {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "target_id is required")
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
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Category not found")
	case errors.Is(err, service.ErrCategoryUndeletable):
		writeError(w, http.StatusConflict, "UNDELETABLE", "This category cannot be deleted")
	case errors.Is(err, service.ErrSlugConflict):
		writeError(w, http.StatusConflict, "SLUG_CONFLICT", "A category with this name already exists")
	case errors.Is(err, service.ErrInvalidParameter):
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred")
	}
}

// SetTransactionCategoryAdminHandler handles POST /admin/api/transactions/{id}/category.
func SetTransactionCategoryAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req struct {
			CategoryID string `json:"category_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
			return
		}
		if req.CategoryID == "" {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "category_id is required")
			return
		}
		if err := svc.SetTransactionCategory(r.Context(), id, req.CategoryID); err != nil {
			if errors.Is(err, service.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Transaction not found")
				return
			}
			if errors.Is(err, service.ErrCategoryNotFound) {
				writeError(w, http.StatusNotFound, "CATEGORY_NOT_FOUND", "Category not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to set category")
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
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Transaction not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to reset category")
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
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
			return
		}
		if len(req.Items) == 0 {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "items array is required")
			return
		}
		if len(req.Items) > 500 {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Maximum 500 items per batch")
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

// flattenCategories converts a category tree into a flat list suitable for dropdown selects.
func flattenCategories(tree []service.CategoryResponse) []service.CategoryResponse {
	var flat []service.CategoryResponse
	for _, parent := range tree {
		flat = append(flat, parent)
		flat = append(flat, parent.Children...)
	}
	return flat
}

// ExportCategoriesTSVAdminHandler handles GET /admin/api/categories/export-tsv.
func ExportCategoriesTSVAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tsv, err := svc.ExportCategoriesTSV(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to export categories"})
			return
		}
		w.Header().Set("Content-Type", "text/tab-separated-values")
		w.Header().Set("Content-Disposition", "attachment; filename=categories.tsv")
		w.Write([]byte(tsv))
	}
}

// ImportCategoriesTSVAdminHandler handles POST /admin/api/categories/import-tsv.
func ImportCategoriesTSVAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Failed to read request body"})
			return
		}
		if len(body) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Request body is empty"})
			return
		}

		replaceMode := r.URL.Query().Get("replace") == "true"

		result, err := svc.ImportCategoriesTSV(r.Context(), string(body), replaceMode)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": map[string]string{"code": "IMPORT_ERROR", "message": err.Error()},
			})
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

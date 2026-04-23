package admin

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// CategoriesPageHandler serves GET /admin/categories. This is a
// configuration-only page — no spending aggregates, no period selector.
func CategoriesPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		categories, err := svc.ListCategoryTree(r.Context())
		if err != nil {
			http.Error(w, "Failed to load categories", http.StatusInternalServerError)
			return
		}

		data := BaseTemplateData(r, sm, "categories", "Categories")
		tr.RenderWithTempl(w, r, data, pages.Categories(pages.CategoriesProps{
			Categories: categories,
		}))
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
			{Label: "Categories", Href: "/categories"},
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
			{Label: "Categories", Href: "/categories"},
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
		if !decodeJSON(w, r, &req) {
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
		if !decodeJSON(w, r, &req) {
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
		if !decodeJSON(w, r, &req) {
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
		if !decodeJSON(w, r, &req) {
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
		if !decodeJSON(w, r, &req) {
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

		writeJSON(w, http.StatusOK, map[string]any{
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

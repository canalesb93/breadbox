package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// ListCategoriesHandler returns the full category tree (parents with children).
func ListCategoriesHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		categories, err := svc.ListCategoryTree(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list categories")
			return
		}
		writeData(w, categories)
	}
}

// GetCategoryHandler returns a single category by ID.
func GetCategoryHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		category, err := svc.GetCategory(r.Context(), id)
		if err != nil {
			if errors.Is(err, service.ErrCategoryNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Category not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get category")
			return
		}

		writeData(w, category)
	}
}

type createCategoryRequest struct {
	DisplayName string  `json:"display_name"`
	Slug        string  `json:"slug,omitempty"`
	ParentID    *string `json:"parent_id,omitempty"`
	Icon        *string `json:"icon,omitempty"`
	Color       *string `json:"color,omitempty"`
	SortOrder   int32   `json:"sort_order"`
}

// CreateCategoryHandler creates a new category.
func CreateCategoryHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input createCategoryRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body")
			return
		}

		if input.DisplayName == "" {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "display_name is required")
			return
		}

		category, err := svc.CreateCategory(r.Context(), service.CreateCategoryParams{
			DisplayName: input.DisplayName,
			Slug:        input.Slug,
			ParentID:    input.ParentID,
			Icon:        input.Icon,
			Color:       input.Color,
			SortOrder:   input.SortOrder,
		})
		if err != nil {
			if errors.Is(err, service.ErrSlugConflict) {
				writeError(w, http.StatusConflict, "SLUG_CONFLICT", "A category with this slug already exists")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create category")
			return
		}

		writeJSON(w, http.StatusCreated, category)
	}
}

type updateCategoryRequest struct {
	DisplayName string  `json:"display_name"`
	Icon        *string `json:"icon,omitempty"`
	Color       *string `json:"color,omitempty"`
	SortOrder   int32   `json:"sort_order"`
	Hidden      bool    `json:"hidden"`
}

// UpdateCategoryHandler updates an existing category.
func UpdateCategoryHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var input updateCategoryRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body")
			return
		}

		if input.DisplayName == "" {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "display_name is required")
			return
		}

		category, err := svc.UpdateCategory(r.Context(), id, service.UpdateCategoryParams{
			DisplayName: input.DisplayName,
			Icon:        input.Icon,
			Color:       input.Color,
			SortOrder:   input.SortOrder,
			Hidden:      input.Hidden,
		})
		if err != nil {
			if errors.Is(err, service.ErrCategoryNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Category not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update category")
			return
		}

		writeData(w, category)
	}
}

// DeleteCategoryHandler deletes a category and returns the count of affected transactions.
func DeleteCategoryHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		affected, err := svc.DeleteCategory(r.Context(), id)
		if err != nil {
			if errors.Is(err, service.ErrCategoryNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Category not found")
				return
			}
			if errors.Is(err, service.ErrCategoryUndeletable) {
				writeError(w, http.StatusConflict, "CATEGORY_UNDELETABLE", "This category cannot be deleted")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete category")
			return
		}

		writeData(w, map[string]int64{"affected_transactions": affected})
	}
}

type mergeCategoriesRequest struct {
	TargetID string `json:"target_id"`
}

// MergeCategoriesHandler merges the source category into a target category.
func MergeCategoriesHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var input mergeCategoriesRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body")
			return
		}

		if input.TargetID == "" {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "target_id is required")
			return
		}

		err := svc.MergeCategories(r.Context(), id, input.TargetID)
		if err != nil {
			if errors.Is(err, service.ErrCategoryNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Category not found")
				return
			}
			if errors.Is(err, service.ErrInvalidParameter) {
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to merge categories")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

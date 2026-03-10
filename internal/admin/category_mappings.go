package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// flattenCategories converts a category tree into a flat list suitable for dropdown selects.
func flattenCategories(tree []service.CategoryResponse) []service.CategoryResponse {
	var flat []service.CategoryResponse
	for _, parent := range tree {
		flat = append(flat, parent)
		for _, child := range parent.Children {
			flat = append(flat, child)
		}
	}
	return flat
}

// MappingsPageHandler serves GET /admin/categories/mappings.
func MappingsPageHandler(svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Parse optional provider filter.
		var providerPtr *string
		if p := r.URL.Query().Get("provider"); p == "plaid" || p == "teller" || p == "csv" {
			providerPtr = &p
		}

		mappings, err := svc.ListMappings(ctx, providerPtr)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		tree, err := svc.ListCategoryTree(ctx)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		unmapped, err := svc.ListUnmappedCategories(ctx)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		filterProvider := ""
		if providerPtr != nil {
			filterProvider = *providerPtr
		}

		data := BaseTemplateData(r, sm, "categories", "Category Mappings")
		data["Mappings"] = mappings
		data["AllCategories"] = flattenCategories(tree)
		data["UnmappedCategories"] = unmapped
		data["FilterProvider"] = filterProvider

		tr.Render(w, r, "category_mappings.html", data)
	}
}

// CreateMappingAdminHandler handles POST /admin/api/category-mappings.
func CreateMappingAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Provider         string `json:"provider"`
			ProviderCategory string `json:"provider_category"`
			CategoryID       string `json:"category_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		if input.Provider == "" || input.ProviderCategory == "" || input.CategoryID == "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error": map[string]string{"code": "VALIDATION_ERROR", "message": "provider, provider_category, and category_id are required"},
			})
			return
		}

		result, err := svc.CreateMapping(r.Context(), input.Provider, input.ProviderCategory, input.CategoryID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create mapping"})
			return
		}
		writeJSON(w, http.StatusCreated, result)
	}
}

// UpdateMappingAdminHandler handles PUT /admin/api/category-mappings/{id}.
func UpdateMappingAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var input struct {
			CategoryID string `json:"category_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		if input.CategoryID == "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error": map[string]string{"code": "VALIDATION_ERROR", "message": "category_id is required"},
			})
			return
		}

		result, err := svc.UpdateMapping(r.Context(), id, input.CategoryID)
		if err != nil {
			if errors.Is(err, service.ErrMappingNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Mapping not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update mapping"})
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// DeleteMappingAdminHandler handles DELETE /admin/api/category-mappings/{id}.
func DeleteMappingAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		if err := svc.DeleteMapping(r.Context(), id); err != nil {
			if errors.Is(err, service.ErrMappingNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Mapping not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete mapping"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// BulkUpsertMappingsAdminHandler handles PUT /admin/api/category-mappings/bulk.
func BulkUpsertMappingsAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Mappings []service.BulkMappingEntry `json:"mappings"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		if len(input.Mappings) == 0 {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error": map[string]string{"code": "VALIDATION_ERROR", "message": "mappings array is required and must not be empty"},
			})
			return
		}

		count, err := svc.BulkUpsertMappings(r.Context(), input.Mappings)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to bulk upsert mappings"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"upserted": count})
	}
}

// ExportMappingsAdminHandler handles GET /admin/api/category-mappings/export.
func ExportMappingsAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mappings, err := svc.ListMappings(r.Context(), nil)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to export mappings"})
			return
		}
		writeJSON(w, http.StatusOK, mappings)
	}
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

// ExportMappingsTSVAdminHandler handles GET /admin/api/category-mappings/export-tsv.
func ExportMappingsTSVAdminHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tsv, err := svc.ExportMappingsTSV(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to export mappings"})
			return
		}
		w.Header().Set("Content-Type", "text/tab-separated-values")
		w.Header().Set("Content-Disposition", "attachment; filename=category_mappings.tsv")
		w.Write([]byte(tsv))
	}
}

// ImportMappingsTSVAdminHandler handles POST /admin/api/category-mappings/import-tsv.
func ImportMappingsTSVAdminHandler(svc *service.Service) http.HandlerFunc {
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

		applyRetroactively := r.URL.Query().Get("apply_retroactively") == "true"
		replaceMode := r.URL.Query().Get("replace") == "true"

		result, err := svc.ImportMappingsTSV(r.Context(), string(body), applyRetroactively, replaceMode)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": map[string]string{"code": "IMPORT_ERROR", "message": err.Error()},
			})
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

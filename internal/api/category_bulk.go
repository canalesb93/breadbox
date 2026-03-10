package api

import (
	"io"
	"net/http"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"
)

// ExportCategoriesTSVHandler handles GET /api/v1/categories/export.
func ExportCategoriesTSVHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tsv, err := svc.ExportCategoriesTSV(r.Context())
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to export categories")
			return
		}
		w.Header().Set("Content-Type", "text/tab-separated-values")
		w.Header().Set("Content-Disposition", "attachment; filename=categories.tsv")
		w.Write([]byte(tsv))
	}
}

// ImportCategoriesTSVHandler handles POST /api/v1/categories/import.
func ImportCategoriesTSVHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Failed to read request body")
			return
		}
		if len(body) == 0 {
			mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Request body is empty")
			return
		}

		result, err := svc.ImportCategoriesTSV(r.Context(), string(body))
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "IMPORT_ERROR", err.Error())
			return
		}
		writeData(w, result)
	}
}

// ExportMappingsTSVHandler handles GET /api/v1/category-mappings/export.
func ExportMappingsTSVHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tsv, err := svc.ExportMappingsTSV(r.Context())
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to export mappings")
			return
		}
		w.Header().Set("Content-Type", "text/tab-separated-values")
		w.Header().Set("Content-Disposition", "attachment; filename=category_mappings.tsv")
		w.Write([]byte(tsv))
	}
}

// ImportMappingsTSVHandler handles POST /api/v1/category-mappings/import.
func ImportMappingsTSVHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Failed to read request body")
			return
		}
		if len(body) == 0 {
			mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Request body is empty")
			return
		}

		applyRetroactively := r.URL.Query().Get("apply_retroactively") == "true"

		result, err := svc.ImportMappingsTSV(r.Context(), string(body), applyRetroactively)
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "IMPORT_ERROR", err.Error())
			return
		}
		writeData(w, result)
	}
}

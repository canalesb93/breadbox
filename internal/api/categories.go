package api

import (
	"net/http"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"
)

func ListCategoriesHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		categories, err := svc.ListDistinctCategories(r.Context())
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list categories")
			return
		}
		writeData(w, categories)
	}
}

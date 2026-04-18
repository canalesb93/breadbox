package api

import (
	"net/http"

	"breadbox/internal/service"
)

// ListUsersHandler returns all users.
func ListUsersHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		users, err := svc.ListUsers(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list users")
			return
		}

		writeData(w, users)
	}
}

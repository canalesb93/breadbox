//go:build !lite

package api

import (
	"net/http"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// ListUsersHandler returns all users.
func ListUsersHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		users, err := svc.ListUsers(r.Context())
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list users")
			return
		}

		writeData(w, users)
	}
}

// GetUserHandler returns a single household member by UUID or short_id.
// GET /api/v1/users/{id}.
func GetUserHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		user, err := svc.GetUser(r.Context(), id)
		if err != nil {
			writeServiceError(w, err, "User not found", "Failed to get user")
			return
		}
		writeData(w, user)
	}
}

// createUserRequest is the JSON body shape for POST /api/v1/users.
type createUserRequest struct {
	Name  string  `json:"name"`
	Email *string `json:"email,omitempty"`
}

// CreateUserHandler creates a new household member.
// POST /api/v1/users — name required, email optional.
func CreateUserHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input createUserRequest
		if !decodeJSON(w, r, &input) {
			return
		}

		user, err := svc.CreateUser(r.Context(), service.CreateUserParams{
			Name:  input.Name,
			Email: input.Email,
		})
		if err != nil {
			writeServiceError(w, err, "User not found", "Failed to create user")
			return
		}
		writeJSON(w, http.StatusCreated, user)
	}
}

// updateUserRequest is the JSON body shape for PATCH /api/v1/users/{id}.
// Every field is optional. Pass `email: ""` (non-null empty string) to
// clear the stored email.
type updateUserRequest struct {
	Name  *string `json:"name,omitempty"`
	Email *string `json:"email,omitempty"`
}

// UpdateUserHandler partially updates a household member.
// PATCH /api/v1/users/{id}.
func UpdateUserHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var input updateUserRequest
		if !decodeJSON(w, r, &input) {
			return
		}

		user, err := svc.UpdateUser(r.Context(), id, service.UpdateUserParams{
			Name:  input.Name,
			Email: input.Email,
		})
		if err != nil {
			writeServiceError(w, err, "User not found", "Failed to update user")
			return
		}
		writeData(w, user)
	}
}

// DeleteUserHandler removes a household member. Refuses (409) when the
// user still has bank connections attached — callers should hit
// POST /users/{id}/wipe-data first.
// DELETE /api/v1/users/{id}.
func DeleteUserHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.DeleteUser(r.Context(), id); err != nil {
			writeServiceError(w, err, "User not found", "Failed to delete user")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// WipeUserDataHandler is a destructive endpoint that removes all bank
// connections, accounts, and transactions belonging to a household
// member. Mirrors the admin /-/users/{id}/wipe action; protected by
// the write scope.
// POST /api/v1/users/{id}/wipe-data.
func WipeUserDataHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		// Verify the user exists up-front so we can return 404 with a
		// clean message rather than a wrapped "invalid user_id" error.
		if _, err := svc.GetUser(r.Context(), id); err != nil {
			writeServiceError(w, err, "User not found", "Failed to wipe user data")
			return
		}

		count, err := svc.WipeUserData(r.Context(), id)
		if err != nil {
			writeServiceError(w, err, "User not found", "Failed to wipe user data")
			return
		}
		writeData(w, map[string]any{
			"status":               "ok",
			"deleted_transactions": count,
		})
	}
}

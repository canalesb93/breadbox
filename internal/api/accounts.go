package api

import (
	"errors"
	"net/http"

	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// ListAccountsHandler returns all accounts, optionally filtered by user_id.
func ListAccountsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.URL.Query().Get("user_id")
		var userIDPtr *string
		if userID != "" {
			userIDPtr = &userID
		}

		accounts, err := svc.ListAccounts(r.Context(), userIDPtr)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list accounts")
			return
		}

		writeData(w, accounts)
	}
}

// GetAccountHandler returns a single account by ID.
func GetAccountHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		account, err := svc.GetAccount(r.Context(), id)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Account not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get account")
			return
		}

		writeData(w, account)
	}
}

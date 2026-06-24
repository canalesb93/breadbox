//go:build !lite

package api

import (
	"net/http"

	mw "breadbox/internal/middleware"
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
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list accounts")
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
			writeServiceError(w, err, "Account not found", "Failed to get account")
			return
		}

		writeData(w, account)
	}
}

// GetAccountDetailHandler returns the full detail payload for an account,
// including the most recent transactions and balances by currency.
// GET /api/v1/accounts/{id}/detail
func GetAccountDetailHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		detail, err := svc.GetAccountDetailResponse(r.Context(), id, 25)
		if err != nil {
			writeServiceError(w, err, "Account not found", "Failed to get account detail")
			return
		}

		writeData(w, detail)
	}
}

// updateAccountRequest is the JSON body for PATCH /accounts/{id}. Every
// field is optional; omit a key to leave the corresponding column unchanged.
// To clear `display_name`, send an explicit empty string. To clear the
// per-account owner override (inherit the connection owner), send
// `owner_user_id` as an explicit empty string.
type updateAccountRequest struct {
	DisplayName       *string `json:"display_name,omitempty"`
	IsExcluded        *bool   `json:"is_excluded,omitempty"`
	IsDependentLinked *bool   `json:"is_dependent_linked,omitempty"`
	OwnerUserID       *string `json:"owner_user_id,omitempty"`
}

// UpdateAccountHandler partially updates a single account.
// PATCH /api/v1/accounts/{id}
func UpdateAccountHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var req updateAccountRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		account, err := svc.UpdateAccount(r.Context(), id, service.UpdateAccountParams{
			DisplayName:       req.DisplayName,
			IsExcluded:        req.IsExcluded,
			IsDependentLinked: req.IsDependentLinked,
			OwnerUserID:       req.OwnerUserID,
		})
		if err != nil {
			writeServiceError(w, err, "Account not found", "Failed to update account")
			return
		}

		writeData(w, account)
	}
}

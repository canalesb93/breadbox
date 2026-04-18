package api

import (
	"errors"
	"net/http"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// ListAccountLinksHandler returns all account links.
func ListAccountLinksHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		links, err := svc.ListAccountLinks(r.Context())
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list account links")
			return
		}
		writeData(w, links)
	}
}

// CreateAccountLinkHandler creates a new account link.
func CreateAccountLinkHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			PrimaryAccountID   string `json:"primary_account_id"`
			DependentAccountID string `json:"dependent_account_id"`
			MatchStrategy      string `json:"match_strategy"`
			MatchToleranceDays int    `json:"match_tolerance_days"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}

		if input.PrimaryAccountID == "" || input.DependentAccountID == "" {
			mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "primary_account_id and dependent_account_id are required")
			return
		}

		link, err := svc.CreateAccountLink(r.Context(), service.CreateAccountLinkParams{
			PrimaryAccountID:   input.PrimaryAccountID,
			DependentAccountID: input.DependentAccountID,
			MatchStrategy:      input.MatchStrategy,
			MatchToleranceDays: input.MatchToleranceDays,
		})
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
				return
			}
			if errors.Is(err, service.ErrInvalidParameter) {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create account link")
			return
		}

		writeJSON(w, http.StatusCreated, link)
	}
}

// GetAccountLinkHandler returns a single account link.
func GetAccountLinkHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		link, err := svc.GetAccountLink(r.Context(), id)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Account link not found")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get account link")
			return
		}
		writeData(w, link)
	}
}

// UpdateAccountLinkHandler updates an account link.
func UpdateAccountLinkHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var input struct {
			MatchStrategy      *string `json:"match_strategy"`
			MatchToleranceDays *int    `json:"match_tolerance_days"`
			Enabled            *bool   `json:"enabled"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}

		link, err := svc.UpdateAccountLink(r.Context(), id, service.UpdateAccountLinkParams{
			MatchStrategy:      input.MatchStrategy,
			MatchToleranceDays: input.MatchToleranceDays,
			Enabled:            input.Enabled,
		})
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Account link not found")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update account link")
			return
		}
		writeData(w, link)
	}
}

// DeleteAccountLinkHandler deletes an account link.
func DeleteAccountLinkHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.DeleteAccountLink(r.Context(), id); err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Account link not found")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete account link")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ReconcileAccountLinkHandler triggers match reconciliation for a link.
func ReconcileAccountLinkHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		result, err := svc.RunMatchReconciliation(r.Context(), id)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Account link not found")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to reconcile")
			return
		}
		writeData(w, result)
	}
}

// ListTransactionMatchesHandler returns matches for a link.
func ListTransactionMatchesHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		_ = r.URL.Query().Get("limit") // unused for now, matches are unbounded per link

		matches, err := svc.ListTransactionMatches(r.Context(), id)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Account link not found")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list matches")
			return
		}
		writeData(w, matches)
	}
}

// ConfirmMatchHandler confirms an auto-match.
func ConfirmMatchHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.ConfirmMatch(r.Context(), id); err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Match not found")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to confirm match")
			return
		}
		writeData(w, map[string]string{"status": "confirmed"})
	}
}

// RejectMatchHandler rejects a match and clears attribution.
func RejectMatchHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.RejectMatch(r.Context(), id); err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Match not found")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to reject match")
			return
		}
		writeData(w, map[string]string{"status": "rejected"})
	}
}

// ManualMatchHandler creates a manual match between two transactions.
func ManualMatchHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			LinkID              string `json:"link_id"`
			PrimaryTransactionID   string `json:"primary_transaction_id"`
			DependentTransactionID string `json:"dependent_transaction_id"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}

		if input.LinkID == "" || input.PrimaryTransactionID == "" || input.DependentTransactionID == "" {
			mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "link_id, primary_transaction_id, and dependent_transaction_id are required")
			return
		}

		match, err := svc.ManualMatch(r.Context(), input.LinkID, input.PrimaryTransactionID, input.DependentTransactionID)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
				return
			}
			if errors.Is(err, service.ErrInvalidParameter) {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create match")
			return
		}

		writeJSON(w, http.StatusCreated, match)
	}
}
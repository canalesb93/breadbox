//go:build !lite

package api

import (
	"net/http"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// Flag a transaction for human attention. The reason (optional) is stored as a
// comment annotation, not a column. Reads expose flagged_at on the transaction;
// filter the list with ?flagged=true. Both require full_access scope.

// FlagTransactionHandler — POST /transactions/{id}/flag  body: {"reason": "..."} (optional)
func FlagTransactionHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var input struct {
			Reason string `json:"reason"`
		}
		// Body is optional; tolerate an empty/absent body but reject malformed JSON.
		if err := decodeJSONOptional(r, &input); err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body")
			return
		}
		actor := service.ActorFromContext(r.Context())
		if err := svc.FlagTransaction(r.Context(), id, input.Reason, actor); err != nil {
			writeServiceError(w, err, "Transaction not found", "Failed to flag transaction")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// UnflagTransactionHandler — DELETE /transactions/{id}/flag
func UnflagTransactionHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.UnflagTransaction(r.Context(), id); err != nil {
			writeServiceError(w, err, "Transaction not found", "Failed to unflag transaction")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

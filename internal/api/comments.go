//go:build !lite

package api

import (
	"net/http"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// writeCommentServiceError maps a service error from a comment handler to the
// canonical envelope, overriding NOT_FOUND and FORBIDDEN messages with
// resource-specific text. Falls back to a 500 INTERNAL_ERROR for anything not
// covered by the service-error sentinel set.
func writeCommentServiceError(w http.ResponseWriter, err error, notFoundMessage, forbiddenMessage, internalMessage string) {
	if resp, ok := mw.MapServiceError(err); ok {
		switch resp.Code {
		case "NOT_FOUND":
			if notFoundMessage != "" {
				resp.Message = notFoundMessage
			}
		case "FORBIDDEN":
			if forbiddenMessage != "" {
				resp.Message = forbiddenMessage
			}
		}
		mw.WriteError(w, resp.Status, resp.Code, resp.Message)
		return
	}
	mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", internalMessage)
}

// ListCommentsHandler returns all comments for a transaction.
func ListCommentsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txnID := chi.URLParam(r, "transaction_id")

		comments, err := svc.ListComments(r.Context(), txnID)
		if err != nil {
			writeServiceError(w, err, "Transaction not found", "Failed to list comments")
			return
		}

		writeData(w, map[string]any{"comments": comments})
	}
}

// CreateCommentHandler creates a new comment on a transaction.
func CreateCommentHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txnID := chi.URLParam(r, "transaction_id")

		var input struct {
			Content string `json:"content"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}

		actor := service.ActorFromContext(r.Context())

		comment, err := svc.CreateComment(r.Context(), service.CreateCommentParams{
			TransactionID: txnID,
			Content:       input.Content,
			Actor:         actor,
		})
		if err != nil {
			writeServiceError(w, err, "Transaction not found", "Failed to create comment")
			return
		}

		writeJSON(w, http.StatusCreated, comment)
	}
}

// UpdateCommentHandler updates a comment's content.
func UpdateCommentHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		commentID := chi.URLParam(r, "id")

		var input struct {
			Content string `json:"content"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}

		actor := service.ActorFromContext(r.Context())

		comment, err := svc.UpdateComment(r.Context(), commentID, service.UpdateCommentParams{
			Content: input.Content,
			Actor:   actor,
		})
		if err != nil {
			writeCommentServiceError(w, err, "Comment not found", "You can only edit your own comments", "Failed to update comment")
			return
		}

		writeData(w, comment)
	}
}

// DeleteCommentHandler deletes a comment.
func DeleteCommentHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		commentID := chi.URLParam(r, "id")
		actor := service.ActorFromContext(r.Context())

		if err := svc.DeleteComment(r.Context(), commentID, actor); err != nil {
			writeCommentServiceError(w, err, "Comment not found", "You can only delete your own comments", "Failed to delete comment")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

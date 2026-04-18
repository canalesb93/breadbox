package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// ListCommentsHandler returns all comments for a transaction.
func ListCommentsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txnID := chi.URLParam(r, "transaction_id")

		comments, err := svc.ListComments(r.Context(), txnID)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Transaction not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list comments")
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
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body")
			return
		}

		actor := service.ActorFromContext(r.Context())

		comment, err := svc.CreateComment(r.Context(), service.CreateCommentParams{
			TransactionID: txnID,
			Content:       input.Content,
			Actor:         actor,
		})
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Transaction not found")
				return
			}
			if strings.Contains(err.Error(), "content must be") {
				writeError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create comment")
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
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body")
			return
		}

		actor := service.ActorFromContext(r.Context())

		comment, err := svc.UpdateComment(r.Context(), commentID, service.UpdateCommentParams{
			Content: input.Content,
			Actor:   actor,
		})
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Comment not found")
				return
			}
			if errors.Is(err, service.ErrForbidden) {
				writeError(w, http.StatusForbidden, "FORBIDDEN", "You can only edit your own comments")
				return
			}
			if strings.Contains(err.Error(), "content must be") {
				writeError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update comment")
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
			if errors.Is(err, service.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Comment not found")
				return
			}
			if errors.Is(err, service.ErrForbidden) {
				writeError(w, http.StatusForbidden, "FORBIDDEN", "You can only delete your own comments")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete comment")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

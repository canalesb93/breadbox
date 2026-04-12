package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"breadbox/internal/db"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const maxCommentLength = 10000

// CreateComment adds a comment to a transaction.
func (s *Service) CreateComment(ctx context.Context, params CreateCommentParams) (*CommentResponse, error) {
	content := strings.TrimSpace(params.Content)
	if content == "" || len(content) > maxCommentLength {
		return nil, fmt.Errorf("content must be between 1 and %d characters", maxCommentLength)
	}

	// Verify transaction exists and is not soft-deleted.
	txnID, err := s.resolveTransactionID(ctx, params.TransactionID)
	if err != nil {
		return nil, ErrNotFound
	}

	var deletedAt pgtype.Timestamptz
	err = s.Pool.QueryRow(ctx, "SELECT deleted_at FROM transactions WHERE id = $1", txnID).Scan(&deletedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("check transaction: %w", err)
	}
	if deletedAt.Valid {
		return nil, ErrNotFound
	}

	var authorID pgtype.Text
	if params.Actor.ID != "" {
		authorID = pgtype.Text{String: params.Actor.ID, Valid: true}
	}

	var reviewID pgtype.UUID
	if params.ReviewID != "" {
		parsed, err := parseUUID(params.ReviewID)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid review_id", ErrInvalidParameter)
		}
		reviewID = parsed
	}

	comment, err := s.Queries.CreateComment(ctx, db.CreateCommentParams{
		TransactionID: txnID,
		AuthorType:    params.Actor.Type,
		AuthorID:      authorID,
		AuthorName:    params.Actor.Name,
		Content:       content,
		ReviewID:      reviewID,
	})
	if err != nil {
		return nil, fmt.Errorf("create comment: %w", err)
	}

	resp := commentFromRow(comment)
	return &resp, nil
}

// ListComments returns all comments for a transaction, ordered by created_at ASC.
func (s *Service) ListComments(ctx context.Context, transactionID string) ([]CommentResponse, error) {
	txnID, err := s.resolveTransactionID(ctx, transactionID)
	if err != nil {
		return nil, ErrNotFound
	}

	rows, err := s.Queries.ListCommentsByTransaction(ctx, txnID)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}

	result := make([]CommentResponse, len(rows))
	for i, r := range rows {
		result[i] = commentFromRow(r)
	}
	return result, nil
}

// UpdateComment updates the content of an existing comment.
func (s *Service) UpdateComment(ctx context.Context, id string, params UpdateCommentParams) (*CommentResponse, error) {
	content := strings.TrimSpace(params.Content)
	if content == "" || len(content) > maxCommentLength {
		return nil, fmt.Errorf("content must be between 1 and %d characters", maxCommentLength)
	}

	commentID, err := s.resolveCommentID(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}

	existing, err := s.Queries.GetComment(ctx, commentID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get comment: %w", err)
	}

	// Check authorization: author match or admin.
	if !canModifyComment(existing, params.Actor) {
		return nil, ErrForbidden
	}

	updated, err := s.Queries.UpdateComment(ctx, db.UpdateCommentParams{
		ID:      commentID,
		Content: content,
	})
	if err != nil {
		return nil, fmt.Errorf("update comment: %w", err)
	}

	resp := commentFromRow(updated)
	return &resp, nil
}

// DeleteComment hard-deletes a comment.
func (s *Service) DeleteComment(ctx context.Context, id string, actor Actor) error {
	commentID, err := s.resolveCommentID(ctx, id)
	if err != nil {
		return ErrNotFound
	}

	existing, err := s.Queries.GetComment(ctx, commentID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrNotFound
		}
		return fmt.Errorf("get comment: %w", err)
	}

	if !canModifyComment(existing, actor) {
		return ErrForbidden
	}

	if err := s.Queries.DeleteComment(ctx, commentID); err != nil {
		return fmt.Errorf("delete comment: %w", err)
	}

	return nil
}

// canModifyComment checks if the actor can edit/delete a comment.
// The original author can always modify. Admins (user type) can moderate.
func canModifyComment(comment db.TransactionComment, actor Actor) bool {
	if actor.Type == "user" {
		return true // admins can moderate
	}
	return comment.AuthorType == actor.Type && comment.AuthorID.Valid && comment.AuthorID.String == actor.ID
}

func commentFromRow(c db.TransactionComment) CommentResponse {
	resp := CommentResponse{
		ID:            formatUUID(c.ID),
		ShortID:       c.ShortID,
		TransactionID: formatUUID(c.TransactionID),
		AuthorType:    c.AuthorType,
		AuthorID:      textPtr(c.AuthorID),
		AuthorName:    c.AuthorName,
		Content:       c.Content,
		CreatedAt:     c.CreatedAt.Time.UTC().Format(time.RFC3339),
		UpdatedAt:     c.UpdatedAt.Time.UTC().Format(time.RFC3339),
	}
	if c.ReviewID.Valid {
		rid := formatUUID(c.ReviewID)
		resp.ReviewID = &rid
	}
	return resp
}

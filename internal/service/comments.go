package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaxCommentLength caps the character length of a comment body. The admin
// composer surfaces this in its char counter so the client mirrors the
// server-side guard.
const MaxCommentLength = 10000

// Comments are stored as annotations with kind='comment'. CreateComment /
// ListComments / UpdateComment / DeleteComment read and write the annotations
// table directly; REST and MCP callers see the same API surface.

// CreateComment adds a comment annotation to a transaction.
func (s *Service) CreateComment(ctx context.Context, params CreateCommentParams) (*CommentResponse, error) {
	content := strings.TrimSpace(params.Content)
	if content == "" || len(content) > MaxCommentLength {
		return nil, fmt.Errorf("content must be between 1 and %d characters", MaxCommentLength)
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

	actorType := normalizeAnnotationActorType(params.Actor.Type)
	payload := map[string]interface{}{
		"content": content,
	}
	if params.ReviewID != "" {
		payload["review_id"] = params.ReviewID
	}

	actorID := pgtype.Text{}
	if params.Actor.ID != "" {
		actorID = pgconv.Text(params.Actor.ID)
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal comment payload: %w", err)
	}

	ann, err := s.Queries.InsertAnnotation(ctx, db.InsertAnnotationParams{
		TransactionID: txnID,
		Kind:          "comment",
		ActorType:     actorType,
		ActorID:       actorID,
		ActorName:     params.Actor.Name,
		Payload:       payloadBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("insert comment annotation: %w", err)
	}

	resp := commentFromAnnotation(ann, content, params.ReviewID)
	return &resp, nil
}

// normalizeAnnotationActorType coerces a free-form actor type string into one
// of the values the annotations.actor_type CHECK constraint accepts.
func normalizeAnnotationActorType(t string) string {
	switch t {
	case "user", "agent", "system":
		return t
	case "rule":
		// Rule-originated annotations have their own rule_id column — the
		// actor_type still uses "system" in that case.
		return "system"
	default:
		return "user"
	}
}

// ListComments returns all comment annotations for a transaction, ordered by
// created_at ASC.
func (s *Service) ListComments(ctx context.Context, transactionID string) ([]CommentResponse, error) {
	txnID, err := s.resolveTransactionID(ctx, transactionID)
	if err != nil {
		return nil, ErrNotFound
	}

	rows, err := s.Queries.ListAnnotationsByTransaction(ctx, txnID)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}

	result := make([]CommentResponse, 0, len(rows))
	for _, r := range rows {
		if r.Kind != "comment" {
			continue
		}
		content, reviewID := contentFromAnnotationPayload(r.Payload)
		result = append(result, commentFromAnnotation(r, content, reviewID))
	}
	return result, nil
}

// UpdateComment updates the content of an existing comment annotation. Comments
// are identified by the annotation's short_id or UUID.
func (s *Service) UpdateComment(ctx context.Context, id string, params UpdateCommentParams) (*CommentResponse, error) {
	content := strings.TrimSpace(params.Content)
	if content == "" || len(content) > MaxCommentLength {
		return nil, fmt.Errorf("content must be between 1 and %d characters", MaxCommentLength)
	}

	annID, err := s.resolveAnnotationID(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}

	existing, err := s.Queries.GetAnnotationByID(ctx, annID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get annotation: %w", err)
	}

	if existing.Kind != "comment" {
		return nil, ErrNotFound
	}

	// Check authorization: author match or admin.
	if !canModifyAnnotation(existing, params.Actor) {
		return nil, ErrForbidden
	}

	// Merge content into existing payload preserving review_id etc.
	var payload map[string]interface{}
	if len(existing.Payload) > 0 {
		_ = json.Unmarshal(existing.Payload, &payload)
	}
	if payload == nil {
		payload = map[string]interface{}{}
	}
	payload["content"] = content
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal updated comment payload: %w", err)
	}

	updated, err := s.Queries.UpdateAnnotationPayload(ctx, db.UpdateAnnotationPayloadParams{
		ID:      annID,
		Payload: payloadBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("update comment annotation: %w", err)
	}

	updatedContent, reviewID := contentFromAnnotationPayload(updated.Payload)
	resp := commentFromAnnotation(updated, updatedContent, reviewID)
	return &resp, nil
}

// DeleteComment hard-deletes a comment annotation.
func (s *Service) DeleteComment(ctx context.Context, id string, actor Actor) error {
	annID, err := s.resolveAnnotationID(ctx, id)
	if err != nil {
		return ErrNotFound
	}

	existing, err := s.Queries.GetAnnotationByID(ctx, annID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrNotFound
		}
		return fmt.Errorf("get annotation: %w", err)
	}

	if existing.Kind != "comment" {
		return ErrNotFound
	}

	if !canModifyAnnotation(existing, actor) {
		return ErrForbidden
	}

	if err := s.Queries.DeleteAnnotation(ctx, annID); err != nil {
		return fmt.Errorf("delete comment annotation: %w", err)
	}

	return nil
}

// canModifyAnnotation checks if the actor can edit/delete an annotation.
// The original author can always modify. Admins (user type) can moderate.
func canModifyAnnotation(ann db.Annotation, actor Actor) bool {
	if actor.Type == "user" {
		return true // admins can moderate
	}
	return ann.ActorType == actor.Type && ann.ActorID.Valid && ann.ActorID.String == actor.ID
}

// contentFromAnnotationPayload pulls the content + optional review_id out of a
// comment annotation's payload.
func contentFromAnnotationPayload(raw []byte) (content string, reviewID string) {
	if len(raw) == 0 {
		return "", ""
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", ""
	}
	if v, ok := payload["content"].(string); ok {
		content = v
	}
	if v, ok := payload["review_id"].(string); ok {
		reviewID = v
	}
	return content, reviewID
}

// commentFromAnnotation converts a comment annotation into a CommentResponse.
// The content + review_id are extracted from the annotation payload.
func commentFromAnnotation(a db.Annotation, content, reviewID string) CommentResponse {
	resp := CommentResponse{
		ID:            formatUUID(a.ID),
		ShortID:       a.ShortID,
		TransactionID: formatUUID(a.TransactionID),
		AuthorType:    a.ActorType,
		AuthorID:      textPtr(a.ActorID),
		AuthorName:    a.ActorName,
		Content:       content,
		CreatedAt:     pgconv.TimestampStr(a.CreatedAt),
		UpdatedAt:     pgconv.TimestampStr(a.CreatedAt),
	}
	if reviewID != "" {
		rid := reviewID
		resp.ReviewID = &rid
	}
	return resp
}

package service

import (
	"context"
	"encoding/json"
	"fmt"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5/pgtype"
)

// Annotation is the canonical timeline event for a transaction — the single
// source of truth for comments, rule applications, tag changes, and category
// sets.
type Annotation struct {
	ID            string                 `json:"id"`
	ShortID       string                 `json:"short_id"`
	TransactionID string                 `json:"transaction_id"`
	Kind          string                 `json:"kind"` // comment | rule_applied | tag_added | tag_removed | category_set
	ActorType     string                 `json:"actor_type"`
	ActorID       *string                `json:"actor_id,omitempty"`
	ActorName     string                 `json:"actor_name"`
	SessionID     *string                `json:"session_id,omitempty"`
	Payload       map[string]interface{} `json:"payload,omitempty"`
	TagID         *string                `json:"tag_id,omitempty"`
	RuleID        *string                `json:"rule_id,omitempty"`
	CreatedAt     string                 `json:"created_at"`
}

// writeAnnotationParams is the shared input for writing an annotation row via
// either the pool-backed Queries or a transaction-scoped db.Queries (WithTx).
type writeAnnotationParams struct {
	TransactionID pgtype.UUID
	Kind          string
	ActorType     string
	ActorID       string
	ActorName     string
	SessionID     pgtype.UUID
	Payload       map[string]interface{}
	TagID         pgtype.UUID
	RuleID        pgtype.UUID
}

// writeAnnotation inserts an annotation row via the supplied db.Queries handle.
// Passing a tx-scoped handle (from WithTx) keeps the write atomic with any
// surrounding DB transaction. Failures are returned to the caller.
func writeAnnotation(ctx context.Context, q *db.Queries, params writeAnnotationParams) error {
	var payload []byte
	if params.Payload != nil {
		b, err := json.Marshal(params.Payload)
		if err != nil {
			return fmt.Errorf("marshal annotation payload: %w", err)
		}
		payload = b
	} else {
		payload = []byte(`{}`)
	}

	actorID := pgtype.Text{}
	if params.ActorID != "" {
		actorID = pgconv.Text(params.ActorID)
	}

	_, err := q.InsertAnnotation(ctx, db.InsertAnnotationParams{
		TransactionID: params.TransactionID,
		Kind:          params.Kind,
		ActorType:     params.ActorType,
		ActorID:       actorID,
		ActorName:     params.ActorName,
		SessionID:     params.SessionID,
		Payload:       payload,
		TagID:         params.TagID,
		RuleID:        params.RuleID,
	})
	if err != nil {
		return fmt.Errorf("insert annotation: %w", err)
	}
	return nil
}

// ListAnnotations returns all annotations for a transaction, ordered by
// created_at ASC. Drives the transaction detail activity timeline.
func (s *Service) ListAnnotations(ctx context.Context, transactionID string) ([]Annotation, error) {
	txnID, err := s.resolveTransactionID(ctx, transactionID)
	if err != nil {
		return nil, ErrNotFound
	}

	rows, err := s.Queries.ListAnnotationsByTransaction(ctx, txnID)
	if err != nil {
		return nil, fmt.Errorf("list annotations: %w", err)
	}

	result := make([]Annotation, len(rows))
	for i, r := range rows {
		result[i] = annotationFromRow(r)
	}
	return result, nil
}

// annotationFromRow converts a db.Annotation into its service-layer response.
func annotationFromRow(a db.Annotation) Annotation {
	ann := Annotation{
		ID:            formatUUID(a.ID),
		ShortID:       a.ShortID,
		TransactionID: formatUUID(a.TransactionID),
		Kind:          a.Kind,
		ActorType:     a.ActorType,
		ActorID:       textPtr(a.ActorID),
		ActorName:     a.ActorName,
		CreatedAt:     pgconv.TimestampStr(a.CreatedAt),
	}

	if a.SessionID.Valid {
		s := formatUUID(a.SessionID)
		ann.SessionID = &s
	}
	if a.TagID.Valid {
		s := formatUUID(a.TagID)
		ann.TagID = &s
	}
	if a.RuleID.Valid {
		s := formatUUID(a.RuleID)
		ann.RuleID = &s
	}

	if len(a.Payload) > 0 && string(a.Payload) != "{}" {
		var payload map[string]interface{}
		if err := json.Unmarshal(a.Payload, &payload); err == nil {
			ann.Payload = payload
		}
	}

	return ann
}

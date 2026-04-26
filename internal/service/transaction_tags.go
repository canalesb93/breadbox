package service

import (
	"context"
	"errors"
	"fmt"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TransactionTagResponse is the enriched per-transaction tag entry returned to
// API/MCP consumers. Includes the tag's slug plus provenance info.
type TransactionTagResponse struct {
	Slug        string  `json:"slug"`
	DisplayName string  `json:"display_name"`
	Color       *string `json:"color,omitempty"`
	Icon        *string `json:"icon,omitempty"`
	AddedAt     string  `json:"added_at"`
	AddedByType string  `json:"added_by_type"`
	AddedByName string  `json:"added_by_name"`
}

// AddTransactionTag applies a tag to a transaction. If the slug doesn't exist,
// it is auto-created as a persistent tag. Writes an `tag_added` annotation in
// the same DB transaction.
//
// Return values:
//   - added:          true when a new (transaction, tag) pair was created.
//   - alreadyPresent: true when the pair already existed (no-op, no annotation
//     written).
func (s *Service) AddTransactionTag(ctx context.Context, txnID, slug string, actor Actor) (added bool, alreadyPresent bool, _ error) {
	txnUUID, err := s.resolveTransactionID(ctx, txnID)
	if err != nil {
		return false, false, ErrNotFound
	}
	if err := validateTagSlug(slug); err != nil {
		return false, false, err
	}
	if actor.Type == "" {
		actor = SystemActor()
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, false, fmt.Errorf("begin add_transaction_tag: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.Queries.WithTx(tx)

	// Resolve or auto-create the tag.
	tag, err := s.getOrCreateTagBySlug(ctx, qtx, slug)
	if err != nil {
		return false, false, err
	}

	addedByType := actor.Type
	if addedByType == "system" || addedByType == "" {
		addedByType = "system"
	}
	var addedByID pgtype.Text
	if actor.ID != "" {
		addedByID = pgconv.Text(actor.ID)
	}

	rows, err := qtx.AddTransactionTag(ctx, db.AddTransactionTagParams{
		TransactionID: txnUUID,
		TagID:         tag.ID,
		AddedByType:   addedByType,
		AddedByID:     addedByID,
		AddedByName:   actor.Name,
	})
	if err != nil {
		return false, false, fmt.Errorf("add transaction tag: %w", err)
	}

	if rows == 0 {
		// Already present. No annotation written.
		if err := tx.Commit(ctx); err != nil {
			return false, true, fmt.Errorf("commit add_transaction_tag: %w", err)
		}
		return false, true, nil
	}

	// Write annotation for the audit trail.
	payload := map[string]interface{}{"slug": slug}
	actorType := actor.Type
	if actorType == "rule" {
		// Annotations carry a constrained actor_type set; map "rule" onto
		// "system" so check constraints pass. The tag row still records
		// added_by_type="rule" for its own provenance.
		actorType = "system"
	}
	if err := writeAnnotation(ctx, qtx, writeAnnotationParams{
		TransactionID: txnUUID,
		Kind:          "tag_added",
		ActorType:     actorType,
		ActorID:       actor.ID,
		ActorName:     actor.Name,
		Payload:       payload,
		TagID:         tag.ID,
	}); err != nil {
		return false, false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, false, fmt.Errorf("commit add_transaction_tag: %w", err)
	}
	return true, false, nil
}

// RemoveTransactionTag removes a tag from a transaction. Returns:
//   - removed:      true when the (transaction, tag) pair was deleted.
//   - alreadyAbsent: true when the pair wasn't present (no-op, no error).
func (s *Service) RemoveTransactionTag(ctx context.Context, txnID, slug string, actor Actor) (removed bool, alreadyAbsent bool, _ error) {
	txnUUID, err := s.resolveTransactionID(ctx, txnID)
	if err != nil {
		return false, false, ErrNotFound
	}
	if err := validateTagSlug(slug); err != nil {
		return false, false, err
	}
	if actor.Type == "" {
		actor = SystemActor()
	}

	tag, err := s.Queries.GetTagBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, true, nil
		}
		return false, false, fmt.Errorf("get tag by slug: %w", err)
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, false, fmt.Errorf("begin remove_transaction_tag: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.Queries.WithTx(tx)

	rows, err := qtx.RemoveTransactionTag(ctx, db.RemoveTransactionTagParams{
		TransactionID: txnUUID,
		TagID:         tag.ID,
	})
	if err != nil {
		return false, false, fmt.Errorf("remove transaction tag: %w", err)
	}

	if rows == 0 {
		if err := tx.Commit(ctx); err != nil {
			return false, true, fmt.Errorf("commit remove_transaction_tag: %w", err)
		}
		return false, true, nil
	}

	payload := map[string]interface{}{"slug": slug}
	actorType := actor.Type
	if actorType == "rule" {
		actorType = "system"
	}
	if err := writeAnnotation(ctx, qtx, writeAnnotationParams{
		TransactionID: txnUUID,
		Kind:          "tag_removed",
		ActorType:     actorType,
		ActorID:       actor.ID,
		ActorName:     actor.Name,
		Payload:       payload,
		TagID:         tag.ID,
	}); err != nil {
		return false, false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, false, fmt.Errorf("commit remove_transaction_tag: %w", err)
	}
	return true, false, nil
}

// ListTransactionTags returns all tags currently attached to a transaction,
// enriched with the most-recent added_by provenance for each.
func (s *Service) ListTransactionTags(ctx context.Context, txnID string) ([]TransactionTagResponse, error) {
	txnUUID, err := s.resolveTransactionID(ctx, txnID)
	if err != nil {
		return nil, ErrNotFound
	}

	// ListTagsByTransaction returns the joined tag rows ordered by added_at ASC.
	// We also need the added_by_* provenance from transaction_tags; a small
	// dynamic query covers both in a single round-trip.
	rows, err := s.Pool.Query(ctx, `
		SELECT t.slug, t.display_name, t.color, t.icon,
		       tt.added_at, tt.added_by_type, tt.added_by_name
		FROM tags t
		JOIN transaction_tags tt ON tt.tag_id = t.id
		WHERE tt.transaction_id = $1
		ORDER BY tt.added_at ASC`, txnUUID)
	if err != nil {
		return nil, fmt.Errorf("list transaction tags: %w", err)
	}
	defer rows.Close()

	var result []TransactionTagResponse
	for rows.Next() {
		var slug, displayName, addedByType, addedByName string
		var color, icon pgtype.Text
		var addedAt pgtype.Timestamptz
		if err := rows.Scan(&slug, &displayName, &color, &icon, &addedAt, &addedByType, &addedByName); err != nil {
			return nil, fmt.Errorf("scan transaction tag: %w", err)
		}
		result = append(result, TransactionTagResponse{
			Slug:        slug,
			DisplayName: displayName,
			Color:       textPtr(color),
			Icon:        textPtr(icon),
			AddedAt:     pgconv.TimestampStr(addedAt),
			AddedByType: addedByType,
			AddedByName: addedByName,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transaction tags: %w", err)
	}
	return result, nil
}

// ListTagSlugsForTransaction returns just the slugs of the tags currently on
// the transaction. Lighter than ListTransactionTags for places that only need
// the slug list (e.g. attaching to a transaction response).
func (s *Service) ListTagSlugsForTransaction(ctx context.Context, txnID pgtype.UUID) ([]string, error) {
	slugs, err := s.Queries.ListTagSlugsByTransaction(ctx, txnID)
	if err != nil {
		return nil, fmt.Errorf("list tag slugs: %w", err)
	}
	if slugs == nil {
		return []string{}, nil
	}
	return slugs, nil
}

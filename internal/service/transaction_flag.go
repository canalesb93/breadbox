//go:build !lite

package service

import (
	"context"
	"fmt"
	"strings"
)

// FlagTransaction marks a transaction for human attention by setting flagged_at
// = NOW(). An optional reason is recorded as a comment annotation — annotations
// are the canonical historical-context log, so there is deliberately no
// flag_reason column. The flag + comment are written atomically. Re-flagging an
// already-flagged row refreshes the timestamp (last-write-wins).
func (s *Service) FlagTransaction(ctx context.Context, txnID, reason string, actor Actor) error {
	uid, err := s.resolveTransactionID(ctx, txnID)
	if err != nil {
		return ErrNotFound
	}
	reason = strings.TrimSpace(reason)
	if len(reason) > MaxCommentLength {
		return fmt.Errorf("%w: reason must be at most %d characters", ErrInvalidParameter, MaxCommentLength)
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin flag transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.Queries.WithTx(tx)

	tag, err := tx.Exec(ctx,
		`UPDATE transactions SET flagged_at = NOW() WHERE id = $1 AND deleted_at IS NULL`, uid)
	if err != nil {
		return fmt.Errorf("flag transaction: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	if reason != "" {
		if err := writeAnnotation(ctx, qtx, writeAnnotationParams{
			TransactionID: uid,
			Kind:          "comment",
			ActorType:     normalizeAnnotationActorType(actor.Type),
			ActorID:       actor.ID,
			ActorName:     actor.Name,
			Payload:       map[string]interface{}{"content": reason},
		}); err != nil {
			return fmt.Errorf("write flag reason comment: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit flag transaction: %w", err)
	}
	return nil
}

// UnflagTransaction clears the flag (flagged_at = NULL). No-op-safe: succeeds as
// long as the transaction exists.
func (s *Service) UnflagTransaction(ctx context.Context, txnID string) error {
	uid, err := s.resolveTransactionID(ctx, txnID)
	if err != nil {
		return ErrNotFound
	}
	tag, err := s.Pool.Exec(ctx,
		`UPDATE transactions SET flagged_at = NULL WHERE id = $1 AND deleted_at IS NULL`, uid)
	if err != nil {
		return fmt.Errorf("unflag transaction: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

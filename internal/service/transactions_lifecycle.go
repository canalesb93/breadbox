package service

import (
	"context"
	"fmt"

	"breadbox/internal/db"

	"github.com/jackc/pgx/v5/pgtype"
)

// SoftDeleteTransaction marks a transaction as deleted by setting its
// deleted_at timestamp. All read paths in the service layer filter on
// `deleted_at IS NULL`, so a soft-deleted row immediately disappears from
// list/get/summary endpoints — no separate write happens elsewhere.
//
// The call is idempotent at the API surface in the sense that a second call
// returns ErrNotFound (because the row is no longer "live"). The REST
// handler maps that to 404, signalling "no live row matches this id" — which
// is the same response a non-existent id would produce.
//
// Writes a `transaction_deleted` annotation row attributed to the supplied
// actor in the same DB transaction as the soft-delete update so the activity
// timeline records who deleted what and when.
func (s *Service) SoftDeleteTransaction(ctx context.Context, txnID string, actor Actor) error {
	txnUID, err := s.resolveTransactionID(ctx, txnID)
	if err != nil {
		return ErrNotFound
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin soft_delete_transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.Queries.WithTx(tx)

	rowsAffected, err := qtx.SoftDeleteTransactionByID(ctx, txnUID)
	if err != nil {
		return fmt.Errorf("soft delete transaction: %w", err)
	}
	if rowsAffected == 0 {
		// Row is already deleted (or doesn't exist) — treat as not found so
		// the REST handler returns 404 and the call is idempotent at the
		// API surface.
		return ErrNotFound
	}

	if err := writeLifecycleAnnotation(ctx, qtx, txnUID, actor, "transaction_deleted"); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit soft_delete_transaction: %w", err)
	}
	return nil
}

// RestoreTransaction clears the deleted_at flag on a previously soft-deleted
// transaction, bringing it back into all read paths. Returns ErrNotFound if
// the transaction doesn't exist or isn't currently soft-deleted (nothing to
// restore) — the REST handler maps that to 404 so restore is idempotent at
// the API surface.
//
// Writes a `transaction_restored` annotation row attributed to the supplied
// actor in the same DB transaction as the restore update.
//
// Note: resolveTransactionID hits GetTransactionUUIDByShortID, which itself
// filters on `deleted_at IS NULL`, so short_id input for a soft-deleted row
// won't resolve. UUID input still works because the resolver short-circuits
// when the input parses as a UUID — that's the path used by the REST
// handler when restoring.
func (s *Service) RestoreTransaction(ctx context.Context, txnID string, actor Actor) error {
	txnUID, err := s.resolveTransactionID(ctx, txnID)
	if err != nil {
		return ErrNotFound
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin restore_transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.Queries.WithTx(tx)

	rowsAffected, err := qtx.RestoreTransactionByID(ctx, txnUID)
	if err != nil {
		return fmt.Errorf("restore transaction: %w", err)
	}
	if rowsAffected == 0 {
		// Row exists but isn't soft-deleted — nothing to restore.
		return ErrNotFound
	}

	if err := writeLifecycleAnnotation(ctx, qtx, txnUID, actor, "transaction_restored"); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit restore_transaction: %w", err)
	}
	return nil
}

// writeLifecycleAnnotation writes a transaction lifecycle annotation
// (`transaction_deleted` or `transaction_restored`). Falls back to the
// SystemActor when the caller didn't pass one — defensive, since both
// service entry points always have a context-derived actor in practice.
func writeLifecycleAnnotation(ctx context.Context, q *db.Queries, txnUID pgtype.UUID, actor Actor, kind string) error {
	if actor.Type == "" {
		actor = SystemActor()
	}
	return writeAnnotation(ctx, q, writeAnnotationParams{
		TransactionID: txnUID,
		Kind:          kind,
		ActorType:     normalizeAnnotationActorType(actor.Type),
		ActorID:       actor.ID,
		ActorName:     actor.Name,
		Payload: map[string]interface{}{
			"source": "rest_api",
		},
	})
}

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// UpdateTransactionsOp describes a single per-transaction compound op used by
// UpdateTransactions. Each op can set or reset a category, add/remove tags,
// and attach a comment. All four changes are applied atomically per
// transaction. CategorySlug and ResetCategory are mutually exclusive — set
// CategorySlug to override the category, or ResetCategory to clear an
// existing override and let rules re-categorize, never both.
type UpdateTransactionsOp struct {
	TransactionID string
	CategorySlug  *string
	ResetCategory bool
	TagsToAdd     []UpdateTransactionsTagOp
	TagsToRemove  []UpdateTransactionsTagOp
	Comment       *string
}

// UpdateTransactionsTagOp is a single tag add/remove entry.
type UpdateTransactionsTagOp struct {
	Slug string
}

// UpdateTransactionsResult is the per-op outcome returned by UpdateTransactions.
type UpdateTransactionsResult struct {
	TransactionID string                         `json:"transaction_id"`
	Status        string                         `json:"status"` // "ok" or "error"
	Error         *UpdateTransactionsResultError `json:"error,omitempty"`
}

// UpdateTransactionsResultError captures a per-op error code + message.
type UpdateTransactionsResultError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// UpdateTransactionsParams is the batch input for the UpdateTransactions service method.
type UpdateTransactionsParams struct {
	Operations []UpdateTransactionsOp
	OnError    string // "continue" (default) or "abort"
	Actor      Actor
}

// UpdateTransactions applies each operation to its target transaction. In
// "continue" mode (default) each op runs in its own DB tx — partial failures
// don't undo successful items. In "abort" mode a single DB tx wraps every
// operation and the entire batch rolls back on the first error.
//
// Within each operation the order is: set category (optional) → add tags →
// remove tags → attach comment. All four are written inside the same DB tx for
// that op, so annotations are atomic with the underlying change.
func (s *Service) UpdateTransactions(ctx context.Context, params UpdateTransactionsParams) ([]UpdateTransactionsResult, error) {
	ops := params.Operations
	if len(ops) == 0 {
		return nil, fmt.Errorf("%w: operations array is empty", ErrInvalidParameter)
	}
	if len(ops) > 50 {
		return nil, fmt.Errorf("%w: maximum 50 operations per request", ErrInvalidParameter)
	}

	mode := strings.TrimSpace(params.OnError)
	if mode == "" {
		mode = "continue"
	}
	if mode != "continue" && mode != "abort" {
		return nil, fmt.Errorf("%w: on_error must be 'continue' or 'abort'", ErrInvalidParameter)
	}

	actor := params.Actor
	if actor.Type == "" {
		actor = SystemActor()
	}

	results := make([]UpdateTransactionsResult, len(ops))

	if mode == "abort" {
		return s.updateTransactionsAbort(ctx, ops, actor)
	}

	for i, op := range ops {
		res := UpdateTransactionsResult{TransactionID: op.TransactionID, Status: "ok"}
		if err := s.runUpdateOp(ctx, op, actor); err != nil {
			res.Status = "error"
			res.Error = errorToResultError(err)
		}
		results[i] = res
	}
	return results, nil
}

// updateTransactionsAbort wraps every op in a single DB tx. On the first
// error the tx rolls back and the error is returned.
func (s *Service) updateTransactionsAbort(ctx context.Context, ops []UpdateTransactionsOp, actor Actor) ([]UpdateTransactionsResult, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin update_transactions: %w", err)
	}
	defer tx.Rollback(ctx)

	results := make([]UpdateTransactionsResult, len(ops))
	for i, op := range ops {
		res := UpdateTransactionsResult{TransactionID: op.TransactionID, Status: "ok"}
		if err := s.runUpdateOpInTx(ctx, tx, op, actor); err != nil {
			res.Status = "error"
			res.Error = errorToResultError(err)
			results[i] = res
			// Fill remaining with skipped marker; include op id if present.
			for j := i + 1; j < len(ops); j++ {
				results[j] = UpdateTransactionsResult{
					TransactionID: ops[j].TransactionID,
					Status:        "error",
					Error: &UpdateTransactionsResultError{
						Code:    "ABORTED",
						Message: "batch rolled back due to prior error",
					},
				}
			}
			return results, fmt.Errorf("op %d (%s): %w", i, op.TransactionID, err)
		}
		results[i] = res
	}

	if err := tx.Commit(ctx); err != nil {
		return results, fmt.Errorf("commit update_transactions: %w", err)
	}
	return results, nil
}

// runUpdateOp executes a single op with its own DB transaction.
func (s *Service) runUpdateOp(ctx context.Context, op UpdateTransactionsOp, actor Actor) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin op tx: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := s.runUpdateOpInTx(ctx, tx, op, actor); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit op tx: %w", err)
	}
	return nil
}

// runUpdateOpInTx performs the compound operation against the supplied tx.
// Writes happen in order: category → add tags → remove tags → comment. Each
// change also writes an annotation for the audit trail.
func (s *Service) runUpdateOpInTx(ctx context.Context, tx pgx.Tx, op UpdateTransactionsOp, actor Actor) error {
	if strings.TrimSpace(op.TransactionID) == "" {
		return fmt.Errorf("%w: transaction_id is required", ErrInvalidParameter)
	}

	txnUID, err := s.resolveTransactionID(ctx, op.TransactionID)
	if err != nil {
		return fmt.Errorf("transaction not found: %w", ErrNotFound)
	}

	qtx := s.Queries.WithTx(tx)

	if op.CategorySlug != nil && op.ResetCategory {
		return fmt.Errorf("%w: category_slug and reset_category are mutually exclusive", ErrInvalidParameter)
	}

	// 1a. Reset (clear override + drop to uncategorized).
	if op.ResetCategory {
		rowsAffected, err := qtx.ClearTransactionCategoryOverride(ctx, txnUID)
		if err != nil {
			return fmt.Errorf("clear override: %w", err)
		}
		if rowsAffected == 0 {
			return fmt.Errorf("%w: transaction not found", ErrNotFound)
		}
		uncategorized, err := qtx.GetCategoryBySlug(ctx, "uncategorized")
		if err != nil {
			return fmt.Errorf("get uncategorized: %w", err)
		}
		if _, err := tx.Exec(ctx, "UPDATE transactions SET category_id = $1 WHERE id = $2", uncategorized.ID, txnUID); err != nil {
			return fmt.Errorf("reset category: %w", err)
		}
		if err := writeCategorySetAnnotation(ctx, qtx, txnUID, actor, "uncategorized", true); err != nil {
			return err
		}
	}

	// 1b. Set category override.
	if op.CategorySlug != nil {
		slug := strings.TrimSpace(*op.CategorySlug)
		if slug == "" {
			return fmt.Errorf("%w: category_slug cannot be empty string — omit the field to skip category update", ErrInvalidParameter)
		}
		cat, err := qtx.GetCategoryBySlug(ctx, slug)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("%w: category slug %q not found", ErrCategoryNotFound, slug)
			}
			return fmt.Errorf("resolve category slug %q: %w", slug, err)
		}
		rows, err := qtx.SetTransactionCategoryOverride(ctx, db.SetTransactionCategoryOverrideParams{
			ID:         txnUID,
			CategoryID: cat.ID,
		})
		if err != nil {
			return fmt.Errorf("set category override: %w", err)
		}
		if rows == 0 {
			return fmt.Errorf("%w: transaction not found", ErrNotFound)
		}
		// Write category_set annotation.
		payload := map[string]interface{}{
			"category_slug": slug,
			"source":        "manual",
		}
		if err := writeAnnotation(ctx, qtx, writeAnnotationParams{
			TransactionID: txnUID,
			Kind:          "category_set",
			ActorType:     normalizeAnnotationActorType(actor.Type),
			ActorID:       actor.ID,
			ActorName:     actor.Name,
			Payload:       payload,
		}); err != nil {
			return fmt.Errorf("write category_set annotation: %w", err)
		}
	}

	// 2. Add tags.
	for _, t := range op.TagsToAdd {
		slug := strings.TrimSpace(t.Slug)
		if slug == "" {
			return fmt.Errorf("%w: tag slug cannot be empty in tags_to_add", ErrInvalidParameter)
		}
		if err := validateTagSlug(slug); err != nil {
			return err
		}
		tag, err := s.getOrCreateTagBySlug(ctx, qtx, slug)
		if err != nil {
			return err
		}
		addedByType := actor.Type
		if addedByType == "" || addedByType == "system" {
			addedByType = "system"
		}
		actorID := pgtype.Text{}
		if actor.ID != "" {
			actorID = pgconv.Text(actor.ID)
		}
		rows, err := qtx.AddTransactionTag(ctx, db.AddTransactionTagParams{
			TransactionID: txnUID,
			TagID:         tag.ID,
			AddedByType:   addedByType,
			AddedByID:     actorID,
			AddedByName:   actor.Name,
		})
		if err != nil {
			return fmt.Errorf("add tag %q: %w", slug, err)
		}
		if rows == 0 {
			// Already present. Skip annotation.
			continue
		}
		payload := map[string]interface{}{"slug": slug}
		if err := writeAnnotation(ctx, qtx, writeAnnotationParams{
			TransactionID: txnUID,
			Kind:          "tag_added",
			ActorType:     normalizeAnnotationActorType(actor.Type),
			ActorID:       actor.ID,
			ActorName:     actor.Name,
			Payload:       payload,
			TagID:         tag.ID,
		}); err != nil {
			return fmt.Errorf("write tag_added annotation: %w", err)
		}
	}

	// 3. Remove tags.
	for _, t := range op.TagsToRemove {
		slug := strings.TrimSpace(t.Slug)
		if slug == "" {
			return fmt.Errorf("%w: tag slug cannot be empty in tags_to_remove", ErrInvalidParameter)
		}
		if err := validateTagSlug(slug); err != nil {
			return err
		}
		tag, err := qtx.GetTagBySlug(ctx, slug)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// Unknown slug — treat as no-op (same as RemoveTransactionTag).
				continue
			}
			return fmt.Errorf("get tag %q: %w", slug, err)
		}
		rows, err := qtx.RemoveTransactionTag(ctx, db.RemoveTransactionTagParams{
			TransactionID: txnUID,
			TagID:         tag.ID,
		})
		if err != nil {
			return fmt.Errorf("remove tag %q: %w", slug, err)
		}
		if rows == 0 {
			// Wasn't present — skip annotation.
			continue
		}
		payload := map[string]interface{}{"slug": slug}
		if err := writeAnnotation(ctx, qtx, writeAnnotationParams{
			TransactionID: txnUID,
			Kind:          "tag_removed",
			ActorType:     normalizeAnnotationActorType(actor.Type),
			ActorID:       actor.ID,
			ActorName:     actor.Name,
			Payload:       payload,
			TagID:         tag.ID,
		}); err != nil {
			return fmt.Errorf("write tag_removed annotation: %w", err)
		}
	}

	// 4. Comment.
	if op.Comment != nil {
		content := strings.TrimSpace(*op.Comment)
		if content != "" {
			if len(content) > MaxCommentLength {
				return fmt.Errorf("%w: comment exceeds %d chars", ErrInvalidParameter, MaxCommentLength)
			}
			payload := map[string]interface{}{"content": content}
			if err := writeAnnotation(ctx, qtx, writeAnnotationParams{
				TransactionID: txnUID,
				Kind:          "comment",
				ActorType:     normalizeAnnotationActorType(actor.Type),
				ActorID:       actor.ID,
				ActorName:     actor.Name,
				Payload:       payload,
			}); err != nil {
				return fmt.Errorf("write comment annotation: %w", err)
			}
		}
	}

	return nil
}

// errorToResultError maps a service-layer error to the per-op result.
func errorToResultError(err error) *UpdateTransactionsResultError {
	switch {
	case errors.Is(err, ErrNotFound):
		return &UpdateTransactionsResultError{Code: "NOT_FOUND", Message: err.Error()}
	case errors.Is(err, ErrCategoryNotFound):
		return &UpdateTransactionsResultError{Code: "CATEGORY_NOT_FOUND", Message: err.Error()}
	case errors.Is(err, ErrInvalidParameter):
		return &UpdateTransactionsResultError{Code: "INVALID_PARAMETER", Message: err.Error()}
	case errors.Is(err, ErrForbidden):
		return &UpdateTransactionsResultError{Code: "FORBIDDEN", Message: err.Error()}
	default:
		return &UpdateTransactionsResultError{Code: "INTERNAL", Message: err.Error()}
	}
}


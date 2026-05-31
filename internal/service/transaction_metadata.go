//go:build !lite

package service

import (
	"context"
	"encoding/json"
	"fmt"
)

// Metadata guardrails. The metadata blob is a free-form enrichment store, but
// it is not a dumping ground: keys are bounded and the whole object is capped so
// a runaway agent can't bloat a transaction row.
const (
	maxMetadataKeyLen   = 128
	maxMetadataBytes    = 8 << 10 // 8 KiB serialized object ceiling
	maxMetadataValBytes = 4 << 10 // 4 KiB per single value (set op)
)

// metadataToRaw converts a JSONB []byte scan result into a json.RawMessage for
// TransactionResponse. An empty/nil slice becomes the JSON empty object so the
// field is always a valid object in responses (never null, never empty bytes).
func metadataToRaw(b []byte) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(b)
}

// validateMetadataKey enforces the key constraints shared by the scoped ops.
func validateMetadataKey(key string) error {
	if key == "" {
		return fmt.Errorf("%w: metadata key must not be empty", ErrInvalidParameter)
	}
	if len(key) > maxMetadataKeyLen {
		return fmt.Errorf("%w: metadata key exceeds %d chars", ErrInvalidParameter, maxMetadataKeyLen)
	}
	return nil
}

// SetTransactionMetadata upserts a single key in the transaction's metadata
// blob, leaving every other key untouched. Creates the key if absent. The value
// may be any JSON-serializable Go value (string, number, bool, object, array).
// This op touches ONLY the metadata column — it can never write a first-class
// field (category, tags, amount, …).
func (s *Service) SetTransactionMetadata(ctx context.Context, id, key string, value any) error {
	if err := validateMetadataKey(key); err != nil {
		return err
	}
	uid, err := s.resolveTransactionID(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	valBytes, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("%w: metadata value is not JSON-serializable: %v", ErrInvalidParameter, err)
	}
	if len(valBytes) > maxMetadataValBytes {
		return fmt.Errorf("%w: metadata value exceeds %d bytes", ErrInvalidParameter, maxMetadataValBytes)
	}
	tag, err := s.Pool.Exec(ctx,
		`UPDATE transactions
		   SET metadata = metadata || jsonb_build_object($2::text, $3::jsonb),
		       updated_at = NOW()
		 WHERE id = $1 AND deleted_at IS NULL`,
		uid, key, string(valBytes))
	if err != nil {
		return fmt.Errorf("set transaction metadata: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RemoveTransactionMetadata deletes one key from the metadata blob. No-op (still
// success) if the key is absent.
func (s *Service) RemoveTransactionMetadata(ctx context.Context, id, key string) error {
	if err := validateMetadataKey(key); err != nil {
		return err
	}
	uid, err := s.resolveTransactionID(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	tag, err := s.Pool.Exec(ctx,
		`UPDATE transactions
		   SET metadata = metadata - $2::text, updated_at = NOW()
		 WHERE id = $1 AND deleted_at IS NULL`,
		uid, key)
	if err != nil {
		return fmt.Errorf("remove transaction metadata: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ReplaceTransactionMetadata atomically replaces the entire metadata object.
// Pass an empty map (or nil) to clear all keys while keeping the column an
// empty object.
func (s *Service) ReplaceTransactionMetadata(ctx context.Context, id string, obj map[string]any) error {
	uid, err := s.resolveTransactionID(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	if obj == nil {
		obj = map[string]any{}
	}
	for k := range obj {
		if err := validateMetadataKey(k); err != nil {
			return err
		}
	}
	objBytes, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("%w: metadata object is not JSON-serializable: %v", ErrInvalidParameter, err)
	}
	if len(objBytes) > maxMetadataBytes {
		return fmt.Errorf("%w: metadata object exceeds %d bytes", ErrInvalidParameter, maxMetadataBytes)
	}
	tag, err := s.Pool.Exec(ctx,
		`UPDATE transactions
		   SET metadata = $2::jsonb, updated_at = NOW()
		 WHERE id = $1 AND deleted_at IS NULL`,
		uid, string(objBytes))
	if err != nil {
		return fmt.Errorf("replace transaction metadata: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ClearTransactionMetadata resets the metadata blob to the empty object.
func (s *Service) ClearTransactionMetadata(ctx context.Context, id string) error {
	return s.ReplaceTransactionMetadata(ctx, id, nil)
}

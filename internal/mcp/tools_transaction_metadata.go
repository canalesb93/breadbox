//go:build !lite

package mcp

import (
	"context"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// The transaction-metadata tools expose the free-form `metadata` JSONB store on
// a transaction through four deliberately-scoped operations. They touch ONLY the
// metadata column — none of them can write a first-class field (category, tags,
// amount). Each op states exactly what it does so an agent can't accidentally
// clobber sibling keys or other fields:
//   - set_transaction_metadata     → upsert ONE key (other keys untouched)
//   - remove_transaction_metadata  → delete ONE key
//   - replace_transaction_metadata → replace the WHOLE object atomically
//   - clear_transaction_metadata   → reset to the empty object {}
//
// Metadata is read back on every transaction (query_transactions, the
// breadbox://transaction/{id} resource, GET /transactions/{id}).

type setTransactionMetadataInput struct {
	TransactionID string `json:"transaction_id" jsonschema:"required,UUID or short ID of the transaction."`
	Key           string `json:"key" jsonschema:"required,Metadata key to set. Slug-like string, max 128 chars (e.g. 'tax_deductible', 'trip')."`
	Value         any    `json:"value" jsonschema:"required,JSON value to store: string, number, boolean, object, or array. Replaces any existing value for this key."`
}

type removeTransactionMetadataInput struct {
	TransactionID string `json:"transaction_id" jsonschema:"required,UUID or short ID of the transaction."`
	Key           string `json:"key" jsonschema:"required,Metadata key to remove. No-op if absent."`
}

type replaceTransactionMetadataInput struct {
	TransactionID string         `json:"transaction_id" jsonschema:"required,UUID or short ID of the transaction."`
	Metadata      map[string]any `json:"metadata" jsonschema:"required,Complete metadata object. Replaces the entire blob atomically. Pass {} to clear all keys."`
}

type clearTransactionMetadataInput struct {
	TransactionID string `json:"transaction_id" jsonschema:"required,UUID or short ID of the transaction."`
}

func (s *MCPServer) handleSetTransactionMetadata(ctx context.Context, _ *mcpsdk.CallToolRequest, input setTransactionMetadataInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.TransactionID == "" {
		return errorResult(fmt.Errorf("transaction_id is required")), nil, nil
	}
	if err := s.svc.SetTransactionMetadata(context.Background(), input.TransactionID, input.Key, input.Value); err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(map[string]any{"transaction_id": input.TransactionID, "key": input.Key, "set": true})
}

func (s *MCPServer) handleRemoveTransactionMetadata(ctx context.Context, _ *mcpsdk.CallToolRequest, input removeTransactionMetadataInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.TransactionID == "" {
		return errorResult(fmt.Errorf("transaction_id is required")), nil, nil
	}
	if err := s.svc.RemoveTransactionMetadata(context.Background(), input.TransactionID, input.Key); err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(map[string]any{"transaction_id": input.TransactionID, "key": input.Key, "removed": true})
}

func (s *MCPServer) handleReplaceTransactionMetadata(ctx context.Context, _ *mcpsdk.CallToolRequest, input replaceTransactionMetadataInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.TransactionID == "" {
		return errorResult(fmt.Errorf("transaction_id is required")), nil, nil
	}
	if err := s.svc.ReplaceTransactionMetadata(context.Background(), input.TransactionID, input.Metadata); err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(map[string]any{"transaction_id": input.TransactionID, "replaced": true})
}

func (s *MCPServer) handleClearTransactionMetadata(ctx context.Context, _ *mcpsdk.CallToolRequest, input clearTransactionMetadataInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.TransactionID == "" {
		return errorResult(fmt.Errorf("transaction_id is required")), nil, nil
	}
	if err := s.svc.ClearTransactionMetadata(context.Background(), input.TransactionID); err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(map[string]any{"transaction_id": input.TransactionID, "cleared": true})
}

//go:build !lite

package mcp

import (
	"context"
	"fmt"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// set_transaction_metadata is the single compound write over a transaction's
// free-form `metadata` JSONB store. It touches ONLY the metadata column — it
// can never write a first-class field (category, tags, amount). One tool covers
// every shape the four legacy ops did:
//   - upsert keys      → set:{...}              (merge; other keys untouched)
//   - delete keys      → unset:[...]            (no-op for absent keys)
//   - replace the blob → replace:true + set:{}  (clear every pre-existing key)
//   - clear everything → replace:true           (set omitted → {})
//
// Metadata is read back on every transaction (query_transactions, the
// breadbox://transaction/{id} resource, GET /transactions/{id}).

type setTransactionMetadataInput struct {
	TransactionID string         `json:"transaction_id" jsonschema:"required,UUID or short ID of the transaction."`
	Set           map[string]any `json:"set,omitempty" jsonschema:"Key→value pairs to upsert. Each key is slug-like, max 128 chars (e.g. 'tax_deductible', 'trip'); each value may be any JSON (string, number, boolean, object, array). By default these are MERGED — keys you don't list are left untouched. Example: {\"tax_deductible\":true,\"trip\":\"q2-offsite\"}."`
	Unset         []string       `json:"unset,omitempty" jsonschema:"Keys to delete from the metadata object. No-op (still succeeds) for keys that aren't present. Applied after set."`
	Replace       bool           `json:"replace,omitempty" jsonschema:"When true, the resulting metadata is EXACTLY the set object — every pre-existing key is cleared first (then unset is applied to the new object). Pass replace:true with set omitted to clear all metadata. Prefer the default merge (replace:false) when you only mean to change a few keys."`
}

func (s *MCPServer) handleSetTransactionMetadata(ctx context.Context, _ *mcpsdk.CallToolRequest, input setTransactionMetadataInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.TransactionID == "" {
		return errorResult(fmt.Errorf("transaction_id is required")), nil, nil
	}
	if !input.Replace && len(input.Set) == 0 && len(input.Unset) == 0 {
		return errorResult(fmt.Errorf("nothing to do: provide set, unset, or replace")), nil, nil
	}

	bg := context.Background()

	// Replace path: the resulting blob is exactly `set` (minus any unset keys).
	// Written atomically as one object.
	if input.Replace {
		next := make(map[string]any, len(input.Set))
		for k, v := range input.Set {
			if strings.TrimSpace(k) == "" {
				return errorResult(fmt.Errorf("metadata key cannot be empty")), nil, nil
			}
			next[k] = v
		}
		for _, k := range input.Unset {
			delete(next, k)
		}
		if err := s.svc.ReplaceTransactionMetadata(bg, input.TransactionID, next); err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(map[string]any{"transaction_id": input.TransactionID, "replaced": true, "keys": len(next)})
	}

	// Merge path: upsert each set key, then delete each unset key. Keys not
	// named are left untouched.
	setKeys := make([]string, 0, len(input.Set))
	for k, v := range input.Set {
		if strings.TrimSpace(k) == "" {
			return errorResult(fmt.Errorf("metadata key cannot be empty")), nil, nil
		}
		if err := s.svc.SetTransactionMetadata(bg, input.TransactionID, k, v); err != nil {
			return errorResult(err), nil, nil
		}
		setKeys = append(setKeys, k)
	}
	for _, k := range input.Unset {
		if err := s.svc.RemoveTransactionMetadata(bg, input.TransactionID, k); err != nil {
			return errorResult(err), nil, nil
		}
	}
	return jsonResult(map[string]any{
		"transaction_id": input.TransactionID,
		"set":            setKeys,
		"unset":          input.Unset,
	})
}

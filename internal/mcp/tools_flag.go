//go:build !lite

package mcp

import (
	"context"
	"fmt"

	"breadbox/internal/service"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// The flag tools let an agent surface a transaction for human attention without
// changing its category. flag_transaction sets flagged_at; the reason is
// recorded as a comment annotation. Retrieve flagged rows with
// query_transactions(flagged=true). This is the "look at this" escape hatch the
// auto-apply workflow uses when it is unsure about a transaction.

type flagTransactionInput struct {
	TransactionID string `json:"transaction_id" jsonschema:"required,UUID or short ID of the transaction to flag."`
	Reason        string `json:"reason,omitempty" jsonschema:"Optional reason for the flag, recorded as a comment annotation on the transaction's timeline. Keep it short and specific (e.g. 'amount looks high for this merchant')."`
}

type unflagTransactionInput struct {
	TransactionID string `json:"transaction_id" jsonschema:"required,UUID or short ID of the transaction to unflag."`
}

func (s *MCPServer) handleFlagTransaction(ctx context.Context, _ *mcpsdk.CallToolRequest, input flagTransactionInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.TransactionID == "" {
		return errorResult(fmt.Errorf("transaction_id is required")), nil, nil
	}
	actor := service.ActorFromContext(ctx)
	if err := s.svc.FlagTransaction(context.Background(), input.TransactionID, input.Reason, actor); err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(map[string]any{"transaction_id": input.TransactionID, "flagged": true})
}

func (s *MCPServer) handleUnflagTransaction(ctx context.Context, _ *mcpsdk.CallToolRequest, input unflagTransactionInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.TransactionID == "" {
		return errorResult(fmt.Errorf("transaction_id is required")), nil, nil
	}
	if err := s.svc.UnflagTransaction(context.Background(), input.TransactionID); err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(map[string]any{"transaction_id": input.TransactionID, "flagged": false})
}

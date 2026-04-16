package mcp

import (
	"context"
	"fmt"

	"breadbox/internal/service"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// updateTransactionsInput is the top-level input for the update_transactions tool.
type updateTransactionsInput struct {
	WriteSessionContext
	Operations []transactionOperationInput `json:"operations" jsonschema:"required,Array of per-transaction operations. Max 50. Each item can set a category AND add/remove tags AND attach a comment in a single atomic op. Example: [{\"transaction_id\":\"k7Xm9pQ2\",\"category_slug\":\"food_and_drink_groceries\",\"tags_to_remove\":[{\"slug\":\"needs-review\",\"note\":\"clearly groceries\"}]}]"`
	OnError    string                      `json:"on_error,omitempty" jsonschema:"\"continue\" (default — each op runs in its own DB tx, partial failures don't undo successful items) or \"abort\" (whole batch is one DB tx, rolls back on first error)."`
}

// transactionOperationInput describes a single compound operation applied to
// one transaction.
type transactionOperationInput struct {
	TransactionID string           `json:"transaction_id" jsonschema:"required,UUID or short ID."`
	CategorySlug  *string          `json:"category_slug,omitempty" jsonschema:"Category slug to set (e.g. 'food_and_drink_groceries'). Sets category_override=true. Omit to leave the category unchanged."`
	TagsToAdd     []tagOpEntryInput `json:"tags_to_add,omitempty" jsonschema:"List of tags to add. Each item: {slug, note?}. Auto-creates persistent tags if the slug is unknown."`
	TagsToRemove  []tagOpEntryInput `json:"tags_to_remove,omitempty" jsonschema:"List of tags to remove. Each item: {slug, note?}. note is recommended for auditability."`
	Comment       *string          `json:"comment,omitempty" jsonschema:"Optional free-form comment written as an annotation attributed to you. Max 10000 chars. Prefer including narrative inline here instead of a separate add_transaction_comment call."`
}

// tagOpEntryInput is a single tag add/remove entry.
type tagOpEntryInput struct {
	Slug string `json:"slug" jsonschema:"required,Tag slug (lowercase alphanumerics with hyphens/colons)."`
	Note string `json:"note,omitempty" jsonschema:"Short rationale. Recommended for auditability."`
}

// handleUpdateTransactions runs the compound batch.
func (s *MCPServer) handleUpdateTransactions(ctx context.Context, _ *mcpsdk.CallToolRequest, input updateTransactionsInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if len(input.Operations) == 0 {
		return errorResult(fmt.Errorf("operations array is required and must not be empty")), nil, nil
	}
	if len(input.Operations) > 50 {
		return errorResult(fmt.Errorf("maximum 50 operations per update_transactions call")), nil, nil
	}

	actor := service.ActorFromContext(ctx)

	ops := make([]service.UpdateTransactionsOp, len(input.Operations))
	for i, op := range input.Operations {
		ops[i] = service.UpdateTransactionsOp{
			TransactionID: op.TransactionID,
			CategorySlug:  op.CategorySlug,
			Comment:       op.Comment,
		}
		for _, t := range op.TagsToAdd {
			ops[i].TagsToAdd = append(ops[i].TagsToAdd, service.UpdateTransactionsTagOp{
				Slug: t.Slug,
				Note: t.Note,
			})
		}
		for _, t := range op.TagsToRemove {
			ops[i].TagsToRemove = append(ops[i].TagsToRemove, service.UpdateTransactionsTagOp{
				Slug: t.Slug,
				Note: t.Note,
			})
		}
	}

	results, err := s.svc.UpdateTransactions(context.Background(), service.UpdateTransactionsParams{
		Operations: ops,
		OnError:    input.OnError,
		Actor:      actor,
	})
	if err != nil && input.OnError != "abort" {
		// Unexpected top-level failure (not a per-op error).
		return errorResult(err), nil, nil
	}

	// Count successes/failures for the summary block.
	succeeded := 0
	failed := 0
	for _, r := range results {
		if r.Status == "ok" {
			succeeded++
		} else {
			failed++
		}
	}

	payload := map[string]any{
		"results":   results,
		"succeeded": succeeded,
		"failed":    failed,
	}
	if err != nil {
		// abort mode: return the top-level error plus partial results.
		payload["aborted"] = true
		payload["error"] = err.Error()
	}
	return jsonResult(payload)
}

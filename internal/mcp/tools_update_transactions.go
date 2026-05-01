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
	Operations []transactionOperationInput `json:"operations" jsonschema:"required,Array of per-transaction operations. Max 50. Each item can set a category AND add/remove tags AND attach a comment in a single atomic op. Use the 'comment' field to record decision rationale; tag adds/removes carry no per-action note. Example: [{\"transaction_id\":\"k7Xm9pQ2\",\"category_slug\":\"food_and_drink_groceries\",\"tags_to_remove\":[{\"slug\":\"needs-review\"}],\"comment\":\"clearly groceries\"}]"`
	OnError    string                      `json:"on_error,omitempty" jsonschema:"\"continue\" (default — each op runs in its own DB tx, partial failures don't undo successful items) or \"abort\" (whole batch is one DB tx, rolls back on first error)."`
}

// transactionOperationInput describes a single compound operation applied to
// one transaction.
type transactionOperationInput struct {
	TransactionID string            `json:"transaction_id" jsonschema:"required,UUID or short ID."`
	CategorySlug  *string           `json:"category_slug,omitempty" jsonschema:"Category slug to set (e.g. 'food_and_drink_groceries'). Sets category_override=true. Omit to leave the category unchanged. Mutually exclusive with reset_category."`
	ResetCategory bool              `json:"reset_category,omitempty" jsonschema:"Clear an existing manual category override and drop the transaction back to 'uncategorized' so rules can re-categorize it. Mutually exclusive with category_slug. Use this to undo a prior categorize/update_transactions decision."`
	TagsToAdd     []tagOpEntryInput `json:"tags_to_add,omitempty" jsonschema:"List of tags to add. Each item: {slug}. Auto-creates persistent tags if the slug is unknown."`
	TagsToRemove  []tagOpEntryInput `json:"tags_to_remove,omitempty" jsonschema:"List of tags to remove. Each item: {slug}."`
	Comment       *string           `json:"comment,omitempty" jsonschema:"Free-form comment written as an annotation attributed to you. Max 10000 chars. This is the canonical place to record decision context (e.g. why a tag was added or a category set) — prefer the comment over a per-tag note."`
}

// tagOpEntryInput is a single tag add/remove entry.
type tagOpEntryInput struct {
	Slug string `json:"slug" jsonschema:"required,Tag slug (lowercase alphanumerics with hyphens/colons)."`
}

// handleUpdateTransactions runs the compound batch — the universal per-row
// write tool. Each operation can set a category (or reset an override),
// add/remove tags, and attach a comment, all atomic per transaction.
//
// Example call (close two needs-review transactions in one batch):
//
//	{
//	  "operations": [
//	    {
//	      "transaction_id": "k7Xm9pQ2",
//	      "category_slug": "food_and_drink_groceries",
//	      "tags_to_remove": [{"slug":"needs-review"}],
//	      "comment": "Costco run, clearly groceries"
//	    },
//	    {
//	      "transaction_id": "x4Lz1mNa",
//	      "category_slug": "transportation_taxi_and_ride_share",
//	      "tags_to_remove": [{"slug":"needs-review"}],
//	      "comment": "Lyft to airport"
//	    }
//	  ],
//	  "on_error": "continue"
//	}
//
// Example call (undo a manual override and let rules re-categorize):
//
//	{
//	  "operations": [{
//	    "transaction_id": "k7Xm9pQ2",
//	    "reset_category": true,
//	    "comment": "Withdraw my prior override — let the rule pipeline take it."
//	  }]
//	}
//
// Example response:
//
//	{
//	  "results": [
//	    {"transaction_id":"k7Xm9pQ2","status":"ok"},
//	    {"transaction_id":"x4Lz1mNa","status":"ok"}
//	  ],
//	  "succeeded": 2,
//	  "failed": 0
//	}
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
			ResetCategory: op.ResetCategory,
			Comment:       op.Comment,
		}
		for _, t := range op.TagsToAdd {
			ops[i].TagsToAdd = append(ops[i].TagsToAdd, service.UpdateTransactionsTagOp{Slug: t.Slug})
		}
		for _, t := range op.TagsToRemove {
			ops[i].TagsToRemove = append(ops[i].TagsToRemove, service.UpdateTransactionsTagOp{Slug: t.Slug})
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

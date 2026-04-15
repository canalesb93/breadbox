package mcp

import (
	"context"
	"errors"
	"fmt"

	"breadbox/internal/service"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- Input types ---

type listTagsInput struct {
	ReadSessionContext
}

type listAnnotationsInput struct {
	ReadSessionContext
	TransactionID string `json:"transaction_id" jsonschema:"required,UUID or short ID of the transaction"`
}

type addTransactionTagInput struct {
	WriteSessionContext
	TransactionID string `json:"transaction_id" jsonschema:"required,UUID or short ID of the transaction"`
	TagSlug       string `json:"tag_slug" jsonschema:"required,Tag slug to add (e.g. 'needs-review'). Auto-created as persistent if not registered."`
	Note          string `json:"note,omitempty" jsonschema:"Optional note attached to the tag_added annotation."`
}

type removeTransactionTagInput struct {
	WriteSessionContext
	TransactionID string `json:"transaction_id" jsonschema:"required,UUID or short ID of the transaction"`
	TagSlug       string `json:"tag_slug" jsonschema:"required,Tag slug to remove"`
	Note          string `json:"note,omitempty" jsonschema:"Required when the tag's lifecycle is 'ephemeral'. Short rationale for removal."`
}

// --- Handlers ---

func (s *MCPServer) handleListTags(_ context.Context, _ *mcpsdk.CallToolRequest, _ listTagsInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	tags, err := s.svc.ListTags(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(tags)
}

func (s *MCPServer) handleListAnnotations(_ context.Context, _ *mcpsdk.CallToolRequest, input listAnnotationsInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	if input.TransactionID == "" {
		return errorResult(fmt.Errorf("transaction_id is required")), nil, nil
	}
	annotations, err := s.svc.ListAnnotations(ctx, input.TransactionID)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return errorResult(fmt.Errorf("transaction not found")), nil, nil
		}
		return errorResult(err), nil, nil
	}
	return jsonResult(annotations)
}

func (s *MCPServer) handleAddTransactionTag(ctx context.Context, _ *mcpsdk.CallToolRequest, input addTransactionTagInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.TransactionID == "" || input.TagSlug == "" {
		return errorResult(fmt.Errorf("transaction_id and tag_slug are required")), nil, nil
	}
	actor := service.ActorFromContext(ctx)
	added, alreadyPresent, err := s.svc.AddTransactionTag(context.Background(), input.TransactionID, input.TagSlug, actor, input.Note)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(map[string]any{
		"added":           added,
		"already_present": alreadyPresent,
		"tag_slug":        input.TagSlug,
		"transaction_id":  input.TransactionID,
	})
}

func (s *MCPServer) handleRemoveTransactionTag(ctx context.Context, _ *mcpsdk.CallToolRequest, input removeTransactionTagInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.TransactionID == "" || input.TagSlug == "" {
		return errorResult(fmt.Errorf("transaction_id and tag_slug are required")), nil, nil
	}
	actor := service.ActorFromContext(ctx)
	removed, alreadyAbsent, err := s.svc.RemoveTransactionTag(context.Background(), input.TransactionID, input.TagSlug, actor, input.Note)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(map[string]any{
		"removed":        removed,
		"already_absent": alreadyAbsent,
		"tag_slug":       input.TagSlug,
		"transaction_id": input.TransactionID,
	})
}
